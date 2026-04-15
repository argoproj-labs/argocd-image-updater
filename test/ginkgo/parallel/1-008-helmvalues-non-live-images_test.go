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
	"fmt"

	appv1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/argoproj/gitops-engine/pkg/health"
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

	// This test is a regression test for issue #1584.
	//
	// When multiple image aliases share a single helmvalues write-back target, only the
	// images that are actually running in the cluster ("live") should have their tags
	// written. Images that are configured in the ImageUpdater CR but are NOT currently
	// deployed ("non-live") must not have their existing tag values overwritten with "".
	//
	// Setup:
	//   - A Helm chart with three image entries in values.yaml: manager, session, reaper.
	//   - The Deployment template uses only the manager image, so only manager will appear
	//     in app.Status.Summary.Images (live image).
	//   - Session and reaper are present in values.yaml but are not used by any pod
	//     template, so they will never appear in app.Status.Summary.Images (non-live).
	//   - A ConfigMap rendered by the chart exposes the session and reaper tags directly
	//     from values.yaml so the test can assert their values after an ArgoCD sync.
	//
	// Expected behaviour after Image Updater runs:
	//   - manager.image.tag is updated to the latest semver tag found in the registry.
	//   - session.image.tag and reaper.image.tag remain at "1.0.0" (not blanked to "").
	Context("1-008-helmvalues-non-live-images_test", func() {

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

		It("ensures that helmvalues write-back does not blank non-live image tags (issue #1584)", func() {

			By("creating simple namespace-scoped Argo CD instance with image updater enabled")
			ns, cleanupFunc = fixture.CreateRandomE2ETestNamespaceWithCleanupFunc()

			By("Creating local git repo")
			iuFixture.CreateLocalGitRepo(ctx, k8sClient, ns.Name)

			By("waiting for local git repo to be ready")
			gitDepl := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: iuFixture.Name, Namespace: ns.Name}}
			Eventually(gitDepl).Should(k8sFixture.ExistByName())
			Eventually(gitDepl, "2m", "3s").Should(deplFixture.HaveReadyReplicas(1), "git repo server was not ready")

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

			By("creating Helm Application with multiple image aliases sharing one helmvalues file")
			gitRepoURL := fmt.Sprintf("https://%s.%s.svc.cluster.local:8081/testdata.git", iuFixture.Name, ns.Name)
			app := &appv1alpha1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multi-image-helm-app",
					Namespace: ns.Name,
				},
				Spec: appv1alpha1.ApplicationSpec{
					Project: "default",
					Source: &appv1alpha1.ApplicationSource{
						RepoURL:        gitRepoURL,
						Path:           "1-008-helmvalues-non-live-images/helm",
						TargetRevision: "HEAD",
						Helm: &appv1alpha1.ApplicationSourceHelm{
							ValueFiles: []string{"values.yaml"},
						},
					},
					Destination: appv1alpha1.ApplicationDestination{
						Server:    "https://kubernetes.default.svc",
						Namespace: ns.Name,
					},
					SyncPolicy: &appv1alpha1.SyncPolicy{
						Automated: &appv1alpha1.SyncPolicyAutomated{
							Prune: true,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, app)).To(Succeed())

			By("verifying deploying the Application succeeded")
			Eventually(app, "4m", "3s").Should(applicationFixture.HaveHealthStatusCode(health.HealthStatusHealthy))
			Eventually(app, "4m", "3s").Should(applicationFixture.HaveSyncStatusCode(appv1alpha1.SyncStatusCodeSynced))

			By("creating ImageUpdater CR with three image aliases sharing one helmvalues write-back target")
			updateStrategy := "semver"
			forceUpdate := false
			method := fmt.Sprintf("git:secret:%s/%s", ns.Name, iuFixture.Name)
			branch := "master"
			repository := gitRepoURL
			// All three images share the same helmvalues target.
			writeBackTarget := "helmvalues:/1-008-helmvalues-non-live-images/helm/values.yaml"

			managerHelmImageName := "manager.image.repository"
			managerHelmImageTag := "manager.image.tag"
			sessionHelmImageName := "session.image.repository"
			sessionHelmImageTag := "session.image.tag"
			reaperHelmImageName := "reaper.image.repository"
			reaperHelmImageTag := "reaper.image.tag"

			imageUpdater = &imageUpdaterApi.ImageUpdater{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "image-updater",
					Namespace: ns.Name,
				},
				Spec: imageUpdaterApi.ImageUpdaterSpec{
					CommonUpdateSettings: &imageUpdaterApi.CommonUpdateSettings{
						UpdateStrategy: &updateStrategy,
						ForceUpdate:    &forceUpdate,
					},
					WriteBackConfig: &imageUpdaterApi.WriteBackConfig{
						Method: &method,
						GitConfig: &imageUpdaterApi.GitConfig{
							Branch:          &branch,
							Repository:      &repository,
							WriteBackTarget: &writeBackTarget,
						},
					},
					ApplicationRefs: []imageUpdaterApi.ApplicationRef{
						{
							NamePattern: "multi-image-helm-app",
							Images: []imageUpdaterApi.ImageConfig{
								// manager: live image — its Deployment container is running,
								// so it appears in app.Status.Summary.Images. AIU will find
								// a newer semver tag and update manager.image.tag.
								{
									Alias:     "manager",
									ImageName: "quay.io/dkarpele/my-guestbook:29437546.X",
									ManifestTarget: &imageUpdaterApi.ManifestTarget{
										Helm: &imageUpdaterApi.HelmTarget{
											Name: &managerHelmImageName,
											Tag:  &managerHelmImageTag,
										},
									},
								},
								// session: non-live image — no container runs this image,
								// so it never appears in app.Status.Summary.Images.
								// A distinct repository name is required so that ContainsImage
								// does NOT match this alias against the running manager pod.
								// AIU must NOT blank session.image.tag in the helmvalues file.
								{
									Alias:     "session",
									ImageName: "quay.io/dkarpele/my-guestbook-session:29437546.X",
									ManifestTarget: &imageUpdaterApi.ManifestTarget{
										Helm: &imageUpdaterApi.HelmTarget{
											Name: &sessionHelmImageName,
											Tag:  &sessionHelmImageTag,
										},
									},
								},
								// reaper: non-live image — same constraint as session.
								{
									Alias:     "reaper",
									ImageName: "quay.io/dkarpele/my-guestbook-reaper:29437546.X",
									ManifestTarget: &imageUpdaterApi.ManifestTarget{
										Helm: &imageUpdaterApi.HelmTarget{
											Name: &reaperHelmImageName,
											Tag:  &reaperHelmImageTag,
										},
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, imageUpdater)).To(Succeed())

			By("ensuring that the manager image has been updated to the latest semver tag")
			triggerRefresh := iuFixture.TriggerArgoCDRefresh(ctx, k8sClient, app)
			Eventually(func() string {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(app), app)
				if err != nil {
					return ""
				}

				triggerRefresh()

				// For git write-back, changes are pushed to git and ArgoCD syncs from there.
				// The updated image appears in Status.Summary.Images once ArgoCD re-syncs.
				for _, img := range app.Status.Summary.Images {
					if img == "quay.io/dkarpele/my-guestbook:29437546.0" {
						return img
					}
				}
				return ""
			}, "5m", "3s").Should(Equal("quay.io/dkarpele/my-guestbook:29437546.0"))

			By("verifying the Application is still healthy after the helmvalues update")
			Eventually(app, "4m", "3s").Should(applicationFixture.HaveHealthStatusCode(health.HealthStatusHealthy))
			Eventually(app, "4m", "3s").Should(applicationFixture.HaveSyncStatusCode(appv1alpha1.SyncStatusCodeSynced))

			// At this point ArgoCD has already re-synced from the updated helmvalues file,
			// so the ConfigMap rendered by the chart reflects the current values.yaml state.
			// If issue #1584 has regressed, session.image.tag and reaper.image.tag will have
			// been overwritten with "" in the helmvalues file, and the ConfigMap will expose
			// that corruption here.
			By("verifying that non-live image tags were not blanked in the helmvalues file (issue #1584)")
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "non-live-image-tags",
					Namespace: ns.Name,
				},
			}
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(configMap), configMap)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(configMap.Data["session-tag"]).ToNot(BeEmpty(),
					"session image tag must not be blanked by helmvalues write-back (issue #1584)")
				g.Expect(configMap.Data["reaper-tag"]).ToNot(BeEmpty(),
					"reaper image tag must not be blanked by helmvalues write-back (issue #1584)")
				g.Expect(configMap.Data["session-tag"]).To(Equal("1.0.0"),
					"session image tag must retain its original value after manager-only update")
				g.Expect(configMap.Data["reaper-tag"]).To(Equal("1.0.0"),
					"reaper image tag must retain its original value after manager-only update")
			}, "2m", "3s").Should(Succeed())
		})
	})
})
