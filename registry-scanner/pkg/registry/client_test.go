package registry

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/distribution/distribution/v3/manifest"
	"github.com/distribution/distribution/v3/manifest/manifestlist"
	"github.com/distribution/distribution/v3/manifest/ocischema"
	"github.com/distribution/distribution/v3/manifest/schema2"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/manifest/schema1" //nolint:staticcheck
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/image"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
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

func TestNewRepository(t *testing.T) {
	t.Run("Invalid Reference Format", func(t *testing.T) {
		ep, err := GetRegistryEndpoint(&image.ContainerImage{RegistryURL: ""})
		require.NoError(t, err)
		client, err := NewClient(ep, "", "")
		require.NoError(t, err)
		err = client.NewRepository("test@test")
		require.Error(t, err)
		assert.Contains(t, "invalid reference format", err.Error())

	})
	t.Run("Success Ping", func(t *testing.T) {
		ep, err := GetRegistryEndpoint(&image.ContainerImage{RegistryURL: ""})
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
		ep, err := GetRegistryEndpoint(&image.ContainerImage{RegistryURL: ""})
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
		tags, err := client.Tags()
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
		_, err := client.Tags()
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
		_, err := client.ManifestForTag("tagStr")
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
		_, err := client.ManifestForTag("tagStr")
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
		_, err := client.ManifestForTag("tagStr")
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
		_, err := client.ManifestForDigest("dgst")
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
		_, err := client.ManifestForDigest("dgst")
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
		_, err := client.ManifestForDigest("dgst")
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
		opts.WithLogger(log.NewContext())
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
		tag, err := TagInfoFromReferences(&client, opts, log.NewContext(), tagInfo, descriptor)
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
		opts.WithLogger(log.NewContext())
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
		tag, err := TagInfoFromReferences(&client, opts, log.NewContext(), tagInfo, descriptor)
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
		opts.WithLogger(log.NewContext())
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
		_, err := TagInfoFromReferences(&client, opts, log.NewContext(), tagInfo, descriptor)
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
		opts.WithLogger(log.NewContext())
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
		_, err := TagInfoFromReferences(&client, opts, log.NewContext(), tagInfo, descriptor)
		require.Error(t, err)
	})
}

func Test_TagMetadata(t *testing.T) {
	t.Run("Check for correct error handling when manifest contains no history", func(t *testing.T) {
		meta1 := &schema1.SignedManifest{ //nolint:staticcheck
			Manifest: schema1.Manifest{ //nolint:staticcheck
				History: []schema1.History{}, //nolint:staticcheck
			},
		}
		ep, err := GetRegistryEndpoint(&image.ContainerImage{RegistryURL: ""})
		require.NoError(t, err)
		client, err := NewClient(ep, "", "")
		require.NoError(t, err)
		_, err = client.TagMetadata(meta1, &options.ManifestOptions{})
		require.Error(t, err)
	})

	t.Run("Check for correct error handling when manifest contains invalid history", func(t *testing.T) {
		meta1 := &schema1.SignedManifest{ //nolint:staticcheck
			Manifest: schema1.Manifest{ //nolint:staticcheck
				History: []schema1.History{ //nolint:staticcheck
					{
						V1Compatibility: `{"created": {"something": "notastring"}}`,
					},
				},
			},
		}

		ep, err := GetRegistryEndpoint(&image.ContainerImage{RegistryURL: ""})
		require.NoError(t, err)
		client, err := NewClient(ep, "", "")
		require.NoError(t, err)
		_, err = client.TagMetadata(meta1, &options.ManifestOptions{})
		require.Error(t, err)
	})

	t.Run("Check for correct error handling when manifest contains invalid history", func(t *testing.T) {
		meta1 := &schema1.SignedManifest{ //nolint:staticcheck
			Manifest: schema1.Manifest{ //nolint:staticcheck
				History: []schema1.History{ //nolint:staticcheck
					{
						V1Compatibility: `{"something": "something"}`,
					},
				},
			},
		}

		ep, err := GetRegistryEndpoint(&image.ContainerImage{RegistryURL: ""})
		require.NoError(t, err)
		client, err := NewClient(ep, "", "")
		require.NoError(t, err)
		_, err = client.TagMetadata(meta1, &options.ManifestOptions{})
		require.Error(t, err)

	})

	t.Run("Check for invalid/valid timestamp and non-match platforms", func(t *testing.T) {
		ts := "invalid"
		meta1 := &schema1.SignedManifest{ //nolint:staticcheck
			Manifest: schema1.Manifest{ //nolint:staticcheck
				History: []schema1.History{ //nolint:staticcheck
					{
						V1Compatibility: `{"created":"` + ts + `"}`,
					},
				},
			},
		}
		ep, err := GetRegistryEndpoint(&image.ContainerImage{RegistryURL: ""})
		require.NoError(t, err)
		client, err := NewClient(ep, "", "")
		require.NoError(t, err)
		_, err = client.TagMetadata(meta1, &options.ManifestOptions{})
		require.Error(t, err)

		ts = time.Now().Format(time.RFC3339Nano)
		opts := &options.ManifestOptions{}
		meta1.Manifest.History[0].V1Compatibility = `{"created":"` + ts + `"}`
		tagInfo, _ := client.TagMetadata(meta1, opts)
		assert.Equal(t, ts, tagInfo.CreatedAt.Format(time.RFC3339Nano))

		opts.WithPlatform("testOS", "testArch", "testVariant")
		tagInfo, err = client.TagMetadata(meta1, opts)
		assert.Nil(t, tagInfo)
		assert.Nil(t, err)
	})
}

