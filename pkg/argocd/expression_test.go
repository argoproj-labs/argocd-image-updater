package argocd

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"

	iuapi "github.com/argoproj-labs/argocd-image-updater/api/v1alpha1"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/image"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/tag"
)

func TestExpressionStrategySelectTag(t *testing.T) {
	candidates := tag.NewImageTagList()
	for _, name := range []string{"build-9", "build-20", "build-100", "latest"} {
		candidates.Add(tag.NewImageTag(name, time.Time{}, ""))
	}
	for _, tc := range []struct {
		name    string
		filter  string
		sortKey string
		want    string
		wantErr string
	}{
		{name: "numeric build order", filter: `tag.name.startsWith("build-")`, sortKey: `int(tag.name.split("-")[1])`, want: "build-100"},
		{name: "string order", sortKey: "tag.name", want: "latest"},
		{name: "equal keys", filter: `tag.name.startsWith("build-")`, sortKey: "0", want: "build-9"},
		{name: "no match", filter: "false", sortKey: "0"},
		{name: "evaluation failure", sortKey: "int(tag.name)", wantErr: "sortKey for tag"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			strategy, err := compileExpressionStrategy(&iuapi.ExpressionStrategy{Filter: tc.filter, SortKey: tc.sortKey})
			require.NoError(t, err)
			selected, err := strategy.selectTag(context.Background(), candidates)
			switch {
			case tc.wantErr != "":
				require.ErrorContains(t, err, tc.wantErr)
				require.Nil(t, selected)
			case tc.want == "":
				require.NoError(t, err)
				require.Nil(t, selected)
			default:
				require.NoError(t, err)
				require.NotNil(t, selected)
				require.Equal(t, tc.want, selected.TagName)
			}
		})
	}
}

func TestExpressionStrategyStringAllocationLimits(t *testing.T) {
	for _, key := range []string{
		`tag.name.replace("a", tag.name)`,
		`[tag.name].join()`,
		`"%s".format([tag.name])`,
	} {
		_, err := compileExpressionStrategy(&iuapi.ExpressionStrategy{SortKey: key})
		require.ErrorContains(t, err, "undeclared reference")
	}

	strategy, err := compileExpressionStrategy(&iuapi.ExpressionStrategy{SortKey: `tag.name.split("")[0]`})
	require.NoError(t, err)
	candidates := tag.NewImageTagList()
	candidates.Add(tag.NewImageTag(strings.Repeat("a", maxSplitInputBytes+1), time.Time{}, ""))
	_, err = strategy.selectTag(context.Background(), candidates)
	require.ErrorContains(t, err, "split input exceeds")
}

func TestParseImageListExpressionStrategy(t *testing.T) {
	defaults := &iuapi.CommonUpdateSettings{
		UpdateStrategy:     ptr.To("expression"),
		ExpressionStrategy: &iuapi.ExpressionStrategy{Filter: "false", SortKey: "0"},
	}
	configs := []iuapi.ImageConfig{
		{Alias: "inherited", ImageName: "nginx"},
		{Alias: "override", ImageName: "nginx", CommonUpdateSettings: &iuapi.CommonUpdateSettings{
			ExpressionStrategy: &iuapi.ExpressionStrategy{SortKey: "tag.name"},
		}},
		{Alias: "invalid", ImageName: "nginx", CommonUpdateSettings: &iuapi.CommonUpdateSettings{
			ExpressionStrategy: &iuapi.ExpressionStrategy{SortKey: "true"},
		}},
		{Alias: "builtin", ImageName: "nginx", CommonUpdateSettings: &iuapi.CommonUpdateSettings{
			UpdateStrategy: ptr.To("alphabetical"),
		}},
	}
	images := parseImageList(context.Background(), nil, "", configs, defaults, nil, nil)
	require.Len(t, *images, 3)
	candidates := tag.NewImageTagList()
	candidate := tag.NewImageTag("latest", time.Time{}, "")
	candidates.Add(candidate)
	inherited, err := (*images)[0].expressionStrategy.selectTag(context.Background(), candidates)
	require.NoError(t, err)
	require.Nil(t, inherited)
	overridden, err := (*images)[1].expressionStrategy.selectTag(context.Background(), candidates)
	require.NoError(t, err)
	require.Equal(t, candidate, overridden)
	require.Equal(t, image.StrategyAlphabetical, (*images)[2].UpdateStrategy)
}
