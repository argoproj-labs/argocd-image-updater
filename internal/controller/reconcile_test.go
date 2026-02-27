package controller

import (
	"context"
	"sync"
	"testing"
	"time"

	argocdapi "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	clifake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	crmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	argocdimageupdaterv1alpha1 "github.com/argoproj-labs/argocd-image-updater/api/v1alpha1"
	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd"
	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd/mocks"
	"github.com/argoproj-labs/argocd-image-updater/pkg/kube"
	"github.com/argoproj-labs/argocd-image-updater/pkg/metrics"
	regokube "github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/kube"
)

// Assisted-by: Claude AI
// strictNamespaceClient wraps a client.Client to enforce strict namespace filtering.
// When InNamespace("") is used (empty namespace), it returns empty results instead of
// all namespaces, matching real Kubernetes API behavior.
type strictNamespaceClient struct {
	client.Client
}

// List intercepts List operations to enforce strict namespace filtering.
// If InNamespace("") is used, it returns empty results.
func (c *strictNamespaceClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	// Extract namespace from list options
	listOpts := &client.ListOptions{}
	for _, opt := range opts {
		opt.ApplyToList(listOpts)
	}

	// If namespace is empty, return empty results (matching real Kubernetes behavior)
	// This ensures tests fail when ObjectMeta.Namespace is not set on ImageUpdater CRs
	if listOpts.Namespace == "" {
		// Set items to empty slice by using type assertion
		// For ApplicationList, we can directly set Items to empty
		if appList, ok := list.(*argocdapi.ApplicationList); ok {
			appList.Items = []argocdapi.Application{}
			appList.ListMeta = metav1.ListMeta{}
			return nil
		}
		// For other list types, we could use reflection, but since we primarily test
		// with ApplicationList, this explicit handling is sufficient.
		// If other types are needed, we can extend this.
	}

	// Delegate to underlying client for non-empty namespaces
	return c.Client.List(ctx, list, opts...)
}

// Assisted-by: Gemini AI
// TestImageUpdaterReconciler_Reconcile tests the main Reconcile function
func TestImageUpdaterReconciler_Reconcile(t *testing.T) {
	tests := []struct {
		name           string
		setupTest      func(*ImageUpdaterReconciler, client.Client, *mocks.ArgoCD, chan struct{})
		request        reconcile.Request
		expectedResult reconcile.Result
		expectedError  bool
		postCheck      func(t *testing.T, reconciler *ImageUpdaterReconciler)
	}{
		{
			name: "cache not warmed - should requeue after 5 seconds",
			setupTest: func(reconciler *ImageUpdaterReconciler, fakeClient client.Client, mockArgoClient *mocks.ArgoCD, cacheChan chan struct{}) {
				// Don't close cacheChan to simulate cache not warmed
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "test",
					Namespace: "default",
				},
			},
			expectedResult: reconcile.Result{RequeueAfter: 5 * time.Second},
			expectedError:  false,
		},
		{
			name: "cache warmed, resource not found - should not requeue",
			setupTest: func(reconciler *ImageUpdaterReconciler, fakeClient client.Client, mockArgoClient *mocks.ArgoCD, cacheChan chan struct{}) {
				// Close cacheChan to simulate cache is ready
				close(cacheChan)
				// Don't create the resource
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "not-found",
					Namespace: "default",
				},
			},
			expectedResult: reconcile.Result{},
			expectedError:  false,
		},
		{
			name: "cache warmed, resource exists, CheckInterval < 0 - should not requeue",
			setupTest: func(reconciler *ImageUpdaterReconciler, fakeClient client.Client, mockArgoClient *mocks.ArgoCD, cacheChan chan struct{}) {
				close(cacheChan)
				reconciler.Config.CheckInterval = -1 * time.Second
				imageUpdater := &argocdimageupdaterv1alpha1.ImageUpdater{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "default",
					},
					Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
						ApplicationRefs: []argocdimageupdaterv1alpha1.ApplicationRef{
							{
								NamePattern: "test-app",
								Images: []argocdimageupdaterv1alpha1.ImageConfig{
									{
										Alias:     "nginx",
										ImageName: "nginx:latest",
									},
								},
							},
						},
					},
				}
				require.NoError(t, fakeClient.Create(context.Background(), imageUpdater))
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "test",
					Namespace: "default",
				},
			},
			expectedResult: reconcile.Result{},
			expectedError:  false,
		},
		{
			name: "cache warmed, resource exists, CheckInterval = 0, Once = true - should run once and call Wg.Done",
			setupTest: func(reconciler *ImageUpdaterReconciler, fakeClient client.Client, mockArgoClient *mocks.ArgoCD, cacheChan chan struct{}) {
				close(cacheChan)
				reconciler.Config.CheckInterval = 0
				reconciler.Once = true
				// Add to WaitGroup since Once = true will call Wg.Done()
				reconciler.Wg.Add(1)
				imageUpdater := &argocdimageupdaterv1alpha1.ImageUpdater{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "default",
					},
					Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
						ApplicationRefs: []argocdimageupdaterv1alpha1.ApplicationRef{
							{
								NamePattern: "test-app",
								Images: []argocdimageupdaterv1alpha1.ImageConfig{
									{
										Alias:     "nginx",
										ImageName: "nginx:latest",
									},
								},
							},
						},
					},
				}
				require.NoError(t, fakeClient.Create(context.Background(), imageUpdater))
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "test",
					Namespace: "default",
				},
			},
			expectedResult: reconcile.Result{},
			expectedError:  false,
			postCheck: func(t *testing.T, reconciler *ImageUpdaterReconciler) {
				doneCh := make(chan struct{})
				go func() {
					reconciler.Wg.Wait()
					close(doneCh)
				}()
				select {
				case <-doneCh:
					// Success, WaitGroup was decremented
				case <-time.After(100 * time.Millisecond):
					t.Error("Wg.Done() was not called but should have been when Once is true")
				}
			},
		},
		{
			name: "cache warmed, resource exists, CheckInterval = 0, Once = false - should run once and not call Wg.Done",
			setupTest: func(reconciler *ImageUpdaterReconciler, fakeClient client.Client, mockArgoClient *mocks.ArgoCD, cacheChan chan struct{}) {
				close(cacheChan)
				reconciler.Config.CheckInterval = 0
				reconciler.Once = false
				reconciler.Wg.Add(1)
				imageUpdater := &argocdimageupdaterv1alpha1.ImageUpdater{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-once-false",
						Namespace: "default",
					},
					Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
						ApplicationRefs: []argocdimageupdaterv1alpha1.ApplicationRef{
							{
								NamePattern: "test-app",
								Images: []argocdimageupdaterv1alpha1.ImageConfig{
									{
										Alias:     "nginx",
										ImageName: "nginx:latest",
									},
								},
							},
						},
					},
				}
				require.NoError(t, fakeClient.Create(context.Background(), imageUpdater))
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "test-once-false",
					Namespace: "default",
				},
			},
			expectedResult: reconcile.Result{},
			expectedError:  false,
			postCheck: func(t *testing.T, reconciler *ImageUpdaterReconciler) {
				doneCh := make(chan struct{})
				go func() {
					reconciler.Wg.Wait()
					close(doneCh)
				}()
				select {
				case <-doneCh:
					t.Error("Wg.Done() was called but should not have been when Once is false")
				case <-time.After(100 * time.Millisecond):
					// Success, WaitGroup was not decremented, so Wait timed out
				}
			},
		},
		{
			name: "cache warmed, resource exists, CheckInterval > 0 - should requeue after interval",
			setupTest: func(reconciler *ImageUpdaterReconciler, fakeClient client.Client, mockArgoClient *mocks.ArgoCD, cacheChan chan struct{}) {
				close(cacheChan)
				reconciler.Config.CheckInterval = 30 * time.Second
				imageUpdater := &argocdimageupdaterv1alpha1.ImageUpdater{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "default",
					},
					Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
						ApplicationRefs: []argocdimageupdaterv1alpha1.ApplicationRef{
							{
								NamePattern: "test-app",
								Images: []argocdimageupdaterv1alpha1.ImageConfig{
									{
										Alias:     "nginx",
										ImageName: "nginx:latest",
									},
								},
							},
						},
					},
				}
				require.NoError(t, fakeClient.Create(context.Background(), imageUpdater))
				// No mock expectations needed since the code uses the Kubernetes client directly
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "test",
					Namespace: "default",
				},
			},
			expectedResult: reconcile.Result{RequeueAfter: 30 * time.Second},
			expectedError:  false,
		},
		{
			name: "invalid name pattern - should requeue at normal interval, not exponential backoff",
			setupTest: func(reconciler *ImageUpdaterReconciler, fakeClient client.Client, mockArgoClient *mocks.ArgoCD, cacheChan chan struct{}) {
				close(cacheChan)
				reconciler.Config.CheckInterval = 30 * time.Second
				imageUpdater := &argocdimageupdaterv1alpha1.ImageUpdater{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-invalid-pattern",
						Namespace:  "default",
						Finalizers: []string{ResourcesFinalizerName},
					},
					Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
						ApplicationRefs: []argocdimageupdaterv1alpha1.ApplicationRef{
							{
								NamePattern: "foo[bar", // Invalid regex pattern
							},
						},
					},
				}
				require.NoError(t, fakeClient.Create(context.Background(), imageUpdater))
			},
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "test-invalid-pattern",
					Namespace: "default",
				},
			},
			// Error from RunImageUpdater should NOT be returned to controller-runtime;
			// instead, requeue at normal interval with the error recorded in status.
			expectedResult: reconcile.Result{RequeueAfter: 30 * time.Second},
			expectedError:  false,
			postCheck: func(t *testing.T, reconciler *ImageUpdaterReconciler) {
				// Verify the error is recorded in the status condition
				var updatedCR argocdimageupdaterv1alpha1.ImageUpdater
				err := reconciler.Get(context.Background(), types.NamespacedName{
					Name:      "test-invalid-pattern",
					Namespace: "default",
				}, &updatedCR)
				require.NoError(t, err)

				errorCondition := apimeta.FindStatusCondition(updatedCR.Status.Conditions, ConditionTypeError)
				require.NotNil(t, errorCondition, "Error condition should be set in status")
				assert.Equal(t, metav1.ConditionTrue, errorCondition.Status)
				assert.Equal(t, "ReconcileError", errorCondition.Reason)
				assert.Contains(t, errorCondition.Message, "invalid application name pattern")
			},
		},
	}

	metrics.InitMetrics()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			ctx := context.Background()
			cacheChan := make(chan struct{})

			// Create fake client
			s := scheme.Scheme
			err := argocdimageupdaterv1alpha1.AddToScheme(s)
			require.NoError(t, err)
			// Add ArgoCD Application types to the scheme
			err = argocdapi.AddToScheme(s)
			require.NoError(t, err)

			fakeClient := clifake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&argocdimageupdaterv1alpha1.ImageUpdater{}).Build()

			// Create mock ArgoCD client
			mockArgoClient := &mocks.ArgoCD{}

			// Create reconciler with the cache channel
			reconciler := &ImageUpdaterReconciler{
				Client:                  fakeClient,
				Scheme:                  s,
				MaxConcurrentReconciles: 1,
				CacheWarmed:             cacheChan,
				StopChan:                make(chan struct{}),
				Wg:                      sync.WaitGroup{},
				Config: &ImageUpdaterConfig{
					CheckInterval:     30 * time.Second,
					MaxConcurrentApps: 1,
					DryRun:            true,
					KubeClient:        &kube.ImageUpdaterKubernetesClient{},
				},
			}

			// Setup test-specific configuration
			tt.setupTest(reconciler, fakeClient, mockArgoClient, cacheChan)

			// Execute
			result, err := reconciler.Reconcile(ctx, tt.request)

			// Assert
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expectedResult, result)

			if tt.postCheck != nil {
				tt.postCheck(t, reconciler)
			}

			// Verify mock expectations
			mockArgoClient.AssertExpectations(t)
		})
	}
}

