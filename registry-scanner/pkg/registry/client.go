package registry

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/argoproj/pkg/json"

	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
    "github.com/argoproj-labs/argocd-image-updater/pkg/metrics"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/options"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/tag"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/manifest/manifestlist"
	"github.com/distribution/distribution/v3/manifest/ocischema"
	"github.com/distribution/distribution/v3/manifest/schema1" //nolint:staticcheck
	"github.com/distribution/distribution/v3/manifest/schema2"
	"github.com/distribution/distribution/v3/reference"
	"github.com/distribution/distribution/v3/registry/client"
	"github.com/distribution/distribution/v3/registry/client/auth"
	"github.com/distribution/distribution/v3/registry/client/auth/challenge"
	"github.com/distribution/distribution/v3/registry/client/transport"

	"github.com/opencontainers/go-digest"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"

	"go.uber.org/ratelimit"

    "bytes"
	"net/http"
	"net/url"
    "io"
    "strconv"
	"strings"
	"sync"
    "math/rand"
    sf "golang.org/x/sync/singleflight"
    "github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/env"
)

// TODO: Check image's architecture and OS

// knownMediaTypes is the list of media types we can process
var knownMediaTypes = []string{
	ocischema.SchemaVersion.MediaType,
	schema1.MediaTypeSignedManifest, //nolint:staticcheck
	schema2.SchemaVersion.MediaType,
	manifestlist.SchemaVersion.MediaType,
	ociv1.MediaTypeImageIndex,
}

// RegistryClient defines the methods we need for querying container registries
type RegistryClient interface {
	NewRepository(nameInRepository string) error
	Tags() ([]string, error)
	ManifestForTag(tagStr string) (distribution.Manifest, error)
	ManifestForDigest(dgst digest.Digest) (distribution.Manifest, error)
	TagMetadata(manifest distribution.Manifest, opts *options.ManifestOptions) (*tag.TagInfo, error)
}

type NewRegistryClient func(*RegistryEndpoint, string, string) (RegistryClient, error)

// Helper type for registry clients
type registryClient struct {
	regClient distribution.Repository
	endpoint  *RegistryEndpoint
	creds     credentials
    repoName  string
}

// credentials is an implementation of distribution/V3/session struct
// to manage registry credentials and token
type credentials struct {
	username      string
	password      string
	refreshTokens map[string]string
}

func (c credentials) Basic(url *url.URL) (string, string) {
	return c.username, c.password
}

func (c credentials) RefreshToken(url *url.URL, service string) string {
	return c.refreshTokens[service]
}

func (c credentials) SetRefreshToken(realm *url.URL, service, token string) {
	if c.refreshTokens != nil {
		c.refreshTokens[service] = token
	}
}

// rateLimitTransport encapsulates our custom HTTP round tripper with rate
// limiter from the endpoint.
type rateLimitTransport struct {
	limiter   ratelimit.Limiter
	transport http.RoundTripper
	endpoint  *RegistryEndpoint
}

// jwtObservingTransport wraps the underlying transport for token fetches
// and records JWT auth metrics (duration, TTL, errors).
type jwtObservingTransport struct {
    endpoint *RegistryEndpoint
    base     http.RoundTripper
    singleflight *sf.Group
}

func getJWTRetrySettings() (int, time.Duration, time.Duration) {
    attempts := env.ParseNumFromEnv("REGISTRY_JWT_ATTEMPTS", 7, 1, 100)
    base := env.ParseDurationFromEnv("REGISTRY_JWT_RETRY_BASE", 200*time.Millisecond, 0, time.Hour)
    max := env.ParseDurationFromEnv("REGISTRY_JWT_RETRY_MAX", 3*time.Second, 0, time.Hour)
    return attempts, base, max
}

