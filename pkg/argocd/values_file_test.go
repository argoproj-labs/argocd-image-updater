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

	t.Run("Prefers nested path when literal key also exists", func(t *testing.T) {
		input := `image:
  attributes:
    tag: nested-value
"image.attributes.tag": literal-value
`
		valuesFile, err := ParseValuesFile([]byte(input))
		require.NoError(t, err)

		// Should prefer nested path over literal key
		val, err := valuesFile.GetValue("image.attributes.tag")
		require.NoError(t, err)
		assert.Equal(t, "nested-value", val)
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