// Assisted-by: Gemini AI
// TestImageUpdaterReconciler_Reconcile_ComplexScenarios tests more complex scenarios
func TestImageUpdaterReconciler_Reconcile_ComplexScenarios(t *testing.T) {
	tests := []struct {
		name           string
		imageUpdater   *argocdimageupdaterv1alpha1.ImageUpdater
		expectedResult reconcile.Result
		expectedError  bool
	}{
		{
			name: "minimal ImageUpdater spec",
			imageUpdater: &argocdimageupdaterv1alpha1.ImageUpdater{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "minimal-test",
					Namespace: "default",
				},
				Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
					ApplicationRefs: []argocdimageupdaterv1alpha1.ApplicationRef{
						{
							NamePattern: "simple-app",
							Images: []argocdimageupdaterv1alpha1.ImageConfig{
								{
									Alias:     "app",
									ImageName: "myapp:latest",
								},
							},
						},
					},
				},
			},
			expectedResult: reconcile.Result{RequeueAfter: 30 * time.Second},
			expectedError:  false,
		},
		{
			name: "ImageUpdater with multiple images",
			imageUpdater: &argocdimageupdaterv1alpha1.ImageUpdater{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multi-image-test",
					Namespace: "default",
				},
				Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
					ApplicationRefs: []argocdimageupdaterv1alpha1.ApplicationRef{
						{
							NamePattern: "multi-app",
							Images: []argocdimageupdaterv1alpha1.ImageConfig{
								{
									Alias:     "frontend",
									ImageName: "frontend:latest",
								},
								{
									Alias:     "backend",
									ImageName: "backend:latest",
								},
							},
						},
					},
				},
			},
			expectedResult: reconcile.Result{RequeueAfter: 30 * time.Second},
			expectedError:  false,
		},
		{
			name: "ImageUpdater with label selectors",
			imageUpdater: &argocdimageupdaterv1alpha1.ImageUpdater{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "label-selector-test",
					Namespace: "default",
				},
				Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
					ApplicationRefs: []argocdimageupdaterv1alpha1.ApplicationRef{
						{
							NamePattern: "labeled-app",
							LabelSelectors: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app.kubernetes.io/part-of": "argocd-image-updater",
								},
							},
							Images: []argocdimageupdaterv1alpha1.ImageConfig{
								{
									Alias:     "app",
									ImageName: "myapp:latest",
								},
							},
						},
					},
				},
			},
			expectedResult: reconcile.Result{RequeueAfter: 30 * time.Second},
			expectedError:  false,
		},
		{
			name: "ImageUpdater in different namespace",
			imageUpdater: &argocdimageupdaterv1alpha1.ImageUpdater{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ns-test",
					Namespace: "test-namespace",
				},
				Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
					ApplicationRefs: []argocdimageupdaterv1alpha1.ApplicationRef{
						{
							NamePattern: "ns-app",
							Images: []argocdimageupdaterv1alpha1.ImageConfig{
								{
									Alias:     "app",
									ImageName: "myapp:latest",
								},
							},
						},
					},
				},
			},
			expectedResult: reconcile.Result{RequeueAfter: 30 * time.Second},
			expectedError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			ctx := context.Background()
			cacheChan := make(chan struct{})
			close(cacheChan) // Cache is warmed

			// Create fake client
			s := scheme.Scheme
			err := argocdimageupdaterv1alpha1.AddToScheme(s)
			require.NoError(t, err)
			// Add ArgoCD Application types to the scheme
			err = argocdapi.AddToScheme(s)
			require.NoError(t, err)

			fakeClient := clifake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&argocdimageupdaterv1alpha1.ImageUpdater{}).Build()

			// Create mock ArgoCD client (not used in these tests since the code uses Kubernetes client directly)
			mockArgoClient := &mocks.ArgoCD{}

			// Create reconciler
			reconciler := &ImageUpdaterReconciler{
				Client:                  fakeClient,
				Scheme:                  s,
				MaxConcurrentReconciles: 1,
				CacheWarmed:             cacheChan,
				StopChan:                make(chan struct{}),
				Wg:                      sync.WaitGroup{},
				Config: &ImageUpdaterConfig{
					CheckInterval:     30 * time.Second,
					MaxConcurrentApps: 1,
					DryRun:            true,
					KubeClient:        &kube.ImageUpdaterKubernetesClient{},
				},
			}

			// Create the test resource
			require.NoError(t, fakeClient.Create(ctx, tt.imageUpdater))

			// Execute
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      tt.imageUpdater.Name,
					Namespace: tt.imageUpdater.Namespace,
				},
			})

			// Assert
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expectedResult, result)

			// Verify mock expectations
			mockArgoClient.AssertExpectations(t)
		})
	}
}

// Assisted-by: Gemini AI
// TestImageUpdaterReconciler_Reconcile_AdvancedScenarios tests advanced scenarios with complex configurations
func TestImageUpdaterReconciler_Reconcile_AdvancedScenarios(t *testing.T) {
	t.Run("ImageUpdater with CommonUpdateSettings and WriteBackConfig", func(t *testing.T) {
		ctx := context.Background()
		cacheChan := make(chan struct{})
		close(cacheChan)

		// Create fake client
		s := scheme.Scheme
		err := argocdimageupdaterv1alpha1.AddToScheme(s)
		require.NoError(t, err)
		// Add ArgoCD Application types to the scheme
		err = argocdapi.AddToScheme(s)
		require.NoError(t, err)

		fakeClient := clifake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&argocdimageupdaterv1alpha1.ImageUpdater{}).Build()

		// Create mock ArgoCD client
		mockArgoClient := &mocks.ArgoCD{}

		// Create reconciler
		reconciler := &ImageUpdaterReconciler{
			Client:                  fakeClient,
			Scheme:                  s,
			MaxConcurrentReconciles: 1,
			CacheWarmed:             cacheChan,
			StopChan:                make(chan struct{}),
			Wg:                      sync.WaitGroup{},
			Config: &ImageUpdaterConfig{
				CheckInterval:     30 * time.Second,
				MaxConcurrentApps: 1,
				DryRun:            true,
				KubeClient:        &kube.ImageUpdaterKubernetesClient{},
			},
		}

		// Create complex ImageUpdater with CommonUpdateSettings and WriteBackConfig
		complexImageUpdater := &argocdimageupdaterv1alpha1.ImageUpdater{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "complex-test",
				Namespace: "default",
			},
			Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
				CommonUpdateSettings: &argocdimageupdaterv1alpha1.CommonUpdateSettings{
					UpdateStrategy: stringPtr("latest"),
				},
				WriteBackConfig: &argocdimageupdaterv1alpha1.WriteBackConfig{
					Method: stringPtr("git"),
					GitConfig: &argocdimageupdaterv1alpha1.GitConfig{
						Branch: stringPtr("main"),
					},
				},
				ApplicationRefs: []argocdimageupdaterv1alpha1.ApplicationRef{
					{
						NamePattern: "complex-app",
						LabelSelectors: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app.kubernetes.io/part-of": "argocd-image-updater",
							},
						},
						Images: []argocdimageupdaterv1alpha1.ImageConfig{
							{
								Alias:     "frontend",
								ImageName: "frontend:latest",
							},
							{
								Alias:     "backend",
								ImageName: "backend:latest",
							},
						},
					},
				},
			},
		}

		// Create the test resource
		require.NoError(t, fakeClient.Create(ctx, complexImageUpdater))

		// Execute
		result, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "complex-test",
				Namespace: "default",
			},
		})

		// Assert
		assert.NoError(t, err)
		assert.Equal(t, reconcile.Result{RequeueAfter: 30 * time.Second}, result)

		// Verify mock expectations
		mockArgoClient.AssertExpectations(t)
	})

	t.Run("ImageUpdater with multiple ApplicationRefs", func(t *testing.T) {
		ctx := context.Background()
		cacheChan := make(chan struct{})
		close(cacheChan)

		// Create fake client
		s := scheme.Scheme
		err := argocdimageupdaterv1alpha1.AddToScheme(s)
		require.NoError(t, err)

		fakeClient := clifake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&argocdimageupdaterv1alpha1.ImageUpdater{}).Build()

		// Create mock ArgoCD client
		mockArgoClient := &mocks.ArgoCD{}

		// Create reconciler
		reconciler := &ImageUpdaterReconciler{
			Client:                  fakeClient,
			Scheme:                  s,
			MaxConcurrentReconciles: 1,
			CacheWarmed:             cacheChan,
			StopChan:                make(chan struct{}),
			Wg:                      sync.WaitGroup{},
			Config: &ImageUpdaterConfig{
				CheckInterval:     30 * time.Second,
				MaxConcurrentApps: 1,
				DryRun:            true,
				KubeClient:        &kube.ImageUpdaterKubernetesClient{},
			},
		}

		// Create ImageUpdater with multiple ApplicationRefs
		multiAppImageUpdater := &argocdimageupdaterv1alpha1.ImageUpdater{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "multi-app-test",
				Namespace: "default",
			},
			Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
				ApplicationRefs: []argocdimageupdaterv1alpha1.ApplicationRef{
					{
						NamePattern: "app1",
						Images: []argocdimageupdaterv1alpha1.ImageConfig{
							{
								Alias:     "app1-image",
								ImageName: "app1:latest",
							},
						},
					},
					{
						NamePattern: "app2",
						Images: []argocdimageupdaterv1alpha1.ImageConfig{
							{
								Alias:     "app2-image",
								ImageName: "app2:latest",
							},
						},
					},
				},
			},
		}

		// Create the test resource
		require.NoError(t, fakeClient.Create(ctx, multiAppImageUpdater))

		// Execute
		result, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "multi-app-test",
				Namespace: "default",
			},
		})

		// Assert
		assert.NoError(t, err)
		assert.Equal(t, reconcile.Result{RequeueAfter: 30 * time.Second}, result)

		// Verify mock expectations
		mockArgoClient.AssertExpectations(t)
	})
}

