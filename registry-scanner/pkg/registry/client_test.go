package registry

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/distribution/distribution/v3/manifest/manifestlist"
	"github.com/distribution/distribution/v3/manifest/ocischema"
	"github.com/distribution/distribution/v3/manifest/schema2"
	godigest "github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"

	distclient "github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/registry/internal/client"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/registry/api/errcode"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"go.uber.org/ratelimit"

	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/image"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/options"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/registry/mocks"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/tag"
)

func TestBasic(t *testing.T) {
	creds := credentials{
		username: "testuser",
		password: "testpass",
	}

	testURL, _ := url.Parse("https://example.com")
	username, password := creds.Basic(testURL)

	if username != "testuser" {
		t.Errorf("Expected username to be 'testuser', got '%s'", username)
	}
	if password != "testpass" {
		t.Errorf("Expected password to be 'testpass', got '%s'", password)
	}
}

// TestNewRepository_ACR_Actions tests that ACR endpoints get additional
// metadata_read and content_read actions for OAuth2 Bearer token requests,
// while non-ACR endpoints only request the "pull" action.
func TestNewRepository_ACR_Actions(t *testing.T) {

	t.Run("ACR endpoint includes metadata_read and content_read actions", func(t *testing.T) {
		acrURLs := []string{
			"https://myregistry.azurecr.io",
			"https://test.azurecr.io",
			"https://prod-registry.azurecr.io",
			"https://dev.azurecr.io/v2",
		}
		for _, registryAPI := range acrURLs {
			actions := getTokenActions(registryAPI)
			assert.Equal(t, []string{"pull", "metadata_read", "content_read"}, actions,
				"ACR endpoint %s should have pull, metadata_read, content_read actions", registryAPI)
		}
	})

	t.Run("Non-ACR endpoint only includes pull action", func(t *testing.T) {
		nonACRURLs := []string{
			"https://registry-1.docker.io",
			"https://ghcr.io",
			"https://quay.io",
			"https://gcr.io",
			"https://harbor.example.com",
			"https://registry.gitlab.com",
		}
		for _, registryAPI := range nonACRURLs {
			actions := getTokenActions(registryAPI)
			assert.Equal(t, []string{"pull"}, actions,
				"Non-ACR endpoint %s should only have pull action", registryAPI)
		}
	})

	t.Run("NewRepository with mock server validates non-ACR token scope", func(t *testing.T) {
		// Mock registry server that simulates /v2/ ping with Bearer challenge
		var capturedTokenRequest *http.Request
		var serverURL string

		mux := http.NewServeMux()
		mux.HandleFunc("/v2/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("WWW-Authenticate",
				fmt.Sprintf(`Bearer realm="%s/oauth2/token",service="mock-registry"`, serverURL))
			w.WriteHeader(http.StatusUnauthorized)
		})
		mux.HandleFunc("/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
			capturedTokenRequest = r
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"access_token":"mock-token","expires_in":300}`)
		})

		mux.HandleFunc("/v2/test/myimage/tags/list", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"name":"test/myimage","tags":["latest"]}`)
		})
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		mockServer := httptest.NewServer(mux)
		serverURL = mockServer.URL
		defer mockServer.Close()

		ep := &RegistryEndpoint{
			RegistryAPI: mockServer.URL,
			Limiter:     ratelimit.New(100),
		}
		client, err := NewClient(ep, "testuser", "testpass")
		require.NoError(t, err)
		err = client.NewRepository("test/myimage")
		require.NoError(t, err)

		_, _ = client.Tags(context.Background())

		require.NotNil(t, capturedTokenRequest, "Token request should have been captured")

		scope := capturedTokenRequest.URL.Query().Get("scope")
		assert.Contains(t, scope, "pull")
		assert.NotContains(t, scope, "metadata_read", "Non-ACR endpoint should not request metadata_read")
		assert.NotContains(t, scope, "content_read", "Non-ACR endpoint should not request content_read")
	})

}

