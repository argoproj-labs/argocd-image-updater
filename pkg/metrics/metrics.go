package metrics

import (
	"sync"

	crmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Metrics struct {
	Endpoint     *EndpointMetrics
	Applications *ApplicationMetrics
	Clients      *ClientMetrics
}

var (
	defaultMetrics  *Metrics
	initMetricsOnce sync.Once
)

// EndpointMetrics stores metrics for registry endpoints
type EndpointMetrics struct {
	requestsTotal  *prometheus.CounterVec
	requestsFailed *prometheus.CounterVec
}

// ApplicationMetrics stores metrics for applications
type ApplicationMetrics struct {
	ApplicationsTotal        *prometheus.GaugeVec
	imagesWatchedTotal       *prometheus.GaugeVec
	imagesUpdatedTotal       *prometheus.CounterVec
	imagesUpdatedErrorsTotal *prometheus.CounterVec
}

// ClientMetrics stores metrics for K8s client
type ClientMetrics struct {
	kubeAPIRequestsTotal       prometheus.Counter
	kubeAPIRequestsErrorsTotal prometheus.Counter
}

// NewEndpointMetrics returns a new endpoint metrics object
func NewEndpointMetrics() *EndpointMetrics {
	metrics := &EndpointMetrics{}

	metrics.requestsTotal = promauto.With(crmetrics.Registry).NewCounterVec(prometheus.CounterOpts{
		Name: "argocd_image_updater_registry_requests_total",
		Help: "The total number of requests to this endpoint",
	}, []string{"registry"})
	metrics.requestsFailed = promauto.With(crmetrics.Registry).NewCounterVec(prometheus.CounterOpts{
		Name: "argocd_image_updater_registry_requests_failed_total",
		Help: "The number of failed requests to this endpoint",
	}, []string{"registry"})

	return metrics
}

// NewApplicationsMetrics returns a new application metrics object
func NewApplicationsMetrics() *ApplicationMetrics {
	metrics := &ApplicationMetrics{}

	metrics.ApplicationsTotal = promauto.With(crmetrics.Registry).NewGaugeVec(prometheus.GaugeOpts{
		Name: "argocd_image_updater_applications_watched_total",
		Help: "The total number of applications watched by Argo CD Image Updater CR",
	}, []string{"image_updater_cr_name", "image_updater_cr_namespace"})

	metrics.imagesWatchedTotal = promauto.With(crmetrics.Registry).NewGaugeVec(prometheus.GaugeOpts{
		Name: "argocd_image_updater_images_watched_total",
		Help: "Number of images watched by Argo CD Image Updater",
	}, []string{"application"})

	metrics.imagesUpdatedTotal = promauto.With(crmetrics.Registry).NewCounterVec(prometheus.CounterOpts{
		Name: "argocd_image_updater_images_updated_total",
		Help: "Number of images updates by Argo CD Image Updater",
	}, []string{"application"})

	metrics.imagesUpdatedErrorsTotal = promauto.With(crmetrics.Registry).NewCounterVec(prometheus.CounterOpts{
		Name: "argocd_image_updater_images_errors_total",
		Help: "Number of errors reported by Argo CD Image Updater",
	}, []string{"application"})

	return metrics
}

// NewClientMetrics returns a new client metrics object
func NewClientMetrics() *ClientMetrics {
	metrics := &ClientMetrics{}

	metrics.kubeAPIRequestsTotal = promauto.With(crmetrics.Registry).NewCounter(prometheus.CounterOpts{
		Name: "argocd_image_updater_k8s_api_requests_total",
		Help: "The total number of K8S API requests performed by the Argo CD Image Updater",
	})

	metrics.kubeAPIRequestsErrorsTotal = promauto.With(crmetrics.Registry).NewCounter(prometheus.CounterOpts{
		Name: "argocd_image_updater_k8s_api_errors_total",
		Help: "The total number of K8S API requests resulting in error",
	})

	return metrics
}

func NewMetrics() *Metrics {
	return &Metrics{
		Endpoint:     NewEndpointMetrics(),
		Applications: NewApplicationsMetrics(),
		Clients:      NewClientMetrics(),
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

// IncreaseRequest increases the request counter of EndpointMetrics object
func (epm *EndpointMetrics) IncreaseRequest(registryURL string, isFailed bool) {
	epm.requestsTotal.WithLabelValues(registryURL).Inc()
	if isFailed {
		epm.requestsFailed.WithLabelValues(registryURL).Inc()
	}
}

// SetNumberOfApplications sets the total number of currently watched applications
func (apm *ApplicationMetrics) SetNumberOfApplications(name, namespace string, num int) {
	apm.ApplicationsTotal.WithLabelValues(name, namespace).Set(float64(num))
}

// RemoveNumberOfApplications removes the application gauge for a given CR
func (apm *ApplicationMetrics) RemoveNumberOfApplications(name, namespace string) {
	apm.ApplicationsTotal.DeleteLabelValues(name, namespace)
}

// ResetApplicationsTotal resets the total number of applications to handle deletion
func (apm *ApplicationMetrics) ResetApplicationsTotal() {
	apm.ApplicationsTotal.Reset()
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

// RemoveNumberOfImages removes the images gauge for a given CR
func (apm *ApplicationMetrics) RemoveNumberOfImages(application string) {
	apm.imagesWatchedTotal.DeleteLabelValues(application)
	apm.imagesUpdatedTotal.DeleteLabelValues(application)
	apm.imagesUpdatedErrorsTotal.DeleteLabelValues(application)
}

// IncreaseK8sClientRequest increases the number of K8s API requests
func (cpm *ClientMetrics) IncreaseK8sClientRequest(by int) {
	cpm.kubeAPIRequestsTotal.Add(float64(by))
}

// IncreaseK8sClientError increases the number of failed K8s API requests
func (cpm *ClientMetrics) IncreaseK8sClientError(by int) {
	cpm.kubeAPIRequestsErrorsTotal.Add(float64(by))
}

// InitMetrics initializes the global metrics objects
func InitMetrics() {
	initMetricsOnce.Do(func() {
		defaultMetrics = NewMetrics()
	})
}
