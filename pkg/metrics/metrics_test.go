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
}