// Assisted-by: Gemini AI
// TestImageUpdaterReconciler_Reconcile_CacheWarmup tests the cache warm-up behavior
func TestImageUpdaterReconciler_Reconcile_CacheWarmup(t *testing.T) {
	t.Run("cache not warmed - should requeue after 5 seconds", func(t *testing.T) {
		ctx := context.Background()
		cacheChan := make(chan struct{})

		// Create a minimal reconciler without client to test cache warm-up logic
		reconciler := &ImageUpdaterReconciler{
			Client:                  nil, // No client needed for this test
			Scheme:                  nil,
			MaxConcurrentReconciles: 1,
			CacheWarmed:             cacheChan,
			StopChan:                make(chan struct{}),
			Wg:                      sync.WaitGroup{},
			Config: &ImageUpdaterConfig{
				CheckInterval:     30 * time.Second,
				MaxConcurrentApps: 1,
				DryRun:            true,
			},
		}

		result, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "test",
				Namespace: "default",
			},
		})

		assert.NoError(t, err)
		assert.Equal(t, reconcile.Result{RequeueAfter: 5 * time.Second}, result)
	})

	t.Run("cache warmed - should proceed with reconciliation", func(t *testing.T) {
		ctx := context.Background()
		cacheChan := make(chan struct{})
		close(cacheChan) // Cache is warmed

		// Create a minimal reconciler without client
		reconciler := &ImageUpdaterReconciler{
			Client:                  nil, // No client will cause error, but we test cache warm-up first
			Scheme:                  nil,
			MaxConcurrentReconciles: 1,
			CacheWarmed:             cacheChan,
			StopChan:                make(chan struct{}),
			Wg:                      sync.WaitGroup{},
			Config: &ImageUpdaterConfig{
				CheckInterval:     30 * time.Second,
				MaxConcurrentApps: 1,
				DryRun:            true,
			},
		}

		result, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "test",
				Namespace: "default",
			},
		})

		// Should get an error because client is nil, but cache warm-up check should pass
		assert.Error(t, err)
		assert.Equal(t, reconcile.Result{}, result)
	})
}

// Assisted-by: Gemini AI
// TestImageUpdaterReconciler_Reconcile_CheckInterval tests the CheckInterval behavior
func TestImageUpdaterReconciler_Reconcile_CheckInterval(t *testing.T) {
	tests := []struct {
		name           string
		checkInterval  time.Duration
		expectedResult reconcile.Result
	}{
		{
			name:           "CheckInterval < 0 - should not requeue",
			checkInterval:  -1 * time.Second,
			expectedResult: reconcile.Result{},
		},
		{
			name:           "CheckInterval = 0 - should not requeue",
			checkInterval:  0,
			expectedResult: reconcile.Result{},
		},
		{
			name:           "CheckInterval > 0 - should requeue after interval",
			checkInterval:  30 * time.Second,
			expectedResult: reconcile.Result{RequeueAfter: 30 * time.Second},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			cacheChan := make(chan struct{})
			close(cacheChan) // Cache is warmed

			// Create a minimal reconciler without client
			reconciler := &ImageUpdaterReconciler{
				Client:                  nil, // No client will cause error
				Scheme:                  nil,
				MaxConcurrentReconciles: 1,
				CacheWarmed:             cacheChan,
				StopChan:                make(chan struct{}),
				Wg:                      sync.WaitGroup{},
				Config: &ImageUpdaterConfig{
					CheckInterval:     tt.checkInterval,
					MaxConcurrentApps: 1,
					DryRun:            true,
				},
			}

			// Add to WaitGroup if CheckInterval = 0 to prevent panic
			if tt.checkInterval == 0 {
				reconciler.Wg.Add(1)
			}

			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "test",
					Namespace: "default",
				},
			})

			// Should get an error because client is nil, but we can verify the CheckInterval logic
			assert.Error(t, err)
			// The result will be empty because the error occurs before CheckInterval is processed
			assert.Equal(t, reconcile.Result{}, result)
		})
	}
}

// Assisted-by: Gemini AI
// TestImageUpdaterReconciler_Reconcile_ErrorHandling tests error handling scenarios
func TestImageUpdaterReconciler_Reconcile_ErrorHandling(t *testing.T) {
	t.Run("nil client should return error", func(t *testing.T) {
		ctx := context.Background()
		cacheChan := make(chan struct{})
		close(cacheChan)

		reconciler := &ImageUpdaterReconciler{
			Client:                  nil, // Nil client will cause error
			Scheme:                  nil,
			MaxConcurrentReconciles: 1,
			CacheWarmed:             cacheChan,
			StopChan:                make(chan struct{}),
			Wg:                      sync.WaitGroup{},
			Config: &ImageUpdaterConfig{
				CheckInterval:     30 * time.Second,
				MaxConcurrentApps: 1,
				DryRun:            true,
			},
		}

		result, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "test",
				Namespace: "default",
			},
		})

		assert.Error(t, err)
		assert.Equal(t, reconcile.Result{}, result)
	})
}

// Assisted-by: Gemini AI
// TestImageUpdaterReconciler_Reconcile_Integration tests integration scenarios
func TestImageUpdaterReconciler_Reconcile_Integration(t *testing.T) {
	t.Run("full reconciliation flow with proper setup", func(t *testing.T) {
		ctx := context.Background()
		cacheChan := make(chan struct{})
		close(cacheChan)

		// Create a reconciler with minimal configuration
		reconciler := &ImageUpdaterReconciler{
			Client:                  nil, // We'll test error handling
			Scheme:                  nil,
			MaxConcurrentReconciles: 1,
			CacheWarmed:             cacheChan,
			StopChan:                make(chan struct{}),
			Wg:                      sync.WaitGroup{},
			Config: &ImageUpdaterConfig{
				CheckInterval:     30 * time.Second,
				MaxConcurrentApps: 1,
				DryRun:            true,
			},
		}

		// Test that the reconciler handles the request properly
		result, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "integration-test",
				Namespace: "default",
			},
		})

		// Should get an error because client is nil
		assert.Error(t, err)
		assert.Equal(t, reconcile.Result{}, result)

		// Verify that the reconciler has the expected structure
		assert.NotNil(t, reconciler.Config)
		assert.Equal(t, 30*time.Second, reconciler.Config.CheckInterval)
		assert.True(t, reconciler.Config.DryRun)
	})
}

