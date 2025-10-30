package registry

import (
    "net/http"
    "net/url"
    "os"
    "sync"
    "testing"

    sf "golang.org/x/sync/singleflight"

    "github.com/stretchr/testify/require"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"

    "github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/registry/mocks"
    "github.com/argoproj-labs/argocd-image-updater/pkg/metrics"
    "github.com/prometheus/client_golang/prometheus"
)

// helper to build a /jwt/auth request with given service/scope
func newJWTReq(t *testing.T, service, scope string) *http.Request {
    t.Helper()
    u, err := url.Parse("https://gitlab.example.com/jwt/auth")
    require.NoError(t, err)
    q := u.Query()
    q.Set("service", service)
    q.Set("scope", scope)
    u.RawQuery = q.Encode()
    req, err := http.NewRequest(http.MethodGet, u.String(), nil)
    require.NoError(t, err)
    return req
}

// reset globals to avoid cross-test interference and duplicate collectors
func resetMetricsAndFlights() {
    reg := prometheus.NewRegistry()
    prometheus.DefaultRegisterer = reg
    prometheus.DefaultGatherer = reg
    metrics.InitMetrics()
    jwtAuthSingleflight = sf.Group{}
    tagsInFlight = sync.Map{}
    manifestInFlight = sync.Map{}
}

func TestJWT_Singleflight_DeduplicatesSameScope(t *testing.T) {
    resetMetricsAndFlights()
    mockRT := new(mocks.RoundTripper)
    // First (and only) underlying call returns 200
    mockRT.On("RoundTrip", mock.AnythingOfType("*http.Request")).Return(&http.Response{StatusCode: http.StatusOK}, nil).Once()

    e := &RegistryEndpoint{RegistryAPI: "https://registry.example.com"}
    j := &jwtObservingTransport{endpoint: e, base: mockRT, singleflight: &sf.Group{}}

    // Two concurrent requests for the same (service,scope)
    req1 := newJWTReq(t, "container_registry", "repository:org/repo:pull")
    req2 := newJWTReq(t, "container_registry", "repository:org/repo:pull")

    var wg sync.WaitGroup
    wg.Add(2)
    go func(){ defer wg.Done(); _, _ = j.RoundTrip(req1) }()
    go func(){ defer wg.Done(); _, _ = j.RoundTrip(req2) }()
    wg.Wait()

    mockRT.AssertNumberOfCalls(t, "RoundTrip", 1)
}

func TestJWT_Singleflight_AllowsDifferentScopes(t *testing.T) {
    resetMetricsAndFlights()
    mockRT := new(mocks.RoundTripper)
    mockRT.On("RoundTrip", mock.AnythingOfType("*http.Request")).Return(&http.Response{StatusCode: http.StatusOK}, nil).Twice()

    e := &RegistryEndpoint{RegistryAPI: "https://registry.example.com"}
    j := &jwtObservingTransport{endpoint: e, base: mockRT, singleflight: &sf.Group{}}

    req1 := newJWTReq(t, "container_registry", "repository:org/repoA:pull")
    req2 := newJWTReq(t, "container_registry", "repository:org/repoB:pull")

    var wg sync.WaitGroup
    wg.Add(2)
    go func(){ defer wg.Done(); _, _ = j.RoundTrip(req1) }()
    go func(){ defer wg.Done(); _, _ = j.RoundTrip(req2) }()
    wg.Wait()

    mockRT.AssertNumberOfCalls(t, "RoundTrip", 2)
}

func TestJWT_Retry_Backoff_AttemptsHonored(t *testing.T) {
    resetMetricsAndFlights()
    // Make the underlying transport fail N-1 times, then succeed
    attempts := 4
    os.Setenv("REGISTRY_JWT_ATTEMPTS", "4")
    t.Cleanup(func(){ os.Unsetenv("REGISTRY_JWT_ATTEMPTS") })

    callCount := 0
    mockRT := new(mocks.RoundTripper)
    mockRT.Mock.Test(t)
    mockRT.On("RoundTrip", mock.AnythingOfType("*http.Request")).Return(func(_ *http.Request) *http.Response {
        callCount++
        if callCount < attempts {
            return nil
        }
        return &http.Response{StatusCode: http.StatusOK}
    }, func(_ *http.Request) error {
        if callCount < attempts { return assert.AnError }
        return nil
    }).Times(attempts)

    e := &RegistryEndpoint{RegistryAPI: "https://registry.example.com"}
    j := &jwtObservingTransport{endpoint: e, base: mockRT, singleflight: &sf.Group{}}
    req := newJWTReq(t, "container_registry", "repository:org/repo:pull")
    resp, err := j.RoundTrip(req)
    require.NoError(t, err)
    require.Equal(t, http.StatusOK, resp.StatusCode)
    assert.Equal(t, attempts, callCount)
}


