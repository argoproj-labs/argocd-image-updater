/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package parallel

import (
	"context"

	appv1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/argoproj/gitops-engine/pkg/health"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	applicationFixture "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/application"

	imageUpdaterApi "github.com/argoproj-labs/argocd-image-updater/api/v1alpha1"

	argov1beta1api "github.com/argoproj-labs/argocd-operator/api/v1beta1"

	"github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture"
	argocdFixture "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/argocd"
	deplFixture "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/deployment"
	k8sFixture "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/k8s"
	ssFixture "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/statefulset"
	fixtureUtils "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/utils"
)

var _ = Describe("ArgoCD Image Updater Parallel E2E Tests", func() {

	Context("1-007-status-subresource_test", func() {

		var (
			k8sClient    client.Client
			ctx          context.Context
			ns           *corev1.Namespace
			cleanupFunc  func()
			imageUpdater *imageUpdaterApi.ImageUpdater
			argoCD       *argov1beta1api.ArgoCD
		)

		BeforeEach(func() {
			fixture.EnsureParallelCleanSlate()

			k8sClient, _ = fixtureUtils.GetE2ETestKubeClient()
			ctx = context.Background()
		})

		AfterEach(func() {
			// Delete the ImageUpdater CR first and wait for its finalizer to be
			// processed before tearing down the ArgoCD CR (which removes the controller).

			if imageUpdater != nil {
				By("deleting ImageUpdater CR")
				_ = k8sClient.Delete(ctx, imageUpdater)
				Eventually(imageUpdater, "2m", "3s").Should(k8sFixture.NotExistByName())
			}

			if argoCD != nil {
				By("deleting ArgoCD CR")
				_ = k8sClient.Delete(ctx, argoCD)
			}

			if cleanupFunc != nil {
				cleanupFunc()
			}

			fixture.OutputDebugOnFail(ns)

		})

		// setupArgoCDInstance creates a namespace-scoped ArgoCD instance with image updater
		// enabled and waits for all workloads to be ready. This is shared setup for all
		// status subresource tests.
		setupArgoCDInstance := func() {
			By("creating simple namespace-scoped Argo CD instance with image updater enabled")
			ns, cleanupFunc = fixture.CreateRandomE2ETestNamespaceWithCleanupFunc()

			argoCD = &argov1beta1api.ArgoCD{
				ObjectMeta: metav1.ObjectMeta{Name: "argocd", Namespace: ns.Name},
				Spec: argov1beta1api.ArgoCDSpec{
					ImageUpdater: argov1beta1api.ArgoCDImageUpdaterSpec{
						Env: []corev1.EnvVar{
							{
								Name:  "IMAGE_UPDATER_LOGLEVEL",
								Value: "trace",
							},
							{
								Name:  "IMAGE_UPDATER_INTERVAL",
								Value: "30",
							},
						},
						Enabled: true},
				},
			}
			Expect(k8sClient.Create(ctx, argoCD)).To(Succeed())

			By("waiting for ArgoCD CR to be reconciled and the instance to be ready")
			Eventually(argoCD, "5m", "3s").Should(argocdFixture.BeAvailable())

			By("verifying all workloads are started")
			deploymentsShouldExist := []string{"argocd-redis", "argocd-server", "argocd-repo-server", "argocd-argocd-image-updater-controller"}
			for _, depl := range deploymentsShouldExist {
				depl := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: depl, Namespace: ns.Name}}
				Eventually(depl).Should(k8sFixture.ExistByName())
				Eventually(depl).Should(deplFixture.HaveReplicas(1))
				Eventually(depl, "3m", "3s").Should(deplFixture.HaveReadyReplicas(1), depl.Name+" was not ready")
			}

			statefulSet := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "argocd-application-controller", Namespace: ns.Name}}
			Eventually(statefulSet).Should(k8sFixture.ExistByName())
			Eventually(statefulSet).Should(ssFixture.HaveReplicas(1))
			Eventually(statefulSet, "3m", "3s").Should(ssFixture.HaveReadyReplicas(1))
		}

		It("verifies status reflects normal operation with matched applications", func() {

			setupArgoCDInstance()

			By("creating Application")
			app := &appv1alpha1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "status-app-01",
					Namespace: ns.Name,
				},
				Spec: appv1alpha1.ApplicationSpec{
					Project: "default",
					Source: &appv1alpha1.ApplicationSource{
						RepoURL:        "https://github.com/argoproj-labs/argocd-image-updater/",
						Path:           "test/e2e/testdata/005-public-guestbook",
						TargetRevision: "HEAD",
					},
					Destination: appv1alpha1.ApplicationDestination{
						Server:    "https://kubernetes.default.svc",
						Namespace: ns.Name,
					},
					SyncPolicy: &appv1alpha1.SyncPolicy{Automated: &appv1alpha1.SyncPolicyAutomated{}},
				},
			}
			Expect(k8sClient.Create(ctx, app)).To(Succeed())

			By("verifying deploying the Application succeeded")
			Eventually(app, "4m", "3s").Should(applicationFixture.HaveHealthStatusCode(health.HealthStatusHealthy))
			Eventually(app, "4m", "3s").Should(applicationFixture.HaveSyncStatusCode(appv1alpha1.SyncStatusCodeSynced))

			By("creating ImageUpdater CR")
			updateStrategy := "semver"
			imageUpdater = &imageUpdaterApi.ImageUpdater{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "image-updater-status",
					Namespace: ns.Name,
				},
				Spec: imageUpdaterApi.ImageUpdaterSpec{
					ApplicationRefs: []imageUpdaterApi.ApplicationRef{
						{
							NamePattern: "status-app*",
							Images: []imageUpdaterApi.ImageConfig{
								{
									Alias:     "guestbook",
									ImageName: "quay.io/dkarpele/my-guestbook:~29437546.0",
									CommonUpdateSettings: &imageUpdaterApi.CommonUpdateSettings{
										UpdateStrategy: &updateStrategy,
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, imageUpdater)).To(Succeed())

			By("verifying status shows matched applications and Ready condition")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(imageUpdater), imageUpdater)
				g.Expect(err).ToNot(HaveOccurred())

				// Verify applications matched
				g.Expect(imageUpdater.Status.ApplicationsMatched).To(BeNumerically(">=", int32(1)),
					"should match at least 1 application")

				// Verify images managed
				g.Expect(imageUpdater.Status.ImagesManaged).To(BeNumerically(">=", int32(1)),
					"should manage at least 1 image")

				// Verify lastCheckedAt is set
				g.Expect(imageUpdater.Status.LastCheckedAt).ToNot(BeNil(),
					"lastCheckedAt should be set after reconciliation")

				// Verify Ready condition
				readyCondition := apimeta.FindStatusCondition(imageUpdater.Status.Conditions, "Ready")
				g.Expect(readyCondition).ToNot(BeNil(), "Ready condition should be present")
				g.Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue),
					"Ready condition should be True")

				// Verify Reconciling condition is False (idle)
				reconcilingCondition := apimeta.FindStatusCondition(imageUpdater.Status.Conditions, "Reconciling")
				g.Expect(reconcilingCondition).ToNot(BeNil(), "Reconciling condition should be present")
				g.Expect(reconcilingCondition.Status).To(Equal(metav1.ConditionFalse),
					"Reconciling condition should be False after reconciliation completes")

				// Verify Error condition is False
				errorCondition := apimeta.FindStatusCondition(imageUpdater.Status.Conditions, "Error")
				g.Expect(errorCondition).ToNot(BeNil(), "Error condition should be present")
				g.Expect(errorCondition.Status).To(Equal(metav1.ConditionFalse),
					"Error condition should be False for successful reconciliation")

				// Verify observedGeneration is set
				g.Expect(imageUpdater.Status.ObservedGeneration).To(BeNumerically(">", int64(0)),
					"observedGeneration should be set")
			}, "3m", "5s").Should(Succeed())
		})

		It("verifies status reflects zero matched applications without error", func() {

			setupArgoCDInstance()

			By("creating ImageUpdater CR with a pattern that matches no applications")
			imageUpdater = &imageUpdaterApi.ImageUpdater{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "image-updater-no-match",
					Namespace: ns.Name,
				},
				Spec: imageUpdaterApi.ImageUpdaterSpec{
					ApplicationRefs: []imageUpdaterApi.ApplicationRef{
						{
							NamePattern: "nonexistent-app-*",
							Images: []imageUpdaterApi.ImageConfig{
								{
									Alias:     "nginx",
									ImageName: "nginx:1.20",
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, imageUpdater)).To(Succeed())

			By("verifying status shows zero matched applications and Ready condition without error")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(imageUpdater), imageUpdater)
				g.Expect(err).ToNot(HaveOccurred())

				// Verify no applications matched
				g.Expect(imageUpdater.Status.ApplicationsMatched).To(Equal(int32(0)),
					"should match 0 applications")

				// Verify no images managed
				g.Expect(imageUpdater.Status.ImagesManaged).To(Equal(int32(0)),
					"should manage 0 images")

				// Verify lastCheckedAt is set (reconciliation still ran)
				g.Expect(imageUpdater.Status.LastCheckedAt).ToNot(BeNil(),
					"lastCheckedAt should be set even with no matched applications")

				// Verify lastUpdatedAt is NOT set (no updates performed)
				g.Expect(imageUpdater.Status.LastUpdatedAt).To(BeNil(),
					"lastUpdatedAt should not be set when no updates were performed")

				// Verify no recent updates
				g.Expect(imageUpdater.Status.RecentUpdates).To(BeEmpty(),
					"recentUpdates should be empty when no applications matched")

				// Verify Ready condition is True (no error, just nothing to do)
				readyCondition := apimeta.FindStatusCondition(imageUpdater.Status.Conditions, "Ready")
				g.Expect(readyCondition).ToNot(BeNil(), "Ready condition should be present")
				g.Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue),
					"Ready condition should be True even with zero matched applications")

				// Verify Error condition is False
				errorCondition := apimeta.FindStatusCondition(imageUpdater.Status.Conditions, "Error")
				g.Expect(errorCondition).ToNot(BeNil(), "Error condition should be present")
				g.Expect(errorCondition.Status).To(Equal(metav1.ConditionFalse),
					"Error condition should be False when no applications matched (this is not an error)")
			}, "3m", "5s").Should(Succeed())
		})

		It("verifies status reflects error condition for misconfigured CR", func() {

			setupArgoCDInstance()

			By("creating ImageUpdater CR with invalid namePattern")
			imageUpdater = &imageUpdaterApi.ImageUpdater{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "image-updater-bad-pattern",
					Namespace: ns.Name,
				},
				Spec: imageUpdaterApi.ImageUpdaterSpec{
					ApplicationRefs: []imageUpdaterApi.ApplicationRef{
						{
							NamePattern: "foo[bar", // Invalid glob pattern (unclosed bracket)
							Images: []imageUpdaterApi.ImageConfig{
								{
									Alias:     "nginx",
									ImageName: "nginx:1.20",
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, imageUpdater)).To(Succeed())

			By("verifying status shows error condition without crashing the controller")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(imageUpdater), imageUpdater)
				g.Expect(err).ToNot(HaveOccurred())

				// Verify Error condition is True
				errorCondition := apimeta.FindStatusCondition(imageUpdater.Status.Conditions, "Error")
				g.Expect(errorCondition).ToNot(BeNil(), "Error condition should be present")
				g.Expect(errorCondition.Status).To(Equal(metav1.ConditionTrue),
					"Error condition should be True for invalid namePattern")
				g.Expect(errorCondition.Reason).To(Equal("ReconcileError"),
					"Error reason should be ReconcileError")
				g.Expect(errorCondition.Message).To(ContainSubstring("invalid application name pattern"),
					"Error message should mention invalid application name pattern")

				// Verify Ready condition is False
				readyCondition := apimeta.FindStatusCondition(imageUpdater.Status.Conditions, "Ready")
				g.Expect(readyCondition).ToNot(BeNil(), "Ready condition should be present")
				g.Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse),
					"Ready condition should be False when reconciliation failed")

				// Verify lastCheckedAt is set (reconciliation attempted)
				g.Expect(imageUpdater.Status.LastCheckedAt).ToNot(BeNil(),
					"lastCheckedAt should be set even for failed reconciliation")
			}, "3m", "5s").Should(Succeed())

			By("verifying the controller is still running after processing misconfigured CR")
			controllerDepl := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "argocd-argocd-image-updater-controller",
					Namespace: ns.Name,
				},
			}
			Consistently(controllerDepl, "30s", "5s").Should(deplFixture.HaveReadyReplicas(1),
				"controller should remain running after encountering misconfigured CR")
		})
	})
})