func Test_TagMetadata_2(t *testing.T) {
	t.Run("ocischema DeserializedManifest invalid digest format", func(t *testing.T) {
		meta1 := &ocischema.DeserializedManifest{
			Manifest: ocischema.Manifest{
				Versioned: manifest.Versioned{
					SchemaVersion: 1,
					MediaType:     "",
				},
			},
		}
		ep, err := GetRegistryEndpoint(&image.ContainerImage{RegistryURL: ""})
		require.NoError(t, err)
		client, err := NewClient(ep, "", "")

		require.NoError(t, err)
		err = client.NewRepository("test/test")
		require.NoError(t, err)
		_, err = client.TagMetadata(meta1, &options.ManifestOptions{})
		require.Error(t, err) // invalid digest format
	})
	t.Run("schema2 DeserializedManifest invalid digest format", func(t *testing.T) {
		meta1 := &schema2.DeserializedManifest{
			Manifest: schema2.Manifest{
				Versioned: manifest.Versioned{
					SchemaVersion: 1,
					MediaType:     "",
				},
				Config: distribution.Descriptor{
					MediaType: "",
					Digest:    "sha256:abc",
				},
			},
		}
		ep, err := GetRegistryEndpoint(&image.ContainerImage{RegistryURL: ""})
		require.NoError(t, err)
		client, err := NewClient(ep, "", "")

		require.NoError(t, err)
		err = client.NewRepository("test/test")
		require.NoError(t, err)
		_, err = client.TagMetadata(meta1, &options.ManifestOptions{})
		require.Error(t, err) // invalid digest format
	})
	t.Run("ocischema DeserializedImageIndex empty index not supported", func(t *testing.T) {
		meta1 := &ocischema.DeserializedImageIndex{
			ImageIndex: ocischema.ImageIndex{
				Versioned: manifest.Versioned{
					SchemaVersion: 1,
					MediaType:     "",
				},
				Manifests:   nil,
				Annotations: nil,
			},
		}
		ep, err := GetRegistryEndpoint(&image.ContainerImage{RegistryURL: ""})
		require.NoError(t, err)
		client, err := NewClient(ep, "", "")

		require.NoError(t, err)
		err = client.NewRepository("test/test")
		require.NoError(t, err)
		_, err = client.TagMetadata(meta1, &options.ManifestOptions{})
		require.Error(t, err) // empty index not supported
	})
	t.Run("ocischema DeserializedImageIndex empty manifestlist not supported", func(t *testing.T) {
		meta1 := &manifestlist.DeserializedManifestList{
			ManifestList: manifestlist.ManifestList{
				Versioned: manifest.Versioned{
					SchemaVersion: 1,
					MediaType:     "",
				},
				Manifests: nil,
			},
		}
		ep, err := GetRegistryEndpoint(&image.ContainerImage{RegistryURL: ""})
		require.NoError(t, err)
		client, err := NewClient(ep, "", "")

		require.NoError(t, err)
		err = client.NewRepository("test/test")
		require.NoError(t, err)
		_, err = client.TagMetadata(meta1, &options.ManifestOptions{})
		require.Error(t, err) // empty manifestlist not supported
	})
}

func TestPing(t *testing.T) {
	t.Run("fail ping", func(t *testing.T) {
		mockManager := new(mocks.Manager)
		ep, err := GetRegistryEndpoint(&image.ContainerImage{RegistryURL: ""})
		require.NoError(t, err)
		mockManager.On("AddResponse", mock.Anything).Return(fmt.Errorf("fail ping"))
		_, err = ping(mockManager, ep, "")
		require.Error(t, err)
	})

	t.Run("success ping", func(t *testing.T) {
		mockManager := new(mocks.Manager)
		ep, err := GetRegistryEndpoint(&image.ContainerImage{RegistryURL: ""})
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
