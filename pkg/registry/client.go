package registry

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	Tags(nameInRepository string, limiter ratelimit.Limiter) ([]string, error)
	ManifestV1(repository string, reference string, limiter ratelimit.Limiter) (*schema1.SignedManifest, error)
	ManifestV2(repository string, reference string, limiter ratelimit.Limiter) (*schema2.DeserializedManifest, error)
	TagMetadata(repository string, manifest distribution.Manifest, limiter ratelimit.Limiter) (*tag.TagInfo, error)
}

type NewRegistryClient func(*RegistryEndpoint, string, string) (RegistryClient, error)

// Helper type for registry clients
type registryClient struct {
	regClient *registry.Registry
}

// NewClient returns a new RegistryClient for the given endpoint information
func NewClient(endpoint *RegistryEndpoint, username, password string) (RegistryClient, error) {

	if username == "" && endpoint.Username != "" {
		username = endpoint.Username
	}
	if password == "" && endpoint.Password != "" {
		password = endpoint.Password
	}

	client, err := registry.NewCustom(endpoint.RegistryAPI, registry.Options{
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
func (client *registryClient) Tags(nameInRepository string, limiter ratelimit.Limiter) ([]string, error) {
	limiter.Take()
	return client.regClient.Tags(nameInRepository)
}

// ManifestV1 returns a signed V1 manifest for a given tag in given repository
func (client *registryClient) ManifestV1(repository string, reference string, limiter ratelimit.Limiter) (*schema1.SignedManifest, error) {
	limiter.Take()
	return client.regClient.ManifestV1(repository, reference)
}

// ManifestV2 returns a deserialized V2 manifest for a given tag in given repository
func (client *registryClient) ManifestV2(repository string, reference string, limiter ratelimit.Limiter) (*schema2.DeserializedManifest, error) {
	limiter.Take()
	return client.regClient.ManifestV2(repository, reference)
}

// GetTagInfo retrieves metadata for a given manifest of given repository
func (client *registryClient) TagMetadata(repository string, manifest distribution.Manifest, limiter ratelimit.Limiter) (*tag.TagInfo, error) {
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
		limiter.Take()
		_, err := client.regClient.BlobMetadata(repository, man.Config.Digest)
		if err != nil {
			return nil, fmt.Errorf("could not get metadata: %v", err)
		}

		limiter.Take()
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