func TestNewRepository(t *testing.T) {
	t.Run("Invalid Reference Format", func(t *testing.T) {
		ep, err := GetRegistryEndpoint(context.Background(), &image.ContainerImage{RegistryURL: ""})
		require.NoError(t, err)
		client, err := NewClient(ep, "", "")
		require.NoError(t, err)
		err = client.NewRepository("test@test")
		require.Error(t, err)
		assert.Contains(t, "invalid reference format", err.Error())

	})
	t.Run("Success Ping", func(t *testing.T) {
		ep, err := GetRegistryEndpoint(context.Background(), &image.ContainerImage{RegistryURL: ""})
		require.NoError(t, err)
		client, err := NewClient(ep, "", "")
		require.NoError(t, err)
		err = client.NewRepository("test/test")
		require.NoError(t, err)
	})

	t.Run("Fail Ping", func(t *testing.T) {
		testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		ep := &RegistryEndpoint{RegistryAPI: testServer.URL}
		client, err := NewClient(ep, "", "")
		require.NoError(t, err)
		err = client.NewRepository("")
		require.Error(t, err)
	})

}

func TestRoundTrip_Success(t *testing.T) {
	// Create mocks
	mockLimiter := new(mocks.Limiter)
	mockTransport := new(mocks.RoundTripper)
	endpoint := &RegistryEndpoint{RegistryAPI: "http://example.com"}
	// Create an instance of rateLimitTransport with mocks
	rlt := &rateLimitTransport{
		limiter:   mockLimiter,
		transport: mockTransport,
		endpoint:  endpoint,
	}
	// Create a sample HTTP request
	req, err := http.NewRequest("GET", "http://example.com", nil)
	assert.NoError(t, err)
	resp := &http.Response{StatusCode: http.StatusOK}
	// Set up expectations
	mockLimiter.On("Take").Return(time.Now())
	mockTransport.On("RoundTrip", req).Return(resp, nil)
	// Call the method under test
	actualResp, err := rlt.RoundTrip(req)
	// Assert the expectations
	mockLimiter.AssertExpectations(t)
	mockTransport.AssertExpectations(t)
	assert.NoError(t, err)
	assert.Equal(t, resp, actualResp)
}
func TestRoundTrip_Failure(t *testing.T) {
	// Create mocks
	mockLimiter := new(mocks.Limiter)
	mockTransport := new(mocks.RoundTripper)
	endpoint := &RegistryEndpoint{RegistryAPI: "http://example.com"}
	// Create an instance of rateLimitTransport with mocks
	rlt := &rateLimitTransport{
		limiter:   mockLimiter,
		transport: mockTransport,
		endpoint:  endpoint,
	}
	// Create a sample HTTP request
	req := httptest.NewRequest("GET", "http://example.com", nil)
	// Set up expectations
	mockLimiter.On("Take").Return(time.Now())
	mockTransport.On("RoundTrip", req).Return(nil, errors.New("Error on caling func RoundTrip"))
	// Call the method under test
	actualResp, err := rlt.RoundTrip(req)
	// Assert the expectations
	mockLimiter.AssertExpectations(t)
	mockTransport.AssertExpectations(t)
	assert.Error(t, err)
	assert.Nil(t, actualResp)
}

func TestRefreshToken(t *testing.T) {
	creds := credentials{
		refreshTokens: map[string]string{
			"service1": "token1",
		},
	}
	testURL, _ := url.Parse("https://example.com")
	token := creds.RefreshToken(testURL, "service1")
	if token != "token1" {
		t.Errorf("Expected token to be 'token1', got '%s'", token)
	}
}

func TestSetRefreshToken(t *testing.T) {
	creds := credentials{
		refreshTokens: make(map[string]string),
	}
	testURL, _ := url.Parse("https://example.com")
	creds.SetRefreshToken(testURL, "service1", "token1")

	if token, exists := creds.refreshTokens["service1"]; !exists {
		t.Error("Expected token for 'service1' to exist")
	} else if token != "token1" {
		t.Errorf("Expected token to be 'token1', got '%s'", token)
	}
}
func TestNewClient(t *testing.T) {
	t.Run("Create client with provided username and password", func(t *testing.T) {
		ep, err := GetRegistryEndpoint(context.Background(), &image.ContainerImage{RegistryURL: ""})
		require.NoError(t, err)
		_, err = NewClient(ep, "testuser", "pass")
		require.NoError(t, err)
	})
	t.Run("Create client with empty username and password", func(t *testing.T) {
		ep := &RegistryEndpoint{Username: "testuser", Password: "pass"}
		_, err := NewClient(ep, "", "")
		require.NoError(t, err)
	})
}

