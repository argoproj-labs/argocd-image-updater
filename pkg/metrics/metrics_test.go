package metrics

import (
	"testing"

	crmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func TestMetricsInitialization(t *testing.T) {
	t.Run("NewEndpointMetrics", func(t *testing.T) {
		crmetrics.Registry = prometheus.NewRegistry()
		prometheus.DefaultRegisterer = prometheus.NewRegistry()
		epm := NewEndpointMetrics()
		assert.NotNil(t, epm)
		assert.NotNil(t, epm.requestsTotal)
		assert.NotNil(t, epm.requestsFailed)

		crmetrics.Registry = prometheus.NewRegistry()
		prometheus.DefaultRegisterer = nil
		epm = NewEndpointMetrics()
		assert.NotNil(t, epm)
		assert.NotNil(t, epm.requestsTotal)
		assert.NotNil(t, epm.requestsFailed)
	})

	t.Run("NewClientMetrics", func(t *testing.T) {
		crmetrics.Registry = prometheus.NewRegistry()
		prometheus.DefaultRegisterer = prometheus.NewRegistry()
		cpm := NewClientMetrics()
		assert.NotNil(t, cpm)
		assert.NotNil(t, cpm.kubeAPIRequestsTotal)
		assert.NotNil(t, cpm.kubeAPIRequestsErrorsTotal)

		crmetrics.Registry = prometheus.NewRegistry()
		prometheus.DefaultRegisterer = nil
		cpm = NewClientMetrics()
		assert.NotNil(t, cpm)
		assert.NotNil(t, cpm.kubeAPIRequestsTotal)
		assert.NotNil(t, cpm.kubeAPIRequestsErrorsTotal)
	})

	t.Run("NewImageUpdaterCRMetrics", func(t *testing.T) {
		crmetrics.Registry = prometheus.NewRegistry()
		apm := NewImageUpdaterCRMetrics()
		assert.NotNil(t, apm)
		assert.NotNil(t, apm.ApplicationsTotal)
		assert.NotNil(t, apm.ImagesWatchedTotal)
		assert.NotNil(t, apm.ImagesUpdatedTotal)
		assert.NotNil(t, apm.ImagesUpdatedErrorsTotal)
	})

	t.Run("InitMetrics is idempotent", func(t *testing.T) {
		// Replace the default registry with a new one for this test.
		crmetrics.Registry = prometheus.NewRegistry()
		prometheus.DefaultRegisterer = crmetrics.Registry

		// We cannot reset initMetricsOnce, so we test for idempotency.
		// defaultMetrics may or may not be nil at this point, depending on test execution order.
		InitMetrics()
		firstInstance := defaultMetrics
		assert.NotNil(t, firstInstance)

		// Calling it again should have no effect.
		InitMetrics()
		secondInstance := defaultMetrics

		// The key is that the instance must be the same.
		assert.Same(t, firstInstance, secondInstance)
	})
}

func TestMetricsOperations(t *testing.T) {
	crmetrics.Registry = prometheus.NewRegistry()

	InitMetrics()
	epm := Endpoint()
	epm.IncreaseRequest("/registry1", false)
	epm.IncreaseRequest("/registry1", true)

	cpm := Clients()
	cpm.IncreaseK8sClientRequest(3)
	cpm.IncreaseK8sClientError(4)

	apm := ImageUpdaterCR()
	apm.IncreaseImageUpdate("cr1", "ns1", 1)
	apm.IncreaseUpdateErrors("cr1", "ns1", 2)
	apm.SetNumberOfApplications("cr1", "ns1", 3)
	apm.SetNumberOfImagesWatched("cr1", "ns1", 4)
}

func TestImageUpdaterCRMetricsRemovals(t *testing.T) {
	t.Run("RemoveNumberOfApplications", func(t *testing.T) {
		crmetrics.Registry = prometheus.NewRegistry()
		apm := NewImageUpdaterCRMetrics()
		apm.SetNumberOfApplications("cr1", "ns1", 5)
		apm.SetNumberOfApplications("cr2", "ns2", 10)
		assert.Equal(t, 2, testutil.CollectAndCount(apm.ApplicationsTotal))

		apm.RemoveNumberOfApplications("cr1", "ns1")
		assert.Equal(t, 1, testutil.CollectAndCount(apm.ApplicationsTotal))
		assert.Equal(t, float64(10), testutil.ToFloat64(apm.ApplicationsTotal.WithLabelValues("cr2", "ns2")))
	})

	t.Run("ResetApplicationsTotal", func(t *testing.T) {
		crmetrics.Registry = prometheus.NewRegistry()
		apm := NewImageUpdaterCRMetrics()
		apm.SetNumberOfApplications("cr1", "ns1", 5)
		apm.SetNumberOfApplications("cr2", "ns2", 10)
		assert.Equal(t, 2, testutil.CollectAndCount(apm.ApplicationsTotal))

		apm.ResetApplicationsTotal()
		assert.Equal(t, 0, testutil.CollectAndCount(apm.ApplicationsTotal))
	})

	t.Run("RemoveImageUpdaterMetrics", func(t *testing.T) {
		crmetrics.Registry = prometheus.NewRegistry()
		apm := NewImageUpdaterCRMetrics()

		apm.SetNumberOfApplications("cr1", "ns1", 1)
		apm.SetNumberOfImagesWatched("cr1", "ns1", 10)
		apm.IncreaseImageUpdate("cr1", "ns1", 5)
		apm.IncreaseUpdateErrors("cr1", "ns1", 2)

		apm.SetNumberOfApplications("cr2", "ns2", 1)
		apm.SetNumberOfImagesWatched("cr2", "ns2", 20)
		apm.IncreaseImageUpdate("cr2", "ns2", 6)
		apm.IncreaseUpdateErrors("cr2", "ns2", 3)

		assert.Equal(t, 2, testutil.CollectAndCount(apm.ImagesWatchedTotal))
		assert.Equal(t, 2, testutil.CollectAndCount(apm.ImagesUpdatedTotal))
		assert.Equal(t, 2, testutil.CollectAndCount(apm.ImagesUpdatedErrorsTotal))

		apm.RemoveImageUpdaterMetrics("cr1", "ns1")

		assert.Equal(t, 1, testutil.CollectAndCount(apm.ImagesWatchedTotal))
		assert.Equal(t, float64(20), testutil.ToFloat64(apm.ImagesWatchedTotal.WithLabelValues("cr2", "ns2")))

		assert.Equal(t, 1, testutil.CollectAndCount(apm.ImagesUpdatedTotal))
		assert.Equal(t, float64(6), testutil.ToFloat64(apm.ImagesUpdatedTotal.WithLabelValues("cr2", "ns2")))

		assert.Equal(t, 1, testutil.CollectAndCount(apm.ImagesUpdatedErrorsTotal))
		assert.Equal(t, float64(3), testutil.ToFloat64(apm.ImagesUpdatedErrorsTotal.WithLabelValues("cr2", "ns2")))
	})
}
