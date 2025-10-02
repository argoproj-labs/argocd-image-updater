package registry

import (
	"context"
	"crypto/tls"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/cache"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/image"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"

	memcache "github.com/patrickmn/go-cache"
	"go.uber.org/ratelimit"
	"golang.org/x/sync/singleflight"
)

// TagListSort defines how the registry returns the list of tags
type TagListSort int

const (
	TagListSortUnknown           TagListSort = -1
	TagListSortUnsorted          TagListSort = 0
	TagListSortLatestFirst       TagListSort = 1
	TagListSortLatestLast        TagListSort = 2
	TagListSortUnsortedString    string      = "unsorted"
	TagListSortLatestFirstString string      = "latest-first"
	TagListSortLatestLastString  string      = "latest-last"
	TagListSortUnknownString     string      = "unknown"
)

const (
	RateLimitNone    = math.MaxInt32
	RateLimitDefault = 10
)

// IsTimeSorted returns whether a tag list is time sorted
func (tls TagListSort) IsTimeSorted() bool {
	return tls == TagListSortLatestFirst || tls == TagListSortLatestLast
}

// TagListSortFromString gets the TagListSort value from a given string
func TagListSortFromString(tls string) TagListSort {
	switch strings.ToLower(tls) {
	case "latest-first":
		return TagListSortLatestFirst
	case "latest-last":
		return TagListSortLatestLast
	case "none", "":
		return TagListSortUnsorted
	default:
		log.Warnf("unknown tag list sort mode: %s", tls)
		return TagListSortUnknown
	}
}

// String returns the string representation of a TagListSort value
func (tls TagListSort) String() string {
	switch tls {
	case TagListSortLatestFirst:
		return TagListSortLatestFirstString
	case TagListSortLatestLast:
		return TagListSortLatestLastString
	case TagListSortUnsorted:
		return TagListSortUnsortedString
	}

	return TagListSortUnknownString
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
	IsDefault      bool
	lock           sync.RWMutex
	limit          int
}

// registryTweaks should contain a list of registries whose settings cannot be
// inferred by just looking at the image prefix. Prominent example here is the
// Docker Hub registry, which is referred to as docker.io from the image, but
// its API endpoint is https://registry-1.docker.io (and not https://docker.io)
var registryTweaks map[string]*RegistryEndpoint = map[string]*RegistryEndpoint{
	"docker.io": {
		RegistryName:   "Docker Hub",
		RegistryPrefix: "docker.io",
		RegistryAPI:    "https://registry-1.docker.io",
		Ping:           true,
		Insecure:       false,
		DefaultNS:      "library",
		Cache:          cache.NewMemCache(),
		Limiter:        ratelimit.New(RateLimitDefault),
		IsDefault:      true,
	},
}

var registries map[string]*RegistryEndpoint = make(map[string]*RegistryEndpoint)

// Default registry points to the registry that is to be used as the default,
// e.g. when no registry prefix is given for a certain image.
var defaultRegistry *RegistryEndpoint

// Simple RW mutex for concurrent access to registries map
var registryLock sync.RWMutex

// credentialGroup ensures only one credential refresh happens per registry
var credentialGroup singleflight.Group

// Transport cache to avoid creating new transports for each request
// Using go-cache with 30 minute expiration and 10 minute cleanup interval
var transportCache = memcache.New(30*time.Minute, 10*time.Minute)

func AddRegistryEndpointFromConfig(ctx context.Context, epc RegistryConfiguration) error {
	ep := NewRegistryEndpoint(epc.Prefix, epc.Name, epc.ApiURL, epc.Credentials, epc.DefaultNS, epc.Insecure, TagListSortFromString(epc.TagSortMode), epc.Limit, epc.CredsExpire)
	return AddRegistryEndpoint(ctx, ep)
}

