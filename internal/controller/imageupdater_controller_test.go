package controller

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	crmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/argoproj-labs/argocd-image-updater/api/v1alpha1"
	"github.com/argoproj-labs/argocd-image-updater/pkg/metrics"
)

func TestReconcile_DeleteFinalizer_RemovesMetrics(t *testing.T) {
	// Initialize a new prometheus registry for the test.
	crmetrics.Registry = prometheus.NewRegistry()
	metrics.InitMetrics()

	crName := "test-iu"
	crNamespace := "test-ns"

	apm := metrics.Applications()
	// Pre-set a metric for our test CR
	apm.SetNumberOfApplications(crName, crNamespace, 1)
	if apm != nil {
		assert.Equal(t, 1, testutil.CollectAndCount(apm.ApplicationsTotal))
	}

	// Create a fake ImageUpdater resource that is marked for deletion
	imageUpdater := &v1alpha1.ImageUpdater{
		ObjectMeta: metav1.ObjectMeta{
			Name:              crName,
			Namespace:         crNamespace,
			DeletionTimestamp: &metav1.Time{Time: time.Now()},
			Finalizers:        []string{ResourcesFinalizerName},
		},
	}

	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(imageUpdater).Build()

	warmedCh := make(chan struct{})
	close(warmedCh)

	reconciler := &ImageUpdaterReconciler{
		Client:      fakeClient,
		Scheme:      scheme,
		Config:      &ImageUpdaterConfig{},
		CacheWarmed: warmedCh,
	}

	req := ctrl.Request{
		NamespacedName: client.ObjectKey{
			Name:      crName,
			Namespace: crNamespace,
		},
	}

	_, err := reconciler.Reconcile(context.Background(), req)
	assert.NoError(t, err)

	// The metric should be gone after reconciliation of the deleted resource
	assert.Equal(t, 0, testutil.CollectAndCount(apm.ApplicationsTotal))
}
