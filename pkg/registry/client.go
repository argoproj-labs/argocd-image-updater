package registry

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/argoproj-labs/argocd-image-updater/pkg/log"
	"github.com/argoproj-labs/argocd-image-updater/pkg/tag"

	"github.com/docker/distribution"
	"github.com/docker/distribution/manifest/schema1"
	"github.com/docker/distribution/manifest/schema2"
	"github.com/nokia/docker-registry-client/registry"
	"go.uber.org/ratelimit"
)

// TODO: Check image's architecture and OS

// RegistryClient defines the methods we need for querying container registries
type RegistryClient interface {
	Tags(nameInRepository string) ([]string, error)
	ManifestV1(repository string, reference string) (*schema1.SignedManifest, error)
	ManifestV2(repository string, reference string) (*schema2.DeserializedManifest, error)
	TagMetadata(repository string, manifest distribution.Manifest) (*tag.TagInfo, error)
}

type NewRegistryClient func(*RegistryEndpoint, string, string) (RegistryClient, error)

// Helper type for registry clients
type registryClient struct {
	regClient *registry.Registry
}

// rateLimitTransport encapsulates our custom HTTP round tripper with rate
// limiter from the endpoint.
type rateLimitTransport struct {
	limiter   ratelimit.Limiter
	transport http.RoundTripper
}

// RoundTrip is a custom RoundTrip method with rate-limiter
func (rlt *rateLimitTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	rlt.limiter.Take()
	log.Tracef("%s", r.URL)
	return rlt.transport.RoundTrip(r)
}

// newRegistry is a wrapper for creating a registry client that is possibly
// rate-limited by using a custom HTTP round tripper method.
func newRegistry(ep *RegistryEndpoint, opts registry.Options) (*registry.Registry, error) {
	url := strings.TrimSuffix(ep.RegistryAPI, "/")
	var transport http.RoundTripper
	if opts.Insecure {
		transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}
	} else {
		transport = http.DefaultTransport
	}
	transport = registry.WrapTransport(transport, url, opts)

	rlt := &rateLimitTransport{
		limiter:   ep.Limiter,
		transport: transport,
	}

	logf := opts.Logf
	if logf == nil {
		logf = registry.Log
	}
	registry := &registry.Registry{
		URL: url,
		Client: &http.Client{
			Transport: rlt,
		},
		Logf: logf,
	}
	if opts.DoInitialPing {
		if err := registry.Ping(); err != nil {
			return nil, err
		}
	}
	return registry, nil

}

// NewClient returns a new RegistryClient for the given endpoint information
func NewClient(endpoint *RegistryEndpoint, username, password string) (RegistryClient, error) {

	if username == "" && endpoint.Username != "" {
		username = endpoint.Username
	}
	if password == "" && endpoint.Password != "" {
		password = endpoint.Password
	}

	client, err := newRegistry(endpoint, registry.Options{
		DoInitialPing: endpoint.Ping,
		Logf:          registry.Quiet,
		Username:      username,
		Password:      password,
		Insecure:      endpoint.Insecure,
	})
	if err != nil {
		return nil, err
	}
	return &registryClient{
		regClient: client,
	}, nil
}

// Tags returns a list of tags for given name in repository
func (client *registryClient) Tags(nameInRepository string) ([]string, error) {
	return client.regClient.Tags(nameInRepository)
}

// ManifestV1 returns a signed V1 manifest for a given tag in given repository
func (client *registryClient) ManifestV1(repository string, reference string) (*schema1.SignedManifest, error) {
	return client.regClient.ManifestV1(repository, reference)
}

// ManifestV2 returns a deserialized V2 manifest for a given tag in given repository
func (client *registryClient) ManifestV2(repository string, reference string) (*schema2.DeserializedManifest, error) {
	return client.regClient.ManifestV2(repository, reference)
}

// GetTagInfo retrieves metadata for a given manifest of given repository
func (client *registryClient) TagMetadata(repository string, manifest distribution.Manifest) (*tag.TagInfo, error) {
	ti := &tag.TagInfo{}

	var info struct {
		Arch    string `json:"architecture"`
		Created string `json:"created"`
		OS      string `json:"os"`
	}

	// We support both V1 and V2 manifest schemas. Everything else will trigger
	// an error.
	switch deserialized := manifest.(type) {

	case *schema1.SignedManifest:
		var man schema1.Manifest = deserialized.Manifest
		if len(man.History) == 0 {
			return nil, fmt.Errorf("no history information found in schema V1")
		}
		if err := json.Unmarshal([]byte(man.History[0].V1Compatibility), &info); err != nil {
			return nil, err
		}
		if createdAt, err := time.Parse(time.RFC3339Nano, info.Created); err != nil {
			return nil, err
		} else {
			ti.CreatedAt = createdAt
		}
		return ti, nil

	case *schema2.DeserializedManifest:
		var man schema2.Manifest = deserialized.Manifest

		// The data we require from a V2 manifest is in a blob that we need to
		// fetch from the registry.
		_, err := client.regClient.BlobMetadata(repository, man.Config.Digest)
		if err != nil {
			return nil, fmt.Errorf("could not get metadata: %v", err)
		}

		blobReader, err := client.regClient.DownloadBlob(repository, man.Config.Digest)
		if err != nil {
			return nil, err
		}
		defer blobReader.Close()

		blobBytes := bytes.Buffer{}
		n, err := blobBytes.ReadFrom(blobReader)
		if err != nil {
			return nil, err
		}

		log.Tracef("read %d bytes of blob data for %s", n, repository)

		if err := json.Unmarshal(blobBytes.Bytes(), &info); err != nil {
			return nil, err
		}

		if ti.CreatedAt, err = time.Parse(time.RFC3339Nano, info.Created); err != nil {
			return nil, err
		}
		return ti, nil

	default:
		return nil, fmt.Errorf("invalid manifest type")
	}
}
