package argocd

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
	"github.com/goccy/go-yaml/token"
)

// arrayIndexPattern matches array index notation like "images[0]"
var arrayIndexPattern = regexp.MustCompile(`^(.*)\[(\d+)\]$`)

// ValuesFile wraps a parsed YAML document for format-preserving operations.
// Comments, blank lines, and indentation are retained when the file is serialized.
type ValuesFile struct {
	file          *ast.File
	headerComment string
}

// ParseValuesFile parses YAML bytes into a ValuesFile that preserves formatting.
func ParseValuesFile(data []byte) (*ValuesFile, error) {
	file, err := parser.ParseBytes(data, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}
	return &ValuesFile{file: file}, nil
}

// String returns the YAML as a string, preserving original formatting.
func (y *ValuesFile) String() string {
	output := y.file.String()
	if y.headerComment != "" {
		return "# " + y.headerComment + "\n\n" + output
	}
	return output
}

// Bytes returns the YAML as bytes, preserving original formatting.
func (y *ValuesFile) Bytes() []byte {
	return []byte(y.String())
}

// SetValue sets a value at the given key path, preserving formatting.
// Supports nested paths (image.tag), literal keys, and array paths (images[0].tag).
func (y *ValuesFile) SetValue(key string, value string) error {
	if len(y.file.Docs) == 0 {
		return fmt.Errorf("empty document")
	}

	doc := y.file.Docs[0]
	root := doc.Body

	// Handle comment-only YAML or empty documents by initializing an empty mapping
	if root == nil {
		root = makeEmptyMapping(0)
		doc.Body = root
	} else if commentGroup, isCommentOnly := root.(*ast.CommentGroupNode); isCommentOnly {
		// Comment-only YAML - create empty mapping and preserve comments
		newRoot := makeEmptyMapping(0)
		if err := newRoot.SetComment(commentGroup); err != nil {
			return fmt.Errorf("failed to preserve comments: %w", err)
		}
		doc.Body = newRoot
		root = newRoot
	}

	rootMapping, ok := root.(*ast.MappingNode)
	if !ok {
		return fmt.Errorf("root is not a mapping node")
	}

	// First, check if the full key exists as a literal key
	if node := findKeyInMapping(rootMapping, key); node != nil {
		return setNodeValue(node, value)
	}

	// Try navigating as a nested path
	keys := strings.Split(key, ".")
	return setNestedValue(rootMapping, keys, value)
}

// GetValue gets a value at the given key path.
func (y *ValuesFile) GetValue(key string) (string, error) {
	if len(y.file.Docs) == 0 {
		return "", fmt.Errorf("empty document")
	}

	doc := y.file.Docs[0]
	root := doc.Body

	if root == nil {
		return "", fmt.Errorf("empty document body")
	}

	rootMapping, ok := root.(*ast.MappingNode)
	if !ok {
		return "", fmt.Errorf("root is not a mapping node")
	}

	// First, check if the full key exists as a literal key
	if node := findKeyInMapping(rootMapping, key); node != nil {
		return getScalarValue(node)
	}

	// Try navigating as a nested path
	keys := strings.Split(key, ".")
	return getNestedValue(rootMapping, keys)
}

// findKeyInMapping finds a key in a mapping node and returns its value node.
func findKeyInMapping(mapping *ast.MappingNode, key string) ast.Node {
	for _, item := range mapping.Values {
		if keyNode, ok := item.Key.(*ast.StringNode); ok && keyNode.Value == key {
			return item.Value
		}
	}
	return nil
}

// updateNodeToken updates the token value for a node and returns its comment.
func updateNodeToken(node ast.Node, value string) (*ast.CommentGroupNode, error) {
	type tokenNode interface {
		GetComment() *ast.CommentGroupNode
		GetToken() *token.Token
	}

	tn, ok := node.(tokenNode)
	if !ok {
		return nil, fmt.Errorf("cannot set value on non-scalar node (type: %T)", node)
	}

	// Validate value based on node type
	switch n := node.(type) {
	case *ast.IntegerNode:
		if _, err := strconv.ParseInt(value, 10, 64); err != nil {
			return nil, fmt.Errorf("cannot set integer value to %q: %w", value, err)
		}
	case *ast.FloatNode:
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			return nil, fmt.Errorf("cannot set float value to %q: %w", value, err)
		}
	case *ast.BoolNode:
		if _, err := strconv.ParseBool(value); err != nil {
			return nil, fmt.Errorf("cannot set bool value to %q: %w", value, err)
		}
	case *ast.StringNode:
		// String node - update the value field
		n.Value = value
	}

	// Update token for serialization
	if tok := tn.GetToken(); tok != nil {
		tok.Value = value
	}

	return tn.GetComment(), nil
}

