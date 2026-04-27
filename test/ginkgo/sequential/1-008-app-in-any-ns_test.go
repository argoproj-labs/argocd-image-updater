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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	imageUpdaterApi "github.com/argoproj-labs/argocd-image-updater/api/v1alpha1"

	argov1beta1api "github.com/argoproj-labs/argocd-operator/api/v1beta1"

	"github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture"
	argocdFixture "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/argocd"
	deplFixture "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/deployment"
	k8sFixture "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/k8s"
	ssFixture "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/statefulset"
	fixtureUtils "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/utils"
)

var _ = Describe("ArgoCD Image Updater Sequential E2E Tests", func() {

	Context("1-008-app-in-any-ns_test", func() {

		var (
			k8sClient      client.Client
			ctx            context.Context
			ns             *corev1.Namespace
			nsDev          *corev1.Namespace
			nsQE           *corev1.Namespace
			cleanupFunc    func()
			cleanupFuncDev func()
			cleanupFuncQE  func()
			imageUpdater   *imageUpdaterApi.ImageUpdater
			imageUpdaterQE *imageUpdaterApi.ImageUpdater
			argoCD         *argov1beta1api.ArgoCD
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
				if operatorDeploy.Spec.Template.ObjectMeta.Annotations == nil {
					operatorDeploy.Spec.Template.ObjectMeta.Annotations = map[string]string{}
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

		It("ensures that Image Updater will update Argo CD Application in any namespace", func() {

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

			By("creating simple namespace-scoped Argo CD instance with image updater enabled")
			argoCD = &argov1beta1api.ArgoCD{
				ObjectMeta: metav1.ObjectMeta{Name: "argocd", Namespace: ns.Name},
				Spec: argov1beta1api.ArgoCDSpec{
					SourceNamespaces: []string{
						nsDev.Name,
						nsQE.Name,
					},
					CmdParams: map[string]string{
						"application.namespaces": nsDev.Name + "," + nsQE.Name,
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
								Value: nsDev.Name + "," + nsQE.Name,
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

			// TODO: Remove after ArgoCD Operator fix https://github.com/argoproj-labs/argocd-operator/pull/2172
			createImageUpdaterRoleAndBinding(ctx, k8sClient, nsDev.Name, ns.Name)
			createImageUpdaterRoleAndBinding(ctx, k8sClient, nsQE.Name, ns.Name)

			createAppProject(ctx, k8sClient, nsDev.Name, ns.Name)
			createAppProject(ctx, k8sClient, nsQE.Name, ns.Name)

			app := createGuestbookApp(ctx, k8sClient, nsDev.Name, nsDev.Name)
			appQE := createGuestbookApp(ctx, k8sClient, nsQE.Name, nsQE.Name)

			imageUpdater = createImageUpdaterAndVerify(ctx, k8sClient, nsDev.Name, app)
			imageUpdaterQE = createImageUpdaterAndVerify(ctx, k8sClient, nsQE.Name, appQE)
		})
	})
})
