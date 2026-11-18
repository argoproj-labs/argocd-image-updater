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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	applicationFixture "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/application"

	imageUpdaterApi "github.com/argoproj-labs/argocd-image-updater/api/v1alpha1"

	argov1beta1api "github.com/argoproj-labs/argocd-operator/api/v1beta1"

	"github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture"
	argocdFixture "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/argocd"
	deplFixture "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/deployment"
	iuFixture "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/imageupdater"
	k8sFixture "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/k8s"
	ssFixture "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/statefulset"
	fixtureUtils "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/utils"
)

var _ = Describe("ArgoCD Image Updater Parallel E2E Tests", func() {

	Context("1-013-deployed-image-name-test", func() {

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

		It("ensures that tags are looked up with imageName while deployedImageName is matched and written back", func() {

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
								Value: "0",
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

			iuFixture.WaitForControllerRBAC(ctx, k8sClient, ns.Name)

			// The Application pulls the image through a mirror, much as it would
			// pull it through a pull-through cache. The mirror's registry does not
			// resolve, so the update can only succeed if tags are listed in the
			// upstream registry in imageName, not in the mirror.
			const upstreamImage = "quay.io/dkarpele/my-guestbook"
			const mirrorImage = "mirror.invalid/dkarpele/my-guestbook"

			By("creating Application that runs the image from the mirror")
			app := &appv1alpha1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "app-01",
					Namespace: ns.Name,
				},
				Spec: appv1alpha1.ApplicationSpec{
					Project: "default",
					Source: &appv1alpha1.ApplicationSource{
						RepoURL:        "https://github.com/argoproj-labs/argocd-image-updater/",
						Path:           "test/e2e/testdata/005-public-guestbook",
						TargetRevision: "HEAD",
						Kustomize: &appv1alpha1.ApplicationSourceKustomize{
							Images: appv1alpha1.KustomizeImages{
								appv1alpha1.KustomizeImage(upstreamImage + "=" + mirrorImage + ":1.0.0"),
							},
						},
					},
					Destination: appv1alpha1.ApplicationDestination{
						Server:    "https://kubernetes.default.svc",
						Namespace: ns.Name,
					},
					SyncPolicy: &appv1alpha1.SyncPolicy{Automated: &appv1alpha1.SyncPolicyAutomated{}},
				},
			}
			Expect(k8sClient.Create(ctx, app)).To(Succeed())

			// The mirror cannot be pulled from, so the Application never becomes
			// healthy, but Argo CD still reports the image it runs.
			By("verifying the Application is synced and reports the mirror image")
			Eventually(app, "4m", "3s").Should(applicationFixture.HaveSyncStatusCode(appv1alpha1.SyncStatusCodeSynced))
			Eventually(func() []string {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(app), app); err != nil {
					return nil
				}
				return app.Status.Summary.Images
			}, "4m", "3s").Should(ContainElement(mirrorImage + ":1.0.0"))

			By("creating ImageUpdater CR with deployedImageName")
			updateStrategy := "semver"
			kustomizeName := upstreamImage
			deployedImageName := mirrorImage
			imageUpdater = &imageUpdaterApi.ImageUpdater{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "image-updater",
					Namespace: ns.Name,
				},
				Spec: imageUpdaterApi.ImageUpdaterSpec{
					ApplicationRefs: []imageUpdaterApi.ApplicationRef{
						{
							NamePattern: "app*",
							Images: []imageUpdaterApi.ImageConfig{
								{
									Alias:             "guestbook",
									ImageName:         upstreamImage + ":~29437546.0",
									DeployedImageName: &deployedImageName,
									CommonUpdateSettings: &imageUpdaterApi.CommonUpdateSettings{
										UpdateStrategy: &updateStrategy,
									},
									ManifestTarget: &imageUpdaterApi.ManifestTarget{
										Kustomize: &imageUpdaterApi.KustomizeTarget{
											Name: &kustomizeName,
										},
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, imageUpdater)).To(Succeed())

			By("ensuring that the mirror image is written back with the `29437546.0` tag from upstream")
			triggerRefresh := iuFixture.TriggerArgoCDRefresh(ctx, k8sClient, app)
			Eventually(func() string {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(app), app); err != nil {
					return ""
				}

				// Trigger ArgoCD refresh periodically to force immediate git check
				triggerRefresh()

				if app.Spec.Source.Kustomize != nil && len(app.Spec.Source.Kustomize.Images) > 0 {
					return string(app.Spec.Source.Kustomize.Images[0])
				}
				return ""
			}, "5m", "3s").Should(Equal(upstreamImage + "=" + mirrorImage + ":29437546.0"))
		})

		It("rejects a deployedImageName that is not a bare image repository", func() {
			ns, cleanupFunc = fixture.CreateRandomE2ETestNamespaceWithCleanupFunc()

			for _, name := range []string{
				"mirror.invalid/dkarpele/my-guestbook:1.0.0",
				"mirror.invalid/dkarpele/my-guestbook@sha256:0000000000000000000000000000000000000000000000000000000000000000",
				"guestbook=mirror.invalid/dkarpele/my-guestbook",
				"mirror.invalid/dkarpele/My-Guestbook",
				"mirror.invalid/dkarpele//my-guestbook",
				" mirror.invalid/dkarpele/my-guestbook",
			} {
				By("creating ImageUpdater CR with deployedImageName " + name)
				invalid := &imageUpdaterApi.ImageUpdater{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "image-updater-invalid",
						Namespace: ns.Name,
					},
					Spec: imageUpdaterApi.ImageUpdaterSpec{
						ApplicationRefs: []imageUpdaterApi.ApplicationRef{
							{
								NamePattern: "app*",
								Images: []imageUpdaterApi.ImageConfig{
									{
										Alias:             "guestbook",
										ImageName:         "quay.io/dkarpele/my-guestbook:~29437546.0",
										DeployedImageName: &name,
									},
								},
							},
						},
					},
				}
				err := k8sClient.Create(ctx, invalid)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("deployedImageName"))
			}
		})
	})
})
