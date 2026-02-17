/*
Copyright 2025.

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

	// This test verifies that Image Updater correctly handles single-source Helm Applications
	// using git write-back with an external values file.
	Context("1-004-git-helm-test", func() {

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
			// Cleanup is best-effort. Issue deletes and give some time for controllers
			// to process, but don't fail the test if cleanup takes too long.
			fixture.OutputDebugOnFail(ns)

			if imageUpdater != nil {
				By("deleting ImageUpdater CR")
				_ = k8sClient.Delete(ctx, imageUpdater)
				// Wait for the controller to process the finalizer and fully remove the CR
				// before deleting the ArgoCD CR (which tears down the controller).
				Eventually(imageUpdater, "2m", "3s").Should(k8sFixture.NotExistByName())
			}

			if argoCD != nil {
				By("deleting ArgoCD CR")
				_ = k8sClient.Delete(ctx, argoCD)
			}

			if cleanupFunc != nil {
				cleanupFunc()
			}
		})

		It("ensures that Image Updater will update single-source Helm Argo CD Application using git write-back policy", func() {

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

			By("creating single-source Helm Application")
			gitRepoURL := fmt.Sprintf("https://%s.%s.svc.cluster.local:8081/testdata.git", iuFixture.Name, ns.Name)
			app := &appv1alpha1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "helm-app",
					Namespace: ns.Name,
				},
				Spec: appv1alpha1.ApplicationSpec{
					Project: "default",
					Source: &appv1alpha1.ApplicationSource{
						RepoURL:        gitRepoURL,
						Path:           "1-004-git-helm-test/helm",
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

			By("creating ImageUpdater CR with git write-back targeting the Helm values file")
			updateStrategy := "semver"
			forceUpdate := false
			method := fmt.Sprintf("git:secret:%s/%s", ns.Name, iuFixture.Name)
			branch := "master"
			repository := gitRepoURL
			// Target the helm values.yaml file directly
			writeBackTarget := "helmvalues:/1-004-git-helm-test/helm/values.yaml"
			// Helm value paths for image name and tag
			helmImageName := "image.name"
			helmImageTag := "image.tag"

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
							NamePattern: "helm-*",
							Images: []imageUpdaterApi.ImageConfig{
								{
									Alias:     "guestbook",
									ImageName: "quay.io/dkarpele/my-guestbook:29437546.X",
									ManifestTarget: &imageUpdaterApi.ManifestTarget{
										Helm: &imageUpdaterApi.HelmTarget{
											Name: &helmImageName,
											Tag:  &helmImageTag,
										},
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, imageUpdater)).To(Succeed())

			By("ensuring that the Application image has `29437546.0` version after update")
			triggerRefresh := iuFixture.TriggerArgoCDRefresh(ctx, k8sClient, app)
			Eventually(func() string {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(app), app)

				if err != nil {
					return "" // Let Eventually retry on error
				}

				// Trigger ArgoCD refresh periodically to force immediate git check
				triggerRefresh()

				// For git write-back method, the image updater writes changes to git, and ArgoCD syncs from git.
				// The image appears in Status.Summary.Images (not in Spec.Source.Kustomize.Images like argocd write-back).
				if len(app.Status.Summary.Images) > 0 {
					return app.Status.Summary.Images[0]
				}

				// Return an empty string to signify the condition is not yet met.
				return ""
			}, "5m", "3s").Should(Equal("quay.io/dkarpele/my-guestbook:29437546.0"))
		})
	})
})
