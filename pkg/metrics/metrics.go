package metrics

import (
    "fmt"
    "net/http"
    "time"

    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	Endpoint     *EndpointMetrics
	Applications *ApplicationMetrics
	Clients      *ClientMetrics
    Singleflight *SingleflightMetrics
}

var defaultMetrics *Metrics

// EndpointMetrics stores metrics for registry endpoints
type EndpointMetrics struct {
	requestsTotal  *prometheus.CounterVec
	requestsFailed *prometheus.CounterVec
    inFlight       *prometheus.GaugeVec
    requestDur     *prometheus.HistogramVec
    httpStatus     *prometheus.CounterVec
    errorKind      *prometheus.CounterVec
    retriesTotal   *prometheus.CounterVec
    jwtAuthRequests *prometheus.CounterVec
    jwtAuthErrors   *prometheus.CounterVec
    jwtAuthDur      *prometheus.HistogramVec
    jwtTokenTTL     *prometheus.HistogramVec
}

// ApplicationMetrics stores metrics for applications
type ApplicationMetrics struct {
	applicationsTotal        prometheus.Gauge
	imagesWatchedTotal       *prometheus.GaugeVec
	imagesUpdatedTotal       *prometheus.CounterVec
	imagesUpdatedErrorsTotal *prometheus.CounterVec
    appUpdateDuration        *prometheus.HistogramVec
    lastAttemptTs            *prometheus.GaugeVec
    lastSuccessTs            *prometheus.GaugeVec
    cycleDuration            prometheus.Histogram
    cycleLastEndTs           prometheus.Gauge
    imagesConsideredTotal    *prometheus.CounterVec
    imagesSkippedTotal       *prometheus.CounterVec
    schedulerSkippedTotal    *prometheus.CounterVec
}

// SingleflightMetrics captures dedup effectiveness
type SingleflightMetrics struct {
    leadersTotal   *prometheus.CounterVec
    followersTotal *prometheus.CounterVec
}

// ClientMetrics stores metrics for K8s and ArgoCD clients
type ClientMetrics struct {
	argoCDRequestsTotal        *prometheus.CounterVec
	argoCDRequestsErrorsTotal  *prometheus.CounterVec
	kubeAPIRequestsTotal       prometheus.Counter
	kubeAPIRequestsErrorsTotal prometheus.Counter
}

// StartMetricsServer starts a new HTTP server for metrics on given port
func StartMetricsServer(port int) chan error {
	errCh := make(chan error)
	go func() {
		sm := http.NewServeMux()
		sm.Handle("/metrics", promhttp.Handler())
		errCh <- http.ListenAndServe(fmt.Sprintf(":%d", port), sm)
	}()
	return errCh
}

// NewEndpointMetrics returns a new endpoint metrics object
func NewEndpointMetrics() *EndpointMetrics {
	metrics := &EndpointMetrics{}

	metrics.requestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "argocd_image_updater_registry_requests_total",
		Help: "The total number of requests to this endpoint",
	}, []string{"registry"})
	metrics.requestsFailed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "argocd_image_updater_registry_requests_failed_total",
		Help: "The number of failed requests to this endpoint",
	}, []string{"registry"})

    metrics.inFlight = promauto.NewGaugeVec(prometheus.GaugeOpts{
        Name: "argocd_image_updater_registry_in_flight_requests",
        Help: "Current number of in-flight registry requests",
    }, []string{"registry"})

    metrics.requestDur = promauto.NewHistogramVec(prometheus.HistogramOpts{
        Name:    "argocd_image_updater_registry_request_duration_seconds",
        Help:    "Registry request duration",
        Buckets: prometheus.DefBuckets,
    }, []string{"registry"})

    metrics.httpStatus = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "argocd_image_updater_registry_http_status_total",
        Help: "HTTP status codes returned by registry",
    }, []string{"registry", "code"})

    metrics.errorKind = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "argocd_image_updater_registry_errors_total",
        Help: "Categorized registry request errors",
    }, []string{"registry", "kind"})

    metrics.retriesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "argocd_image_updater_registry_request_retries_total",
        Help: "Number of retries performed for registry operations",
    }, []string{"registry", "op"})

    metrics.jwtAuthRequests = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "argocd_image_updater_registry_jwt_auth_requests_total",
        Help: "Number of JWT auth requests",
    }, []string{"registry", "service", "scope"})

    metrics.jwtAuthErrors = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "argocd_image_updater_registry_jwt_auth_errors_total",
        Help: "JWT auth errors by reason",
    }, []string{"registry", "service", "scope", "reason"})

    metrics.jwtAuthDur = promauto.NewHistogramVec(prometheus.HistogramOpts{
        Name:    "argocd_image_updater_registry_jwt_auth_duration_seconds",
        Help:    "JWT auth request duration",
        Buckets: prometheus.DefBuckets,
    }, []string{"registry", "service", "scope"})

    metrics.jwtTokenTTL = promauto.NewHistogramVec(prometheus.HistogramOpts{
        Name:    "argocd_image_updater_registry_jwt_token_ttl_seconds",
        Help:    "JWT token TTL as reported by registry",
        Buckets: prometheus.DefBuckets,
    }, []string{"registry", "service", "scope"})

	return metrics
}

