package registry

import (
	"fmt"
	"sync"
	"testing"

	"github.com/argoproj-labs/argocd-image-updater/pkg/image"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_GetEndpoints(t *testing.T) {
	RestoreDefaultRegistryConfiguration()

	t.Run("Get default endpoint", func(t *testing.T) {
		ep, err := GetRegistryEndpoint(&image.ContainerImage{RegistryURL: ""})
		require.NoError(t, err)
		require.NotNil(t, ep)
		assert.Equal(t, "docker.io", ep.RegistryPrefix)
	})

	t.Run("Get GCR endpoint", func(t *testing.T) {
		ep, err := GetRegistryEndpoint(&image.ContainerImage{RegistryURL: "gcr.io"})
		require.NoError(t, err)
		require.NotNil(t, ep)
		assert.Equal(t, ep.RegistryPrefix, "gcr.io")
	})

	t.Run("Infer endpoint", func(t *testing.T) {
		ep, err := GetRegistryEndpoint(&image.ContainerImage{RegistryURL: "foobar.com"})
		require.NoError(t, err)
		require.NotNil(t, ep)
		assert.Equal(t, "foobar.com", ep.RegistryPrefix)
		assert.Equal(t, "https://foobar.com", ep.RegistryAPI)
	})
}

func Test_AddEndpoint(t *testing.T) {
	RestoreDefaultRegistryConfiguration()

	t.Run("Add new endpoint", func(t *testing.T) {
		err := AddRegistryEndpoint(NewRegistryEndpoint("example.com", "Example", "https://example.com", "", "", false, TagListSortUnsorted, 5, 0))
		require.NoError(t, err)
	})
	t.Run("Get example.com endpoint", func(t *testing.T) {
		ep, err := GetRegistryEndpoint(&image.ContainerImage{RegistryURL: "example.com"})
		require.NoError(t, err)
		require.NotNil(t, ep)
		assert.Equal(t, ep.RegistryPrefix, "example.com")
		assert.Equal(t, ep.RegistryName, "Example")
		assert.Equal(t, ep.RegistryAPI, "https://example.com")
		assert.Equal(t, ep.Insecure, false)
		assert.Equal(t, ep.DefaultNS, "")
		assert.Equal(t, ep.TagListSort, TagListSortUnsorted)
	})
	t.Run("Change existing endpoint", func(t *testing.T) {
		err := AddRegistryEndpoint(NewRegistryEndpoint("example.com", "Example", "https://example.com", "", "library", true, TagListSortLatestFirst, 5, 0))
		require.NoError(t, err)
		ep, err := GetRegistryEndpoint(&image.ContainerImage{RegistryURL: "example.com"})
		require.NoError(t, err)
		require.NotNil(t, ep)
		assert.Equal(t, ep.Insecure, true)
		assert.Equal(t, ep.DefaultNS, "library")
		assert.Equal(t, ep.TagListSort, TagListSortLatestFirst)
	})
}

func Test_SetEndpointCredentials(t *testing.T) {
	RestoreDefaultRegistryConfiguration()

	t.Run("Set credentials on default registry", func(t *testing.T) {
		err := SetRegistryEndpointCredentials("", "env:FOOBAR")
		require.NoError(t, err)
		ep, err := GetRegistryEndpoint(&image.ContainerImage{RegistryURL: ""})
		require.NoError(t, err)
		require.NotNil(t, ep)
		assert.Equal(t, ep.Credentials, "env:FOOBAR")
	})

	t.Run("Unset credentials on default registry", func(t *testing.T) {
		err := SetRegistryEndpointCredentials("", "")
		require.NoError(t, err)
		ep, err := GetRegistryEndpoint(&image.ContainerImage{RegistryURL: ""})
		require.NoError(t, err)
		require.NotNil(t, ep)
		assert.Equal(t, ep.Credentials, "")
	})
}

func Test_SelectRegistryBasedOnMaxPrefixContains(t *testing.T) {
	RestoreDefaultRegistryConfiguration()

	t.Run("Set credentials on default registry", func(t *testing.T) {
		err := SetRegistryEndpointCredentials("foo.bar/prefix1", "env:FOOBAR_1")
		require.NoError(t, err)
		err = SetRegistryEndpointCredentials("foo.bar/prefix2", "env:FOOBAR_2")
		require.NoError(t, err)
		err = SetRegistryEndpointCredentials("foo.bar/prefix1/sub-prefix", "env:FOOBAR_SUB_1")
		require.NoError(t, err)

		ep, err := GetRegistryEndpoint(&image.ContainerImage{RegistryURL: "foo.bar", ImageName: "prefix1/sub-prefix/image"})
		require.NoError(t, err)
		require.NotNil(t, ep)
		assert.Equal(t, ep.Credentials, "env:FOOBAR_SUB_1")
	})
}

func Test_EndpointConcurrentAccess(t *testing.T) {
	RestoreDefaultRegistryConfiguration()
	const numRuns = 50
	// Make sure we're not deadlocking on read
	t.Run("Concurrent read access", func(t *testing.T) {
		var wg sync.WaitGroup
		wg.Add(numRuns)
		for i := 0; i < numRuns; i++ {
			go func() {
				ep, err := GetRegistryEndpoint(&image.ContainerImage{RegistryURL: "gcr.io"})
				require.NoError(t, err)
				require.NotNil(t, ep)
				wg.Done()
			}()
		}
		wg.Wait()
	})

	// Make sure we're not deadlocking on write
	t.Run("Concurrent write access", func(t *testing.T) {
		var wg sync.WaitGroup
		wg.Add(numRuns)
		for i := 0; i < numRuns; i++ {
			go func(i int) {
				creds := fmt.Sprintf("secret:foo/secret-%d", i)
				err := SetRegistryEndpointCredentials("", creds)
				require.NoError(t, err)
				ep, err := GetRegistryEndpoint(&image.ContainerImage{RegistryURL: ""})
				require.NoError(t, err)
				require.NotNil(t, ep)
				wg.Done()
			}(i)
		}
		wg.Wait()
	})
}

func Test_SetDefault(t *testing.T) {
	RestoreDefaultRegistryConfiguration()

	dep := GetDefaultRegistry()
	require.NotNil(t, dep)
	assert.Equal(t, "docker.io", dep.RegistryPrefix)
	assert.True(t, dep.IsDefault)

	ep, err := GetRegistryEndpoint(&image.ContainerImage{RegistryURL: "ghcr.io"})
	require.NoError(t, err)
	require.NotNil(t, ep)
	require.False(t, ep.IsDefault)

	SetDefaultRegistry(ep)
	assert.True(t, ep.IsDefault)
	assert.False(t, dep.IsDefault)
	require.NotNil(t, GetDefaultRegistry())
	assert.Equal(t, ep.RegistryPrefix, GetDefaultRegistry().RegistryPrefix)
}

func Test_DeepCopy(t *testing.T) {
	t.Run("DeepCopy endpoint object", func(t *testing.T) {
		ep, err := GetRegistryEndpoint(&image.ContainerImage{RegistryURL: "docker.pkg.github.com"})
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
		assert.Equal(t, TagListSortLatestFirst, tls)
	})
	t.Run("Get latest-last sorting", func(t *testing.T) {
		tls := TagListSortFromString("latest-last")
		assert.Equal(t, TagListSortLatestLast, tls)
	})
	t.Run("Get none sorting explicit", func(t *testing.T) {
		tls := TagListSortFromString("none")
		assert.Equal(t, TagListSortUnsorted, tls)
	})
	t.Run("Get none sorting implicit", func(t *testing.T) {
		tls := TagListSortFromString("")
		assert.Equal(t, TagListSortUnsorted, tls)
	})
	t.Run("Get unknown sorting from unknown string", func(t *testing.T) {
		tls := TagListSortFromString("unknown")
		assert.Equal(t, TagListSortUnknown, tls)
	})
}