// NewRegistryEndpoint returns an endpoint object with the given configuration
// pre-populated and a fresh cache.
func NewRegistryEndpoint(prefix, name, apiUrl, credentials, defaultNS string, insecure bool, tagListSort TagListSort, limit int, credsExpire time.Duration) *RegistryEndpoint {
	if limit <= 0 {
		limit = RateLimitNone
	}
	ep := &RegistryEndpoint{
		RegistryName:   name,
		RegistryPrefix: prefix,
		RegistryAPI:    strings.TrimSuffix(apiUrl, "/"),
		Credentials:    credentials,
		CredsExpire:    credsExpire,
		Cache:          cache.NewMemCache(),
		Insecure:       insecure,
		DefaultNS:      defaultNS,
		TagListSort:    tagListSort,
		Limiter:        ratelimit.New(limit),
		limit:          limit,
	}
	return ep
}

// AddRegistryEndpoint adds registry endpoint information with the given details
func AddRegistryEndpoint(ctx context.Context, ep *RegistryEndpoint) error {
	prefix := ep.RegistryPrefix
	logCtx := log.LoggerFromContext(ctx)

	registryLock.Lock()
	// If the endpoint is supposed to be the default endpoint, make sure that
	// any previously set default endpoint is unset.
	if ep.IsDefault {
		if dep := GetDefaultRegistry(); dep != nil {
			dep.IsDefault = false
		}
		SetDefaultRegistry(ep)
	}
	registries[prefix] = ep
	registryLock.Unlock()

	logCtx = logCtx.WithField("registry", ep.RegistryAPI).WithField("prefix", ep.RegistryPrefix)
	if ep.limit != RateLimitNone {
		logCtx.Debugf("setting rate limit to %d requests per second", ep.limit)
	} else {
		logCtx.Debugf("rate limiting is disabled")
	}
	return nil
}

// inferRegistryEndpointFromPrefix returns a registry endpoint with the API
// URL inferred from the prefix and adds it to the list of the configured
// registries.
func inferRegistryEndpointFromPrefix(prefix string) *RegistryEndpoint {
	apiURL := "https://" + prefix
	return NewRegistryEndpoint(prefix, prefix, apiURL, "", "", false, TagListSortUnsorted, 20, 0)
}

// findRegistryEndpointByImage finds registry by prefix based on full image name
func findRegistryEndpointByImage(ctx context.Context, img *image.ContainerImage) (ep *RegistryEndpoint) {
	log := log.LoggerFromContext(ctx)

	imgName := fmt.Sprintf("%s/%s", img.RegistryURL, img.ImageName)
	log.Debugf("Try to find endpoint by image: %s", imgName)
	registryLock.RLock()
	defer registryLock.RUnlock()

	for _, registry := range registries {
		matchRegistryPrefix := strings.HasPrefix(imgName, registry.RegistryPrefix)
		if (ep == nil && matchRegistryPrefix) || (matchRegistryPrefix && len(registry.RegistryPrefix) > len(ep.RegistryPrefix)) {
			log.Debugf("Selected registry: '%s' (last selection in log - final)", registry.RegistryName)
			ep = registry
		}
	}

	return
}

// GetRegistryEndpoint retrieves the endpoint information for the given image
func GetRegistryEndpoint(ctx context.Context, img *image.ContainerImage) (*RegistryEndpoint, error) {
	log := log.LoggerFromContext(ctx)

	prefix := img.RegistryURL
	if prefix == "" {
		if defaultRegistry == nil {
			return nil, fmt.Errorf("no default endpoint configured")
		} else {
			return defaultRegistry, nil
		}
	}

	registryLock.RLock()
	registry, ok := registries[prefix]
	registryLock.RUnlock()

	if ok {
		return registry, nil
	} else {
		ep := findRegistryEndpointByImage(ctx, img)
		if ep != nil {
			return ep, nil
		}

		var err error
		ep = inferRegistryEndpointFromPrefix(prefix)
		if ep != nil {
			err = AddRegistryEndpoint(ctx, ep)
		} else {
			err = fmt.Errorf("could not infer registry configuration from prefix %s", prefix)
		}
		if err == nil {
			log.Debugf("Inferred registry from prefix %s to use API %s", prefix, ep.RegistryAPI)
		}
		return ep, err
	}
}

// SetDefaultRegistry sets a given registry endpoint as the default
func SetDefaultRegistry(ep *RegistryEndpoint) {
	log.Debugf("Setting default registry endpoint to %s", ep.RegistryPrefix)
	ep.IsDefault = true
	if defaultRegistry != nil {
		log.Debugf("Previous default registry was %s", defaultRegistry.RegistryPrefix)
		defaultRegistry.IsDefault = false
	}
	defaultRegistry = ep
}

