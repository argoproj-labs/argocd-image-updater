package argocd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ValuesFile_SetValue(t *testing.T) {
	t.Run("Simple nested value update preserves formatting", func(t *testing.T) {
		input := `image:
  tag: v1.0.0
`
		expected := `image:
  tag: v2.0.0
`
		valuesFile, err := ParseValuesFile([]byte(input))
		require.NoError(t, err)

		err = valuesFile.SetValue("image.tag", "v2.0.0")
		require.NoError(t, err)

		assert.Equal(t, expected, valuesFile.String())
	})

	t.Run("Preserves blank lines between sections", func(t *testing.T) {
		input := `section1:
  key1: value1

section2:
  key2: value2

section3:
  image:
    tag: v1.0.0
`
		expected := `section1:
  key1: value1

section2:
  key2: value2

section3:
  image:
    tag: v2.0.0
`
		valuesFile, err := ParseValuesFile([]byte(input))
		require.NoError(t, err)

		err = valuesFile.SetValue("section3.image.tag", "v2.0.0")
		require.NoError(t, err)

		assert.Equal(t, expected, valuesFile.String())
	})

	t.Run("Preserves header comments", func(t *testing.T) {
		input := `# This is a header comment
image:
  tag: v1.0.0
`
		expected := `# This is a header comment
image:
  tag: v2.0.0
`
		valuesFile, err := ParseValuesFile([]byte(input))
		require.NoError(t, err)

		err = valuesFile.SetValue("image.tag", "v2.0.0")
		require.NoError(t, err)

		assert.Equal(t, expected, valuesFile.String())
	})

	t.Run("Preserves inline comments", func(t *testing.T) {
		input := `image:
  tag: v1.0.0 # current version
`
		expected := `image:
  tag: v2.0.0 # current version
`
		valuesFile, err := ParseValuesFile([]byte(input))
		require.NoError(t, err)

		err = valuesFile.SetValue("image.tag", "v2.0.0")
		require.NoError(t, err)

		assert.Equal(t, expected, valuesFile.String())
	})

	t.Run("Preserves anchors and aliases", func(t *testing.T) {
		input := `global: &defaults
  tag: v1.0.0

app:
  <<: *defaults
  name: myapp
`
		expected := `global: &defaults
  tag: v2.0.0

app:
  <<: *defaults
  name: myapp
`
		valuesFile, err := ParseValuesFile([]byte(input))
		require.NoError(t, err)

		err = valuesFile.SetValue("global.tag", "v2.0.0")
		require.NoError(t, err)

		assert.Equal(t, expected, valuesFile.String())
	})

	t.Run("Updates array element", func(t *testing.T) {
		input := `images:
- name: app1
  tag: v1.0.0
- name: app2
  tag: v1.0.0
`
		expected := `images:
- name: app1
  tag: v2.0.0
- name: app2
  tag: v1.0.0
`
		valuesFile, err := ParseValuesFile([]byte(input))
		require.NoError(t, err)

		err = valuesFile.SetValue("images[0].tag", "v2.0.0")
		require.NoError(t, err)

		assert.Equal(t, expected, valuesFile.String())
	})

	t.Run("Creates new key when not exists", func(t *testing.T) {
		input := `image:
  repository: nginx
`
		valuesFile, err := ParseValuesFile([]byte(input))
		require.NoError(t, err)

		err = valuesFile.SetValue("image.tag", "v1.0.0")
		require.NoError(t, err)

		output := valuesFile.String()
		assert.Contains(t, output, "tag: v1.0.0")
		assert.Contains(t, output, "repository: nginx")
	})

	t.Run("Handles literal key with dots", func(t *testing.T) {
		input := `"image.tag": v1.0.0
`
		valuesFile, err := ParseValuesFile([]byte(input))
		require.NoError(t, err)

		err = valuesFile.SetValue("image.tag", "v2.0.0")
		// This should match the literal key "image.tag"
		require.NoError(t, err)

		output := valuesFile.String()
		assert.Contains(t, output, "v2.0.0")
	})

	t.Run("Fails when trying to create missing array index", func(t *testing.T) {
		input := `other:
  field: value
`
		valuesFile, err := ParseValuesFile([]byte(input))
		require.NoError(t, err)

		// Trying to set images[0].tag when images doesn't exist should fail
		err = valuesFile.SetValue("images[0].tag", "v1.0.0")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "does not exist")
	})

	t.Run("Updates integer value", func(t *testing.T) {
		input := `replicas: 3
`
		expected := `replicas: 5
`
		valuesFile, err := ParseValuesFile([]byte(input))
		require.NoError(t, err)

		err = valuesFile.SetValue("replicas", "5")
		require.NoError(t, err)

		assert.Equal(t, expected, valuesFile.String())
	})

	t.Run("Updates float value", func(t *testing.T) {
		input := `cpu: 0.5
`
		expected := `cpu: 1.5
`
		valuesFile, err := ParseValuesFile([]byte(input))
		require.NoError(t, err)

		err = valuesFile.SetValue("cpu", "1.5")
		require.NoError(t, err)

		assert.Equal(t, expected, valuesFile.String())
	})

	t.Run("Updates boolean value", func(t *testing.T) {
		input := `enabled: true
`
		expected := `enabled: false
`
		valuesFile, err := ParseValuesFile([]byte(input))
		require.NoError(t, err)

		err = valuesFile.SetValue("enabled", "false")
		require.NoError(t, err)

		assert.Equal(t, expected, valuesFile.String())
	})

	t.Run("Validates integer format", func(t *testing.T) {
		input := `replicas: 3
`
		valuesFile, err := ParseValuesFile([]byte(input))
		require.NoError(t, err)

		err = valuesFile.SetValue("replicas", "not-a-number")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot set integer value")
	})

	t.Run("Handles comment-only YAML file", func(t *testing.T) {
		input := `# This is just a comment
# Another comment
`
		valuesFile, err := ParseValuesFile([]byte(input))
		require.NoError(t, err)

		// Should be able to add a value to a comment-only file
		err = valuesFile.SetValue("image.tag", "v1.0.0")
		require.NoError(t, err)

		output := valuesFile.String()
		assert.Contains(t, output, "tag: v1.0.0")
		// Comments should be preserved
		assert.Contains(t, output, "# This is just a comment")
	})

	t.Run("Infers 4-space indent from existing file", func(t *testing.T) {
		input := `section1:
    key1:
        nested: value1
`
		valuesFile, err := ParseValuesFile([]byte(input))
		require.NoError(t, err)

		// Add a new nested key - should use 4-space indent like existing structure
		err = valuesFile.SetValue("section2.key2.nested", "value2")
		require.NoError(t, err)

		output := valuesFile.String()
		// The new section should match the existing 4-space indent pattern
		assert.Contains(t, output, "section2:")
		assert.Contains(t, output, "    key2:")
		assert.Contains(t, output, "        nested: value2")
	})

	t.Run("Uses default 2-space indent when structure is flat", func(t *testing.T) {
		input := `key1: value1
key2: value2
`
		valuesFile, err := ParseValuesFile([]byte(input))
		require.NoError(t, err)

		// Add nested structure - should use default 2-space indent
		err = valuesFile.SetValue("section.nested.key", "value")
		require.NoError(t, err)

		output := valuesFile.String()
		assert.Contains(t, output, "section:")
		assert.Contains(t, output, "  nested:")
		assert.Contains(t, output, "    key: value")
	})
}

