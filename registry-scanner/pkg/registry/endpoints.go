package registry

import (
	"crypto/tls"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
	"strconv"

	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/cache"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/env"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"

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
    // in-flight limiter channel (nil until first use). Controls concurrent HTTP
    // requests per registry to prevent socket/port exhaustion under bursts.
    inflightCh     chan struct{}
    // desired capacity for inflight channel
    inflightCap    int
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

// transportCache stores reusable HTTP transports per registry API URL.
// Reusing transports enables HTTP keep-alives/connection pooling and avoids
// excessive TLS handshakes when the updater queries registries frequently.
var transportCache = make(map[string]*http.Transport)
var transportCacheLock sync.RWMutex

func AddRegistryEndpointFromConfig(epc RegistryConfiguration) error {
	ep := NewRegistryEndpoint(epc.Prefix, epc.Name, epc.ApiURL, epc.Credentials, epc.DefaultNS, epc.Insecure, TagListSortFromString(epc.TagSortMode), epc.Limit, epc.CredsExpire)
	return AddRegistryEndpoint(ep)
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
        inflightCap:    15,
	}
	return ep
}

// AddRegistryEndpoint adds registry endpoint information with the given details
func AddRegistryEndpoint(ep *RegistryEndpoint) error {
	prefix := ep.RegistryPrefix

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

	logCtx := log.WithContext()
	logCtx.AddField("registry", ep.RegistryAPI)
	logCtx.AddField("prefix", ep.RegistryPrefix)
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
func findRegistryEndpointByImage(img *image.ContainerImage) (ep *RegistryEndpoint) {
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
func GetRegistryEndpoint(img *image.ContainerImage) (*RegistryEndpoint, error) {
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
		ep := findRegistryEndpointByImage(img)
		if ep != nil {
			return ep, nil
		}

		var err error
		ep = inferRegistryEndpointFromPrefix(prefix)
		if ep != nil {
			err = AddRegistryEndpoint(ep)
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
func SetRegistryEndpointCredentials(prefix, credentials string) error {
	registry, err := GetRegistryEndpoint(&image.ContainerImage{RegistryURL: prefix})
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
    newEp.inflightCap = ep.inflightCap
	ep.lock.RUnlock()
	return newEp
}

// ClearTransportCache clears cached transports (e.g., after registry config reload)
func ClearTransportCache() {
	transportCacheLock.Lock()
	defer transportCacheLock.Unlock()
    // Proactively close idle connections on existing transports before clearing
    for _, tr := range transportCache {
        tr.CloseIdleConnections()
    }
    transportCache = make(map[string]*http.Transport)
	log.Debugf("Transport cache cleared.")
}

// StartTransportJanitor periodically closes idle connections on all cached
// transports to prevent idle socket accumulation. Returns a stop function.
func StartTransportJanitor(interval time.Duration) func() {
    if interval <= 0 {
        return func() {}
    }
    stopCh := make(chan struct{})
    ticker := time.NewTicker(interval)
    go func() {
        for {
            select {
            case <-ticker.C:
                transportCacheLock.RLock()
                for _, tr := range transportCache {
                    tr.CloseIdleConnections()
                }
                transportCacheLock.RUnlock()
            case <-stopCh:
                ticker.Stop()
                return
            }
        }
    }()
    return func() { close(stopCh) }
}

// GetTransport returns a cached transport configured with sane defaults.
// The transport is keyed by the endpoint's RegistryAPI and shared by callers
// to maximize connection reuse and apply timeouts consistently.
func (ep *RegistryEndpoint) GetTransport() *http.Transport {
	// Cache key must account for TLS mode to avoid reusing a secure transport for insecure endpoints
	key := ep.RegistryAPI + "|insecure=" + strconv.FormatBool(ep.Insecure)
	// Fast path: return cached transport if present
	transportCacheLock.RLock()
	if tr, ok := transportCache[key]; ok {
		transportCacheLock.RUnlock()
		return tr
	}
	transportCacheLock.RUnlock()

	tlsC := &tls.Config{}
	if ep.Insecure {
		tlsC.InsecureSkipVerify = true
	}

	// Create and cache a transport with sane defaults
    // Allow overriding key HTTP transport timeouts via environment
    respHdrTimeout := env.ParseDurationFromEnv("REGISTRY_RESPONSE_HEADER_TIMEOUT", 60*time.Second, 1*time.Second, time.Hour)
    tlsHsTimeout := env.ParseDurationFromEnv("REGISTRY_TLS_HANDSHAKE_TIMEOUT", 10*time.Second, 1*time.Second, time.Hour)
    idleConnTimeout := env.ParseDurationFromEnv("REGISTRY_IDLE_CONN_TIMEOUT", 90*time.Second, 0, 24*time.Hour)
    maxConnsPerHost := env.ParseNumFromEnv("REGISTRY_MAX_CONNS_PER_HOST", 30, 1, 10000)
    maxIdleConns := env.ParseNumFromEnv("REGISTRY_MAX_IDLE_CONNS", 1000, 1, 100000)
    maxIdleConnsPerHost := env.ParseNumFromEnv("REGISTRY_MAX_IDLE_CONNS_PER_HOST", 200, 1, 100000)

    tr := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		TLSClientConfig:       tlsC,
        // Prefer reuse of a larger idle pool to minimize new dials under load
        MaxIdleConns:          maxIdleConns,
        MaxIdleConnsPerHost:   maxIdleConnsPerHost,
        // Cap parallel dials per host to avoid ephemeral port exhaustion
        MaxConnsPerHost:       maxConnsPerHost,

		IdleConnTimeout:       idleConnTimeout,
		TLSHandshakeTimeout:   tlsHsTimeout,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
		ResponseHeaderTimeout: respHdrTimeout,
	}
	transportCacheLock.Lock()
	transportCache[key] = tr
	transportCacheLock.Unlock()
	return tr
}

// getInflightChan returns the per-registry inflight channel, creating it on first use.
func (ep *RegistryEndpoint) getInflightChan() chan struct{} {
    ep.lock.Lock()
    defer ep.lock.Unlock()
    if ep.inflightCh == nil {
        cap := ep.inflightCap
        if cap <= 0 { cap = 15 }
        ep.inflightCh = make(chan struct{}, cap)
    }
    return ep.inflightCh
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
