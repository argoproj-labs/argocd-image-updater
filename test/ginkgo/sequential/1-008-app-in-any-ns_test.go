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

package sequential

import (
	"context"
	"time"

	applicationFixture "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/application"
	appv1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/argoproj/gitops-engine/pkg/health"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	imageUpdaterApi "github.com/argoproj-labs/argocd-image-updater/api/v1alpha1"

	"github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture"
	argocdFixture "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/argocd"
	deplFixture "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/deployment"
	iuFixture "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/imageupdater"
	k8sFixture "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/k8s"
	ssFixture "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/statefulset"
	fixtureUtils "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/utils"
	argov1beta1api "github.com/argoproj-labs/argocd-operator/api/v1beta1"
)

var _ = Describe("ArgoCD Image Updater Parallel E2E Tests", func() {

	Context("1-008-app-in-any-ns_test", func() {

		var (
			k8sClient      client.Client
			ctx            context.Context
			ns             *corev1.Namespace
			nsDev          *corev1.Namespace
			cleanupFunc    func()
			cleanupFuncDev func()
			imageUpdater   *imageUpdaterApi.ImageUpdater
			argoCD         *argov1beta1api.ArgoCD
		)

		BeforeEach(func() {
			fixture.EnsureParallelCleanSlate()

			k8sClient, _ = fixtureUtils.GetE2ETestKubeClient()
			ctx = context.Background()
		})

		AfterEach(func() {
			// Cleanup is best-effort. Issue deletes and give some time for controllers
			// to process, but don't fail the test if cleanup takes too long.

			if imageUpdater != nil {
				By("deleting ImageUpdater CR")
				_ = k8sClient.Delete(ctx, imageUpdater)
			}

			if argoCD != nil {
				By("deleting ArgoCD CR")
				_ = k8sClient.Delete(ctx, argoCD)
			}

			if cleanupFunc != nil {
				cleanupFunc()
			}

			if cleanupFuncDev != nil {
				cleanupFuncDev()
			}

			fixture.OutputDebugOnFail(ns)

		})

		It("ensures that Image Updater will update Argo CD Application in any namespace", func() {

			By("creating namespaces")
			ns, cleanupFunc = fixture.CreateRandomE2ETestNamespaceWithCleanupFunc()
			nsDev, cleanupFuncDev = fixture.CreateRandomE2ETestNamespaceWithCleanupFunc()

			By("updating argocd-operator deployment with ARGOCD_CLUSTER_CONFIG_NAMESPACES including random test namespace and restarting")
			operatorDeploy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "argocd-operator-controller-manager", Namespace: "argocd-operator-system"}}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(operatorDeploy), operatorDeploy)).To(Succeed())
			clusterConfigNamespaces := "argocd-operator-system, " + ns.Name
			for i := range operatorDeploy.Spec.Template.Spec.Containers {
				if operatorDeploy.Spec.Template.Spec.Containers[i].Name == "manager" {
					found := false
					for j := range operatorDeploy.Spec.Template.Spec.Containers[i].Env {
						if operatorDeploy.Spec.Template.Spec.Containers[i].Env[j].Name == "ARGOCD_CLUSTER_CONFIG_NAMESPACES" {
							operatorDeploy.Spec.Template.Spec.Containers[i].Env[j].Value = clusterConfigNamespaces
							found = true
							break
						}
					}
					if !found {
						operatorDeploy.Spec.Template.Spec.Containers[i].Env = append(operatorDeploy.Spec.Template.Spec.Containers[i].Env, corev1.EnvVar{Name: "ARGOCD_CLUSTER_CONFIG_NAMESPACES", Value: clusterConfigNamespaces})
					}
					break
				}
			}
			if operatorDeploy.Spec.Template.ObjectMeta.Annotations == nil {
				operatorDeploy.Spec.Template.ObjectMeta.Annotations = map[string]string{}
			}
			operatorDeploy.Spec.Template.ObjectMeta.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)
			Expect(k8sClient.Update(ctx, operatorDeploy)).To(Succeed())

			By("waiting for argocd-operator to be ready after restart")
			Eventually(func() int32 {
				d := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "argocd-operator-controller-manager", Namespace: "argocd-operator-system"}}
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(d), d); err != nil {
					return 0
				}
				return d.Status.ReadyReplicas
			}, "2m", "3s").Should(BeEquivalentTo(1))

			By("creating simple namespace-scoped Argo CD instance with image updater enabled")
			argoCD = &argov1beta1api.ArgoCD{
				ObjectMeta: metav1.ObjectMeta{Name: "argocd", Namespace: ns.Name},
				Spec: argov1beta1api.ArgoCDSpec{
					SourceNamespaces: []string{
						nsDev.Name,
					},
					CmdParams: map[string]string{
						"application.namespaces": nsDev.Name,
					},
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

			By("creating AppProject")
			appProject := &appv1alpha1.AppProject{
				ObjectMeta: metav1.ObjectMeta{
					Name:      nsDev.Name,
					Namespace: ns.Name,
				},
				Spec: appv1alpha1.AppProjectSpec{
					SourceRepos:      []string{"*"},
					SourceNamespaces: []string{nsDev.Name},
					Destinations: []appv1alpha1.ApplicationDestination{{
						Server:    "*",
						Namespace: "*",
					}},
					ClusterResourceWhitelist: []appv1alpha1.ClusterResourceRestrictionItem{{
						Group: "*",
						Kind:  "*",
					}},
				},
			}
			Expect(k8sClient.Create(ctx, appProject)).To(Succeed())

			By("creating Application")
			app := &appv1alpha1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "app-01",
					Namespace: nsDev.Name,
				},
				Spec: appv1alpha1.ApplicationSpec{
					Project: nsDev.Name,
					Source: &appv1alpha1.ApplicationSource{
						RepoURL:        "https://github.com/argoproj-labs/argocd-image-updater/",
						Path:           "test/e2e/testdata/005-public-guestbook",
						TargetRevision: "HEAD",
					},
					Destination: appv1alpha1.ApplicationDestination{
						Server:    "https://kubernetes.default.svc",
						Namespace: nsDev.Name,
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
					Name:      "image-updater",
					Namespace: nsDev.Name,
				},
				Spec: imageUpdaterApi.ImageUpdaterSpec{
					ApplicationRefs: []imageUpdaterApi.ApplicationRef{
						{
							NamePattern: "app*",
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

			By("ensuring that the Application image has `29437546.0` version after update")
			triggerRefresh := iuFixture.TriggerArgoCDRefresh(ctx, k8sClient, app)
			Eventually(func() string {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(app), app)

				if err != nil {
					return "" // Let Eventually retry on error
				}

				// Trigger ArgoCD refresh periodically to force immediate git check
				triggerRefresh()

				// Nil-safe check: The Kustomize block is only added by the Image Updater after its first run.
				// We must check that it and its Images field exist before trying to access them.
				if app.Spec.Source.Kustomize != nil && len(app.Spec.Source.Kustomize.Images) > 0 {
					return string(app.Spec.Source.Kustomize.Images[0])
				}

				// Return an empty string to signify the condition is not yet met.
				return ""
			}, "5m", "3s").Should(Equal("quay.io/dkarpele/my-guestbook:29437546.0"))
		})
	})
})
