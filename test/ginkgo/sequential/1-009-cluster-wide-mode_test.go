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

	appv1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/argoproj/gitops-engine/pkg/health"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	applicationFixture "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/application"

	"sigs.k8s.io/controller-runtime/pkg/client"

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

var _ = Describe("ArgoCD Image Updater Sequential E2E Tests", func() {

	Context("1-009-cluster-wide-mode_test", func() {

		var (
			k8sClient           client.Client
			ctx                 context.Context
			ns                  *corev1.Namespace
			nsDev               *corev1.Namespace
			nsQE                *corev1.Namespace
			cleanupFunc         func()
			cleanupFuncDev      func()
			cleanupFuncQE       func()
			imageUpdater        *imageUpdaterApi.ImageUpdater
			imageUpdaterQE      *imageUpdaterApi.ImageUpdater
			argoCD              *argov1beta1api.ArgoCD
			clusterRoleName     string
			clusterRoleBinding  *rbacv1.ClusterRoleBinding
		)

		BeforeEach(func() {
			fixture.EnsureSequentialCleanSlate()

			k8sClient, _ = fixtureUtils.GetE2ETestKubeClient()
			ctx = context.Background()
		})

		AfterEach(func() {
			// Cleanup is best-effort. Issue deletes and give some time for controllers
			// to process, but don't fail the test if cleanup takes too long.
			By("restoring argocd-operator deployment ARGOCD_CLUSTER_CONFIG_NAMESPACES to default")
			operatorDeploy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "argocd-operator-controller-manager", Namespace: "argocd-operator-system"}}
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(operatorDeploy), operatorDeploy); err == nil {
				for i := range operatorDeploy.Spec.Template.Spec.Containers {
					if operatorDeploy.Spec.Template.Spec.Containers[i].Name == "manager" {
						for j := range operatorDeploy.Spec.Template.Spec.Containers[i].Env {
							if operatorDeploy.Spec.Template.Spec.Containers[i].Env[j].Name == "ARGOCD_CLUSTER_CONFIG_NAMESPACES" {
								operatorDeploy.Spec.Template.Spec.Containers[i].Env[j].Value = "argocd-operator-system"
								break
							}
						}
						break
					}
				}
				operatorDeploy.Spec.Template.ObjectMeta.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)
				_ = k8sClient.Update(ctx, operatorDeploy)
			}

			if imageUpdater != nil {
				By("deleting ImageUpdater CR")
				_ = k8sClient.Delete(ctx, imageUpdater)
			}

			if imageUpdaterQE != nil {
				By("deleting ImageUpdater CR for QE namespace")
				_ = k8sClient.Delete(ctx, imageUpdaterQE)
			}

			if argoCD != nil {
				By("deleting ArgoCD CR")
				_ = k8sClient.Delete(ctx, argoCD)
			}

			if clusterRoleBinding != nil {
				By("deleting ClusterRoleBinding")
				_ = k8sClient.Delete(ctx, clusterRoleBinding)
			}

			if clusterRoleName != "" {
				By("deleting ClusterRole")
				_ = k8sClient.Delete(ctx, &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: clusterRoleName}})
			}

			if cleanupFunc != nil {
				cleanupFunc()
			}

			if cleanupFuncDev != nil {
				cleanupFuncDev()
			}

			if cleanupFuncQE != nil {
				cleanupFuncQE()
			}

			fixture.OutputDebugOnFail(ns)
			fixture.OutputDebugOnFail(nsDev)
			fixture.OutputDebugOnFail(nsQE)
		})

		It("ensures that Image Updater in cluster-wide mode will update Argo CD Applications across namespaces", func() {

			By("creating namespaces")
			ns, cleanupFunc = fixture.CreateRandomE2ETestNamespaceWithCleanupFunc()
			nsDev, cleanupFuncDev = fixture.CreateRandomE2ETestNamespaceWithCleanupFunc()
			nsQE, cleanupFuncQE = fixture.CreateRandomE2ETestNamespaceWithCleanupFunc()

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
			Eventually(
				&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
					Name:      "argocd-operator-controller-manager",
					Namespace: "argocd-operator-system",
				}},
				"2m",
				"3s",
			).Should(deplFixture.HaveReadyReplicas(1))

			By("creating simple namespace-scoped Argo CD instance with image updater enabled in cluster-wide mode")
			argoCD = &argov1beta1api.ArgoCD{
				ObjectMeta: metav1.ObjectMeta{Name: "argocd", Namespace: ns.Name},
				Spec: argov1beta1api.ArgoCDSpec{
					SourceNamespaces: []string{
						nsDev.Name,
						nsQE.Name,
					},
					CmdParams: map[string]string{
						"application.namespaces": "*",
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
							{
								Name:  "IMAGE_UPDATER_WATCH_NAMESPACES",
								Value: "*",
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

			// TODO: ClusterRole+CRB creation should be removed after updating to ArgoCD Operator with the fix https://github.com/argoproj-labs/argocd-operator/pull/2172
			By("creating ClusterRole and ClusterRoleBinding for Image Updater ServiceAccount (cluster-wide mode)")
			// Use the ArgoCD instance namespace as a suffix to keep names unique across parallel test runs.
			clusterRoleName = "argocd-image-updater-manager-role-" + ns.Name
			clusterRole := &rbacv1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{
					Name: clusterRoleName,
				},
				Rules: []rbacv1.PolicyRule{
					{APIGroups: []string{""}, Resources: []string{"events"}, Verbs: []string{"create"}},
					{APIGroups: []string{"argocd-image-updater.argoproj.io"}, Resources: []string{"imageupdaters"}, Verbs: []string{"create", "delete", "get", "list", "patch", "update", "watch"}},
					{APIGroups: []string{"argocd-image-updater.argoproj.io"}, Resources: []string{"imageupdaters/finalizers"}, Verbs: []string{"update"}},
					{APIGroups: []string{"argocd-image-updater.argoproj.io"}, Resources: []string{"imageupdaters/status"}, Verbs: []string{"get", "patch", "update"}},
					{APIGroups: []string{"argoproj.io"}, Resources: []string{"applications"}, Verbs: []string{"get", "list", "patch", "update", "watch"}},
				},
			}
			Expect(k8sClient.Create(ctx, clusterRole)).To(Succeed())

			clusterRoleBinding = &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: "argocd-image-updater-manager-rolebinding-" + ns.Name,
				},
				RoleRef: rbacv1.RoleRef{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "ClusterRole",
					Name:     clusterRoleName,
				},
				Subjects: []rbacv1.Subject{
					{
						Kind:      "ServiceAccount",
						Name:      "argocd-argocd-image-updater-controller",
						Namespace: ns.Name,
					},
				},
			}
			Expect(k8sClient.Create(ctx, clusterRoleBinding)).To(Succeed())

			By("creating AppProject for dev namespace")
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

			By("creating AppProject for QE namespace")
			appProjectQE := &appv1alpha1.AppProject{
				ObjectMeta: metav1.ObjectMeta{
					Name:      nsQE.Name,
					Namespace: ns.Name,
				},
				Spec: appv1alpha1.AppProjectSpec{
					SourceRepos:      []string{"*"},
					SourceNamespaces: []string{nsQE.Name},
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
			Expect(k8sClient.Create(ctx, appProjectQE)).To(Succeed())

			By("creating Application in dev namespace")
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

			By("verifying deploying the dev Application succeeded")
			Eventually(app, "4m", "3s").Should(applicationFixture.HaveHealthStatusCode(health.HealthStatusHealthy))
			Eventually(app, "4m", "3s").Should(applicationFixture.HaveSyncStatusCode(appv1alpha1.SyncStatusCodeSynced))

			By("creating Application in QE namespace")
			appQE := &appv1alpha1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "app-01",
					Namespace: nsQE.Name,
				},
				Spec: appv1alpha1.ApplicationSpec{
					Project: nsQE.Name,
					Source: &appv1alpha1.ApplicationSource{
						RepoURL:        "https://github.com/argoproj-labs/argocd-image-updater/",
						Path:           "test/e2e/testdata/005-public-guestbook",
						TargetRevision: "HEAD",
					},
					Destination: appv1alpha1.ApplicationDestination{
						Server:    "https://kubernetes.default.svc",
						Namespace: nsQE.Name,
					},
					SyncPolicy: &appv1alpha1.SyncPolicy{Automated: &appv1alpha1.SyncPolicyAutomated{}},
				},
			}
			Expect(k8sClient.Create(ctx, appQE)).To(Succeed())

			By("verifying deploying the QE Application succeeded")
			Eventually(appQE, "4m", "3s").Should(applicationFixture.HaveHealthStatusCode(health.HealthStatusHealthy))
			Eventually(appQE, "4m", "3s").Should(applicationFixture.HaveSyncStatusCode(appv1alpha1.SyncStatusCodeSynced))

			By("creating ImageUpdater CR in dev namespace")
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

			By("ensuring that the dev Application image has `29437546.0` version after update")
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

			By("creating ImageUpdater CR in QE namespace")
			updateStrategyQE := "semver"
			imageUpdaterQE = &imageUpdaterApi.ImageUpdater{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "image-updater",
					Namespace: nsQE.Name,
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
										UpdateStrategy: &updateStrategyQE,
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, imageUpdaterQE)).To(Succeed())

			By("ensuring that the QE Application image has `29437546.0` version after update")
			triggerRefreshQE := iuFixture.TriggerArgoCDRefresh(ctx, k8sClient, appQE)
			Eventually(func() string {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(appQE), appQE)

				if err != nil {
					return "" // Let Eventually retry on error
				}

				// Trigger ArgoCD refresh periodically to force immediate git check
				triggerRefreshQE()

				// Nil-safe check: The Kustomize block is only added by the Image Updater after its first run.
				// We must check that it and its Images field exist before trying to access them.
				if appQE.Spec.Source.Kustomize != nil && len(appQE.Spec.Source.Kustomize.Images) > 0 {
					return string(appQE.Spec.Source.Kustomize.Images[0])
				}

				// Return an empty string to signify the condition is not yet met.
				return ""
			}, "5m", "3s").Should(Equal("quay.io/dkarpele/my-guestbook:29437546.0"))
		})
	})
})