// GetDefaultRegistry returns the registry endpoint that is set as default,
// or nil if no default registry endpoint is set
func GetDefaultRegistry() *RegistryEndpoint {
	if defaultRegistry != nil {
		log.Debugf("Getting default registry endpoint: %s", defaultRegistry.RegistryPrefix)
	} else {
		log.Debugf("No default registry defined.")
	}
	return defaultRegistry
}

// SetRegistryEndpointCredentials allows to change the credentials used for
// endpoint access for existing RegistryEndpoint configuration
func SetRegistryEndpointCredentials(ctx context.Context, prefix, credentials string) error {
	registry, err := GetRegistryEndpoint(ctx, &image.ContainerImage{RegistryURL: prefix})
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
	registryLock.RLock()
	defer registryLock.RUnlock()
	r := make([]string, 0, len(registries))
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
	newEp.IsDefault = ep.IsDefault
	newEp.limit = ep.limit
	ep.lock.RUnlock()
	return newEp
}

// ClearTransportCache clears the transport cache
// This is useful when registry configuration changes
func ClearTransportCache() {
	transportCache.Flush()
}

// GetTransport returns a transport object for this endpoint
// Implements connection pooling and reuse to avoid creating new transports for each request
func (ep *RegistryEndpoint) GetTransport() *http.Transport {
	// Check if we have a cached transport for this registry
	if cachedTransport, found := transportCache.Get(ep.RegistryAPI); found {
		transport := cachedTransport.(*http.Transport)
		log.Debugf("Transport cache HIT for %s: %p", ep.RegistryAPI, transport)

		// Validate that the transport is still usable
		if isTransportValid(transport) {
			return transport
		}

		// Transport is stale, remove it from cache
		log.Debugf("Transport for %s is stale, removing from cache", ep.RegistryAPI)
		transportCache.Delete(ep.RegistryAPI)
	}

	log.Debugf("Transport cache MISS for %s", ep.RegistryAPI)

	// Create a new transport with optimized connection pool settings
	tlsC := &tls.Config{}
	if ep.Insecure {
		tlsC.InsecureSkipVerify = true
	}

	// Create transport with aggressive timeout and connection management
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		TLSClientConfig:       tlsC,
		MaxIdleConns:          20,               // Reduced global max idle connections
		MaxIdleConnsPerHost:   5,                // Reduced per-host connections
		IdleConnTimeout:       90 * time.Second, // Reduced idle timeout
		TLSHandshakeTimeout:   10 * time.Second, // Reduced TLS timeout
		ExpectContinueTimeout: 1 * time.Second,  // Expect-Continue timeout
		DisableKeepAlives:     false,            // Enable HTTP Keep-Alive
		ForceAttemptHTTP2:     true,             // Enable HTTP/2 if available
		// Critical timeout settings to prevent hanging connections
		ResponseHeaderTimeout: 10 * time.Second, // Response header timeout
		MaxConnsPerHost:       10,               // Limit total connections per host
	}

	// Cache the transport for reuse with default expiration (30 minutes)
	transportCache.Set(ep.RegistryAPI, transport, memcache.DefaultExpiration)
	log.Debugf("Cached NEW transport for %s: %p", ep.RegistryAPI, transport)

	return transport
}

// isTransportValid checks if a cached transport is still valid and usable
func isTransportValid(transport *http.Transport) bool {
	// Basic validation - check if transport is not nil and has valid configuration
	if transport == nil {
		return false
	}

	// Check if the transport's connection settings are reasonable
	// This is a simple validation, more sophisticated checks could be added
	if transport.MaxIdleConns < 0 || transport.MaxIdleConnsPerHost < 0 {
		return false
	}

	// Transport appears to be valid
	return true
}

// init initializes the registry configuration
func init() {
	for k, v := range registryTweaks {
		registries[k] = v.DeepCopy()
		if v.IsDefault {
			if defaultRegistry == nil {
				defaultRegistry = v
			} else {
				panic("only one default registry can be configured")
			}
		}
	}
}