func (j *jwtObservingTransport) doAuthWithRetry(req *http.Request, reg, service, scope string) (*http.Response, error) {
    attempts, base, maxDelay := getJWTRetrySettings()
    var lastResp *http.Response
    var lastErr error
    for attempt := 0; attempt < attempts; attempt++ {
        start := time.Now()
        resp, err := j.base.RoundTrip(req)
        metrics.Endpoint().IncreaseJWTAuthRequest(reg, service, scope)
        metrics.Endpoint().ObserveJWTAuthDuration(reg, service, scope, time.Since(start))
        lastResp, lastErr = resp, err
        if err == nil && resp != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
            if resp.Body != nil {
                body, rerr := io.ReadAll(resp.Body)
                if rerr == nil {
                    resp.Body = io.NopCloser(bytes.NewBuffer(body))
                    type tokenResp struct { ExpiresIn int `json:"expires_in"` }
                    var tr tokenResp
                    if jerr := json.Unmarshal(body, &tr); jerr == nil && tr.ExpiresIn > 0 {
                        metrics.Endpoint().ObserveJWTTokenTTL(reg, service, scope, float64(tr.ExpiresIn))
                    } else if jerr != nil {
                        metrics.Endpoint().IncreaseJWTAuthError(reg, service, scope, "parse_json_error")
                    }
                } else {
                    metrics.Endpoint().IncreaseJWTAuthError(reg, service, scope, "read_body_error")
                }
            }
            return resp, nil
        }
        if err != nil {
            metrics.Endpoint().IncreaseJWTAuthError(reg, service, scope, "roundtrip_error")
        } else if resp != nil {
            metrics.Endpoint().IncreaseJWTAuthError(reg, service, scope, "http_"+strconv.Itoa(resp.StatusCode))
            if resp.Body != nil { io.Copy(io.Discard, resp.Body); resp.Body.Close() }
        }
        metrics.Endpoint().IncreaseRetry(reg, "auth")
        d := time.Duration(float64(base) * (1 << attempt) * (0.7 + 0.6*rand.Float64()))
        if d > maxDelay { d = maxDelay }
        time.Sleep(d)
    }
    return lastResp, lastErr
}

func (j *jwtObservingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
    // Only intercept /jwt/auth; otherwise pass-through
    if !strings.Contains(req.URL.Path, "/jwt/auth") || j.singleflight == nil {
        reg := j.endpoint.RegistryAPI
        q := req.URL.Query()
        service := q.Get("service")
        scope := q.Get("scope")
        return j.doAuthWithRetry(req, reg, service, scope)
    }
    // Deduplicate concurrent token fetches for the same (service,scope)
    reg := j.endpoint.RegistryAPI
    q := req.URL.Query()
    service := q.Get("service")
    scope := q.Get("scope")
    key := reg + "|auth|" + service + "|" + scope
    v, err, _ := j.singleflight.Do(key, func() (any, error) {
        return j.doAuthWithRetry(req, reg, service, scope)
    })
    if err != nil {
        return v.(*http.Response), err
    }
    return v.(*http.Response), nil
}

// RoundTrip is a custom RoundTrip method with rate-limiter
func (rlt *rateLimitTransport) RoundTrip(r *http.Request) (*http.Response, error) {
    rlt.limiter.Take()
    reg := rlt.endpoint.RegistryAPI
    // per-registry inflight cap
    inflight := rlt.endpoint.getInflightChan()
    select { case inflight <- struct{}{}: default: inflight <- struct{}{} }
    start := time.Now()
    log.Tracef("Performing HTTP %s %s", r.Method, r.URL)
    // Detect JWT auth endpoint query params if any
    service := r.URL.Query().Get("service")
    scope := r.URL.Query().Get("scope")
    isJWT := strings.Contains(r.URL.Path, "/jwt/auth")
    if isJWT {
        metrics.Endpoint().IncreaseJWTAuthRequest(reg, service, scope)
    }
    resp, err := rlt.transport.RoundTrip(r)
    // metrics
    metrics.Endpoint().IncInFlight(reg)
    defer func() { metrics.Endpoint().DecInFlight(reg) }()
    d := time.Since(start)
    metrics.Endpoint().ObserveRequestDuration(reg, d)
    if isJWT {
        metrics.Endpoint().ObserveJWTAuthDuration(reg, service, scope, d)
        if err != nil {
            metrics.Endpoint().IncreaseJWTAuthError(reg, service, scope, "roundtrip_error")
        }
    }
    // classify /jwt/auth for auth metrics later (placeholder)
    // increase request counters
    metrics.Endpoint().IncreaseRequest(reg, err != nil)
    // record status if available
    if resp != nil { metrics.Endpoint().ObserveHTTPStatus(reg, resp.StatusCode) }
    <-inflight
    return resp, err
}