// NewApplicationsMetrics returns a new application metrics object
func NewApplicationsMetrics() *ApplicationMetrics {
	metrics := &ApplicationMetrics{}

	metrics.applicationsTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "argocd_image_updater_applications_watched_total",
		Help: "The total number of applications watched by Argo CD Image Updater",
	})

	metrics.imagesWatchedTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "argocd_image_updater_images_watched_total",
		Help: "Number of images watched by Argo CD Image Updater",
	}, []string{"application"})

	metrics.imagesUpdatedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "argocd_image_updater_images_updated_total",
		Help: "Number of images updates by Argo CD Image Updater",
	}, []string{"application"})

	metrics.imagesUpdatedErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "argocd_image_updater_images_errors_total",
		Help: "Number of errors reported by Argo CD Image Updater",
	}, []string{"application"})

    metrics.appUpdateDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
        Name:    "argocd_image_updater_application_update_duration_seconds",
        Help:    "Time to process a single application",
        Buckets: prometheus.DefBuckets,
    }, []string{"application"})

    metrics.lastAttemptTs = promauto.NewGaugeVec(prometheus.GaugeOpts{
        Name: "argocd_image_updater_application_last_attempt_timestamp",
        Help: "Unix timestamp of the last attempt for an application",
    }, []string{"application"})

    metrics.lastSuccessTs = promauto.NewGaugeVec(prometheus.GaugeOpts{
        Name: "argocd_image_updater_application_last_success_timestamp",
        Help: "Unix timestamp of the last successful attempt for an application",
    }, []string{"application"})

    metrics.cycleDuration = promauto.NewHistogram(prometheus.HistogramOpts{
        Name:    "argocd_image_updater_update_cycle_duration_seconds",
        Help:    "Time to complete a full update cycle across applications",
        Buckets: prometheus.DefBuckets,
    })

    metrics.cycleLastEndTs = promauto.NewGauge(prometheus.GaugeOpts{
        Name: "argocd_image_updater_update_cycle_last_end_timestamp",
        Help: "Unix timestamp of the end of the most recent update cycle",
    })

    metrics.imagesConsideredTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "argocd_image_updater_images_considered_total",
        Help: "Images considered per application",
    }, []string{"application"})

    metrics.imagesSkippedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "argocd_image_updater_images_skipped_total",
        Help: "Images skipped per application",
    }, []string{"application"})

    metrics.schedulerSkippedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "argocd_image_updater_scheduler_skipped_total",
        Help: "Applications skipped by scheduler and reason",
    }, []string{"reason"})

	return metrics
}

// NewClientMetrics returns a new client metrics object
func NewClientMetrics() *ClientMetrics {
	metrics := &ClientMetrics{}

	metrics.argoCDRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "argocd_image_updater_argocd_api_requests_total",
		Help: "The total number of Argo CD API requests performed by the Argo CD Image Updater",
	}, []string{"argocd_server"})

	metrics.argoCDRequestsErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "argocd_image_updater_argocd_api_errors_total",
		Help: "The total number of Argo CD API requests resulting in error",
	}, []string{"argocd_server"})

	metrics.kubeAPIRequestsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "argocd_image_updater_k8s_api_requests_total",
		Help: "The total number of Argo CD API requests resulting in error",
	})

	metrics.kubeAPIRequestsErrorsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "argocd_image_updater_k8s_api_errors_total",
		Help: "The total number of Argo CD API requests resulting in error",
	})

	return metrics
}

// NewSingleflightMetrics returns new singleflight metrics
func NewSingleflightMetrics() *SingleflightMetrics {
    m := &SingleflightMetrics{}
    m.leadersTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "argocd_image_updater_singleflight_leaders_total",
        Help: "Number of leader executions per kind",
    }, []string{"kind"})
    m.followersTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "argocd_image_updater_singleflight_followers_total",
        Help: "Number of follower coalesced calls per kind",
    }, []string{"kind"})
    return m
}