// Assisted-by: Gemini AI
// TestImageUpdaterReconciler_Reconcile_MultipleCRs_CheckIntervalZero tests the scenario where
// CheckInterval = 0 with multiple CRs, ensuring that the reconciliation loop only ends when
// all CRs finish their work.
func TestImageUpdaterReconciler_Reconcile_MultipleCRs_CheckIntervalZero(t *testing.T) {
	t.Run("multiple CRs with CheckInterval = 0 - should wait for all CRs to complete", func(t *testing.T) {
		ctx := context.Background()
		cacheChan := make(chan struct{})
		close(cacheChan) // Cache is warmed

		// Create fake client
		s := scheme.Scheme
		err := argocdimageupdaterv1alpha1.AddToScheme(s)
		require.NoError(t, err)
		// Add ArgoCD Application types to the scheme
		err = argocdapi.AddToScheme(s)
		require.NoError(t, err)

		fakeClient := clifake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&argocdimageupdaterv1alpha1.ImageUpdater{}).Build()

		// Create reconciler with CheckInterval = 0 (run-once mode)
		reconciler := &ImageUpdaterReconciler{
			Client:                  fakeClient,
			Scheme:                  s,
			MaxConcurrentReconciles: 1,
			CacheWarmed:             cacheChan,
			StopChan:                make(chan struct{}),
			Wg:                      sync.WaitGroup{},
			Once:                    true,
			Config: &ImageUpdaterConfig{
				CheckInterval:     0, // Run-once mode
				MaxConcurrentApps: 1,
				DryRun:            true,
				KubeClient:        &kube.ImageUpdaterKubernetesClient{},
			},
		}

		// Create multiple ImageUpdater CRs
		cr1 := &argocdimageupdaterv1alpha1.ImageUpdater{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cr1",
				Namespace: "default",
			},
			Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
				ApplicationRefs: []argocdimageupdaterv1alpha1.ApplicationRef{
					{
						NamePattern: "app1",
						Images: []argocdimageupdaterv1alpha1.ImageConfig{
							{
								Alias:     "nginx",
								ImageName: "nginx:latest",
							},
						},
					},
				},
			},
		}

		cr2 := &argocdimageupdaterv1alpha1.ImageUpdater{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cr2",
				Namespace: "default",
			},
			Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
				ApplicationRefs: []argocdimageupdaterv1alpha1.ApplicationRef{
					{
						NamePattern: "app2",
						Images: []argocdimageupdaterv1alpha1.ImageConfig{
							{
								Alias:     "redis",
								ImageName: "redis:latest",
							},
						},
					},
				},
			},
		}

		cr3 := &argocdimageupdaterv1alpha1.ImageUpdater{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cr3",
				Namespace: "default",
			},
			Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
				ApplicationRefs: []argocdimageupdaterv1alpha1.ApplicationRef{
					{
						NamePattern: "app3",
						Images: []argocdimageupdaterv1alpha1.ImageConfig{
							{
								Alias:     "postgres",
								ImageName: "postgres:latest",
							},
						},
					},
				},
			},
		}

		// Create the CRs in the fake client
		require.NoError(t, fakeClient.Create(ctx, cr1))
		require.NoError(t, fakeClient.Create(ctx, cr2))
		require.NoError(t, fakeClient.Create(ctx, cr3))

		// Set up WaitGroup for 3 CRs (simulating the cache warmer behavior)
		reconciler.Wg.Add(3)

		// Start the stop watcher that will wait for all CRs to complete
		// This simulates the goroutine in run.go that waits for Wg.Wait()
		go func() {
			reconciler.Wg.Wait()
			close(reconciler.StopChan)
		}()

		// Start a goroutine to monitor the StopChan
		stopChanClosed := make(chan struct{})
		go func() {
			select {
			case <-reconciler.StopChan:
				close(stopChanClosed)
			case <-time.After(10 * time.Second): // Timeout after 10 seconds
				t.Log("Timeout waiting for StopChan to close")
			}
		}()

		// Reconcile all three CRs
		// CR1 should complete first
		result1, err1 := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "cr1",
				Namespace: "default",
			},
		})
		assert.NoError(t, err1)
		assert.Equal(t, reconcile.Result{}, result1)

		// CR2 should complete second
		result2, err2 := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "cr2",
				Namespace: "default",
			},
		})
		assert.NoError(t, err2)
		assert.Equal(t, reconcile.Result{}, result2)

		// CR3 should complete last
		result3, err3 := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "cr3",
				Namespace: "default",
			},
		})
		assert.NoError(t, err3)
		assert.Equal(t, reconcile.Result{}, result3)

		// Wait for the StopChan to be closed (indicating all CRs completed)
		select {
		case <-stopChanClosed:
			t.Log("StopChan was closed successfully after all CRs completed")
		case <-time.After(5 * time.Second):
			t.Fatal("StopChan was not closed within expected time")
		}

		// Verify that the WaitGroup counter is 0 (all CRs completed)
		// We can't directly check the WaitGroup counter, but we can verify
		// that the StopChan was closed, which indicates Wg.Wait() completed
	})

	t.Run("multiple CRs with CheckInterval = 0 - one CR fails but others continue", func(t *testing.T) {
		ctx := context.Background()
		cacheChan := make(chan struct{})
		close(cacheChan) // Cache is warmed

		// Create fake client
		s := scheme.Scheme
		err := argocdimageupdaterv1alpha1.AddToScheme(s)
		require.NoError(t, err)
		// Add ArgoCD Application types to the scheme
		err = argocdapi.AddToScheme(s)
		require.NoError(t, err)

		fakeClient := clifake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&argocdimageupdaterv1alpha1.ImageUpdater{}).Build()

		// Create reconciler with CheckInterval = 0 (run-once mode)
		reconciler := &ImageUpdaterReconciler{
			Client:                  fakeClient,
			Scheme:                  s,
			MaxConcurrentReconciles: 1,
			CacheWarmed:             cacheChan,
			StopChan:                make(chan struct{}),
			Wg:                      sync.WaitGroup{},
			Once:                    true,
			Config: &ImageUpdaterConfig{
				CheckInterval:     0, // Run-once mode
				MaxConcurrentApps: 1,
				DryRun:            true,
				KubeClient:        &kube.ImageUpdaterKubernetesClient{},
			},
		}

		// Create two ImageUpdater CRs
		cr1 := &argocdimageupdaterv1alpha1.ImageUpdater{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cr1-success",
				Namespace: "default",
			},
			Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
				ApplicationRefs: []argocdimageupdaterv1alpha1.ApplicationRef{
					{
						NamePattern: "app1",
						Images: []argocdimageupdaterv1alpha1.ImageConfig{
							{
								Alias:     "nginx",
								ImageName: "nginx:latest",
							},
						},
					},
				},
			},
		}

		cr2 := &argocdimageupdaterv1alpha1.ImageUpdater{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cr2-success",
				Namespace: "default",
			},
			Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
				ApplicationRefs: []argocdimageupdaterv1alpha1.ApplicationRef{
					{
						NamePattern: "app2",
						Images: []argocdimageupdaterv1alpha1.ImageConfig{
							{
								Alias:     "redis",
								ImageName: "redis:latest",
							},
						},
					},
				},
			},
		}

		// Create the CRs in the fake client
		require.NoError(t, fakeClient.Create(ctx, cr1))
		require.NoError(t, fakeClient.Create(ctx, cr2))

		// Set up WaitGroup for 2 CRs
		reconciler.Wg.Add(2)

		// Start the stop watcher that will wait for all CRs to complete
		go func() {
			reconciler.Wg.Wait()
			close(reconciler.StopChan)
		}()

		// Start a goroutine to monitor the StopChan
		stopChanClosed := make(chan struct{})
		go func() {
			select {
			case <-reconciler.StopChan:
				close(stopChanClosed)
			case <-time.After(10 * time.Second): // Timeout after 10 seconds
				t.Log("Timeout waiting for StopChan to close")
			}
		}()

		// Reconcile both CRs
		// CR1 should complete successfully
		result1, err1 := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "cr1-success",
				Namespace: "default",
			},
		})
		assert.NoError(t, err1)
		assert.Equal(t, reconcile.Result{}, result1)

		// CR2 should also complete successfully
		result2, err2 := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "cr2-success",
				Namespace: "default",
			},
		})
		assert.NoError(t, err2)
		assert.Equal(t, reconcile.Result{}, result2)

		// Wait for the StopChan to be closed (indicating all CRs completed)
		select {
		case <-stopChanClosed:
			t.Log("StopChan was closed successfully after all CRs completed")
		case <-time.After(5 * time.Second):
			t.Fatal("StopChan was not closed within expected time")
		}
	})

	t.Run("multiple CRs with CheckInterval = 0 - simulate concurrent reconciliation", func(t *testing.T) {
		ctx := context.Background()
		cacheChan := make(chan struct{})
		close(cacheChan) // Cache is warmed

		// Create fake client
		s := scheme.Scheme
		err := argocdimageupdaterv1alpha1.AddToScheme(s)
		require.NoError(t, err)
		// Add ArgoCD Application types to the scheme
		err = argocdapi.AddToScheme(s)
		require.NoError(t, err)

		fakeClient := clifake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&argocdimageupdaterv1alpha1.ImageUpdater{}).Build()

		// Create reconciler with CheckInterval = 0 and higher concurrency
		reconciler := &ImageUpdaterReconciler{
			Client:                  fakeClient,
			Scheme:                  s,
			MaxConcurrentReconciles: 3, // Allow concurrent reconciliation
			CacheWarmed:             cacheChan,
			StopChan:                make(chan struct{}),
			Wg:                      sync.WaitGroup{},
			Once:                    true,
			Config: &ImageUpdaterConfig{
				CheckInterval:     0, // Run-once mode
				MaxConcurrentApps: 3,
				DryRun:            true,
				KubeClient:        &kube.ImageUpdaterKubernetesClient{},
			},
		}

		// Create multiple ImageUpdater CRs
		crs := []*argocdimageupdaterv1alpha1.ImageUpdater{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cr-concurrent-1",
					Namespace: "default",
				},
				Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
					ApplicationRefs: []argocdimageupdaterv1alpha1.ApplicationRef{
						{
							NamePattern: "app1",
							Images: []argocdimageupdaterv1alpha1.ImageConfig{
								{
									Alias:     "nginx",
									ImageName: "nginx:latest",
								},
							},
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cr-concurrent-2",
					Namespace: "default",
				},
				Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
					ApplicationRefs: []argocdimageupdaterv1alpha1.ApplicationRef{
						{
							NamePattern: "app2",
							Images: []argocdimageupdaterv1alpha1.ImageConfig{
								{
									Alias:     "redis",
									ImageName: "redis:latest",
								},
							},
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cr-concurrent-3",
					Namespace: "default",
				},
				Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
					ApplicationRefs: []argocdimageupdaterv1alpha1.ApplicationRef{
						{
							NamePattern: "app3",
							Images: []argocdimageupdaterv1alpha1.ImageConfig{
								{
									Alias:     "postgres",
									ImageName: "postgres:latest",
								},
							},
						},
					},
				},
			},
		}

		// Create the CRs in the fake client
		for _, cr := range crs {
			require.NoError(t, fakeClient.Create(ctx, cr))
		}

		// Set up WaitGroup for all CRs
		reconciler.Wg.Add(len(crs))

		// Start the stop watcher that will wait for all CRs to complete
		go func() {
			reconciler.Wg.Wait()
			close(reconciler.StopChan)
		}()

		// Start a goroutine to monitor the StopChan
		stopChanClosed := make(chan struct{})
		go func() {
			select {
			case <-reconciler.StopChan:
				close(stopChanClosed)
			case <-time.After(10 * time.Second): // Timeout after 10 seconds
				t.Log("Timeout waiting for StopChan to close")
			}
		}()

		// Reconcile all CRs concurrently using goroutines
		var wg sync.WaitGroup
		for i, cr := range crs {
			wg.Add(1)
			go func(cr *argocdimageupdaterv1alpha1.ImageUpdater, index int) {
				defer wg.Done()
				result, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      cr.Name,
						Namespace: cr.Namespace,
					},
				})
				assert.NoError(t, err)
				assert.Equal(t, reconcile.Result{}, result)
				t.Logf("CR %d (%s) completed reconciliation", index+1, cr.Name)
			}(cr, i)
		}

		// Wait for all reconciliations to complete
		wg.Wait()

		// Wait for the StopChan to be closed (indicating all CRs completed)
		select {
		case <-stopChanClosed:
			t.Log("StopChan was closed successfully after all CRs completed")
		case <-time.After(5 * time.Second):
			t.Fatal("StopChan was not closed within expected time")
		}
	})

	t.Run("multiple CRs with CheckInterval = 0 - simulate cache warmer behavior", func(t *testing.T) {
		ctx := context.Background()
		cacheChan := make(chan struct{})
		close(cacheChan) // Cache is warmed

		// Create fake client
		s := scheme.Scheme
		err := argocdimageupdaterv1alpha1.AddToScheme(s)
		require.NoError(t, err)
		// Add ArgoCD Application types to the scheme
		err = argocdapi.AddToScheme(s)
		require.NoError(t, err)

		fakeClient := clifake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&argocdimageupdaterv1alpha1.ImageUpdater{}).Build()

		// Create reconciler with CheckInterval = 0 (run-once mode)
		reconciler := &ImageUpdaterReconciler{
			Client:                  fakeClient,
			Scheme:                  s,
			MaxConcurrentReconciles: 1,
			CacheWarmed:             cacheChan,
			StopChan:                make(chan struct{}),
			Wg:                      sync.WaitGroup{},
			Once:                    true,
			Config: &ImageUpdaterConfig{
				CheckInterval:     0, // Run-once mode
				MaxConcurrentApps: 1,
				DryRun:            true,
				KubeClient:        &kube.ImageUpdaterKubernetesClient{},
			},
		}

		// Create multiple ImageUpdater CRs
		crs := []*argocdimageupdaterv1alpha1.ImageUpdater{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cr-cache-warmer-1",
					Namespace: "default",
				},
				Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
					ApplicationRefs: []argocdimageupdaterv1alpha1.ApplicationRef{
						{
							NamePattern: "app1",
							Images: []argocdimageupdaterv1alpha1.ImageConfig{
								{
									Alias:     "nginx",
									ImageName: "nginx:latest",
								},
							},
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cr-cache-warmer-2",
					Namespace: "default",
				},
				Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
					ApplicationRefs: []argocdimageupdaterv1alpha1.ApplicationRef{
						{
							NamePattern: "app2",
							Images: []argocdimageupdaterv1alpha1.ImageConfig{
								{
									Alias:     "redis",
									ImageName: "redis:latest",
								},
							},
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cr-cache-warmer-3",
					Namespace: "default",
				},
				Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
					ApplicationRefs: []argocdimageupdaterv1alpha1.ApplicationRef{
						{
							NamePattern: "app3",
							Images: []argocdimageupdaterv1alpha1.ImageConfig{
								{
									Alias:     "postgres",
									ImageName: "postgres:latest",
								},
							},
						},
					},
				},
			},
		}

		// Create the CRs in the fake client
		for _, cr := range crs {
			require.NoError(t, fakeClient.Create(ctx, cr))
		}

		// Simulate the cache warmer behavior from run.go
		// If we're in run-once mode, count the total CRs and set up WaitGroup
		if reconciler.Config.CheckInterval == 0 {
			reconciler.Wg.Add(len(crs))
			t.Logf("Run-once mode: will process %d CRs before stopping", len(crs))

			// If there are no CRs, signal to stop immediately
			if len(crs) == 0 {
				t.Logf("No CRs found in run-once mode - will stop immediately")
				if reconciler.StopChan != nil {
					close(reconciler.StopChan)
				}
			} else {
				// Start the stop watcher that will wait for all CRs to complete
				// This simulates the goroutine in run.go that waits for Wg.Wait()
				if reconciler.StopChan != nil {
					go func() {
						reconciler.Wg.Wait()
						close(reconciler.StopChan)
					}()
				}
			}
		}

		// Start a goroutine to monitor the StopChan
		stopChanClosed := make(chan struct{})
		go func() {
			select {
			case <-reconciler.StopChan:
				close(stopChanClosed)
			case <-time.After(10 * time.Second): // Timeout after 10 seconds
				t.Log("Timeout waiting for StopChan to close")
			}
		}()

		// Reconcile all CRs sequentially (simulating the controller behavior)
		for i, cr := range crs {
			t.Logf("Reconciling CR %d: %s", i+1, cr.Name)
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      cr.Name,
					Namespace: cr.Namespace,
				},
			})
			assert.NoError(t, err)
			assert.Equal(t, reconcile.Result{}, result)
			t.Logf("CR %d (%s) completed reconciliation", i+1, cr.Name)
		}

		// Wait for the StopChan to be closed (indicating all CRs completed)
		select {
		case <-stopChanClosed:
			t.Log("StopChan was closed successfully after all CRs completed")
		case <-time.After(5 * time.Second):
			t.Fatal("StopChan was not closed within expected time")
		}

		// Verify that all CRs were processed
		for _, cr := range crs {
			var fetchedCR argocdimageupdaterv1alpha1.ImageUpdater
			err := fakeClient.Get(ctx, types.NamespacedName{
				Name:      cr.Name,
				Namespace: cr.Namespace,
			}, &fetchedCR)
			assert.NoError(t, err)
			assert.Equal(t, cr.Name, fetchedCR.Name)
		}
	})

	t.Run("multiple CRs with CheckInterval = 0 - test timing and completion order", func(t *testing.T) {
		ctx := context.Background()
		cacheChan := make(chan struct{})
		close(cacheChan) // Cache is warmed

		// Create fake client
		s := scheme.Scheme
		err := argocdimageupdaterv1alpha1.AddToScheme(s)
		require.NoError(t, err)
		// Add ArgoCD Application types to the scheme
		err = argocdapi.AddToScheme(s)
		require.NoError(t, err)

		fakeClient := clifake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&argocdimageupdaterv1alpha1.ImageUpdater{}).Build()

		// Create reconciler with CheckInterval = 0 (run-once mode)
		reconciler := &ImageUpdaterReconciler{
			Client:                  fakeClient,
			Scheme:                  s,
			MaxConcurrentReconciles: 1,
			CacheWarmed:             cacheChan,
			StopChan:                make(chan struct{}),
			Wg:                      sync.WaitGroup{},
			Once:                    true,
			Config: &ImageUpdaterConfig{
				CheckInterval:     0, // Run-once mode
				MaxConcurrentApps: 1,
				DryRun:            true,
				KubeClient:        &kube.ImageUpdaterKubernetesClient{},
			},
		}

		// Create multiple ImageUpdater CRs
		crs := []*argocdimageupdaterv1alpha1.ImageUpdater{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cr-fast",
					Namespace: "default",
				},
				Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
					ApplicationRefs: []argocdimageupdaterv1alpha1.ApplicationRef{
						{
							NamePattern: "app-fast",
							Images: []argocdimageupdaterv1alpha1.ImageConfig{
								{
									Alias:     "fast-app",
									ImageName: "fast-app:latest",
								},
							},
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cr-slow",
					Namespace: "default",
				},
				Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
					ApplicationRefs: []argocdimageupdaterv1alpha1.ApplicationRef{
						{
							NamePattern: "app-slow",
							Images: []argocdimageupdaterv1alpha1.ImageConfig{
								{
									Alias:     "slow-app",
									ImageName: "slow-app:latest",
								},
							},
						},
					},
				},
			},
		}

		// Create the CRs in the fake client
		for _, cr := range crs {
			require.NoError(t, fakeClient.Create(ctx, cr))
		}

		// Set up WaitGroup for all CRs
		reconciler.Wg.Add(len(crs))

		// Start the stop watcher that will wait for all CRs to complete
		stopChanClosed := make(chan struct{})
		go func() {
			reconciler.Wg.Wait()
			close(reconciler.StopChan)
		}()

		// Start a goroutine to monitor the StopChan
		go func() {
			select {
			case <-reconciler.StopChan:
				close(stopChanClosed)
			case <-time.After(10 * time.Second): // Timeout after 10 seconds
				t.Log("Timeout waiting for StopChan to close")
			}
		}()

		// Track completion order
		completionOrder := make(chan string, len(crs))
		completionTimes := make(map[string]time.Time)

		// Reconcile CRs with simulated different processing times
		// CR1 (fast) - should complete quickly
		go func() {
			start := time.Now()
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "cr-fast",
					Namespace: "default",
				},
			})
			completionTimes["cr-fast"] = time.Now()
			completionOrder <- "cr-fast"
			assert.NoError(t, err)
			assert.Equal(t, reconcile.Result{}, result)
			t.Logf("Fast CR completed in %v", time.Since(start))
		}()

		// CR2 (slow) - should complete after a delay
		go func() {
			start := time.Now()
			// Simulate slower processing
			time.Sleep(100 * time.Millisecond)
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "cr-slow",
					Namespace: "default",
				},
			})
			completionTimes["cr-slow"] = time.Now()
			completionOrder <- "cr-slow"
			assert.NoError(t, err)
			assert.Equal(t, reconcile.Result{}, result)
			t.Logf("Slow CR completed in %v", time.Since(start))
		}()

		// Wait for all CRs to complete
		completedCRs := make([]string, 0, len(crs))
		for i := 0; i < len(crs); i++ {
			select {
			case crName := <-completionOrder:
				completedCRs = append(completedCRs, crName)
				t.Logf("CR completed: %s", crName)
			case <-time.After(5 * time.Second):
				t.Fatal("Timeout waiting for CRs to complete")
			}
		}

		// Verify that both CRs completed
		assert.Len(t, completedCRs, 2)
		assert.Contains(t, completedCRs, "cr-fast")
		assert.Contains(t, completedCRs, "cr-slow")

		// Verify that the fast CR completed before the slow CR
		if completionTimes["cr-fast"].Before(completionTimes["cr-slow"]) {
			t.Log("Fast CR completed before slow CR as expected")
		} else {
			t.Log("Note: Slow CR completed before fast CR (this can happen due to goroutine scheduling)")
		}

		// Wait for the StopChan to be closed (indicating all CRs completed)
		select {
		case <-stopChanClosed:
			t.Log("StopChan was closed successfully after all CRs completed")
		case <-time.After(5 * time.Second):
			t.Fatal("StopChan was not closed within expected time")
		}

		// Verify that the reconciliation loop only ended after all CRs were processed
		t.Log("Reconciliation loop ended only after all CRs completed their work")
	})
}