func TestTags(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockRegClient := new(mocks.Repository)
		client := registryClient{
			regClient: mockRegClient,
		}
		mockTagService := new(mocks.TagService)
		mockTagService.On("All", mock.Anything).Return([]string{"testTag-1", "testTag-2"}, nil)
		mockRegClient.On("Tags", mock.Anything).Return(mockTagService)
		tags, err := client.Tags(context.Background())
		require.NoError(t, err)
		assert.Contains(t, tags, "testTag-1")
		assert.Contains(t, tags, "testTag-2")
	})
	t.Run("Fail", func(t *testing.T) {
		mockRegClient := new(mocks.Repository)
		client := registryClient{
			regClient: mockRegClient,
		}
		mockTagService := new(mocks.TagService)
		mockTagService.On("All", mock.Anything).Return([]string{}, errors.New("Error on caling func All"))
		mockRegClient.On("Tags", mock.Anything).Return(mockTagService)
		_, err := client.Tags(context.Background())
		require.Error(t, err)
	})
}

func TestManifestForTag(t *testing.T) {
	t.Run("Successful retrieval of Manifest", func(t *testing.T) {
		mockRegClient := new(mocks.Repository)
		client := registryClient{
			regClient: mockRegClient,
		}
		manService := new(mocks.ManifestService)
		manService.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)
		mockRegClient.On("Manifests", mock.Anything).Return(manService, nil)
		_, err := client.ManifestForTag(context.Background(), "tagStr")
		require.NoError(t, err)
	})
	t.Run("Error returned from Manifests call", func(t *testing.T) {
		mockRegClient := new(mocks.Repository)
		client := registryClient{
			regClient: mockRegClient,
		}
		manService := new(mocks.ManifestService)
		manService.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)
		mockRegClient.On("Manifests", mock.Anything).Return(manService, errors.New("Error on caling func Manifests"))
		_, err := client.ManifestForTag(context.Background(), "tagStr")
		require.Error(t, err)
	})

	t.Run("Error returned from Get call", func(t *testing.T) {
		mockRegClient := new(mocks.Repository)
		client := registryClient{
			regClient: mockRegClient,
		}
		manService := new(mocks.ManifestService)
		manService.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("Error on caling func Get"))
		mockRegClient.On("Manifests", mock.Anything).Return(manService, nil)
		_, err := client.ManifestForTag(context.Background(), "tagStr")
		require.Error(t, err)
	})

}

func TestManifestForDigest(t *testing.T) {
	t.Run("Successful retrieval of manifest", func(t *testing.T) {
		mockRegClient := new(mocks.Repository)
		client := registryClient{
			regClient: mockRegClient,
		}
		manService := new(mocks.ManifestService)
		manService.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)
		mockRegClient.On("Manifests", mock.Anything).Return(manService, nil)
		_, err := client.ManifestForDigest(context.Background(), "dgst")
		require.NoError(t, err)
	})
	t.Run("Error returned from Manifests call", func(t *testing.T) {
		mockRegClient := new(mocks.Repository)
		client := registryClient{
			regClient: mockRegClient,
		}
		manService := new(mocks.ManifestService)
		manService.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)
		mockRegClient.On("Manifests", mock.Anything).Return(manService, errors.New("Error on caling func Manifests"))
		_, err := client.ManifestForDigest(context.Background(), "dgst")
		require.Error(t, err)
	})
	t.Run("Error returned from Get call", func(t *testing.T) {
		mockRegClient := new(mocks.Repository)
		client := registryClient{
			regClient: mockRegClient,
		}
		manService := new(mocks.ManifestService)
		manService.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("Error on caling func Get"))
		mockRegClient.On("Manifests", mock.Anything).Return(manService, nil)
		_, err := client.ManifestForDigest(context.Background(), "dgst")
		require.Error(t, err)
	})
}

