package registry

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/argoproj-labs/argocd-image-updater/pkg/cache"
	"github.com/argoproj-labs/argocd-image-updater/pkg/log"

	"go.uber.org/ratelimit"
)

// TagListSort defines how the registry returns the list of tags
type TagListSort int

const (
	SortUnsorted    TagListSort = 0
	SortLatestFirst TagListSort = 1
	SortLatestLast  TagListSort = 2
)

const (
	RateLimitNone    = math.MaxInt32
	RateLimitDefault = 10
)

// IsTimeSorted returns whether a tag list is time sorted
func (tls TagListSort) IsTimeSorted() bool {
	return tls == SortLatestFirst || tls == SortLatestLast
}

// TagListSortFromString gets the TagListSort value from a given string
func TagListSortFromString(tls string) TagListSort {
	switch strings.ToLower(tls) {
	case "latest-first":
		return SortLatestFirst
	case "latest-last":
		return SortLatestLast
	case "none", "":
		return SortUnsorted
	default:
		log.Warnf("unknown tag list sort mode: %s", tls)
		return SortUnsorted
	}
}

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
	Insecure       bool
	DefaultNS      string
	CredsExpire    time.Duration
	CredsUpdated   time.Time
	TagListSort    TagListSort
	Cache          cache.ImageTagCache
	Limiter        ratelimit.Limiter
	lock           sync.RWMutex
}

// Map of configured registries, pre-filled with some well-known registries
var defaultRegistries map[string]*RegistryEndpoint = map[string]*RegistryEndpoint{
	"": {
		RegistryName:   "Docker Hub",
		RegistryPrefix: "",
		RegistryAPI:    "https://registry-1.docker.io",
		Ping:           true,
		Insecure:       false,
		DefaultNS:      "library",
		Cache:          cache.NewMemCache(),
		Limiter:        ratelimit.New(RateLimitDefault),
	},
	"gcr.io": {
		RegistryName:   "Google Container Registry",
		RegistryPrefix: "gcr.io",
		RegistryAPI:    "https://gcr.io",
		Ping:           false,
		Insecure:       false,
		Cache:          cache.NewMemCache(),
		Limiter:        ratelimit.New(RateLimitDefault),
	},
	"quay.io": {
		RegistryName:   "RedHat Quay",
		RegistryPrefix: "quay.io",
		RegistryAPI:    "https://quay.io",
		Ping:           false,
		Insecure:       false,
		Cache:          cache.NewMemCache(),
		Limiter:        ratelimit.New(RateLimitDefault),
	},
	"docker.pkg.github.com": {
		RegistryName:   "GitHub packages",
		RegistryPrefix: "docker.pkg.github.com",
		RegistryAPI:    "https://docker.pkg.github.com",
		Ping:           false,
		Insecure:       false,
		Cache:          cache.NewMemCache(),
		Limiter:        ratelimit.New(RateLimitDefault),
	},
	"ghcr.io": {
		RegistryName:   "GitHub Container Registry",
		RegistryPrefix: "ghcr.io",
		RegistryAPI:    "https://ghcr.io",
		Ping:           false,
		Insecure:       false,
		Cache:          cache.NewMemCache(),
		Limiter:        ratelimit.New(RateLimitDefault),
	},
}

var registries map[string]*RegistryEndpoint = make(map[string]*RegistryEndpoint)

// Simple RW mutex for concurrent access to registries map
var registryLock sync.RWMutex

func AddRegistryEndpointFromConfig(epc RegistryConfiguration) error {
	return AddRegistryEndpoint(epc.Prefix, epc.Name, epc.ApiURL, epc.Credentials, epc.DefaultNS, epc.Insecure, TagListSortFromString(epc.TagSortMode), epc.Limit, epc.CredsExpire)
}

// AddRegistryEndpoint adds registry endpoint information with the given details
func AddRegistryEndpoint(prefix, name, apiUrl, credentials, defaultNS string, insecure bool, tagListSort TagListSort, limit int, credsExpire time.Duration) error {
	registryLock.Lock()
	defer registryLock.Unlock()
	if limit <= 0 {
		limit = RateLimitNone
	}
	log.Debugf("rate limit for %s is %d", apiUrl, limit)
	registries[prefix] = &RegistryEndpoint{
		RegistryName:   name,
		RegistryPrefix: prefix,
		RegistryAPI:    apiUrl,
		Credentials:    credentials,
		CredsExpire:    credsExpire,
		Cache:          cache.NewMemCache(),
		Insecure:       insecure,
		DefaultNS:      defaultNS,
		TagListSort:    tagListSort,
		Limiter:        ratelimit.New(limit),
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
func SetRegistryEndpointCredentials(prefix, credentials string) error {
	registry, err := GetRegistryEndpoint(prefix)
	if err != nil {
		return err
	}
	registry.lock.Lock()
	registry.Credentials = credentials
	registry.lock.Unlock()
	return nil
}

// ConfiguredEndpoints returns a list of prefixes that are configured
func ConfiguredEndpoints() []string {
	r := []string{}
	registryLock.RLock()
	defer registryLock.RUnlock()
	for _, v := range registries {
		r = append(r, v.RegistryPrefix)
	}
	return r
}

// DeepCopy copies the endpoint to a new object, but creating a new Cache
func (ep *RegistryEndpoint) DeepCopy() *RegistryEndpoint {
	ep.lock.RLock()
	newEp := &RegistryEndpoint{}
	newEp.RegistryAPI = ep.RegistryAPI
	newEp.RegistryName = ep.RegistryName
	newEp.RegistryPrefix = ep.RegistryPrefix
	newEp.Credentials = ep.Credentials
	newEp.Ping = ep.Ping
	newEp.TagListSort = ep.TagListSort
	newEp.Cache = cache.NewMemCache()
	newEp.Insecure = ep.Insecure
	newEp.DefaultNS = ep.DefaultNS
	newEp.Limiter = ep.Limiter
	newEp.CredsExpire = ep.CredsExpire
	newEp.CredsUpdated = ep.CredsUpdated
	ep.lock.RUnlock()
	return newEp
}

func init() {
	for k, v := range defaultRegistries {
		registries[k] = v.DeepCopy()
	}
}
