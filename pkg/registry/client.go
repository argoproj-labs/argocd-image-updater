package registry

import (
	"github.com/docker/distribution/manifest/schema1"
	"github.com/nokia/docker-registry-client/registry"
)

// RegistryClient defines the methods we need for querying container registries
type RegistryClient interface {
	Tags(nameInRepository string) ([]string, error)
	ManifestV1(repository string, reference string) (*schema1.SignedManifest, error)
}

// Helper type for registry clients
type registryClient struct {
	regClient *registry.Registry
}

// NewClient returns a new RegistryClient for the given endpoint information
func NewClient(endpoint *RegistryEndpoint) (RegistryClient, error) {
	client, err := registry.NewCustom(endpoint.RegistryAPI, registry.Options{
		DoInitialPing: endpoint.Ping,
		Logf:          registry.Quiet,
		Username:      endpoint.Username,
		Password:      endpoint.Password,
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

// ManifestV1 returns a signed manifest for a given tag in given repository
func (client *registryClient) ManifestV1(repository string, reference string) (*schema1.SignedManifest, error) {
	return client.regClient.ManifestV1(repository, reference)
}