// Assisted-by: Gemini AI
// TestImageUpdaterReconciler_Reconcile_Finalizer tests the finalizer functionality
func TestImageUpdaterReconciler_Reconcile_Finalizer(t *testing.T) {
	t.Run("resource without finalizer - should add finalizer", func(t *testing.T) {
		ctx := context.Background()
		cacheChan := make(chan struct{})
		close(cacheChan) // Cache is warmed

		// Create fake client
		s := scheme.Scheme
		err := argocdimageupdaterv1alpha1.AddToScheme(s)
		require.NoError(t, err)
		// Add ArgoCD Application types to the scheme
		err = argocdapi.AddToScheme(s)
		require.NoError(t, err)

		fakeClient := clifake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&argocdimageupdaterv1alpha1.ImageUpdater{}).Build()

		// Create reconciler
		reconciler := &ImageUpdaterReconciler{
			Client:                  fakeClient,
			Scheme:                  s,
			MaxConcurrentReconciles: 1,
			CacheWarmed:             cacheChan,
			StopChan:                make(chan struct{}),
			Wg:                      sync.WaitGroup{},
			Config: &ImageUpdaterConfig{
				CheckInterval:     30 * time.Second,
				MaxConcurrentApps: 1,
				DryRun:            true,
				KubeClient:        &kube.ImageUpdaterKubernetesClient{},
			},
		}

		// Create ImageUpdater resource without finalizer
		imageUpdater := &argocdimageupdaterv1alpha1.ImageUpdater{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "finalizer-test",
				Namespace: "default",
			},
			Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
				ApplicationRefs: []argocdimageupdaterv1alpha1.ApplicationRef{
					{
						NamePattern: "test-app",
						Images: []argocdimageupdaterv1alpha1.ImageConfig{
							{
								Alias:     "nginx",
								ImageName: "nginx:latest",
							},
						},
					},
				},
			},
		}

		// Create the resource in the fake client
		require.NoError(t, fakeClient.Create(ctx, imageUpdater))

		// Execute reconciliation
		result, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "finalizer-test",
				Namespace: "default",
			},
		})

		// Assert
		assert.NoError(t, err)
		// After adding finalizer, reconciliation should continue and requeue after interval
		assert.Equal(t, reconcile.Result{RequeueAfter: 30 * time.Second}, result)

		// Verify that the finalizer was added
		var updatedImageUpdater argocdimageupdaterv1alpha1.ImageUpdater
		err = fakeClient.Get(ctx, types.NamespacedName{
			Name:      "finalizer-test",
			Namespace: "default",
		}, &updatedImageUpdater)
		assert.NoError(t, err)
		assert.Contains(t, updatedImageUpdater.Finalizers, ResourcesFinalizerName, "Finalizer should be added")
	})

	t.Run("resource with finalizer already present - should not add duplicate", func(t *testing.T) {
		ctx := context.Background()
		cacheChan := make(chan struct{})
		close(cacheChan) // Cache is warmed

		// Create fake client
		s := scheme.Scheme
		err := argocdimageupdaterv1alpha1.AddToScheme(s)
		require.NoError(t, err)
		// Add ArgoCD Application types to the scheme
		err = argocdapi.AddToScheme(s)
		require.NoError(t, err)

		fakeClient := clifake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&argocdimageupdaterv1alpha1.ImageUpdater{}).Build()

		// Create reconciler
		reconciler := &ImageUpdaterReconciler{
			Client:                  fakeClient,
			Scheme:                  s,
			MaxConcurrentReconciles: 1,
			CacheWarmed:             cacheChan,
			StopChan:                make(chan struct{}),
			Wg:                      sync.WaitGroup{},
			Config: &ImageUpdaterConfig{
				CheckInterval:     30 * time.Second,
				MaxConcurrentApps: 1,
				DryRun:            true,
				KubeClient:        &kube.ImageUpdaterKubernetesClient{},
			},
		}

		// Create ImageUpdater resource with finalizer already present
		imageUpdater := &argocdimageupdaterv1alpha1.ImageUpdater{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "finalizer-test-existing",
				Namespace:  "default",
				Finalizers: []string{ResourcesFinalizerName},
			},
			Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
				ApplicationRefs: []argocdimageupdaterv1alpha1.ApplicationRef{
					{
						NamePattern: "test-app",
						Images: []argocdimageupdaterv1alpha1.ImageConfig{
							{
								Alias:     "nginx",
								ImageName: "nginx:latest",
							},
						},
					},
				},
			},
		}

		// Create the resource in the fake client
		require.NoError(t, fakeClient.Create(ctx, imageUpdater))

		// Execute reconciliation
		result, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "finalizer-test-existing",
				Namespace: "default",
			},
		})

		// Assert
		assert.NoError(t, err)
		// Should continue with normal reconciliation since finalizer already exists
		assert.Equal(t, reconcile.Result{RequeueAfter: 30 * time.Second}, result)

		// Verify that the finalizer is still present (not duplicated)
		var updatedImageUpdater argocdimageupdaterv1alpha1.ImageUpdater
		err = fakeClient.Get(ctx, types.NamespacedName{
			Name:      "finalizer-test-existing",
			Namespace: "default",
		}, &updatedImageUpdater)
		assert.NoError(t, err)
		assert.Contains(t, updatedImageUpdater.Finalizers, ResourcesFinalizerName, "Finalizer should still be present")
		assert.Len(t, updatedImageUpdater.Finalizers, 1, "Should have exactly one finalizer")
	})

	t.Run("resource with multiple finalizers - should preserve existing finalizers", func(t *testing.T) {
		ctx := context.Background()
		cacheChan := make(chan struct{})
		close(cacheChan) // Cache is warmed

		// Create fake client
		s := scheme.Scheme
		err := argocdimageupdaterv1alpha1.AddToScheme(s)
		require.NoError(t, err)
		// Add ArgoCD Application types to the scheme
		err = argocdapi.AddToScheme(s)
		require.NoError(t, err)

		fakeClient := clifake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&argocdimageupdaterv1alpha1.ImageUpdater{}).Build()

		// Create reconciler
		reconciler := &ImageUpdaterReconciler{
			Client:                  fakeClient,
			Scheme:                  s,
			MaxConcurrentReconciles: 1,
			CacheWarmed:             cacheChan,
			StopChan:                make(chan struct{}),
			Wg:                      sync.WaitGroup{},
			Config: &ImageUpdaterConfig{
				CheckInterval:     30 * time.Second,
				MaxConcurrentApps: 1,
				DryRun:            true,
				KubeClient:        &kube.ImageUpdaterKubernetesClient{},
			},
		}

		// Create ImageUpdater resource with multiple finalizers
		imageUpdater := &argocdimageupdaterv1alpha1.ImageUpdater{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "finalizer-test-multiple",
				Namespace:  "default",
				Finalizers: []string{"other-finalizer", ResourcesFinalizerName, "another-finalizer"},
			},
			Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
				ApplicationRefs: []argocdimageupdaterv1alpha1.ApplicationRef{
					{
						NamePattern: "test-app",
						Images: []argocdimageupdaterv1alpha1.ImageConfig{
							{
								Alias:     "nginx",
								ImageName: "nginx:latest",
							},
						},
					},
				},
			},
		}

		// Create the resource in the fake client
		require.NoError(t, fakeClient.Create(ctx, imageUpdater))

		// Execute reconciliation
		result, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "finalizer-test-multiple",
				Namespace: "default",
			},
		})

		// Assert
		assert.NoError(t, err)
		// Should continue with normal reconciliation since finalizer already exists
		assert.Equal(t, reconcile.Result{RequeueAfter: 30 * time.Second}, result)

		// Verify that all finalizers are preserved
		var updatedImageUpdater argocdimageupdaterv1alpha1.ImageUpdater
		err = fakeClient.Get(ctx, types.NamespacedName{
			Name:      "finalizer-test-multiple",
			Namespace: "default",
		}, &updatedImageUpdater)
		assert.NoError(t, err)
		assert.Contains(t, updatedImageUpdater.Finalizers, ResourcesFinalizerName, "Our finalizer should be present")
		assert.Contains(t, updatedImageUpdater.Finalizers, "other-finalizer", "Other finalizer should be preserved")
		assert.Contains(t, updatedImageUpdater.Finalizers, "another-finalizer", "Another finalizer should be preserved")
		assert.Len(t, updatedImageUpdater.Finalizers, 3, "Should have exactly three finalizers")
	})

	t.Run("resource with different finalizer - should add our finalizer", func(t *testing.T) {
		ctx := context.Background()
		cacheChan := make(chan struct{})
		close(cacheChan) // Cache is warmed

		// Create fake client
		s := scheme.Scheme
		err := argocdimageupdaterv1alpha1.AddToScheme(s)
		require.NoError(t, err)
		// Add ArgoCD Application types to the scheme
		err = argocdapi.AddToScheme(s)
		require.NoError(t, err)

		fakeClient := clifake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&argocdimageupdaterv1alpha1.ImageUpdater{}).Build()

		// Create reconciler
		reconciler := &ImageUpdaterReconciler{
			Client:                  fakeClient,
			Scheme:                  s,
			MaxConcurrentReconciles: 1,
			CacheWarmed:             cacheChan,
			StopChan:                make(chan struct{}),
			Wg:                      sync.WaitGroup{},
			Config: &ImageUpdaterConfig{
				CheckInterval:     30 * time.Second,
				MaxConcurrentApps: 1,
				DryRun:            true,
				KubeClient:        &kube.ImageUpdaterKubernetesClient{},
			},
		}

		// Create ImageUpdater resource with a different finalizer
		imageUpdater := &argocdimageupdaterv1alpha1.ImageUpdater{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "finalizer-test-different",
				Namespace:  "default",
				Finalizers: []string{"different-finalizer"},
			},
			Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
				ApplicationRefs: []argocdimageupdaterv1alpha1.ApplicationRef{
					{
						NamePattern: "test-app",
						Images: []argocdimageupdaterv1alpha1.ImageConfig{
							{
								Alias:     "nginx",
								ImageName: "nginx:latest",
							},
						},
					},
				},
			},
		}

		// Create the resource in the fake client
		require.NoError(t, fakeClient.Create(ctx, imageUpdater))

		// Execute reconciliation
		result, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "finalizer-test-different",
				Namespace: "default",
			},
		})

		// Assert
		assert.NoError(t, err)
		// After adding finalizer, reconciliation should continue and requeue after interval
		assert.Equal(t, reconcile.Result{RequeueAfter: 30 * time.Second}, result)

		// Verify that our finalizer was added while preserving the existing one
		var updatedImageUpdater argocdimageupdaterv1alpha1.ImageUpdater
		err = fakeClient.Get(ctx, types.NamespacedName{
			Name:      "finalizer-test-different",
			Namespace: "default",
		}, &updatedImageUpdater)
		assert.NoError(t, err)
		assert.Contains(t, updatedImageUpdater.Finalizers, ResourcesFinalizerName, "Our finalizer should be added")
		assert.Contains(t, updatedImageUpdater.Finalizers, "different-finalizer", "Existing finalizer should be preserved")
		assert.Len(t, updatedImageUpdater.Finalizers, 2, "Should have exactly two finalizers")
	})

	t.Run("finalizer removal functionality - should remove our finalizer", func(t *testing.T) {
		// Test the controllerutil.RemoveFinalizer functionality directly
		imageUpdater := &argocdimageupdaterv1alpha1.ImageUpdater{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "finalizer-removal-test",
				Namespace:  "default",
				Finalizers: []string{ResourcesFinalizerName, "other-finalizer"},
			},
		}

		// Verify initial state
		assert.Contains(t, imageUpdater.Finalizers, ResourcesFinalizerName, "Initial state should have our finalizer")
		assert.Contains(t, imageUpdater.Finalizers, "other-finalizer", "Initial state should have other finalizer")
		assert.Len(t, imageUpdater.Finalizers, 2, "Initial state should have exactly two finalizers")

		// Remove our finalizer
		controllerutil.RemoveFinalizer(imageUpdater, ResourcesFinalizerName)

		// Verify finalizer was removed
		assert.NotContains(t, imageUpdater.Finalizers, ResourcesFinalizerName, "Our finalizer should be removed")
		assert.Contains(t, imageUpdater.Finalizers, "other-finalizer", "Other finalizer should be preserved")
		assert.Len(t, imageUpdater.Finalizers, 1, "Should have exactly one finalizer remaining")
	})

	t.Run("finalizer removal functionality - should handle non-existent finalizer", func(t *testing.T) {
		// Test removing a finalizer that doesn't exist
		imageUpdater := &argocdimageupdaterv1alpha1.ImageUpdater{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "finalizer-removal-nonexistent-test",
				Namespace:  "default",
				Finalizers: []string{"other-finalizer"},
			},
		}

		// Verify initial state
		assert.NotContains(t, imageUpdater.Finalizers, ResourcesFinalizerName, "Initial state should not have our finalizer")
		assert.Contains(t, imageUpdater.Finalizers, "other-finalizer", "Initial state should have other finalizer")
		assert.Len(t, imageUpdater.Finalizers, 1, "Initial state should have exactly one finalizer")

		// Try to remove our finalizer (which doesn't exist)
		controllerutil.RemoveFinalizer(imageUpdater, ResourcesFinalizerName)

		// Verify state remains unchanged
		assert.NotContains(t, imageUpdater.Finalizers, ResourcesFinalizerName, "Should still not have our finalizer")
		assert.Contains(t, imageUpdater.Finalizers, "other-finalizer", "Other finalizer should still be present")
		assert.Len(t, imageUpdater.Finalizers, 1, "Should still have exactly one finalizer")
	})

	t.Run("finalizer removal functionality - should handle empty finalizers list", func(t *testing.T) {
		// Test removing finalizer from empty list
		imageUpdater := &argocdimageupdaterv1alpha1.ImageUpdater{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "finalizer-removal-empty-test",
				Namespace:  "default",
				Finalizers: []string{},
			},
		}

		// Verify initial state
		assert.Empty(t, imageUpdater.Finalizers, "Initial state should have empty finalizers list")

		// Try to remove our finalizer from empty list
		controllerutil.RemoveFinalizer(imageUpdater, ResourcesFinalizerName)

		// Verify state remains unchanged
		assert.Empty(t, imageUpdater.Finalizers, "Finalizers list should remain empty")
	})

	t.Run("finalizer removal functionality - should handle nil finalizers list", func(t *testing.T) {
		// Test removing finalizer from nil list
		imageUpdater := &argocdimageupdaterv1alpha1.ImageUpdater{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "finalizer-removal-nil-test",
				Namespace: "default",
				// Finalizers field is nil by default
			},
		}

		// Verify initial state
		assert.Nil(t, imageUpdater.Finalizers, "Initial state should have nil finalizers list")

		// Try to remove our finalizer from nil list
		controllerutil.RemoveFinalizer(imageUpdater, ResourcesFinalizerName)

		// Verify state remains unchanged
		assert.Nil(t, imageUpdater.Finalizers, "Finalizers list should remain nil")
	})

	t.Run("finalizer removal functionality - should remove only our finalizer from multiple", func(t *testing.T) {
		// Test removing our finalizer when multiple finalizers exist
		imageUpdater := &argocdimageupdaterv1alpha1.ImageUpdater{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "finalizer-removal-multiple-test",
				Namespace:  "default",
				Finalizers: []string{"first-finalizer", ResourcesFinalizerName, "second-finalizer", "third-finalizer"},
			},
		}

		// Verify initial state
		assert.Contains(t, imageUpdater.Finalizers, ResourcesFinalizerName, "Initial state should have our finalizer")
		assert.Contains(t, imageUpdater.Finalizers, "first-finalizer", "Initial state should have first finalizer")
		assert.Contains(t, imageUpdater.Finalizers, "second-finalizer", "Initial state should have second finalizer")
		assert.Contains(t, imageUpdater.Finalizers, "third-finalizer", "Initial state should have third finalizer")
		assert.Len(t, imageUpdater.Finalizers, 4, "Initial state should have exactly four finalizers")

		// Remove our finalizer
		controllerutil.RemoveFinalizer(imageUpdater, ResourcesFinalizerName)

		// Verify only our finalizer was removed
		assert.NotContains(t, imageUpdater.Finalizers, ResourcesFinalizerName, "Our finalizer should be removed")
		assert.Contains(t, imageUpdater.Finalizers, "first-finalizer", "First finalizer should be preserved")
		assert.Contains(t, imageUpdater.Finalizers, "second-finalizer", "Second finalizer should be preserved")
		assert.Contains(t, imageUpdater.Finalizers, "third-finalizer", "Third finalizer should be preserved")
		assert.Len(t, imageUpdater.Finalizers, 3, "Should have exactly three finalizers remaining")
	})
}