func TestTagInfoFromReferences(t *testing.T) {
	t.Run("No usable reference in manifest list", func(t *testing.T) {
		mockRegClient := new(mocks.Repository)
		client := registryClient{
			regClient: mockRegClient,
		}
		tagInfo := &tag.TagInfo{}
		tagInfo.CreatedAt = time.Now()
		tagInfo.Digest = [32]byte{}
		opts := &options.ManifestOptions{}
		opts.WithPlatform("testOS", "testArch", "testVarient")
		//opts.WithLogger(log.NewContext())
		opts.WithMetadata(true)
		descriptor := []distribution.Descriptor{
			{
				MediaType: "",
				Digest:    "",
				Size:      0,
				Platform: &v1.Platform{
					Architecture: "mTestArch",
					OS:           "mTestOS",
					OSVersion:    "mTestOSVersion",
					OSFeatures:   []string{},
					Variant:      "mTestVarient",
				},
			},
		}
		tag, err := TagInfoFromReferences(context.Background(), &client, opts, tagInfo, descriptor)
		require.Nil(t, tag)
		require.NoError(t, err)
	})
	t.Run("Return tagInfo when metadata option is false", func(t *testing.T) {
		mockRegClient := new(mocks.Repository)
		client := registryClient{
			regClient: mockRegClient,
		}
		tagInfo := &tag.TagInfo{}
		tagInfo.CreatedAt = time.Now()
		tagInfo.Digest = [32]byte{}
		opts := &options.ManifestOptions{}
		opts.WithMetadata(false)
		opts.WithPlatform("testOS", "testArch", "testVarient")
		//opts.WithLogger(log.NewContext())
		descriptor := []distribution.Descriptor{
			{
				MediaType: "",
				Digest:    "",
				Size:      0,
				Platform: &v1.Platform{
					Architecture: "testArch",
					OS:           "testOS",
					OSVersion:    "testOSVersion",
					OSFeatures:   []string{},
					Variant:      "testVarient",
				},
			},
		}
		tag, err := TagInfoFromReferences(context.Background(), &client, opts, tagInfo, descriptor)
		require.NoError(t, err)
		assert.Equal(t, tag, tagInfo)
		require.NoError(t, err)
	})
	t.Run("Return error from ManifestForDigest", func(t *testing.T) {
		mockRegClient := new(mocks.Repository)
		client := registryClient{
			regClient: mockRegClient,
		}
		tagInfo := &tag.TagInfo{}
		tagInfo.CreatedAt = time.Now()
		tagInfo.Digest = [32]byte{}
		opts := &options.ManifestOptions{}
		opts.WithMetadata(true)
		opts.WithPlatform("testOS", "testArch", "testVarient")
		//opts.WithLogger(log.NewContext())
		descriptor := []distribution.Descriptor{
			{
				MediaType: "",
				Digest:    "",
				Size:      0,
				Platform: &v1.Platform{
					Architecture: "testArch",
					OS:           "testOS",
					OSVersion:    "testOSVersion",
					OSFeatures:   []string{},
					Variant:      "testVarient",
				},
			},
		}
		mockRegClient.On("Manifests", mock.Anything).Return(nil, errors.New("Error from Manifests"))
		_, err := TagInfoFromReferences(context.Background(), &client, opts, tagInfo, descriptor)
		require.Error(t, err)
	})
	t.Run("Return error from TagMetadata", func(t *testing.T) {
		mockRegClient := new(mocks.Repository)
		client := registryClient{
			regClient: mockRegClient,
		}
		tagInfo := &tag.TagInfo{}
		tagInfo.CreatedAt = time.Now()
		tagInfo.Digest = [32]byte{}
		opts := &options.ManifestOptions{}
		opts.WithMetadata(true)
		opts.WithPlatform("testOS", "testArch", "testVarient")
		//opts.WithLogger(log.NewContext())
		descriptor := []distribution.Descriptor{
			{
				MediaType: "",
				Digest:    "",
				Size:      0,
				Platform: &v1.Platform{
					Architecture: "testArch",
					OS:           "testOS",
					OSVersion:    "testOSVersion",
					OSFeatures:   []string{},
					Variant:      "testVarient",
				},
			},
		}
		manService := new(mocks.ManifestService)
		manService.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(new(mocks.Manifest), nil)
		mockRegClient.On("Manifests", mock.Anything).Return(manService, nil)
		_, err := TagInfoFromReferences(context.Background(), &client, opts, tagInfo, descriptor)
		require.Error(t, err)
	})
}

