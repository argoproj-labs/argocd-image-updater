package registry

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInferRegistryEndpointFromPrefix(t *testing.T) {
	prefix := "example.com"
	expectedAPIURL := "https://" + prefix
	endpoint := inferRegistryEndpointFromPrefix(prefix)
	assert.NotNil(t, endpoint)
	assert.Equal(t, prefix, endpoint.RegistryName)
	assert.Equal(t, prefix, endpoint.RegistryPrefix)
	assert.Equal(t, expectedAPIURL, endpoint.RegistryAPI)
	assert.Equal(t, TagListSortUnsorted, endpoint.TagListSort)
	assert.Equal(t, 20, endpoint.limit)
	assert.False(t, endpoint.Insecure)
}

func TestNewRegistryEndpoint(t *testing.T) {
	prefix := "example.com"
	name := "exampleRegistry"
	apiUrl := "https://api.example.com"
	credentials := "user:pass"
	defaultNS := "default"
	insecure := true
	tagListSort := TagListSortLatestFirst
	limit := 10
	credsExpire := time.Minute * 30
	endpoint := NewRegistryEndpoint(prefix, name, apiUrl, credentials, defaultNS, insecure, tagListSort, limit, credsExpire)
	assert.NotNil(t, endpoint)
	assert.Equal(t, name, endpoint.RegistryName)
	assert.Equal(t, prefix, endpoint.RegistryPrefix)
	assert.Equal(t, strings.TrimSuffix(apiUrl, "/"), endpoint.RegistryAPI)
	assert.Equal(t, credentials, endpoint.Credentials)
	assert.Equal(t, credsExpire, endpoint.CredsExpire)
	assert.Equal(t, insecure, endpoint.Insecure)
	assert.Equal(t, defaultNS, endpoint.DefaultNS)
	assert.Equal(t, tagListSort, endpoint.TagListSort)
	assert.Equal(t, limit, endpoint.limit)
}

func TestTagListSortFromString(t *testing.T) {
	t.Run("returns TagListSortLatestFirst for 'latest-first'", func(t *testing.T) {
		result := TagListSortFromString("latest-first")
		assert.Equal(t, TagListSortLatestFirst, result)
	})

	t.Run("returns TagListSortLatestLast for 'latest-last'", func(t *testing.T) {
		result := TagListSortFromString("latest-last")
		assert.Equal(t, TagListSortLatestLast, result)
	})

	t.Run("returns TagListSortUnsorted for 'none'", func(t *testing.T) {
		result := TagListSortFromString("none")
		assert.Equal(t, TagListSortUnsorted, result)
	})

	t.Run("returns TagListSortUnsorted for an empty string", func(t *testing.T) {
		result := TagListSortFromString("")
		assert.Equal(t, TagListSortUnsorted, result)
	})

	t.Run("returns TagListSortUnknown for an unknown value", func(t *testing.T) {
		result := TagListSortFromString("unknown-value")
		assert.Equal(t, TagListSortUnknown, result)
	})
}

func TestIsTimeSorted(t *testing.T) {
	t.Run("returns true for TagListSortLatestFirst", func(t *testing.T) {
		assert.True(t, TagListSortLatestFirst.IsTimeSorted())
	})

	t.Run("returns true for TagListSortLatestLast", func(t *testing.T) {
		assert.True(t, TagListSortLatestLast.IsTimeSorted())
	})

	t.Run("returns false for TagListSortUnsorted", func(t *testing.T) {
		assert.False(t, TagListSortUnsorted.IsTimeSorted())
	})

	t.Run("returns false for TagListSortUnknown", func(t *testing.T) {
		assert.False(t, TagListSortUnknown.IsTimeSorted())
	})
}

func TestTagListSort_String(t *testing.T) {
	t.Run("returns 'latest-first' for TagListSortLatestFirst", func(t *testing.T) {
		assert.Equal(t, TagListSortLatestFirstString, TagListSortLatestFirst.String())
	})

	t.Run("returns 'latest-last' for TagListSortLatestLast", func(t *testing.T) {
		assert.Equal(t, TagListSortLatestLastString, TagListSortLatestLast.String())
	})

	t.Run("returns 'unsorted' for TagListSortUnsorted", func(t *testing.T) {
		assert.Equal(t, TagListSortUnsortedString, TagListSortUnsorted.String())
	})

	t.Run("returns 'unknown' for TagListSortUnknown", func(t *testing.T) {
		assert.Equal(t, TagListSortUnknownString, TagListSortUnknown.String())
	})

	t.Run("returns 'unknown' for an undefined TagListSort value", func(t *testing.T) {
		var undefined TagListSort = 99
		assert.Equal(t, TagListSortUnknownString, undefined.String())
	})
}

