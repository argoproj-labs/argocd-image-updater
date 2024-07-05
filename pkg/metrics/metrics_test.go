package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
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