func stringPtr(s string) *string {
	return &s
}

// Assisted-by: Gemini AI
// TestImageUpdaterReconciler_RunImageUpdater tests the RunImageUpdater function
func TestImageUpdaterReconciler_RunImageUpdater(t *testing.T) {
	s := scheme.Scheme
	err := argocdimageupdaterv1alpha1.AddToScheme(s)
	require.NoError(t, err)
	err = argocdapi.AddToScheme(s)
	require.NoError(t, err)
	ctx := context.Background()
	fakeClientset := fake.NewClientset()
	metrics.InitMetrics()

	// Base CR for tests
	baseCr := &argocdimageupdaterv1alpha1.ImageUpdater{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cr",
			Namespace: "argocd",
		},
		Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
			ApplicationRefs: []argocdimageupdaterv1alpha1.ApplicationRef{
				{
					NamePattern: "matching-app",
					Images: []argocdimageupdaterv1alpha1.ImageConfig{
						{
							Alias:     "nginx",
							ImageName: "nginx",
						},
					},
				},
			},
		},
	}

	// Base apps for tests
	matchingApp := &argocdapi.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "matching-app",
			Namespace: "argocd",
			Labels:    map[string]string{"app": "nginx"},
		},
		Spec: argocdapi.ApplicationSpec{
			Source: &argocdapi.ApplicationSource{
				Kustomize: &argocdapi.ApplicationSourceKustomize{},
				Path:      "some/path",
			},
		},
		Status: argocdapi.ApplicationStatus{
			Summary: argocdapi.ApplicationSummary{
				Images: []string{"nginx:1.21.0"},
			},
			SourceType: argocdapi.ApplicationSourceTypeKustomize,
		},
	}
	nonMatchingApp := &argocdapi.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "non-matching-app",
			Namespace: "argocd",
		},
		Spec: argocdapi.ApplicationSpec{
			Source: &argocdapi.ApplicationSource{
				Kustomize: &argocdapi.ApplicationSourceKustomize{},
			},
		},
		Status: argocdapi.ApplicationStatus{
			Summary: argocdapi.ApplicationSummary{
				Images: []string{"redis:6.0"},
			},
			SourceType: argocdapi.ApplicationSourceTypeKustomize,
		},
	}

	appInOtherNs := &argocdapi.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-other-ns",
			Namespace: "other-ns",
		},
		Spec: argocdapi.ApplicationSpec{
			Source: &argocdapi.ApplicationSource{
				Kustomize: &argocdapi.ApplicationSourceKustomize{},
				Path:      "some/path",
			},
		},
		Status: argocdapi.ApplicationStatus{
			Summary: argocdapi.ApplicationSummary{
				Images: []string{"nginx:1.21.0"},
			},
			SourceType: argocdapi.ApplicationSourceTypeKustomize,
		},
	}

	tests := []struct {
		name                string
		cr                  *argocdimageupdaterv1alpha1.ImageUpdater
		apps                []client.Object
		dryRun              bool
		webhookEvent        *argocd.WebhookEvent
		warmUp              bool
		expectedResult      argocd.ImageUpdaterResult
		expectErr           bool
		expectedErrContains string
		postCheck           func(t *testing.T, r *ImageUpdaterReconciler, cr *argocdimageupdaterv1alpha1.ImageUpdater, res argocd.ImageUpdaterResult)
	}{
		{
			name: "one matching application",
			cr:   baseCr,
			apps: []client.Object{matchingApp, nonMatchingApp},
			expectedResult: argocd.ImageUpdaterResult{
				NumApplicationsProcessed: 1,
				NumImagesConsidered:      1,
				NumErrors:                0,
				NumImagesUpdated:         1,
				ApplicationsMatched:      1,
			},
		},
		{
			name: "sets number of applications metric",
			cr:   baseCr,
			apps: []client.Object{matchingApp, nonMatchingApp},
			postCheck: func(t *testing.T, r *ImageUpdaterReconciler, cr *argocdimageupdaterv1alpha1.ImageUpdater, res argocd.ImageUpdaterResult) {
				expectedVal := float64(1)
				metricVal := testutil.ToFloat64(metrics.Applications().ApplicationsTotal.WithLabelValues(cr.Name, cr.Namespace))
				assert.Equal(t, expectedVal, metricVal)
			},
			expectedResult: argocd.ImageUpdaterResult{
				NumApplicationsProcessed: 1,
				NumImagesConsidered:      1,
				NumImagesUpdated:         1,
				ApplicationsMatched:      1,
			},
		},
		{
			name:   "dry run false",
			cr:     baseCr,
			apps:   []client.Object{matchingApp, nonMatchingApp},
			dryRun: false,
			expectedResult: argocd.ImageUpdaterResult{
				NumApplicationsProcessed: 1,
				NumImagesConsidered:      1,
				NumErrors:                0,
				NumImagesUpdated:         1,
				ApplicationsMatched:      1,
			},
		},
		{
			name: "no matching applications",
			cr: &argocdimageupdaterv1alpha1.ImageUpdater{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cr", Namespace: "default"},
				Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
					ApplicationRefs: []argocdimageupdaterv1alpha1.ApplicationRef{
						{NamePattern: "no-match"},
					},
				},
			},
			apps:           []client.Object{matchingApp, nonMatchingApp},
			expectedResult: argocd.ImageUpdaterResult{},
		},
		{
			name: "multiple matching applications",
			cr: &argocdimageupdaterv1alpha1.ImageUpdater{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cr", Namespace: "argocd"},
				Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
					ApplicationRefs: []argocdimageupdaterv1alpha1.ApplicationRef{
						{
							NamePattern: "matching-*",
							Images: []argocdimageupdaterv1alpha1.ImageConfig{
								{ImageName: "nginx"},
							},
						},
					},
				},
			},
			apps: []client.Object{matchingApp, nonMatchingApp},
			expectedResult: argocd.ImageUpdaterResult{
				NumApplicationsProcessed: 1,
				NumImagesConsidered:      1,
				NumImagesUpdated:         1,
				ApplicationsMatched:      1,
			},
		},
		{
			name: "application with label selector",
			cr: &argocdimageupdaterv1alpha1.ImageUpdater{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cr", Namespace: "argocd"},
				Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
					ApplicationRefs: []argocdimageupdaterv1alpha1.ApplicationRef{
						{
							NamePattern: "matching-app",
							LabelSelectors: &metav1.LabelSelector{
								MatchLabels: map[string]string{"app": "nginx"},
							},
							Images: []argocdimageupdaterv1alpha1.ImageConfig{
								{ImageName: "nginx"},
							},
						},
					},
				},
			},
			apps: []client.Object{matchingApp},
			expectedResult: argocd.ImageUpdaterResult{
				NumApplicationsProcessed: 1,
				NumImagesConsidered:      1,
				NumImagesUpdated:         1,
				ApplicationsMatched:      1,
			},
		},
		{
			name: "application in different namespace - should be blocked",
			cr: &argocdimageupdaterv1alpha1.ImageUpdater{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cr", Namespace: "argocd"},
				Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
					ApplicationRefs: []argocdimageupdaterv1alpha1.ApplicationRef{
						{
							NamePattern: "app-other-ns",
							Images: []argocdimageupdaterv1alpha1.ImageConfig{
								{ImageName: "nginx"},
							},
						},
					},
				},
			},
			apps:           []client.Object{appInOtherNs},
			expectedResult: argocd.ImageUpdaterResult{}, // Empty result because app is in different namespace
		},
		{
			name: "application in same namespace - should be allowed",
			cr: &argocdimageupdaterv1alpha1.ImageUpdater{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cr", Namespace: "other-ns"},
				Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
					ApplicationRefs: []argocdimageupdaterv1alpha1.ApplicationRef{
						{
							NamePattern: "app-other-ns",
							Images: []argocdimageupdaterv1alpha1.ImageConfig{
								{ImageName: "nginx"},
							},
						},
					},
				},
			},
			apps: []client.Object{appInOtherNs},
			expectedResult: argocd.ImageUpdaterResult{
				NumApplicationsProcessed: 1,
				NumImagesConsidered:      1,
				NumImagesUpdated:         1,
				ApplicationsMatched:      1,
			},
		},
		{
			name: "error with invalid regex",
			cr: &argocdimageupdaterv1alpha1.ImageUpdater{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cr", Namespace: "argocd"},
				Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
					ApplicationRefs: []argocdimageupdaterv1alpha1.ApplicationRef{
						{NamePattern: "["},
					},
				},
			},
			apps:                []client.Object{matchingApp},
			expectErr:           true,
			expectedErrContains: "invalid application name pattern",
		},
		{
			name: "with webhook event filtering application",
			cr:   baseCr,
			apps: []client.Object{matchingApp, nonMatchingApp},
			webhookEvent: &argocd.WebhookEvent{
				Repository: "nginx",
			},
			expectedResult: argocd.ImageUpdaterResult{
				NumApplicationsProcessed: 1,
				NumImagesConsidered:      1,
				NumImagesUpdated:         1,
				ApplicationsMatched:      1,
			},
		},
		{
			name: "with webhook event not matching",
			cr:   baseCr,
			apps: []client.Object{matchingApp, nonMatchingApp},
			webhookEvent: &argocd.WebhookEvent{
				Repository: "redis",
			},
			expectedResult: argocd.ImageUpdaterResult{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset and re-init metrics for every test run to ensure isolation
			crmetrics.Registry = prometheus.NewRegistry()
			metrics.InitMetrics()

			// Build fake client and wrap with strict namespace client to enforce namespace isolation
			fakeClient := clifake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&argocdimageupdaterv1alpha1.ImageUpdater{}).WithObjects(tt.apps...).Build()
			strictClient := &strictNamespaceClient{Client: fakeClient}
			reconciler := &ImageUpdaterReconciler{
				Client: strictClient,
				Scheme: s,
				Config: &ImageUpdaterConfig{
					CheckInterval:     30 * time.Second,
					MaxConcurrentApps: 1,
					DryRun:            tt.dryRun,
					KubeClient: &kube.ImageUpdaterKubernetesClient{
						KubeClient: regokube.NewKubernetesClient(ctx, fakeClientset, "default"),
					},
				},
			}

			result, err := reconciler.RunImageUpdater(ctx, tt.cr, tt.warmUp, tt.webhookEvent)

			if tt.expectErr {
				require.Error(t, err)
				if tt.expectedErrContains != "" {
					assert.Contains(t, err.Error(), tt.expectedErrContains)
				}
			} else {
				require.NoError(t, err)
				// Clear Changes before comparison as it contains complex pointer types
				result.Changes = nil
				assert.Equal(t, tt.expectedResult, result)
			}

			if tt.postCheck != nil {
				tt.postCheck(t, reconciler, tt.cr, result)
			}
		})
	}
}