func Test_GetEndpoints(t *testing.T) {
	RestoreDefaultRegistryConfiguration()

	t.Run("Get default endpoint", func(t *testing.T) {
		ep, err := GetRegistryEndpoint("")
		require.NoError(t, err)
		require.NotNil(t, ep)
		assert.Equal(t, "docker.io", ep.RegistryPrefix)
	})

	t.Run("Get GCR endpoint", func(t *testing.T) {
		ep, err := GetRegistryEndpoint("gcr.io")
		require.NoError(t, err)
		require.NotNil(t, ep)
		assert.Equal(t, ep.RegistryPrefix, "gcr.io")
	})

	t.Run("Infer endpoint", func(t *testing.T) {
		ep, err := GetRegistryEndpoint("foobar.com")
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
		ep, err := GetRegistryEndpoint("example.com")
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
		ep, err := GetRegistryEndpoint("example.com")
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
	RestoreDefaultRegistryConfiguration()
	const numRuns = 50
	// Make sure we're not deadlocking on read
	t.Run("Concurrent read access", func(t *testing.T) {
		var wg sync.WaitGroup
		wg.Add(numRuns)
		for i := 0; i < numRuns; i++ {
			go func() {
				ep, err := GetRegistryEndpoint("gcr.io")
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
				ep, err := GetRegistryEndpoint("")
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

	ep, err := GetRegistryEndpoint("ghcr.io")
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

func TestGetTransport(t *testing.T) {
	t.Run("returns transport with default TLS config when Insecure is false", func(t *testing.T) {
		endpoint := &RegistryEndpoint{
			Insecure: false,
		}
		transport := endpoint.GetTransport()

		assert.NotNil(t, transport)
		assert.NotNil(t, transport.TLSClientConfig)
		assert.False(t, transport.TLSClientConfig.InsecureSkipVerify)
	})

	t.Run("returns transport with insecure TLS config when Insecure is true", func(t *testing.T) {
		endpoint := &RegistryEndpoint{
			Insecure: true,
		}
		transport := endpoint.GetTransport()

		assert.NotNil(t, transport)
		assert.NotNil(t, transport.TLSClientConfig)
		assert.True(t, transport.TLSClientConfig.InsecureSkipVerify)
	})
}

func Test_RestoreDefaultRegistryConfiguration(t *testing.T) {
	// Call the function to restore default configuration
	RestoreDefaultRegistryConfiguration()

	// Retrieve the default registry endpoint
	defaultEp := GetDefaultRegistry()

	// Validate that the default registry endpoint is not nil
	require.NotNil(t, defaultEp)

	// Validate that the default registry endpoint has expected properties
	assert.Equal(t, "docker.io", defaultEp.RegistryPrefix)
	assert.True(t, defaultEp.IsDefault)
}

func TestConfiguredEndpoints(t *testing.T) {
	// Set up the registries map
	registries = map[string]*RegistryEndpoint{
		"docker.io": {RegistryPrefix: "docker.io"},
		"ghcr.io":   {RegistryPrefix: "ghcr.io"},
		"quay.io":   {RegistryPrefix: "quay.io"},
	}

	// Test the function
	endpoints := ConfiguredEndpoints()

	// Validate the output
	expected := []string{"docker.io", "ghcr.io", "quay.io"}
	require.Len(t, endpoints, len(expected), "The number of endpoints should match the expected number")
	assert.ElementsMatch(t, expected, endpoints, "The endpoints should match the expected values")

	// Cleanup: reset registries to avoid affecting other tests
	registries = nil
}

func TestAddRegistryEndpointFromConfig(t *testing.T) {
	t.Run("successfully adds registry endpoint from config", func(t *testing.T) {
		config := RegistryConfiguration{
			Prefix:      "example.com",
			Name:        "exampleRegistry",
			ApiURL:      "https://api.example.com",
			Credentials: "user:pass",
			DefaultNS:   "default",
			Insecure:    true,
			TagSortMode: "latest-first",
			Limit:       10,
			CredsExpire: time.Minute * 30,
		}

		err := AddRegistryEndpointFromConfig(config)
		require.NoError(t, err)
	})
}
