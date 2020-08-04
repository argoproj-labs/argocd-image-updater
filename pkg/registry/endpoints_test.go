package registry

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_GetEndpoints(t *testing.T) {
	t.Run("Get default endpoint", func(t *testing.T) {
		ep, err := GetRegistryEndpoint("")
		require.NoError(t, err)
		require.NotNil(t, ep)
		assert.Equal(t, ep.RegistryPrefix, "")
	})
	t.Run("Get GCR endpoint", func(t *testing.T) {
		ep, err := GetRegistryEndpoint("gcr.io")
		require.NoError(t, err)
		require.NotNil(t, ep)
		assert.Equal(t, ep.RegistryPrefix, "gcr.io")
	})

	t.Run("Get non-existing endpoint", func(t *testing.T) {
		ep, err := GetRegistryEndpoint("foobar.com")
		assert.Error(t, err)
		assert.Nil(t, ep)
	})
}

func Test_AddEndpoint(t *testing.T) {
	t.Run("Add new endpoint", func(t *testing.T) {
		err := AddRegistryEndpoint("example.com", "Example", "https://example.com", "", "", "")
		require.NoError(t, err)
	})
	t.Run("Get example.com endpoint", func(t *testing.T) {
		ep, err := GetRegistryEndpoint("example.com")
		require.NoError(t, err)
		require.NotNil(t, ep)
		assert.Equal(t, ep.RegistryPrefix, "example.com")
		assert.Equal(t, ep.RegistryName, "Example")
		assert.Equal(t, ep.RegistryAPI, "https://example.com")
	})
	t.Run("Change existing endpoint", func(t *testing.T) {
		err := AddRegistryEndpoint("example.com", "Example", "https://example.com", "", "", "")
		require.NoError(t, err)
	})
}

func Test_SetEndpointCredentials(t *testing.T) {
	t.Run("Set credentials on default registry", func(t *testing.T) {
		err := SetRegistryEndpointCredentials("", "username", "password")
		require.NoError(t, err)
		ep, err := GetRegistryEndpoint("")
		require.NoError(t, err)
		require.NotNil(t, ep)
		assert.Equal(t, ep.Username, "username")
		assert.Equal(t, ep.Password, "password")
	})

	t.Run("Unset credentials on default registry", func(t *testing.T) {
		err := SetRegistryEndpointCredentials("", "", "")
		require.NoError(t, err)
		ep, err := GetRegistryEndpoint("")
		require.NoError(t, err)
		require.NotNil(t, ep)
		assert.Equal(t, ep.Username, "")
		assert.Equal(t, ep.Password, "")
	})
}

func Test_EndpointConcurrentAccess(t *testing.T) {
	// Make sure we're not deadlocking on read
	t.Run("Concurrent read access", func(t *testing.T) {
		for i := 0; i < 50; i++ {
			go func() {
				ep, err := GetRegistryEndpoint("gcr.io")
				require.NoError(t, err)
				require.NotNil(t, ep)
			}()
		}
	})
	// Make sure we're not deadlocking on write
	t.Run("Concurrent write access", func(t *testing.T) {
		for i := 0; i < 50; i++ {
			go func(i int) {
				username := fmt.Sprintf("Username-%d", i)
				password := fmt.Sprintf("Password-%d", i)
				err := SetRegistryEndpointCredentials("", username, password)
				require.NoError(t, err)
				ep, err := GetRegistryEndpoint("")
				require.NoError(t, err)
				require.NotNil(t, ep)
			}(i)
		}
	})
}