func Test_TagMetadata(t *testing.T) {
	// makeConfigServer creates a local test registry that serves configJSON as
	// the config blob at /v2/test/test/blobs/*. Pass an empty string to
	// simulate a missing blob (404).
	makeConfigServer := func(t *testing.T, configJSON string) (*httptest.Server, godigest.Digest) {
		t.Helper()
		var configDigest godigest.Digest
		if configJSON == "" {
			configDigest = godigest.FromBytes([]byte("placeholder"))
		} else {
			configDigest = godigest.FromBytes([]byte(configJSON))
		}
		mux := http.NewServeMux()
		mux.HandleFunc("/v2/", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		mux.HandleFunc("/v2/test/test/blobs/", func(w http.ResponseWriter, r *http.Request) {
			if configJSON == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, configJSON)
		})
		return httptest.NewServer(mux), configDigest
	}

	makeClient := func(t *testing.T, serverURL string) RegistryClient {
		t.Helper()
		ep := &RegistryEndpoint{RegistryAPI: serverURL, Limiter: ratelimit.New(100)}
		client, err := NewClient(ep, "", "")
		require.NoError(t, err)
		err = client.NewRepository("test/test")
		require.NoError(t, err)
		return client
	}

	makeManifest := func(configDigest godigest.Digest) *schema2.DeserializedManifest {
		return &schema2.DeserializedManifest{
			Manifest: schema2.Manifest{
				Versioned: specs.Versioned{SchemaVersion: 2},
				Config: distribution.Descriptor{
					Digest: configDigest,
				},
			},
		}
	}

	t.Run("Check for correct error handling when config blob is missing", func(t *testing.T) {
		server, configDigest := makeConfigServer(t, "")
		defer server.Close()
		ctx := context.Background()
		client := makeClient(t, server.URL)
		_, err := client.TagMetadata(ctx, makeManifest(configDigest), &options.ManifestOptions{})
		require.Error(t, err)
	})

	t.Run("Check for correct error handling when config blob contains invalid JSON", func(t *testing.T) {
		server, configDigest := makeConfigServer(t, `{not valid json}`)
		defer server.Close()
		ctx := context.Background()
		client := makeClient(t, server.URL)
		_, err := client.TagMetadata(ctx, makeManifest(configDigest), &options.ManifestOptions{})
		require.Error(t, err)
	})

	t.Run("Check for correct error handling when config blob contains no created field", func(t *testing.T) {
		server, configDigest := makeConfigServer(t, `{"something": "something"}`)
		defer server.Close()
		ctx := context.Background()
		client := makeClient(t, server.URL)
		_, err := client.TagMetadata(ctx, makeManifest(configDigest), &options.ManifestOptions{})
		require.Error(t, err)
	})

	t.Run("Check for invalid/valid timestamp and non-match platforms", func(t *testing.T) {
		ctx := context.Background()

		// Invalid timestamp → error
		server, configDigest := makeConfigServer(t, `{"created":"invalid"}`)
		defer server.Close()
		client := makeClient(t, server.URL)
		_, err := client.TagMetadata(ctx, makeManifest(configDigest), &options.ManifestOptions{})
		require.Error(t, err)

		// Valid timestamp → success
		ts := time.Now().Format(time.RFC3339Nano)
		server2, configDigest2 := makeConfigServer(t, `{"created":"`+ts+`"}`)
		defer server2.Close()
		client2 := makeClient(t, server2.URL)
		opts := &options.ManifestOptions{}
		tagInfo, _ := client2.TagMetadata(ctx, makeManifest(configDigest2), opts)
		assert.Equal(t, ts, tagInfo.CreatedAt.Format(time.RFC3339Nano))

		// Platform mismatch (config has no os/arch, opts requires testOS/testArch) → nil, nil
		opts.WithPlatform("testOS", "testArch", "testVariant")
		tagInfo, err = client2.TagMetadata(ctx, makeManifest(configDigest2), opts)
		assert.Nil(t, tagInfo)
		assert.Nil(t, err)
	})

	t.Run("Check manifest labels are extracted", func(t *testing.T) {
		ts := time.Now().Format(time.RFC3339Nano)
		configJSON := `{"created":"` + ts + `","config":{"Labels":{"org.opencontainers.image.source":"https://github.com/org/repo","org.opencontainers.image.revision":"abc123"}}}`
		server, configDigest := makeConfigServer(t, configJSON)
		defer server.Close()
		ctx := context.Background()
		client := makeClient(t, server.URL)
		tagInfo, err := client.TagMetadata(ctx, makeManifest(configDigest), &options.ManifestOptions{})
		require.NoError(t, err)
		require.NotNil(t, tagInfo)
		assert.Equal(t, "https://github.com/org/repo", tagInfo.Labels["org.opencontainers.image.source"])
		assert.Equal(t, "abc123", tagInfo.Labels["org.opencontainers.image.revision"])
	})

	t.Run("Check manifest without labels", func(t *testing.T) {
		ts := time.Now().Format(time.RFC3339Nano)
		server, configDigest := makeConfigServer(t, `{"created":"`+ts+`"}`)
		defer server.Close()
		ctx := context.Background()
		client := makeClient(t, server.URL)
		tagInfo, err := client.TagMetadata(ctx, makeManifest(configDigest), &options.ManifestOptions{})
		require.NoError(t, err)
		require.NotNil(t, tagInfo)
		assert.Nil(t, tagInfo.Labels)
	})
}

