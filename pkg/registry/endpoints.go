package registry

import (
	"fmt"
	"sync"
)

// RegistryEndpoint holds information on how to access any specific registry API
// endpoint.
type RegistryEndpoint struct {
	RegistryName   string
	RegistryPrefix string
	RegistryAPI    string
	Username       string
	Password       string
	Ping           bool
	Credentials    string

	lock sync.RWMutex
}

// Map of configured registries, pre-filled with some well-known registries
var registries map[string]*RegistryEndpoint = map[string]*RegistryEndpoint{
	"": {
		RegistryName:   "Docker Hub",
		RegistryPrefix: "",
		RegistryAPI:    "https://registry-1.docker.io",
		Ping:           true,
	},
	"gcr.io": {
		RegistryName:   "Google Container Registry",
		RegistryPrefix: "gcr.io",
		RegistryAPI:    "https://gcr.io",
		Ping:           false,
	},
	"quay.io": {
		RegistryName:   "RedHat Quay",
		RegistryPrefix: "quay.io",
		RegistryAPI:    "https://quay.io",
		Ping:           false,
	},
}

// Simple RW mutex for concurrent access to registries map
var registryLock sync.RWMutex

// AddRegistryEndpoint adds registry endpoint information with the given details
func AddRegistryEndpoint(prefix, name, apiUrl, username, password, credentials string) error {
	registryLock.Lock()
	defer registryLock.Unlock()
	registries[prefix] = &RegistryEndpoint{
		RegistryName:   name,
		RegistryPrefix: prefix,
		RegistryAPI:    apiUrl,
		Username:       username,
		Password:       password,
		Credentials:    credentials,
	}
	return nil
}

// GetRegistryEndpoint retrieves the endpoint information for the given prefix
func GetRegistryEndpoint(prefix string) (*RegistryEndpoint, error) {
	registryLock.RLock()
	defer registryLock.RUnlock()
	if registry, ok := registries[prefix]; ok {
		return registry, nil
	} else {
		return nil, fmt.Errorf("no registry with prefix '%s' configured", prefix)
	}
}

// SetRegistryEndpointCredentials allows to change the credentials used for
// endpoint access for existing RegistryEndpoint configuration
func SetRegistryEndpointCredentials(prefix, username, password string) error {
	registry, err := GetRegistryEndpoint(prefix)
	if err != nil {
		return err
	}
	registry.lock.Lock()
	registry.Username = username
	registry.Password = password
	registry.lock.Unlock()
	return nil
}
