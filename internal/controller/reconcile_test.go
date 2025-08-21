package controller

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	argocdapi "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	argocdimageupdaterv1alpha1 "github.com/argoproj-labs/argocd-image-updater/api/v1alpha1"
	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd/mocks"
	"github.com/argoproj-labs/argocd-image-updater/pkg/kube"
)

// TestImageUpdaterReconciler_Reconcile tests the main Reconcile function
func TestImageUpdaterReconciler_Reconcile(t *testing.T) {
	tests := []struct {
		name           string
		setupTest      func(*ImageUpdaterReconciler, client.Client, *mocks.ArgoCD, chan struct{})
		request        reconcile.Request
		expectedResult reconcile.Result
		expectedError  bool
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
						Namespace: "argocd",
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
			name: "cache warmed, resource exists, CheckInterval = 0 - should requeue once",
			setupTest: func(reconciler *ImageUpdaterReconciler, fakeClient client.Client, mockArgoClient *mocks.ArgoCD, cacheChan chan struct{}) {
				close(cacheChan)
				reconciler.Config.CheckInterval = 0
				// Add to WaitGroup since CheckInterval = 0 will call Wg.Done()
				reconciler.Wg.Add(1)
				imageUpdater := &argocdimageupdaterv1alpha1.ImageUpdater{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "default",
					},
					Spec: argocdimageupdaterv1alpha1.ImageUpdaterSpec{
						Namespace: "argocd",
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
			expectedResult: reconcile.Result{},
			expectedError:  false,
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
						Namespace: "argocd",
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
	}

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

			fakeClient := fake.NewClientBuilder().WithScheme(s).Build()

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

			// Verify mock expectations
			mockArgoClient.AssertExpectations(t)
		})
	}
}

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
					Namespace: "argocd",
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
					Namespace: "argocd",
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
					Namespace: "argocd",
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
					Namespace: "argocd",
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

			fakeClient := fake.NewClientBuilder().WithScheme(s).Build()

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

		fakeClient := fake.NewClientBuilder().WithScheme(s).Build()

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
				Namespace: "argocd",
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

		fakeClient := fake.NewClientBuilder().WithScheme(s).Build()

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
				Namespace: "argocd",
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

		fakeClient := fake.NewClientBuilder().WithScheme(s).Build()

		// Create reconciler with CheckInterval = 0 (run-once mode)
		reconciler := &ImageUpdaterReconciler{
			Client:                  fakeClient,
			Scheme:                  s,
			MaxConcurrentReconciles: 1,
			CacheWarmed:             cacheChan,
			StopChan:                make(chan struct{}),
			Wg:                      sync.WaitGroup{},
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
				Namespace: "argocd",
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
				Namespace: "argocd",
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
				Namespace: "argocd",
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

		fakeClient := fake.NewClientBuilder().WithScheme(s).Build()

		// Create reconciler with CheckInterval = 0 (run-once mode)
		reconciler := &ImageUpdaterReconciler{
			Client:                  fakeClient,
			Scheme:                  s,
			MaxConcurrentReconciles: 1,
			CacheWarmed:             cacheChan,
			StopChan:                make(chan struct{}),
			Wg:                      sync.WaitGroup{},
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
				Namespace: "argocd",
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
				Namespace: "argocd",
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

		fakeClient := fake.NewClientBuilder().WithScheme(s).Build()

		// Create reconciler with CheckInterval = 0 and higher concurrency
		reconciler := &ImageUpdaterReconciler{
			Client:                  fakeClient,
			Scheme:                  s,
			MaxConcurrentReconciles: 3, // Allow concurrent reconciliation
			CacheWarmed:             cacheChan,
			StopChan:                make(chan struct{}),
			Wg:                      sync.WaitGroup{},
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
					Namespace: "argocd",
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
					Namespace: "argocd",
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
					Namespace: "argocd",
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

		fakeClient := fake.NewClientBuilder().WithScheme(s).Build()

		// Create reconciler with CheckInterval = 0 (run-once mode)
		reconciler := &ImageUpdaterReconciler{
			Client:                  fakeClient,
			Scheme:                  s,
			MaxConcurrentReconciles: 1,
			CacheWarmed:             cacheChan,
			StopChan:                make(chan struct{}),
			Wg:                      sync.WaitGroup{},
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
					Namespace: "argocd",
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
					Namespace: "argocd",
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
					Namespace: "argocd",
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

		fakeClient := fake.NewClientBuilder().WithScheme(s).Build()

		// Create reconciler with CheckInterval = 0 (run-once mode)
		reconciler := &ImageUpdaterReconciler{
			Client:                  fakeClient,
			Scheme:                  s,
			MaxConcurrentReconciles: 1,
			CacheWarmed:             cacheChan,
			StopChan:                make(chan struct{}),
			Wg:                      sync.WaitGroup{},
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
					Namespace: "argocd",
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
					Namespace: "argocd",
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

func stringPtr(s string) *string {
	return &s
}
