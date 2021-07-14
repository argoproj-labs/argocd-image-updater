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
		err := AddRegistryEndpoint("example.com", "Example", "https://example.com", "", "", false, SortUnsorted, 5, 0, false)
		require.NoError(t, err)
	})
	t.Run("Get example.com endpoint", func(t *testing.T) {
		ep, err := GetRegistryEndpoint("example.com")
		require.NoError(t, err)
		require.NotNil(t, ep)
		assert.Equal(t, ep.RegistryPrefix, "example.com")
		assert.Equal(t, ep.RegistryName, "Example")
		assert.Equal(t, ep.RegistryAPI, "https://example.com")
		assert.Equal(t, ep.Insecure, false)
		assert.Equal(t, ep.DefaultNS, "")
		assert.Equal(t, ep.TagListSort, SortUnsorted)
	})
	t.Run("Change existing endpoint", func(t *testing.T) {
		err := AddRegistryEndpoint("example.com", "Example", "https://example.com", "", "library", true, SortLatestFirst, 5, 0, false)
		require.NoError(t, err)
		ep, err := GetRegistryEndpoint("example.com")
		require.NoError(t, err)
		require.NotNil(t, ep)
		assert.Equal(t, ep.Insecure, true)
		assert.Equal(t, ep.DefaultNS, "library")
		assert.Equal(t, ep.TagListSort, SortLatestFirst)
	})
}

func Test_SetEndpointCredentials(t *testing.T) {
	t.Run("Set credentials on default registry", func(t *testing.T) {
		err := SetRegistryEndpointCredentials("", "env:FOOBAR")
		require.NoError(t, err)
		ep, err := GetRegistryEndpoint("")
		require.NoError(t, err)
		require.NotNil(t, ep)
		assert.Equal(t, ep.Credentials, "env:FOOBAR")
	})

	t.Run("Unset credentials on default registry", func(t *testing.T) {
		err := SetRegistryEndpointCredentials("", "")
		require.NoError(t, err)
		ep, err := GetRegistryEndpoint("")
		require.NoError(t, err)
		require.NotNil(t, ep)
		assert.Equal(t, ep.Credentials, "")
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
				creds := fmt.Sprintf("secret:foo/secret-%d", i)
				err := SetRegistryEndpointCredentials("", creds)
				require.NoError(t, err)
				ep, err := GetRegistryEndpoint("")
				require.NoError(t, err)
				require.NotNil(t, ep)
			}(i)
		}
	})
}

func Test_DeepCopy(t *testing.T) {
	t.Run("DeepCopy endpoint object", func(t *testing.T) {
		ep, err := GetRegistryEndpoint("docker.pkg.github.com")
		require.NoError(t, err)
		require.NotNil(t, ep)
		newEp := ep.DeepCopy()
		assert.Equal(t, ep.RegistryAPI, newEp.RegistryAPI)
		assert.Equal(t, ep.RegistryName, newEp.RegistryName)
		assert.Equal(t, ep.RegistryPrefix, newEp.RegistryPrefix)
		assert.Equal(t, ep.Credentials, newEp.Credentials)
		assert.Equal(t, ep.TagListSort, newEp.TagListSort)
		assert.Equal(t, ep.Username, newEp.Username)
		assert.Equal(t, ep.Ping, newEp.Ping)
	})
}

func Test_GetTagListSortFromString(t *testing.T) {
	t.Run("Get latest-first sorting", func(t *testing.T) {
		tls := TagListSortFromString("latest-first")
		assert.Equal(t, SortLatestFirst, tls)
	})
	t.Run("Get latest-last sorting", func(t *testing.T) {
		tls := TagListSortFromString("latest-last")
		assert.Equal(t, SortLatestLast, tls)
	})
	t.Run("Get none sorting explicit", func(t *testing.T) {
		tls := TagListSortFromString("none")
		assert.Equal(t, SortUnsorted, tls)
	})
	t.Run("Get none sorting implicit", func(t *testing.T) {
		tls := TagListSortFromString("")
		assert.Equal(t, SortUnsorted, tls)
	})
	t.Run("Get none sorting from unknown", func(t *testing.T) {
		tls := TagListSortFromString("unknown")
		assert.Equal(t, SortUnsorted, tls)
	})
}