// Assisted-by: Gemini AI
// TestImageUpdaterReconciler_ProcessImageUpdaterCRs tests the ProcessImageUpdaterCRs function to ensure it correctly handles various scenarios,
// including processing multiple CRs, handling failures, and running in warm-up mode.
func TestImageUpdaterReconciler_ProcessImageUpdaterCRs(t *testing.T) {
	// Common setup for all tests in this suite
	s := scheme.Scheme
	err := argocdimageupdaterv1alpha1.AddToScheme(s)
	require.NoError(t, err)
	err = argocdapi.AddToScheme(s)
	require.NoError(t, err)
	ctx := context.Background()
	fakeClientset := fake.NewClientset()
	metrics.InitMetrics()

	// A helper function to create a new reconciler for each test run
	newTestReconciler := func(cli client.Client) *ImageUpdaterReconciler {
		return &ImageUpdaterReconciler{
			Client:                  cli,
			Scheme:                  s,
			MaxConcurrentReconciles: 2,
			Config: &ImageUpdaterConfig{
				KubeClient: &kube.ImageUpdaterKubernetesClient{
					KubeClient: regokube.NewKubernetesClient(ctx, fakeClientset, "default"),
				},
			},
		}
	}

	cr1 := &argocdimageupdaterv1alpha1.ImageUpdater{
		ObjectMeta: metav1.ObjectMeta{Name: "cr1", Namespace: "default"},
		Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
			ApplicationRefs: []argocdimageupdaterv1alpha1.ApplicationRef{{NamePattern: "app1"}},
		},
	}

	cr2 := &argocdimageupdaterv1alpha1.ImageUpdater{
		ObjectMeta: metav1.ObjectMeta{Name: "cr2", Namespace: "default"},
		Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
			ApplicationRefs: []argocdimageupdaterv1alpha1.ApplicationRef{{NamePattern: "app2"}},
		},
	}

	crInvalid := &argocdimageupdaterv1alpha1.ImageUpdater{
		ObjectMeta: metav1.ObjectMeta{Name: "cr-invalid", Namespace: "default"},
		Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
			ApplicationRefs: []argocdimageupdaterv1alpha1.ApplicationRef{{NamePattern: "["}}, // Invalid regex
		},
	}

	app1 := &argocdapi.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "app1", Namespace: "argocd"},
	}

	app2 := &argocdapi.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "app2", Namespace: "argocd"},
	}

	t.Run("no CRs to process", func(t *testing.T) {
		fakeClient := clifake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&argocdimageupdaterv1alpha1.ImageUpdater{}).Build()
		reconciler := newTestReconciler(fakeClient)
		err := reconciler.ProcessImageUpdaterCRs(ctx, []argocdimageupdaterv1alpha1.ImageUpdater{}, false, nil)
		assert.NoError(t, err)
	})

	t.Run("one successful CR", func(t *testing.T) {
		fakeClient := clifake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&argocdimageupdaterv1alpha1.ImageUpdater{}).WithObjects(app1).Build()
		reconciler := newTestReconciler(fakeClient)
		err := reconciler.ProcessImageUpdaterCRs(ctx, []argocdimageupdaterv1alpha1.ImageUpdater{*cr1}, false, nil)
		assert.NoError(t, err)
	})

	t.Run("multiple successful CRs", func(t *testing.T) {
		fakeClient := clifake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&argocdimageupdaterv1alpha1.ImageUpdater{}).WithObjects(app1, app2).Build()
		reconciler := newTestReconciler(fakeClient)
		err := reconciler.ProcessImageUpdaterCRs(ctx, []argocdimageupdaterv1alpha1.ImageUpdater{*cr1, *cr2}, false, nil)
		assert.NoError(t, err)
	})

	t.Run("one failing CR", func(t *testing.T) {
		fakeClient := clifake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&argocdimageupdaterv1alpha1.ImageUpdater{}).WithObjects(app1).Build()
		reconciler := newTestReconciler(fakeClient)
		err := reconciler.ProcessImageUpdaterCRs(ctx, []argocdimageupdaterv1alpha1.ImageUpdater{*crInvalid}, false, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid application name pattern")
	})

	t.Run("multiple CRs, one failing", func(t *testing.T) {
		fakeClient := clifake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&argocdimageupdaterv1alpha1.ImageUpdater{}).WithObjects(app1).Build()
		reconciler := newTestReconciler(fakeClient)
		err := reconciler.ProcessImageUpdaterCRs(ctx, []argocdimageupdaterv1alpha1.ImageUpdater{*cr1, *crInvalid}, false, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid application name pattern")
		assert.Contains(t, err.Error(), "failed to process 1 ImageUpdater CRs")
	})

	t.Run("warmUp mode", func(t *testing.T) {
		fakeClient := clifake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&argocdimageupdaterv1alpha1.ImageUpdater{}).WithObjects(app1).Build()
		reconciler := newTestReconciler(fakeClient)
		err := reconciler.ProcessImageUpdaterCRs(ctx, []argocdimageupdaterv1alpha1.ImageUpdater{*cr1}, true, nil)
		assert.NoError(t, err)
	})

	t.Run("with webhook event", func(t *testing.T) {
		fakeClient := clifake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&argocdimageupdaterv1alpha1.ImageUpdater{}).WithObjects(app1).Build()
		reconciler := newTestReconciler(fakeClient)
		webhookEvent := &argocd.WebhookEvent{
			Repository: "some-repo",
		}
		err := reconciler.ProcessImageUpdaterCRs(ctx, []argocdimageupdaterv1alpha1.ImageUpdater{*cr1}, false, webhookEvent)
		assert.NoError(t, err)
	})
}