// setNodeValue sets the value of a scalar node, preserving comments and type.
func setNodeValue(node ast.Node, value string) error {
	node = unwrapNode(node)

	comment, err := updateNodeToken(node, value)
	if err != nil {
		return err
	}

	// Restore comment
	if comment != nil {
		if err := node.SetComment(comment); err != nil {
			return fmt.Errorf("failed to preserve comment: %w", err)
		}
	}

	return nil
}

// getScalarValue extracts the string value from a node.
func getScalarValue(node ast.Node) (string, error) {
	switch n := node.(type) {
	case *ast.AliasNode:
		return getScalarValue(n.Value)
	case *ast.StringNode:
		return n.Value, nil
	case *ast.IntegerNode:
		return n.GetToken().Value, nil
	case *ast.FloatNode:
		return n.GetToken().Value, nil
	case *ast.BoolNode:
		return n.GetToken().Value, nil
	default:
		return "", fmt.Errorf("node is not a scalar value (type: %T)", node)
	}
}

// unwrapNode unwraps AnchorNode and AliasNode to get the actual value node.
func unwrapNode(node ast.Node) ast.Node {
	for {
		switch n := node.(type) {
		case *ast.AnchorNode:
			node = n.Value
		case *ast.AliasNode:
			node = n.Value
		default:
			return node
		}
	}
}

// setNestedValue sets a value at a nested path, creating nodes as needed.
func setNestedValue(mapping *ast.MappingNode, keys []string, value string) error {
	current := mapping
	// Infer indent step once from the root mapping
	indentStep := getIndentStep(mapping)

	for i, k := range keys {
		keyPart, arrayIdx := parseArrayIndex(k)

		var valueNode ast.Node
		for _, item := range current.Values {
			if keyNode, ok := item.Key.(*ast.StringNode); ok && keyNode.Value == keyPart {
				valueNode = item.Value
				break
			}
		}

		// If key not found, create it
		if valueNode == nil {
			// Cannot create missing array indices
			if arrayIdx != nil {
				return fmt.Errorf("cannot set value: key %q does not exist (array indices must reference existing sequences)", keyPart)
			}
			if i == len(keys)-1 {
				// Last key - create scalar value
				return addKeyValue(current, k, value)
			}
			// Create intermediate mapping with the inferred indent step
			newMapping, err := addKeyMapping(current, keyPart, indentStep)
			if err != nil {
				return err
			}
			current = newMapping
			continue
		}

		// Unwrap anchor/alias nodes
		valueNode = unwrapNode(valueNode)

		// Handle array index
		if arrayIdx != nil {
			seqNode, ok := valueNode.(*ast.SequenceNode)
			if !ok {
				return fmt.Errorf("key %q is not a sequence", keyPart)
			}
			if *arrayIdx < 0 || *arrayIdx >= len(seqNode.Values) {
				return fmt.Errorf("array index %d out of range for key %q", *arrayIdx, keyPart)
			}
			valueNode = unwrapNode(seqNode.Values[*arrayIdx])
		}

		if i == len(keys)-1 {
			return setNodeValue(valueNode, value)
		}

		nextMapping, ok := valueNode.(*ast.MappingNode)
		if !ok {
			return fmt.Errorf("key %q is not a mapping", keyPart)
		}
		current = nextMapping
	}

	return nil
}

// getNestedValue gets a value at a nested path.
func getNestedValue(mapping *ast.MappingNode, keys []string) (string, error) {
	current := mapping

	for i, k := range keys {
		keyPart, arrayIdx := parseArrayIndex(k)

		var valueNode ast.Node
		for _, item := range current.Values {
			if keyNode, ok := item.Key.(*ast.StringNode); ok && keyNode.Value == keyPart {
				valueNode = item.Value
				break
			}
		}

		if valueNode == nil {
			return "", fmt.Errorf("key %q not found", keyPart)
		}

		// Unwrap anchor/alias nodes
		valueNode = unwrapNode(valueNode)

		// Handle array index
		if arrayIdx != nil {
			seqNode, ok := valueNode.(*ast.SequenceNode)
			if !ok {
				return "", fmt.Errorf("key %q is not a sequence", keyPart)
			}
			if *arrayIdx < 0 || *arrayIdx >= len(seqNode.Values) {
				return "", fmt.Errorf("array index %d out of range", *arrayIdx)
			}
			valueNode = unwrapNode(seqNode.Values[*arrayIdx])
		}

		if i == len(keys)-1 {
			return getScalarValue(valueNode)
		}

		nextMapping, ok := valueNode.(*ast.MappingNode)
		if !ok {
			return "", fmt.Errorf("key %q is not a mapping", keyPart)
		}
		current = nextMapping
	}

	return "", fmt.Errorf("value not found")
}

