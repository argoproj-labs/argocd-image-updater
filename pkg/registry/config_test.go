package registry

import (
	"testing"
	"time"

	"github.com/argoproj-labs/argocd-image-updater/test/fixture"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ParseRegistryConfFromYaml(t *testing.T) {
	t.Run("Parse from valid YAML", func(t *testing.T) {
		data := fixture.MustReadFile("../../config/example-config.yaml")
		regList, err := ParseRegistryConfiguration(data)
		require.NoError(t, err)
		assert.Len(t, regList.Items, 4)
	})

	t.Run("Parse from invalid YAML: no name found", func(t *testing.T) {
		registries := `
registries:
- api_url: https://foo.io
  ping: false
`
		regList, err := ParseRegistryConfiguration(registries)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name is missing")
		assert.Len(t, regList.Items, 0)
	})

	t.Run("Parse from invalid YAML: no API URL found", func(t *testing.T) {
		registries := `
registries:
- name: Foobar Registry
  ping: false
`
		regList, err := ParseRegistryConfiguration(registries)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "API URL must be")
		assert.Len(t, regList.Items, 0)
	})

	t.Run("Parse from invalid YAML: multiple registries without prefix", func(t *testing.T) {
		registries := `
registries:
- name: Foobar Registry
  api_url: https://foobar.io
  ping: false
- name: Barbar Registry
  api_url: https://barbar.io
  ping: false
`
		regList, err := ParseRegistryConfiguration(registries)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already is Foobar Registry")
		assert.Len(t, regList.Items, 0)
	})

	t.Run("Parse from invalid YAML: invalid tag sort mode", func(t *testing.T) {
		registries := `
registries:
- name: Foobar Registry
  api_url: https://foobar.io
  ping: false
  tagsortmode: invalid
`
		regList, err := ParseRegistryConfiguration(registries)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown tag sort mode")
		assert.Len(t, regList.Items, 0)
	})

}

func Test_LoadRegistryConfiguration(t *testing.T) {
	t.Run("Load from valid location", func(t *testing.T) {
		err := LoadRegistryConfiguration("../../config/example-config.yaml", false)
		require.NoError(t, err)
		assert.Len(t, registries, 5)
		reg, err := GetRegistryEndpoint("gcr.io")
		require.NoError(t, err)
		assert.Equal(t, "pullsecret:foo/bar", reg.Credentials)
		reg, err = GetRegistryEndpoint("ghcr.io")
		require.NoError(t, err)
		assert.Equal(t, "ext:/some/script", reg.Credentials)
		assert.Equal(t, 5*time.Hour, reg.CredsExpire)
		RestoreDefaultRegistryConfiguration()
		reg, err = GetRegistryEndpoint("gcr.io")
		require.NoError(t, err)
		assert.Equal(t, "", reg.Credentials)
	})
}
