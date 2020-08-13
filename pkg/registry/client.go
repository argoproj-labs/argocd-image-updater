package registry

import (
	"github.com/argoproj-labs/argocd-image-updater/pkg/registry/mocks"

	"github.com/docker/distribution/manifest/schema1"
	"github.com/nokia/docker-registry-client/registry"
)

// RegistryClient defines the methods we need for querying container registries
type RegistryClient interface {
	Tags(nameInRepository string) ([]string, error)
	ManifestV1(repository string, reference string) (*schema1.SignedManifest, error)
}

type NewRegistryClient func(*RegistryEndpoint, string, string) (RegistryClient, error)

// Helper type for registry clients
type registryClient struct {
	regClient *registry.Registry
}

// NewMockClient returns a new mocked RegistryClient
func NewMockClient(endpoint *RegistryEndpoint, username, password string) (RegistryClient, error) {
	return &mocks.RegistryClient{}, nil
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
