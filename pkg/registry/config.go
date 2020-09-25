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
	TagSortMode string `yaml:"tagsortmode,omitempty"`
	Prefix      string `yaml:"prefix,omitempty"`
	Insecure    bool   `yaml:"insecure,omitempty"`
	DefaultNS   string `yaml:"defaultns,omitempty"`
}

// RegistryList contains multiple RegistryConfiguration items
type RegistryList struct {
	Items []RegistryConfiguration `yaml:"registries"`
}

// LoadRegistryConfiguration loads a YAML-formatted registry configuration from
// a given file at path.
func LoadRegistryConfiguration(path string, clear bool) error {
	registryBytes, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	registryList, err := ParseRegistryConfiguration(string(registryBytes))
	if err != nil {
		return err
	}

	if clear {
		registryLock.Lock()
		registries = make(map[string]*RegistryEndpoint)
		registryLock.Unlock()
	}

	for _, reg := range registryList.Items {
		tagSortMode := TagListSortFromString(reg.TagSortMode)
		err = AddRegistryEndpoint(reg.Prefix, reg.Name, reg.ApiURL, reg.Credentials, reg.DefaultNS, reg.Insecure, tagSortMode)
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
	var defaultPrefixFound = ""
	err := yaml.UnmarshalStrict([]byte(yamlSource), &regList)
	if err != nil {
		return RegistryList{}, err
	}

	// validate the parsed list
	for _, registry := range regList.Items {
		if registry.Name == "" {
			err = fmt.Errorf("registry name is missing for entry %v", registry)
		} else if registry.ApiURL == "" {
			err = fmt.Errorf("API URL must be specified for registry %s", registry.Name)
		} else if registry.Prefix == "" {
			if defaultPrefixFound != "" {
				err = fmt.Errorf("there must be only one default registry (already is %s), %s needs a prefix", defaultPrefixFound, registry.Name)
			} else {
				defaultPrefixFound = registry.Name
			}
		}

		if err == nil {
			switch registry.TagSortMode {
			case "latest-first", "latest-last", "none", "":
			default:
				err = fmt.Errorf("unknown tag sort mode for registry %s: %s", registry.Name, registry.TagSortMode)
			}
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
	registryLock.Lock()
	defer registryLock.Unlock()
	registries = make(map[string]*RegistryEndpoint)
	for k, v := range defaultRegistries {
		registries[k] = v.DeepCopy()
	}
}