func Test_ValuesFile_GetValue(t *testing.T) {
	t.Run("Gets nested value", func(t *testing.T) {
		input := `image:
  tag: v1.0.0
`
		valuesFile, err := ParseValuesFile([]byte(input))
		require.NoError(t, err)

		val, err := valuesFile.GetValue("image.tag")
		require.NoError(t, err)
		assert.Equal(t, "v1.0.0", val)
	})

	t.Run("Gets array element value", func(t *testing.T) {
		input := `images:
- name: app1
  tag: v1.0.0
- name: app2
  tag: v2.0.0
`
		valuesFile, err := ParseValuesFile([]byte(input))
		require.NoError(t, err)

		val, err := valuesFile.GetValue("images[1].tag")
		require.NoError(t, err)
		assert.Equal(t, "v2.0.0", val)
	})

	t.Run("Returns error for non-existent key", func(t *testing.T) {
		input := `image:
  tag: v1.0.0
`
		valuesFile, err := ParseValuesFile([]byte(input))
		require.NoError(t, err)

		_, err = valuesFile.GetValue("image.nonexistent")
		assert.Error(t, err)
	})

	t.Run("Follows aliases", func(t *testing.T) {
		input := `defaults: &defaults
  tag: v1.0.0

app:
  settings: *defaults
`
		valuesFile, err := ParseValuesFile([]byte(input))
		require.NoError(t, err)

		// Note: This tests that we can follow aliases in the getter
		val, err := valuesFile.GetValue("defaults.tag")
		require.NoError(t, err)
		assert.Equal(t, "v1.0.0", val)
	})

	t.Run("Gets root level key", func(t *testing.T) {
		input := `name: repo-name
tag: v1.0.0
`
		valuesFile, err := ParseValuesFile([]byte(input))
		require.NoError(t, err)

		val, err := valuesFile.GetValue("name")
		require.NoError(t, err)
		assert.Equal(t, "repo-name", val)
	})

	t.Run("Prefers literal key when both literal and nested exist", func(t *testing.T) {
		input := `image:
  attributes:
    tag: nested-value
"image.attributes.tag": literal-value
`
		valuesFile, err := ParseValuesFile([]byte(input))
		require.NoError(t, err)

		// Should prefer literal key over nested path for consistency with SetValue
		val, err := valuesFile.GetValue("image.attributes.tag")
		require.NoError(t, err)
		assert.Equal(t, "literal-value", val)
	})

	t.Run("Falls back to literal key when nested path does not exist", func(t *testing.T) {
		input := `"image.attributes.tag": literal-value
other:
  field: value
`
		valuesFile, err := ParseValuesFile([]byte(input))
		require.NoError(t, err)

		val, err := valuesFile.GetValue("image.attributes.tag")
		require.NoError(t, err)
		assert.Equal(t, "literal-value", val)
	})

	t.Run("Gets deeply nested value", func(t *testing.T) {
		input := `image:
  attributes:
    name: repo-name
    tag: v1.0.0
`
		valuesFile, err := ParseValuesFile([]byte(input))
		require.NoError(t, err)

		val, err := valuesFile.GetValue("image.attributes.tag")
		require.NoError(t, err)
		assert.Equal(t, "v1.0.0", val)
	})

	t.Run("Round-trip consistency with literal and nested keys", func(t *testing.T) {
		input := `image:
  attributes:
    tag: nested-value
"image.attributes.tag": literal-value
`
		valuesFile, err := ParseValuesFile([]byte(input))
		require.NoError(t, err)

		// Both SetValue and GetValue should operate on the same key
		err = valuesFile.SetValue("image.attributes.tag", "updated-value")
		require.NoError(t, err)

		val, err := valuesFile.GetValue("image.attributes.tag")
		require.NoError(t, err)
		assert.Equal(t, "updated-value", val, "GetValue should return the value that SetValue just set")
	})
}

