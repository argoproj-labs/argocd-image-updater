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

	if root == nil {
		return fmt.Errorf("empty document body")
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

	// First try nested path
	keys := strings.Split(key, ".")
	if val, err := getNestedValue(rootMapping, keys); err == nil {
		return val, nil
	}

	// Try as literal key
	if node := findKeyInMapping(rootMapping, key); node != nil {
		return getScalarValue(node)
	}

	return "", fmt.Errorf("key %q not found", key)
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

// setNodeValue sets the value of a scalar node, preserving comments.
func setNodeValue(node ast.Node, value string) error {
	// Unwrap anchor/alias nodes
	node = unwrapNode(node)

	stringNode, ok := node.(*ast.StringNode)
	if !ok {
		return fmt.Errorf("cannot set value on non-scalar node (type: %T)", node)
	}

	comment := stringNode.GetComment()
	stringNode.Value = value
	if comment != nil {
		stringNode.SetComment(comment)
	}

	return nil
}

// getScalarValue extracts the string value from a node.
func getScalarValue(node ast.Node) (string, error) {
	// Handle alias nodes
	if aliasNode, ok := node.(*ast.AliasNode); ok {
		return getScalarValue(aliasNode.Value)
	}

	if stringNode, ok := node.(*ast.StringNode); ok {
		return stringNode.Value, nil
	}

	if intNode, ok := node.(*ast.IntegerNode); ok {
		return intNode.GetToken().Value, nil
	}

	if floatNode, ok := node.(*ast.FloatNode); ok {
		return floatNode.GetToken().Value, nil
	}

	if boolNode, ok := node.(*ast.BoolNode); ok {
		return boolNode.GetToken().Value, nil
	}

	return "", fmt.Errorf("node is not a scalar value (type: %T)", node)
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

	for i, k := range keys {
		keyPart, arrayIdx := parseArrayIndex(k)

		var valueNode ast.Node
		var mappingValueNode *ast.MappingValueNode

		for _, item := range current.Values {
			if keyNode, ok := item.Key.(*ast.StringNode); ok && keyNode.Value == keyPart {
				valueNode = item.Value
				mappingValueNode = item
				break
			}
		}

		// If key not found, create it
		if valueNode == nil {
			if i == len(keys)-1 {
				// Last key - create scalar value
				return addKeyValue(current, k, value)
			}
			// Create intermediate mapping
			newMapping, err := addKeyMapping(current, keyPart)
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
			if mappingValueNode != nil && arrayIdx == nil {
				return setNodeValue(unwrapNode(mappingValueNode.Value), value)
			}
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

// addKeyValue adds a new key-value pair to a mapping.
func addKeyValue(mapping *ast.MappingNode, key string, value string) error {
	indent := getMappingIndent(mapping)

	keyTok := &token.Token{
		Type:   token.StringType,
		Value:  key,
		Origin: key,
		Position: &token.Position{
			IndentNum:   indent,
			IndentLevel: 0,
			Column:      indent + 1,
		},
	}

	colonTok := &token.Token{
		Type:   token.MappingValueType,
		Value:  ":",
		Origin: ":",
		Position: &token.Position{
			IndentNum:   indent,
			IndentLevel: 0,
			Column:      indent + len(key) + 1,
		},
	}

	valTok := &token.Token{
		Type:   token.StringType,
		Value:  value,
		Origin: value,
		Position: &token.Position{
			IndentNum:   indent,
			IndentLevel: 0,
			Column:      indent + len(key) + 3,
		},
	}

	keyNode := &ast.StringNode{
		BaseNode: &ast.BaseNode{},
		Token:    keyTok,
		Value:    key,
	}

	valNode := &ast.StringNode{
		BaseNode: &ast.BaseNode{},
		Token:    valTok,
		Value:    value,
	}

	newValue := &ast.MappingValueNode{
		BaseNode: &ast.BaseNode{},
		Start:    colonTok,
		Key:      keyNode,
		Value:    valNode,
	}

	mapping.Values = append(mapping.Values, newValue)
	return nil
}

// addKeyMapping adds a new key with an empty mapping value.
func addKeyMapping(mapping *ast.MappingNode, key string) (*ast.MappingNode, error) {
	indent := getMappingIndent(mapping)

	// Calculate child indent (2 more than current)
	childIndent := indent + 2

	keyTok := &token.Token{
		Type:   token.StringType,
		Value:  key,
		Origin: key,
		Position: &token.Position{
			IndentNum:   indent,
			IndentLevel: 0,
			Column:      indent + 1,
		},
	}

	colonTok := &token.Token{
		Type:   token.MappingValueType,
		Value:  ":",
		Origin: ":",
		Position: &token.Position{
			IndentNum:   indent,
			IndentLevel: 0,
			Column:      indent + len(key) + 1,
		},
	}

	keyNode := &ast.StringNode{
		BaseNode: &ast.BaseNode{},
		Token:    keyTok,
		Value:    key,
	}

	newMapping := &ast.MappingNode{
		BaseNode: &ast.BaseNode{},
		Values:   []*ast.MappingValueNode{},
		Start: &token.Token{
			Type: token.MappingStartType,
			Position: &token.Position{
				IndentNum:   childIndent,
				IndentLevel: 0,
			},
		},
	}

	newValue := &ast.MappingValueNode{
		BaseNode: &ast.BaseNode{},
		Start:    colonTok,
		Key:      keyNode,
		Value:    newMapping,
	}

	mapping.Values = append(mapping.Values, newValue)
	return newMapping, nil
}

// CreateEmptyValuesFile creates an empty values file with an optional header comment.
func CreateEmptyValuesFile(headerComment string) *ValuesFile {
	mapping := &ast.MappingNode{
		BaseNode: &ast.BaseNode{},
		Values:   []*ast.MappingValueNode{},
		Start: &token.Token{
			Type: token.MappingStartType,
			Position: &token.Position{
				IndentNum:   0,
				IndentLevel: 0,
			},
		},
	}

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