// NewRepository is a wrapper for creating a registry client that is possibly
// rate-limited by using a custom HTTP round tripper method.
func (clt *registryClient) NewRepository(nameInRepository string) error {
	urlToCall := strings.TrimSuffix(clt.endpoint.RegistryAPI, "/")
	challengeManager1 := challenge.NewSimpleManager()
	_, err := ping(challengeManager1, clt.endpoint, "")
	if err != nil {
		return err
	}

    // Normalize repo key to improve cache hits
    repo := strings.TrimPrefix(nameInRepository, "/")
    cacheKey := clt.endpoint.RegistryAPI + "|" + repo
	// Check for cached auth transport to reuse bearer tokens
	repoAuthTransportCacheLock.RLock()
	cached, ok := repoAuthTransportCache[cacheKey]
	repoAuthTransportCacheLock.RUnlock()
	var baseRT http.RoundTripper
	if ok {
        log.Debugf("authorizer cache HIT key=%s", cacheKey)
		baseRT = cached
	} else {
        log.Debugf("authorizer cache MISS key=%s", cacheKey)
        // Wrap the underlying transport to observe JWT auth responses
        base := clt.endpoint.GetTransport()
        // tokenTransport is used by the token handler to fetch tokens
        tokenTransport := &jwtObservingTransport{endpoint: clt.endpoint, base: base, singleflight: &jwtAuthSingleflight}
        baseRT = transport.NewTransport(
            base, auth.NewAuthorizer(
                challengeManager1,
                auth.NewTokenHandler(tokenTransport, clt.creds, nameInRepository, "pull"),
                auth.NewBasicHandler(clt.creds)))
		repoAuthTransportCacheLock.Lock()
		repoAuthTransportCache[cacheKey] = baseRT
		repoAuthTransportCacheLock.Unlock()
	}

    rlt := &rateLimitTransport{
        limiter:   clt.endpoint.Limiter,
        transport: baseRT,
        endpoint:  clt.endpoint,
    }

	named, err := reference.WithName(nameInRepository)
	if err != nil {
		return err
	}
	clt.regClient, err = client.NewRepository(named, urlToCall, rlt)
	if err != nil {
		return err
	}
    clt.repoName = repo
	return nil
}

// NewClient returns a new RegistryClient for the given endpoint information
func NewClient(endpoint *RegistryEndpoint, username, password string) (RegistryClient, error) {
	if username == "" && endpoint.Username != "" {
		username = endpoint.Username
	}
	if password == "" && endpoint.Password != "" {
		password = endpoint.Password
	}
	// Initialize refreshTokens to enable reusing registry-issued refresh tokens
	// across requests for the same service (e.g., container_registry).
	creds := credentials{
		username:      username,
		password:      password,
		refreshTokens: make(map[string]string),
	}
	return &registryClient{
		creds:    creds,
		endpoint: endpoint,
	}, nil
}

// cache for per-repository auth transports so we reuse bearer tokens across runtime
var repoAuthTransportCache = make(map[string]http.RoundTripper)
var repoAuthTransportCacheLock sync.RWMutex
var jwtAuthSingleflight sf.Group

// singleflight-style maps for deduping concurrent identical calls
var tagsInFlight sync.Map // key string -> chan result
var manifestInFlight sync.Map // key string -> chan result

type tagsResult struct {
    tags []string
    err  error
}

// Tags returns a list of tags for given name in repository
func (clt *registryClient) Tags() ([]string, error) {
    key := clt.endpoint.RegistryAPI + "|tags|" + clt.repoName
    if ch, loaded := tagsInFlight.Load(key); loaded {
        // wait for the leader's result
        res := (<-ch.(chan tagsResult))
        return res.tags, res.err
    }
    ch := make(chan tagsResult, 1)
    actual, loaded := tagsInFlight.LoadOrStore(key, ch)
    if loaded {
        res := (<-actual.(chan tagsResult))
        return res.tags, res.err
    }

    // leader path
    defer func() {
        tagsInFlight.Delete(key)
        close(ch)
    }()

    tagService := clt.regClient.Tags(context.Background())
    var tTags []string
    var err error
    // jittered exponential backoff with per-attempt deadline
    base := 200 * time.Millisecond
    maxDelay := 3 * time.Second
    for attempt := 0; attempt < 3; attempt++ {
        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        tTags, err = tagService.All(ctx)
        cancel()
        if err == nil {
            break
        }
        // jittered backoff
        d := time.Duration(float64(base) * (1 << attempt) * (0.7 + 0.6*rand.Float64()))
        if d > maxDelay { d = maxDelay }
        time.Sleep(d)
    }
    ch <- tagsResult{tags: tTags, err: err}
    if err != nil { return nil, err }
    return tTags, nil
}

