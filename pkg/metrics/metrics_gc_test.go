package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

func TestDeleteAppMetrics(t *testing.T) {
	// Use a fresh registry per test to avoid cross-test pollution
	reg := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = reg
	defer func() { prometheus.DefaultRegisterer = prometheus.NewRegistry() }()

	apm := NewApplicationsMetrics()

	appName := "test-app-1"
	now := time.Now()

	// Set various metrics for the app
	apm.SetNumberOfImagesWatched(appName, 5)
	apm.IncreaseImageUpdate(appName, 3)
	apm.IncreaseUpdateErrors(appName, 1)
	apm.ObserveAppUpdateDuration(appName, 100*time.Millisecond)
	apm.SetLastAttempt(appName, now)
	apm.SetLastSuccess(appName, now)
	apm.IncreaseImagesConsidered(appName, 10)
	apm.IncreaseImagesSkipped(appName, 2)

	// Verify metrics exist by checking the registry
	metrics, err := reg.Gather()
	assert.NoError(t, err)

	// Count metrics with our app label
	var foundMetrics int
	for _, mf := range metrics {
		for _, m := range mf.Metric {
			for _, label := range m.Label {
				if label.GetName() == "application" && label.GetValue() == appName {
					foundMetrics++
					break
				}
			}
		}
	}
	assert.Greater(t, foundMetrics, 0, "expected metrics to exist before deletion")

	// Delete metrics for the app
	apm.DeleteAppMetrics(appName)

	// Verify metrics are gone
	metricsAfter, err := reg.Gather()
	assert.NoError(t, err)

	// Count metrics with our app label (should be 0)
	var foundMetricsAfter int
	for _, mf := range metricsAfter {
		for _, m := range mf.Metric {
			for _, label := range m.Label {
				if label.GetName() == "application" && label.GetValue() == appName {
					foundMetricsAfter++
					break
				}
			}
		}
	}
	assert.Equal(t, 0, foundMetricsAfter, "expected no metrics after deletion, found %d", foundMetricsAfter)
}

func TestDeleteAppMetrics_NonExistentApp(t *testing.T) {
	// Use a fresh registry per test
	reg := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = reg
	defer func() { prometheus.DefaultRegisterer = prometheus.NewRegistry() }()

	apm := NewApplicationsMetrics()

	// Deleting metrics for a non-existent app should not panic
	assert.NotPanics(t, func() {
		apm.DeleteAppMetrics("non-existent-app")
	})
}

func TestDeleteAppMetrics_MultipleApps(t *testing.T) {
	// Use a fresh registry per test
	reg := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = reg
	defer func() { prometheus.DefaultRegisterer = prometheus.NewRegistry() }()

	apm := NewApplicationsMetrics()

	app1 := "app-1"
	app2 := "app-2"
	app3 := "app-3"

	// Set metrics for all three apps
	for _, app := range []string{app1, app2, app3} {
		apm.SetNumberOfImagesWatched(app, 1)
		apm.SetLastSuccess(app, time.Now())
	}

	// Delete metrics for app2 only
	apm.DeleteAppMetrics(app2)

	// Verify app1 and app3 still exist, app2 is gone
	metrics, err := reg.Gather()
	assert.NoError(t, err)

	app1Exists := false
	app2Exists := false
	app3Exists := false

	for _, mf := range metrics {
		for _, m := range mf.Metric {
			for _, label := range m.Label {
				if label.GetName() == "application" {
					appName := label.GetValue()
					switch appName {
					case app1:
						app1Exists = true
					case app2:
						app2Exists = true
					case app3:
						app3Exists = true
					}
				}
			}
		}
	}

	assert.True(t, app1Exists, "app1 metrics should still exist")
	assert.False(t, app2Exists, "app2 metrics should be deleted")
	assert.True(t, app3Exists, "app3 metrics should still exist")
}
