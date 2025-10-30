package metrics

import (
    "io"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
    "time"

    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
    "github.com/stretchr/testify/assert"
)

func TestMetricsInitialization(t *testing.T) {
	t.Run("NewEndpointMetrics", func(t *testing.T) {
		prometheus.DefaultRegisterer = prometheus.NewRegistry()
		epm := NewEndpointMetrics()
		assert.NotNil(t, epm)
		assert.NotNil(t, epm.requestsTotal)
		assert.NotNil(t, epm.requestsFailed)

		prometheus.DefaultRegisterer = nil
		epm = NewEndpointMetrics()
		assert.NotNil(t, epm)
		assert.NotNil(t, epm.requestsTotal)
		assert.NotNil(t, epm.requestsFailed)
	})

	t.Run("NewClientMetrics", func(t *testing.T) {
		prometheus.DefaultRegisterer = prometheus.NewRegistry()
		cpm := NewClientMetrics()
		assert.NotNil(t, cpm)
		assert.NotNil(t, cpm.argoCDRequestsTotal)
		assert.NotNil(t, cpm.argoCDRequestsErrorsTotal)
		assert.NotNil(t, cpm.kubeAPIRequestsTotal)
		assert.NotNil(t, cpm.kubeAPIRequestsErrorsTotal)

		prometheus.DefaultRegisterer = nil
		cpm = NewClientMetrics()
		assert.NotNil(t, cpm)
		assert.NotNil(t, cpm.argoCDRequestsTotal)
		assert.NotNil(t, cpm.argoCDRequestsErrorsTotal)
		assert.NotNil(t, cpm.kubeAPIRequestsTotal)
		assert.NotNil(t, cpm.kubeAPIRequestsErrorsTotal)
	})

	t.Run("NewApplicationsMetrics", func(t *testing.T) {
		apm := NewApplicationsMetrics()
		assert.NotNil(t, apm)
		assert.NotNil(t, apm.applicationsTotal)
		assert.NotNil(t, apm.imagesWatchedTotal)
		assert.NotNil(t, apm.imagesUpdatedTotal)
		assert.NotNil(t, apm.imagesUpdatedErrorsTotal)
	})
}

func TestMetricsOperations(t *testing.T) {
	InitMetrics()
	epm := Endpoint()
	epm.IncreaseRequest("/registry1", false)
	epm.IncreaseRequest("/registry1", true)

	cpm := Clients()
	cpm.IncreaseArgoCDClientRequest("server1", 1)
	cpm.IncreaseArgoCDClientError("server1", 2)
	cpm.IncreaseK8sClientRequest(3)
	cpm.IncreaseK8sClientError(4)

	apm := Applications()
	apm.IncreaseImageUpdate("app1", 1)
	apm.IncreaseUpdateErrors("app1", 2)
	apm.SetNumberOfApplications(3)
	apm.SetNumberOfImagesWatched("app1", 4)
}

func TestMetricWrappers_NoPanic(t *testing.T) {
    prometheus.DefaultRegisterer = prometheus.NewRegistry()
    InitMetrics()

    epm := Endpoint()
    epm.IncInFlight("reg")
    epm.DecInFlight("reg")
    epm.ObserveRequestDuration("reg", 5*time.Millisecond)
    epm.ObserveHTTPStatus("reg", 200)
    epm.IncreaseRetry("reg", "tags")
    epm.IncreaseErrorKind("reg", "timeout")

    apm := Applications()
    apm.ObserveAppUpdateDuration("app", 7*time.Millisecond)
    apm.SetLastAttempt("app", time.Now())
    apm.SetLastSuccess("app", time.Now())
    apm.ObserveCycleDuration(15 * time.Millisecond)
    apm.SetCycleLastEnd(time.Now())
    apm.IncreaseImagesConsidered("app", 2)
    apm.IncreaseImagesSkipped("app", 1)
    apm.SchedulerSkipped("cooldown", 1)

    sf := Singleflight()
    sf.IncreaseLeaders("tags")
    sf.IncreaseFollowers("tags")
}

func TestMetricsEndpoint_Serves(t *testing.T) {
    reg := prometheus.NewRegistry()
    prometheus.DefaultRegisterer = reg
    InitMetrics()
    mux := http.NewServeMux()
    mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
    srv := httptest.NewServer(mux)
    defer srv.Close()

    resp, err := http.Get(srv.URL + "/metrics")
    if err != nil {
        t.Fatalf("GET /metrics failed: %v", err)
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        t.Fatalf("unexpected status: %d", resp.StatusCode)
    }
    body, _ := io.ReadAll(resp.Body)
    // Check a couple of metric names are present
    b := string(body)
    if !strings.Contains(b, "argocd_image_updater_applications_watched_total") {
        t.Fatalf("expected applications_watched_total metric in scrape")
    }
}