// parseArrayIndex parses a key that may contain an array index like "images[0]".
func parseArrayIndex(key string) (string, *int) {
	matches := arrayIndexPattern.FindStringSubmatch(key)
	if matches == nil {
		return key, nil
	}

	idx, err := strconv.Atoi(matches[2])
	if err != nil {
		return key, nil
	}

	return matches[1], &idx
}

// getMappingIndent gets the indentation for children of a mapping node.
func getMappingIndent(mapping *ast.MappingNode) int {
	// First try to get indent from existing Values
	if len(mapping.Values) > 0 {
		return mapping.Values[0].Key.GetToken().Position.IndentNum
	}
	// Otherwise get it from the mapping's Start token
	if mapping.Start != nil && mapping.Start.Position != nil {
		return mapping.Start.Position.IndentNum
	}
	// Default to 0 for root mapping
	return 0
}

// makeToken creates a token with position information.
// Column is 1-indexed, so indent 4 means column 5.
func makeToken(tokenType token.Type, value string, indent int) *token.Token {
	return &token.Token{
		Type:   tokenType,
		Value:  value,
		Origin: value,
		Position: &token.Position{
			IndentNum:   indent,
			IndentLevel: 0,
			Column:      indent + 1,
		},
	}
}

// makeEmptyMapping creates an empty mapping node with the given indent.
func makeEmptyMapping(indent int) *ast.MappingNode {
	startTok := &token.Token{
		Type: token.MappingStartType,
		Position: &token.Position{
			IndentNum:   indent,
			IndentLevel: 0,
			Column:      indent + 1,
		},
	}
	return ast.Mapping(startTok, false)
}

// getIndentStep infers the indent step size from the document.
// Returns the number of spaces to add when creating a child level.
func getIndentStep(mapping *ast.MappingNode) int {
	// Try to infer from a nested mapping value
	for _, item := range mapping.Values {
		if childMapping, ok := unwrapNode(item.Value).(*ast.MappingNode); ok {
			if len(childMapping.Values) > 0 {
				parentIndent := item.Key.GetToken().Position.IndentNum
				childIndent := childMapping.Values[0].Key.GetToken().Position.IndentNum
				if childIndent > parentIndent {
					return childIndent - parentIndent
				}
			}
		}
	}
	// Default to 2 spaces if we can't infer
	return 2
}

// addKeyValue adds a new key-value pair to a mapping.
func addKeyValue(mapping *ast.MappingNode, key string, value string) error {
	indent := getMappingIndent(mapping)

	keyTok := makeToken(token.StringType, key, indent)
	colonTok := makeToken(token.MappingValueType, ":", indent)
	valTok := makeToken(token.StringType, value, indent)

	keyNode := ast.String(keyTok)
	valNode := ast.String(valTok)
	newValue := ast.MappingValue(colonTok, keyNode, valNode)

	mapping.Values = append(mapping.Values, newValue)
	return nil
}

// addKeyMapping adds a new key with an empty mapping value.
func addKeyMapping(mapping *ast.MappingNode, key string, indentStep int) (*ast.MappingNode, error) {
	indent := getMappingIndent(mapping)
	childIndent := indent + indentStep

	keyTok := makeToken(token.StringType, key, indent)
	colonTok := makeToken(token.MappingValueType, ":", indent)

	keyNode := ast.String(keyTok)
	newMapping := makeEmptyMapping(childIndent)
	newValue := ast.MappingValue(colonTok, keyNode, newMapping)

	mapping.Values = append(mapping.Values, newValue)
	return newMapping, nil
}

// CreateEmptyValuesFile creates an empty values file with an optional header comment.
func CreateEmptyValuesFile(headerComment string) *ValuesFile {
	mapping := makeEmptyMapping(0)

	doc := &ast.DocumentNode{
		BaseNode: &ast.BaseNode{},
		Body:     mapping,
	}

	file := &ast.File{
		Docs: []*ast.DocumentNode{doc},
	}

	return &ValuesFile{
		file:          file,
		headerComment: headerComment,
	}
}