// Manifest  returns a Manifest for a given tag in repository
func (clt *registryClient) ManifestForTag(tagStr string) (distribution.Manifest, error) {
    key := clt.endpoint.RegistryAPI + "|manifest|" + clt.repoName + "|tag=" + tagStr
    if ch, loaded := manifestInFlight.Load(key); loaded {
        res := (<-ch.(chan struct{m distribution.Manifest; e error}))
        return res.m, res.e
    }
    ch := make(chan struct{m distribution.Manifest; e error}, 1)
    actual, loaded := manifestInFlight.LoadOrStore(key, ch)
    if loaded { res := (<-actual.(chan struct{m distribution.Manifest; e error})); return res.m, res.e }
    defer func(){ manifestInFlight.Delete(key); close(ch) }()

    manService, err := clt.regClient.Manifests(context.Background())
    if err != nil { ch <- struct{m distribution.Manifest; e error}{nil, err}; return nil, err }
    var manifest distribution.Manifest
    base := 200 * time.Millisecond
    maxDelay := 3 * time.Second
    for attempt := 0; attempt < 3; attempt++ {
        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        manifest, err = manService.Get(ctx, digest.FromString(tagStr), distribution.WithTag(tagStr), distribution.WithManifestMediaTypes(knownMediaTypes))
        cancel()
        if err == nil { break }
        d := time.Duration(float64(base) * (1 << attempt) * (0.7 + 0.6*rand.Float64()))
        if d > maxDelay { d = maxDelay }
        time.Sleep(d)
    }
    ch <- struct{m distribution.Manifest; e error}{manifest, err}
    if err != nil { return nil, err }
    return manifest, nil
}

// ManifestForDigest  returns a Manifest for a given digest in repository
func (clt *registryClient) ManifestForDigest(dgst digest.Digest) (distribution.Manifest, error) {
    key := clt.endpoint.RegistryAPI + "|manifest|" + clt.repoName + "|dgst=" + dgst.String()
    if ch, loaded := manifestInFlight.Load(key); loaded {
        res := (<-ch.(chan struct{m distribution.Manifest; e error}))
        return res.m, res.e
    }
    ch := make(chan struct{m distribution.Manifest; e error}, 1)
    actual, loaded := manifestInFlight.LoadOrStore(key, ch)
    if loaded { res := (<-actual.(chan struct{m distribution.Manifest; e error})); return res.m, res.e }
    defer func(){ manifestInFlight.Delete(key); close(ch) }()

    manService, err := clt.regClient.Manifests(context.Background())
    if err != nil { ch <- struct{m distribution.Manifest; e error}{nil, err}; return nil, err }
    var manifest distribution.Manifest
    base := 200 * time.Millisecond
    maxDelay := 3 * time.Second
    for attempt := 0; attempt < 3; attempt++ {
        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        manifest, err = manService.Get(ctx, dgst, distribution.WithManifestMediaTypes(knownMediaTypes))
        cancel()
        if err == nil { break }
        d := time.Duration(float64(base) * (1 << attempt) * (0.7 + 0.6*rand.Float64()))
        if d > maxDelay { d = maxDelay }
        time.Sleep(d)
    }
    ch <- struct{m distribution.Manifest; e error}{manifest, err}
    if err != nil { return nil, err }
    return manifest, nil
}