func NewMetrics() *Metrics {
	return &Metrics{
		Endpoint:     NewEndpointMetrics(),
		Applications: NewApplicationsMetrics(),
        Clients:      NewClientMetrics(),
        Singleflight: NewSingleflightMetrics(),
	}
}

// Endpoint returns the global EndpointMetrics object
func Endpoint() *EndpointMetrics {
	if defaultMetrics == nil {
		return nil
	}
	return defaultMetrics.Endpoint
}

// Applications returns the global ApplicationMetrics object
func Applications() *ApplicationMetrics {
	if defaultMetrics == nil {
		return nil
	}
	return defaultMetrics.Applications
}

// Clients returns the global ClientMetrics object
func Clients() *ClientMetrics {
	if defaultMetrics == nil {
		return nil
	}
	return defaultMetrics.Clients
}

// Singleflight returns singleflight metrics
func Singleflight() *SingleflightMetrics {
    if defaultMetrics == nil {
        return nil
    }
    return defaultMetrics.Singleflight
}

// IncreaseRequest increases the request counter of EndpointMetrics object
func (epm *EndpointMetrics) IncreaseRequest(registryURL string, isFailed bool) {
	epm.requestsTotal.WithLabelValues(registryURL).Inc()
	if isFailed {
		epm.requestsFailed.WithLabelValues(registryURL).Inc()
	}
}

// IncInFlight increments in-flight gauge for a registry
func (epm *EndpointMetrics) IncInFlight(registryURL string) {
    epm.inFlight.WithLabelValues(registryURL).Inc()
}

// DecInFlight decrements in-flight gauge for a registry
func (epm *EndpointMetrics) DecInFlight(registryURL string) {
    epm.inFlight.WithLabelValues(registryURL).Dec()
}

// ObserveRequestDuration observes a request duration for a registry
func (epm *EndpointMetrics) ObserveRequestDuration(registryURL string, d time.Duration) {
    epm.requestDur.WithLabelValues(registryURL).Observe(d.Seconds())
}

// ObserveHTTPStatus increments per-status counters
func (epm *EndpointMetrics) ObserveHTTPStatus(registryURL string, code int) {
    epm.httpStatus.WithLabelValues(registryURL, fmt.Sprintf("%d", code)).Inc()
}

// IncreaseRetry increases retry counter for an operation
func (epm *EndpointMetrics) IncreaseRetry(registryURL, op string) {
    epm.retriesTotal.WithLabelValues(registryURL, op).Inc()
}

// IncreaseErrorKind categorizes and counts errors
func (epm *EndpointMetrics) IncreaseErrorKind(registryURL, kind string) {
    epm.errorKind.WithLabelValues(registryURL, kind).Inc()
}

// IncreaseJWTAuthRequest increments JWT auth request counter
func (epm *EndpointMetrics) IncreaseJWTAuthRequest(registryURL, service, scope string) {
    epm.jwtAuthRequests.WithLabelValues(registryURL, service, scope).Inc()
}

// IncreaseJWTAuthError increments JWT auth error counter with a reason
func (epm *EndpointMetrics) IncreaseJWTAuthError(registryURL, service, scope, reason string) {
    epm.jwtAuthErrors.WithLabelValues(registryURL, service, scope, reason).Inc()
}

// ObserveJWTAuthDuration records JWT auth request duration
func (epm *EndpointMetrics) ObserveJWTAuthDuration(registryURL, service, scope string, d time.Duration) {
    epm.jwtAuthDur.WithLabelValues(registryURL, service, scope).Observe(d.Seconds())
}

// ObserveJWTTokenTTL records reported JWT token TTL in seconds
func (epm *EndpointMetrics) ObserveJWTTokenTTL(registryURL, service, scope string, ttlSeconds float64) {
    epm.jwtTokenTTL.WithLabelValues(registryURL, service, scope).Observe(ttlSeconds)
}

// SetNumberOfApplications sets the total number of currently watched applications
func (apm *ApplicationMetrics) SetNumberOfApplications(num int) {
	apm.applicationsTotal.Set(float64(num))
}

// SetNumberOfImagesWatched sets the total number of currently watched images for given application
func (apm *ApplicationMetrics) SetNumberOfImagesWatched(application string, num int) {
	apm.imagesWatchedTotal.WithLabelValues(application).Set(float64(num))
}

// IncreaseImageUpdate increases the number of image updates for given application
func (apm *ApplicationMetrics) IncreaseImageUpdate(application string, by int) {
	apm.imagesUpdatedTotal.WithLabelValues(application).Add(float64(by))
}