func Test_TagMetadata_2(t *testing.T) {
	t.Run("ocischema DeserializedManifest invalid digest format", func(t *testing.T) {
		meta1 := &ocischema.DeserializedManifest{
			Manifest: ocischema.Manifest{
				Versioned: specs.Versioned{
					SchemaVersion: 1,
				},
				MediaType: "",
			},
		}
		ctx := context.Background()
		ep, err := GetRegistryEndpoint(ctx, &image.ContainerImage{RegistryURL: ""})
		require.NoError(t, err)
		client, err := NewClient(ep, "", "")

		require.NoError(t, err)
		err = client.NewRepository("test/test")
		require.NoError(t, err)
		_, err = client.TagMetadata(ctx, meta1, &options.ManifestOptions{})
		require.Error(t, err) // invalid digest format
	})
	t.Run("schema2 DeserializedManifest invalid digest format", func(t *testing.T) {
		meta1 := &schema2.DeserializedManifest{
			Manifest: schema2.Manifest{
				Versioned: specs.Versioned{
					SchemaVersion: 1,
				},
				MediaType: "",
				Config: distribution.Descriptor{
					MediaType: "",
					Digest:    "sha256:abc",
				},
			},
		}
		ctx := context.Background()
		ep, err := GetRegistryEndpoint(ctx, &image.ContainerImage{RegistryURL: ""})
		require.NoError(t, err)
		client, err := NewClient(ep, "", "")

		require.NoError(t, err)
		err = client.NewRepository("test/test")
		require.NoError(t, err)
		_, err = client.TagMetadata(ctx, meta1, &options.ManifestOptions{})
		require.Error(t, err) // invalid digest format
	})
	t.Run("ocischema DeserializedImageIndex empty index not supported", func(t *testing.T) {
		meta1 := &ocischema.DeserializedImageIndex{
			ImageIndex: ocischema.ImageIndex{
				Versioned: specs.Versioned{
					SchemaVersion: 1,
				},
				MediaType:   "",
				Manifests:   nil,
				Annotations: nil,
			},
		}
		ctx := context.Background()
		ep, err := GetRegistryEndpoint(ctx, &image.ContainerImage{RegistryURL: ""})
		require.NoError(t, err)
		client, err := NewClient(ep, "", "")

		require.NoError(t, err)
		err = client.NewRepository("test/test")
		require.NoError(t, err)
		_, err = client.TagMetadata(ctx, meta1, &options.ManifestOptions{})
		require.Error(t, err) // empty index not supported
	})
	t.Run("ocischema DeserializedImageIndex empty manifestlist not supported", func(t *testing.T) {
		meta1 := &manifestlist.DeserializedManifestList{
			ManifestList: manifestlist.ManifestList{
				Versioned: specs.Versioned{
					SchemaVersion: 1,
				},
				MediaType: "",
				Manifests: nil,
			},
		}
		ctx := context.Background()
		ep, err := GetRegistryEndpoint(ctx, &image.ContainerImage{RegistryURL: ""})
		require.NoError(t, err)
		client, err := NewClient(ep, "", "")

		require.NoError(t, err)
		err = client.NewRepository("test/test")
		require.NoError(t, err)
		_, err = client.TagMetadata(ctx, meta1, &options.ManifestOptions{})
		require.Error(t, err) // empty manifestlist not supported
	})
}