// TagMetadata retrieves metadata for a given manifest of given repository
func (client *registryClient) TagMetadata(manifest distribution.Manifest, opts *options.ManifestOptions) (*tag.TagInfo, error) {
	ti := &tag.TagInfo{}
	logCtx := opts.Logger()
	var info struct {
		Arch    string `json:"architecture"`
		Created string `json:"created"`
		OS      string `json:"os"`
		Variant string `json:"variant"`
	}

	// We support the following types of manifests as returned by the registry:
	//
	// V1 (legacy, might go away), V2 and OCI
	//
	// Also ManifestLists (e.g. on multi-arch images) are supported.
	//
	switch deserialized := manifest.(type) {

	case *schema1.SignedManifest: //nolint:staticcheck
		var man schema1.Manifest = deserialized.Manifest //nolint:staticcheck
		if len(man.History) == 0 {
			return nil, fmt.Errorf("no history information found in schema V1")
		}

		_, mBytes, err := manifest.Payload()
		if err != nil {
			return nil, err
		}
		ti.Digest = sha256.Sum256(mBytes)

		logCtx.Tracef("v1 SHA digest is %s", ti.EncodedDigest())
		if err := json.Unmarshal([]byte(man.History[0].V1Compatibility), &info); err != nil {
			return nil, err
		}
		if !opts.WantsPlatform(info.OS, info.Arch, "") {
			logCtx.Debugf("ignoring v1 manifest %v. Manifest platform: %s, requested: %s",
				ti.EncodedDigest(), options.PlatformKey(info.OS, info.Arch, info.Variant), strings.Join(opts.Platforms(), ","))
			return nil, nil
		}
		if createdAt, err := time.Parse(time.RFC3339Nano, info.Created); err != nil {
			return nil, err
		} else {
			ti.CreatedAt = createdAt
		}
		return ti, nil

	case *manifestlist.DeserializedManifestList:
		var list manifestlist.DeserializedManifestList = *deserialized

		// List must contain at least one image manifest
		if len(list.Manifests) == 0 {
			return nil, fmt.Errorf("empty manifestlist not supported")
		}

		// We use the SHA from the manifest list to let the container engine
		// decide which image to pull, in case of multi-arch clusters.
		_, mBytes, err := list.Payload()
		if err != nil {
			return nil, fmt.Errorf("could not retrieve manifestlist payload: %v", err)
		}
		ti.Digest = sha256.Sum256(mBytes)

		logCtx.Tracef("SHA256 of manifest parent is %v", ti.EncodedDigest())

		return TagInfoFromReferences(client, opts, logCtx, ti, list.References())

	case *ocischema.DeserializedImageIndex:
		var index ocischema.DeserializedImageIndex = *deserialized

		// Index must contain at least one image manifest
		if len(index.Manifests) == 0 {
			return nil, fmt.Errorf("empty index not supported")
		}

		// We use the SHA from the manifest index to let the container engine
		// decide which image to pull, in case of multi-arch clusters.
		_, mBytes, err := index.Payload()
		if err != nil {
			return nil, fmt.Errorf("could not retrieve index payload: %v", err)
		}
		ti.Digest = sha256.Sum256(mBytes)

		logCtx.Tracef("SHA256 of manifest parent is %v", ti.EncodedDigest())

		return TagInfoFromReferences(client, opts, logCtx, ti, index.References())

	case *schema2.DeserializedManifest:
		var man schema2.Manifest = deserialized.Manifest

		logCtx.Tracef("Manifest digest is %v", man.Config.Digest.Encoded())

		_, mBytes, err := manifest.Payload()
		if err != nil {
			return nil, err
		}
		ti.Digest = sha256.Sum256(mBytes)
		logCtx.Tracef("v2 SHA digest is %s", ti.EncodedDigest())

		// The data we require from a V2 manifest is in a blob that we need to
		// fetch from the registry.
		blobReader, err := client.regClient.Blobs(context.Background()).Get(context.Background(), man.Config.Digest)
		if err != nil {
			return nil, err
		}

		if err := json.Unmarshal(blobReader, &info); err != nil {
			return nil, err
		}

		if !opts.WantsPlatform(info.OS, info.Arch, info.Variant) {
			logCtx.Debugf("ignoring v2 manifest %v. Manifest platform: %s, requested: %s",
				ti.EncodedDigest(), options.PlatformKey(info.OS, info.Arch, info.Variant), strings.Join(opts.Platforms(), ","))
			return nil, nil
		}

		if ti.CreatedAt, err = time.Parse(time.RFC3339Nano, info.Created); err != nil {
			return nil, err
		}

		return ti, nil
	case *ocischema.DeserializedManifest:
		var man ocischema.Manifest = deserialized.Manifest

		_, mBytes, err := manifest.Payload()
		if err != nil {
			return nil, err
		}
		ti.Digest = sha256.Sum256(mBytes)
		logCtx.Tracef("OCI SHA digest is %s", ti.EncodedDigest())

		// The data we require from a V2 manifest is in a blob that we need to
		// fetch from the registry.
		blobReader, err := client.regClient.Blobs(context.Background()).Get(context.Background(), man.Config.Digest)
		if err != nil {
			return nil, err
		}

		if err := json.Unmarshal(blobReader, &info); err != nil {
			return nil, err
		}

		if !opts.WantsPlatform(info.OS, info.Arch, info.Variant) {
			logCtx.Debugf("ignoring OCI manifest %v. Manifest platform: %s, requested: %s",
				ti.EncodedDigest(), options.PlatformKey(info.OS, info.Arch, info.Variant), strings.Join(opts.Platforms(), ","))
			return nil, nil
		}

		if ti.CreatedAt, err = time.Parse(time.RFC3339Nano, info.Created); err != nil {
			return nil, err
		}

		return ti, nil
	default:
		return nil, fmt.Errorf("invalid manifest type %T", manifest)
	}
}