// IncreaseUpdateErrors increases the number of errors for given application occurred during update process
func (apm *ApplicationMetrics) IncreaseUpdateErrors(application string, by int) {
	apm.imagesUpdatedErrorsTotal.WithLabelValues(application).Add(float64(by))
}

// ObserveAppUpdateDuration observes duration for processing an application
func (apm *ApplicationMetrics) ObserveAppUpdateDuration(application string, d time.Duration) {
    apm.appUpdateDuration.WithLabelValues(application).Observe(d.Seconds())
}

// SetLastAttempt records the last attempt timestamp for an application
func (apm *ApplicationMetrics) SetLastAttempt(application string, ts time.Time) {
    apm.lastAttemptTs.WithLabelValues(application).Set(float64(ts.Unix()))
}

// SetLastSuccess records the last success timestamp for an application
func (apm *ApplicationMetrics) SetLastSuccess(application string, ts time.Time) {
    apm.lastSuccessTs.WithLabelValues(application).Set(float64(ts.Unix()))
}

// ObserveCycleDuration observes the duration of a full update cycle
func (apm *ApplicationMetrics) ObserveCycleDuration(d time.Duration) {
    apm.cycleDuration.Observe(d.Seconds())
}

// SetCycleLastEnd sets the timestamp of the end of the most recent cycle
func (apm *ApplicationMetrics) SetCycleLastEnd(ts time.Time) {
    apm.cycleLastEndTs.Set(float64(ts.Unix()))
}

// IncreaseImagesConsidered increases considered counter per app
func (apm *ApplicationMetrics) IncreaseImagesConsidered(application string, by int) {
    apm.imagesConsideredTotal.WithLabelValues(application).Add(float64(by))
}

// IncreaseImagesSkipped increases skipped counter per app
func (apm *ApplicationMetrics) IncreaseImagesSkipped(application string, by int) {
    apm.imagesSkippedTotal.WithLabelValues(application).Add(float64(by))
}

// SchedulerSkipped increments skip reasons
func (apm *ApplicationMetrics) SchedulerSkipped(reason string, by int) {
    apm.schedulerSkippedTotal.WithLabelValues(reason).Add(float64(by))
}

// DeleteAppMetrics removes all per-application metric series for the given app.
// This helps garbage-collect stale series when applications are deleted.
func (apm *ApplicationMetrics) DeleteAppMetrics(application string) {
    // Safe best-effort deletes; ignore return values
    apm.imagesWatchedTotal.DeleteLabelValues(application)
    apm.imagesUpdatedTotal.DeleteLabelValues(application)
    apm.imagesUpdatedErrorsTotal.DeleteLabelValues(application)
    apm.appUpdateDuration.DeleteLabelValues(application)
    apm.lastAttemptTs.DeleteLabelValues(application)
    apm.lastSuccessTs.DeleteLabelValues(application)
    apm.imagesConsideredTotal.DeleteLabelValues(application)
    apm.imagesSkippedTotal.DeleteLabelValues(application)
}

// Singleflight helpers
func (sfm *SingleflightMetrics) IncreaseLeaders(kind string) {
    sfm.leadersTotal.WithLabelValues(kind).Inc()
}

func (sfm *SingleflightMetrics) IncreaseFollowers(kind string) {
    sfm.followersTotal.WithLabelValues(kind).Inc()
}

// IncreaseArgoCDClientRequest increases the number of Argo CD API requests for given server
func (cpm *ClientMetrics) IncreaseArgoCDClientRequest(server string, by int) {
	cpm.argoCDRequestsTotal.WithLabelValues(server).Add(float64(by))
}

// IncreaseArgoCDClientError increases the number of failed Argo CD API requests for given server
func (cpm *ClientMetrics) IncreaseArgoCDClientError(server string, by int) {
	cpm.argoCDRequestsErrorsTotal.WithLabelValues(server).Add(float64(by))
}

// IncreaseK8sClientRequest increases the number of K8s API requests
func (cpm *ClientMetrics) IncreaseK8sClientRequest(by int) {
	cpm.kubeAPIRequestsTotal.Add(float64(by))
}

// IncreaseK8sClientRequest increases the number of failed K8s API requests
func (cpm *ClientMetrics) IncreaseK8sClientError(by int) {
	cpm.kubeAPIRequestsErrorsTotal.Add(float64(by))
}

// InitMetrics initializes the global metrics objects
func InitMetrics() {
	defaultMetrics = NewMetrics()
}
