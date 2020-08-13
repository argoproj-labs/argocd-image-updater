package registry

import (
	"testing"

	"github.com/argoproj-labs/argocd-image-updater/test/fixture"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ParseRegistryConfFromYaml(t *testing.T) {
	t.Run("Parse from valid YAML", func(t *testing.T) {
		data := fixture.MustReadFile("../../config/example-config.yaml")
		regList, err := ParseRegistryConfiguration(data)
		require.NoError(t, err)
		assert.Len(t, regList.Items, 3)
	})
}

func Test_LoadRegistryConfiguration(t *testing.T) {
	t.Run("Load from valid location", func(t *testing.T) {
		err := LoadRegistryConfiguration("../../config/example-config.yaml")
		require.NoError(t, err)
		assert.Len(t, registries, 3)
		reg, err := GetRegistryEndpoint("gcr.io")
		require.NoError(t, err)
		assert.Equal(t, "pullsecret:foo/bar", reg.Credentials)
		RestoreDefaultRegistryConfiguration()
		reg, err = GetRegistryEndpoint("gcr.io")
		require.NoError(t, err)
		assert.Equal(t, "", reg.Credentials)
	})
}
