package registry

import (
	"fmt"
	"io/ioutil"

	"github.com/argoproj-labs/argocd-image-updater/pkg/log"

	"gopkg.in/yaml.v2"
)

// RegistryConfiguration represents a single repository configuration for being
// unmarshaled from YAML.
type RegistryConfiguration struct {
	Name        string `yaml:"name"`
	ApiURL      string `yaml:"api_url"`
	Ping        bool   `yaml:"ping,omitempty"`
	Credentials string `yaml:"credentials,omitempty"`
	Prefix      string `yaml:"prefix,omitempty"`
}

// RegistryList contains multiple RegistryConfiguration items
type RegistryList struct {
	Items []RegistryConfiguration `yaml:"registries"`
}

// LoadRegistryConfiguration loads a YAML-formatted registry configuration from
// a given file at path.
func LoadRegistryConfiguration(path string) error {
	registryBytes, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	registryList, err := ParseRegistryConfiguration(string(registryBytes))
	if err != nil {
		return err
	}
	for _, reg := range registryList.Items {
		err = AddRegistryEndpoint(reg.Prefix, reg.Name, reg.ApiURL, "", "", reg.Credentials)
		if err != nil {
			return err
		}
	}
	log.Infof("Loaded %d registry configurations from %s", len(registryList.Items), path)
	return nil
}

// Parses a registry configuration from a YAML input string and returns a list
// of registries.
func ParseRegistryConfiguration(yamlSource string) (RegistryList, error) {
	var regList RegistryList
	err := yaml.UnmarshalStrict([]byte(yamlSource), &regList)
	if err != nil {
		return RegistryList{}, err
	}

	// validate the parsed list
	for _, registry := range regList.Items {
		if registry.Name == "" {
			err = fmt.Errorf("registry name is missing for entry %v", registry)
		}
	}

	if err != nil {
		return RegistryList{}, err
	}

	return regList, nil
}

// RestRestoreDefaultRegistryConfiguration restores the registry configuration
// to the default values.
func RestoreDefaultRegistryConfiguration() {
	registries = make(map[string]*RegistryEndpoint)
	for k, v := range defaultRegistries {
		registries[k] = v.DeepCopy()
	}
}
