package argocd

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/common/types/traits"

	iuapi "github.com/argoproj-labs/argocd-image-updater/api/v1alpha1"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/tag"
)

type compiledExpressionStrategy struct {
	filter  cel.Program
	sortKey cel.Program
}

func compileExpressionStrategy(config *iuapi.ExpressionStrategy) (*compiledExpressionStrategy, error) {
	if config == nil {
		return nil, fmt.Errorf("expressionStrategy is required")
	}
	// Expose only the string extension used for tag parsing. Functions such
	// as replace and join can allocate large results before CEL charges
	// their evaluation cost.
	env, err := cel.NewEnv(
		cel.Variable("tag", cel.MapType(cel.StringType, cel.StringType)),
		cel.Function("split", cel.MemberOverload("string_split_string",
			[]*cel.Type{cel.StringType, cel.StringType}, cel.ListType(cel.StringType),
			cel.BinaryBinding(splitTagString))),
	)
	if err != nil {
		return nil, err
	}

	strategy := &compiledExpressionStrategy{}
	if config.Filter != "" {
		strategy.filter, err = compileTagExpression(env, config.Filter, cel.BoolType)
		if err != nil {
			return nil, fmt.Errorf("filter: %w", err)
		}
	}
	strategy.sortKey, err = compileTagExpression(env, config.SortKey, cel.IntType, cel.StringType)
	if err != nil {
		return nil, fmt.Errorf("sortKey: %w", err)
	}
	return strategy, nil
}

const maxSplitInputBytes = 4096

func splitTagString(value, separator ref.Val) ref.Val {
	input := string(value.(types.String))
	if len(input) > maxSplitInputBytes {
		return types.NewErr("split input exceeds %d bytes", maxSplitInputBytes)
	}
	return types.NewStringList(types.DefaultTypeAdapter, strings.Split(input, string(separator.(types.String))))
}

func compileTagExpression(env *cel.Env, expression string, allowedTypes ...*cel.Type) (cel.Program, error) {
	ast, issues := env.Compile(expression)
	if err := issues.Err(); err != nil {
		return nil, err
	}
	for _, allowed := range allowedTypes {
		if ast.OutputType().IsExactType(allowed) {
			return env.Program(ast, cel.CostLimit(10000), cel.InterruptCheckFrequency(100))
		}
	}
	return nil, fmt.Errorf("expected result type %v, got %s", allowedTypes, ast.OutputType())
}

func (strategy *compiledExpressionStrategy) selectTag(ctx context.Context, candidates *tag.ImageTagList) (*tag.ImageTag, error) {
	var best *tag.ImageTag
	var bestKey ref.Val
	// Ascending tag names give equal keys a deterministic tie-breaker.
	for _, candidate := range candidates.SortAlphabetically() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		input := map[string]any{"tag": map[string]string{"name": candidate.TagName}}
		if strategy.filter != nil {
			match, _, err := strategy.filter.ContextEval(ctx, input)
			if err != nil {
				return nil, fmt.Errorf("filter for tag %q: %w", candidate.TagName, err)
			}
			if match == types.False {
				continue
			}
		}
		key, _, err := strategy.sortKey.ContextEval(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("sortKey for tag %q: %w", candidate.TagName, err)
		}
		// The compiler restricts keys to CEL int or string. Both are comparable.
		if best == nil || key.(traits.Comparer).Compare(bestKey) != types.IntNegOne {
			best, bestKey = candidate, key
		}
	}
	return best, nil
}