// TagInfoFromReferences is a helper method to retrieve metadata for a given
// list of references. It will return the most recent pushed manifest from the
// list of references.
func TagInfoFromReferences(client *registryClient, opts *options.ManifestOptions, logCtx *log.LogContext, ti *tag.TagInfo, references []distribution.Descriptor) (*tag.TagInfo, error) {
	var ml []distribution.Descriptor
	platforms := []string{}

	for _, ref := range references {
		var refOS, refArch, refVariant string
		if ref.Platform != nil {
			refOS = ref.Platform.OS
			refArch = ref.Platform.Architecture
			refVariant = ref.Platform.Variant
		}
		platform1 := options.PlatformKey(refOS, refArch, refVariant)
		platforms = append(platforms, platform1)
		logCtx.Tracef("Found %s", platform1)
		if !opts.WantsPlatform(refOS, refArch, refVariant) {
			logCtx.Tracef("Ignoring referenced manifest %v because platform %s does not match any of: %s",
				ref.Digest,
				platform1,
				strings.Join(opts.Platforms(), ","))
			continue
		}
		ml = append(ml, ref)
	}

	// We need at least one reference that matches requested platforms
	if len(ml) == 0 {
		logCtx.Debugf("Manifest list did not contain any usable reference. Platforms requested: (%s), platforms included: (%s)",
			strings.Join(opts.Platforms(), ","), strings.Join(platforms, ","))
		return nil, nil
	}

	// For some strategies, we do not need to fetch metadata for further
	// processing.
	if !opts.WantsMetadata() {
		return ti, nil
	}

	// Loop through all referenced manifests to get their metadata. We only
	// consider manifests for platforms we are interested in.
	for _, ref := range ml {
		logCtx.Tracef("Inspecting metadata of reference: %v", ref.Digest)

		man, err := client.ManifestForDigest(ref.Digest)
		if err != nil {
			return nil, fmt.Errorf("could not fetch manifest %v: %v", ref.Digest, err)
		}

		cti, err := client.TagMetadata(man, opts)
		if err != nil {
			return nil, fmt.Errorf("could not fetch metadata for manifest %v: %v", ref.Digest, err)
		}

		// We save the timestamp of the most recent pushed manifest for any
		// given reference, if the metadata for the tag was correctly
		// retrieved. This is important for the latest update strategy to
		// be able to handle multi-arch images. The latest strategy will
		// consider the most recent reference from an image index.
		if cti != nil {
			if cti.CreatedAt.After(ti.CreatedAt) {
				ti.CreatedAt = cti.CreatedAt
			}
		} else {
			logCtx.Warnf("returned metadata for manifest %v is nil, this should not happen.", ref.Digest)
			continue
		}
	}

	return ti, nil
}

// Implementation of ping method to initialize the challenge list
// Without this, tokenHandler and AuthorizationHandler won't work
func ping(manager challenge.Manager, endpoint *RegistryEndpoint, versionHeader string) ([]auth.APIVersion, error) {
	httpc := &http.Client{Transport: endpoint.GetTransport()}
	url := endpoint.RegistryAPI + "/v2/"
	resp, err := httpc.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	// Let's consider only HTTP 200 and 401 valid responses for the initial request
	if resp.StatusCode != 200 && resp.StatusCode != 401 {
		return nil, fmt.Errorf("endpoint %s does not seem to be a valid v2 Docker Registry API (received HTTP code %d for GET %s)", endpoint.RegistryAPI, resp.StatusCode, url)
	}

	if err := manager.AddResponse(resp); err != nil {
		return nil, err
	}

	return auth.APIVersions(resp, versionHeader), err
}
