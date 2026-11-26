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

	imageUpdaterAPI "github.com/argoproj-labs/argocd-image-updater/api/v1alpha1"
	"github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture"
	argocdFixture "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/argocd"
	deplFixture "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/deployment"
	iuFixture "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/imageupdater"
	k8sFixture "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/k8s"
	fixtureUtils "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/utils"
	operatorAPI "github.com/argoproj-labs/argocd-operator/api/v1beta1"
	appAPI "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("1-013 CEL expression strategy", func() {
	DescribeTable("updates the Application and Deployment using",
		func(useAnnotations bool) {
			const (
				repository    = "127.0.0.1:30000/expression-test"
				initialImage  = repository + ":build-9"
				expectedImage = repository + ":build-100"
				filter        = `tag.name.matches("^build-[0-9]+$")`
				sortKey       = `int(tag.name.split("-")[1])`
				annotation    = "argocd-image-updater.argoproj.io/"
			)
			fixture.EnsureParallelCleanSlate()
			ctx := context.Background()
			k8sClient, _, err := fixtureUtils.GetE2ETestKubeClientWithError()
			Expect(err).NotTo(HaveOccurred())
			ns, cleanup := fixture.CreateRandomE2ETestNamespaceWithCleanupFunc()
			DeferCleanup(cleanup)

			argoCD := &operatorAPI.ArgoCD{
				ObjectMeta: metav1.ObjectMeta{Name: "argocd", Namespace: ns.Name},
				Spec: operatorAPI.ArgoCDSpec{
					ImageUpdater: operatorAPI.ArgoCDImageUpdaterSpec{
						Enabled: true,
						Env:     []corev1.EnvVar{{Name: "IMAGE_UPDATER_INTERVAL", Value: "15s"}},
					},
				},
			}
			updater := &imageUpdaterAPI.ImageUpdater{
				ObjectMeta: metav1.ObjectMeta{Name: "cel-updater", Namespace: ns.Name},
			}
			DeferCleanup(func() {
				fixture.OutputDebugOnFail(ns)
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, updater))).To(Succeed())
				Eventually(updater, "2m", "3s").Should(k8sFixture.NotExistByName())
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, argoCD))).To(Succeed())
			})

			By("configuring the in-cluster registry and local Git repository")
			Expect(k8sClient.Create(ctx, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "argocd-image-updater-config", Namespace: ns.Name},
				Data: map[string]string{"registries.conf": `registries:
- name: Local Registry
  api_url: https://e2e-registry-public.argocd-operator-system.svc.cluster.local
  prefix: 127.0.0.1:30000
  insecure: true
`},
			})).To(Succeed())
			iuFixture.CreateLocalGitRepo(ctx, k8sClient, ns.Name)
			gitDeployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: iuFixture.Name, Namespace: ns.Name}}
			Eventually(gitDeployment, "2m", "3s").Should(deplFixture.HaveReadyReplicas(1))

			By("starting Argo CD and Image Updater")
			Expect(k8sClient.Create(ctx, argoCD)).To(Succeed())
			Eventually(argoCD, "5m", "3s").Should(argocdFixture.BeAvailable())
			controller := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "argocd-argocd-image-updater-controller", Namespace: ns.Name}}
			Eventually(controller, "3m", "3s").Should(deplFixture.HaveReadyReplicas(1))
			iuFixture.WaitForControllerRBAC(ctx, k8sClient, ns.Name)

			app := &appAPI.Application{
				ObjectMeta: metav1.ObjectMeta{Name: "cel-demo", Namespace: ns.Name},
				Spec: appAPI.ApplicationSpec{
					Project: "default",
					Source: &appAPI.ApplicationSource{
						RepoURL:        fmt.Sprintf("https://%s.%s.svc.cluster.local:8081/testdata.git", iuFixture.Name, ns.Name),
						Path:           "1-013-expression-strategy",
						TargetRevision: "HEAD",
					},
					Destination: appAPI.ApplicationDestination{Server: "https://kubernetes.default.svc", Namespace: ns.Name},
					SyncPolicy:  &appAPI.SyncPolicy{Automated: &appAPI.SyncPolicyAutomated{}},
				},
			}
			ref := imageUpdaterAPI.ApplicationRef{NamePattern: app.Name}
			if useAnnotations {
				ref.UseAnnotations = ptr.To(true)
				app.Annotations = map[string]string{
					annotation + "image-list":               "demo=" + repository,
					annotation + "demo.update-strategy":     "expression",
					annotation + "demo.expression-filter":   filter,
					annotation + "demo.expression-sort-key": sortKey,
				}
			} else {
				ref.Images = []imageUpdaterAPI.ImageConfig{{
					Alias: "demo", ImageName: repository,
					CommonUpdateSettings: &imageUpdaterAPI.CommonUpdateSettings{
						UpdateStrategy:     ptr.To("expression"),
						ExpressionStrategy: &imageUpdaterAPI.ExpressionStrategy{Filter: filter, SortKey: sortKey},
					},
				}}
			}

			By("syncing the initial build-9 image before creating ImageUpdater")
			Expect(k8sClient.Create(ctx, app)).To(Succeed())
			deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: app.Name, Namespace: ns.Name}}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(app), app)).To(Succeed())
				g.Expect(app.Status.Summary.Images).To(ContainElement(initialImage))
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(deployment), deployment)).To(Succeed())
				g.Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
				g.Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(Equal(initialImage))
			}, "4m", "3s").Should(Succeed())
			Eventually(deployment, "3m", "3s").Should(deplFixture.HaveReadyReplicas(1))

			By("letting Image Updater rank the registry tags using CEL")
			updater.Spec.ApplicationRefs = []imageUpdaterAPI.ApplicationRef{ref}
			Expect(k8sClient.Create(ctx, updater)).To(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(app), app)).To(Succeed())
				g.Expect(app.Spec.Source.Kustomize).NotTo(BeNil())
				g.Expect(app.Spec.Source.Kustomize.Images).To(ContainElement(appAPI.KustomizeImage(expectedImage)))
			}, "5m", "3s").Should(Succeed())

			By("verifying that Argo CD rolled out the selected image")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(deployment), deployment)).To(Succeed())
				g.Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
				g.Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(Equal(expectedImage))
				g.Expect(deployment.Status.ObservedGeneration).To(Equal(deployment.Generation))
				g.Expect(deployment.Status.UpdatedReplicas).To(Equal(int32(1)))
				g.Expect(deployment.Status.AvailableReplicas).To(Equal(int32(1)))
				g.Expect(deployment.Status.Replicas).To(Equal(int32(1)))
			}, "4m", "3s").Should(Succeed())
		},
		Entry("ImageUpdater CR settings", false),
		Entry("Application annotations", true),
	)
})
