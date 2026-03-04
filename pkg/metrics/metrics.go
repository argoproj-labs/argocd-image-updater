package metrics

import (
	"sync"

	crmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Metrics struct {
	Endpoint       *EndpointMetrics
	ImageUpdaterCR *ImageUpdaterCRMetrics
	Clients        *ClientMetrics
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

// ImageUpdaterCRMetrics stores per–ImageUpdater-CR metrics (applications watched, images watched/updated/errors).
type ImageUpdaterCRMetrics struct {
	ApplicationsTotal        *prometheus.GaugeVec
	ImagesWatchedTotal       *prometheus.GaugeVec
	ImagesUpdatedTotal       *prometheus.CounterVec
	ImagesUpdatedErrorsTotal *prometheus.CounterVec
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

// NewImageUpdaterCRMetrics returns a new ImageUpdater CR metrics object.
func NewImageUpdaterCRMetrics() *ImageUpdaterCRMetrics {
	metrics := &ImageUpdaterCRMetrics{}

	metrics.ApplicationsTotal = promauto.With(crmetrics.Registry).NewGaugeVec(prometheus.GaugeOpts{
		Name: "argocd_image_updater_applications_watched_total",
		Help: "The total number of applications watched by Argo CD Image Updater CR",
	}, []string{"image_updater_cr_name", "image_updater_cr_namespace"})

	metrics.ImagesWatchedTotal = promauto.With(crmetrics.Registry).NewGaugeVec(prometheus.GaugeOpts{
		Name: "argocd_image_updater_images_watched_total",
		Help: "Number of images watched by Argo CD Image Updater CR",
	}, []string{"image_updater_cr_name", "image_updater_cr_namespace"})

	metrics.ImagesUpdatedTotal = promauto.With(crmetrics.Registry).NewCounterVec(prometheus.CounterOpts{
		Name: "argocd_image_updater_images_updated_total",
		Help: "Number of images updated by Argo CD Image Updater CR",
	}, []string{"image_updater_cr_name", "image_updater_cr_namespace"})

	metrics.ImagesUpdatedErrorsTotal = promauto.With(crmetrics.Registry).NewCounterVec(prometheus.CounterOpts{
		Name: "argocd_image_updater_images_errors_total",
		Help: "Number of errors reported by Argo CD Image Updater CR",
	}, []string{"image_updater_cr_name", "image_updater_cr_namespace"})

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
		Endpoint:       NewEndpointMetrics(),
		ImageUpdaterCR: NewImageUpdaterCRMetrics(),
		Clients:        NewClientMetrics(),
	}
}

// Endpoint returns the global EndpointMetrics object
func Endpoint() *EndpointMetrics {
	if defaultMetrics == nil {
		return nil
	}
	return defaultMetrics.Endpoint
}

// ImageUpdaterCR returns the global ImageUpdater CR metrics object.
func ImageUpdaterCR() *ImageUpdaterCRMetrics {
	if defaultMetrics == nil {
		return nil
	}
	return defaultMetrics.ImageUpdaterCR
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

// SetNumberOfApplications sets the total number of currently watched applications for the given ImageUpdater CR.
func (iucm *ImageUpdaterCRMetrics) SetNumberOfApplications(name, namespace string, num int) {
	iucm.ApplicationsTotal.WithLabelValues(name, namespace).Set(float64(num))
}

// SetNumberOfImagesWatched sets the total number of currently watched images for the given ImageUpdater CR.
func (iucm *ImageUpdaterCRMetrics) SetNumberOfImagesWatched(name, namespace string, num int) {
	iucm.ImagesWatchedTotal.WithLabelValues(name, namespace).Set(float64(num))
}

// IncreaseImageUpdate increases the number of image updates for the given ImageUpdater CR.
func (iucm *ImageUpdaterCRMetrics) IncreaseImageUpdate(name, namespace string, by int) {
	iucm.ImagesUpdatedTotal.WithLabelValues(name, namespace).Add(float64(by))
}

// IncreaseUpdateErrors increases the number of errors for the given ImageUpdater CR during update.
func (iucm *ImageUpdaterCRMetrics) IncreaseUpdateErrors(name, namespace string, by int) {
	iucm.ImagesUpdatedErrorsTotal.WithLabelValues(name, namespace).Add(float64(by))
}

// RemoveImageUpdaterMetrics removes all metrics for a given ImageUpdater CR (e.g. on CR deletion).
func (iucm *ImageUpdaterCRMetrics) RemoveImageUpdaterMetrics(name, namespace string) {
	iucm.ApplicationsTotal.DeleteLabelValues(name, namespace)
	iucm.ImagesWatchedTotal.DeleteLabelValues(name, namespace)
	iucm.ImagesUpdatedTotal.DeleteLabelValues(name, namespace)
	iucm.ImagesUpdatedErrorsTotal.DeleteLabelValues(name, namespace)
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