func TestPing(t *testing.T) {
	t.Run("fail ping", func(t *testing.T) {
		mockManager := new(mocks.Manager)
		ep, err := GetRegistryEndpoint(context.Background(), &image.ContainerImage{RegistryURL: ""})
		require.NoError(t, err)
		mockManager.On("AddResponse", mock.Anything).Return(fmt.Errorf("fail ping"))
		_, err = ping(mockManager, ep, "")
		require.Error(t, err)
	})

	t.Run("success ping", func(t *testing.T) {
		mockManager := new(mocks.Manager)
		ep, err := GetRegistryEndpoint(context.Background(), &image.ContainerImage{RegistryURL: ""})
		require.NoError(t, err)
		mockManager.On("AddResponse", mock.Anything).Return(nil)
		_, err = ping(mockManager, ep, "")
		require.NoError(t, err)
	})

	t.Run("Invalid Docker Registry", func(t *testing.T) {
		testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		mockManager := new(mocks.Manager)
		ep := &RegistryEndpoint{RegistryAPI: testServer.URL}
		mockManager.On("AddResponse", mock.Anything).Return(nil)
		_, err := ping(mockManager, ep, "")
		require.Error(t, err)
		assert.ErrorContains(t, err, "does not seem to be a valid v2 Docker Registry API")
	})

	t.Run("Empty Registry API", func(t *testing.T) {
		mockManager := new(mocks.Manager)
		ep := &RegistryEndpoint{RegistryAPI: ""}
		mockManager.On("AddResponse", mock.Anything).Return(nil)
		_, err := ping(mockManager, ep, "")
		require.Error(t, err)
		assert.ErrorContains(t, err, "unsupported protocol scheme")
	})
}

func TestIsAuthError(t *testing.T) {
	ctx := context.Background()
	t.Run("nil returns false", func(t *testing.T) {
		assert.False(t, IsAuthError(ctx, nil))
	})

	t.Run("plain error returns false", func(t *testing.T) {
		assert.False(t, IsAuthError(ctx, errors.New("some error")))
	})

	t.Run("UnexpectedHTTPResponseError 401 returns true", func(t *testing.T) {
		err := &distclient.UnexpectedHTTPResponseError{StatusCode: http.StatusUnauthorized}
		assert.True(t, IsAuthError(ctx, err))
	})

	t.Run("UnexpectedHTTPResponseError 403 returns true", func(t *testing.T) {
		err := &distclient.UnexpectedHTTPResponseError{StatusCode: http.StatusForbidden}
		assert.True(t, IsAuthError(ctx, err))
	})

	t.Run("UnexpectedHTTPResponseError 404 returns false", func(t *testing.T) {
		err := &distclient.UnexpectedHTTPResponseError{StatusCode: http.StatusNotFound}
		assert.False(t, IsAuthError(ctx, err))
	})

	t.Run("UnexpectedHTTPStatusError 401 returns true", func(t *testing.T) {
		err := &distclient.UnexpectedHTTPStatusError{Status: "401 Unauthorized"}
		assert.True(t, IsAuthError(ctx, err))
	})

	t.Run("UnexpectedHTTPStatusError 403 returns true", func(t *testing.T) {
		err := &distclient.UnexpectedHTTPStatusError{Status: "403 Forbidden"}
		assert.True(t, IsAuthError(ctx, err))
	})

	t.Run("UnexpectedHTTPStatusError 500 returns false", func(t *testing.T) {
		err := &distclient.UnexpectedHTTPStatusError{Status: "500 Internal Server Error"}
		assert.False(t, IsAuthError(ctx, err))
	})

	t.Run("errcode.Errors with Unauthorized returns true", func(t *testing.T) {
		err := errcode.Errors{errcode.ErrorCodeUnauthorized.WithMessage("authentication required")}
		assert.True(t, IsAuthError(ctx, err))
	})

	t.Run("errcode.Errors with Denied returns true", func(t *testing.T) {
		err := errcode.Errors{errcode.ErrorCodeDenied.WithMessage("access denied")}
		assert.True(t, IsAuthError(ctx, err))
	})

	t.Run("errcode.Errors with other code returns false", func(t *testing.T) {
		err := errcode.Errors{errcode.ErrorCodeUnknown.WithMessage("something else")}
		assert.False(t, IsAuthError(ctx, err))
	})
}