func Test_ValuesFile_FullFormattingPreservation(t *testing.T) {
	t.Run("Complex document with all formatting features", func(t *testing.T) {
		input := `# This is a header comment describing the values file
# It spans multiple lines

# Global settings section
global:
  # Environment configuration
  environment: production
  debug: false

# Image configuration with anchors
images:
  # Primary application image
  app: &app-image
    repository: myapp
    tag: v1.0.0 # Current version
    pullPolicy: IfNotPresent

  # Sidecar image using alias
  sidecar:
    <<: *app-image
    tag: v1.0.0

# Database settings
database:
  host: localhost
  port: 5432

  # Connection pool settings
  pool:
    minSize: 5
    maxSize: 20
`

		expected := `# This is a header comment describing the values file
# It spans multiple lines

# Global settings section
global:
  # Environment configuration
  environment: production
  debug: false

# Image configuration with anchors
images:
  # Primary application image
  app: &app-image
    repository: myapp
    tag: v2.0.0 # Current version
    pullPolicy: IfNotPresent

  # Sidecar image using alias
  sidecar:
    <<: *app-image
    tag: v2.0.0

# Database settings
database:
  host: localhost
  port: 5432

  # Connection pool settings
  pool:
    minSize: 5
    maxSize: 20
`

		valuesFile, err := ParseValuesFile([]byte(input))
		require.NoError(t, err)

		// Update image tags
		err = valuesFile.SetValue("images.app.tag", "v2.0.0")
		require.NoError(t, err)

		err = valuesFile.SetValue("images.sidecar.tag", "v2.0.0")
		require.NoError(t, err)

		assert.Equal(t, expected, valuesFile.String(), "Full formatting should be preserved")
	})
}