// makeRegistryClient is a shared helper that creates a real RegistryClient backed
// by a test HTTP server. The server must respond to GET /v2/ with 200 (for the
// NewRepository ping) plus whatever path the individual test needs.
func makeRegistryClient(t *testing.T, serverURL string) RegistryClient {
	t.Helper()
	ep := &RegistryEndpoint{RegistryAPI: serverURL, Limiter: ratelimit.New(100)}
	client, err := NewClient(ep, "", "")
	require.NoError(t, err)
	err = client.NewRepository("test/test")
	require.NoError(t, err)
	return client
}

func Test_BlobContent(t *testing.T) {
	t.Run("not initialized returns error", func(t *testing.T) {
		ep, err := GetRegistryEndpoint(context.Background(), &image.ContainerImage{RegistryURL: ""})
		require.NoError(t, err)
		client, err := NewClient(ep, "", "")
		require.NoError(t, err)
		// NewRepository NOT called → clt.regClient is nil
		_, err = client.BlobContent(context.Background(), godigest.FromBytes([]byte("x")))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not initialized")
	})

	t.Run("returns blob bytes on success", func(t *testing.T) {
		blobContent := []byte("hello-blob")
		blobDigest := godigest.FromBytes(blobContent)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v2/" {
				w.WriteHeader(http.StatusOK)
				return
			}
			// All other /v2/... requests are blob fetches.
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(blobContent)
		}))
		defer srv.Close()

		client := makeRegistryClient(t, srv.URL)
		got, err := client.BlobContent(context.Background(), blobDigest)
		require.NoError(t, err)
		assert.Equal(t, blobContent, got)
	})

	t.Run("blob not found returns error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v2/" {
				w.WriteHeader(http.StatusOK)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		client := makeRegistryClient(t, srv.URL)
		_, err := client.BlobContent(context.Background(), godigest.FromBytes([]byte("missing")))
		require.Error(t, err)
	})
}

func Test_Referrers(t *testing.T) {
	testDigest := godigest.FromBytes([]byte("image-manifest"))

	t.Run("not initialized returns error", func(t *testing.T) {
		ep, err := GetRegistryEndpoint(context.Background(), &image.ContainerImage{RegistryURL: ""})
		require.NoError(t, err)
		client, err := NewClient(ep, "", "")
		require.NoError(t, err)
		// NewRepository NOT called → clt.httpClient is nil
		_, err = client.Referrers(context.Background(), testDigest)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not initialized")
	})

	t.Run("http 404 returns nil nil (no signature found, not an error)", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v2/" {
				w.WriteHeader(http.StatusOK)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		client := makeRegistryClient(t, srv.URL)
		refs, err := client.Referrers(context.Background(), testDigest)
		require.NoError(t, err)
		assert.Nil(t, refs)
	})

	t.Run("http ok returns manifest list", func(t *testing.T) {
		sigDigest := godigest.FromBytes([]byte("sig-manifest"))
		referrersJSON := fmt.Sprintf(
			`{"manifests":[{"mediaType":"application/vnd.oci.image.manifest.v1+json","artifactType":"application/vnd.dev.sigstore.bundle.v0.3+json","digest":"%s","size":100}]}`,
			sigDigest,
		)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v2/" {
				w.WriteHeader(http.StatusOK)
				return
			}
			w.Header().Set("Content-Type", "application/vnd.oci.image.index.v1+json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, referrersJSON)
		}))
		defer srv.Close()

		client := makeRegistryClient(t, srv.URL)
		refs, err := client.Referrers(context.Background(), testDigest)
		require.NoError(t, err)
		require.Len(t, refs, 1)
		assert.Equal(t, "application/vnd.dev.sigstore.bundle.v0.3+json", refs[0].ArtifactType)
		assert.Equal(t, sigDigest, refs[0].Digest)
	})

	t.Run("non-200 non-404 returns error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v2/" {
				w.WriteHeader(http.StatusOK)
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		client := makeRegistryClient(t, srv.URL)
		refs, err := client.Referrers(context.Background(), testDigest)
		require.Error(t, err)
		assert.Nil(t, refs)
		assert.Contains(t, err.Error(), "HTTP 500")
	})

	t.Run("invalid json returns error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v2/" {
				w.WriteHeader(http.StatusOK)
				return
			}
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "not-valid-json{{")
		}))
		defer srv.Close()

		client := makeRegistryClient(t, srv.URL)
		refs, err := client.Referrers(context.Background(), testDigest)
		require.Error(t, err)
		assert.Nil(t, refs)
		assert.Contains(t, err.Error(), "decoding")
	})
}
