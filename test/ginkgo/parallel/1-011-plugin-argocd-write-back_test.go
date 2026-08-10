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

	"github.com/argoproj/argo-cd/gitops-engine/pkg/health"
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

	// This test verifies that Image Updater correctly updates plugin-type Applications
	// using the argocd (default) write-back method with manifestTargets.plugin.
	// A CMP sidecar is configured on the repo-server to handle the plugin source type.
	Context("1-011-plugin-argocd-write-back-test", func() {

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
			fixture.OutputDebugOnFail(ns)

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
		})

		It("ensures that Image Updater will update plugin-type Argo CD Application using argocd write-back", func() {

			By("creating namespace and local git repo")
			ns, cleanupFunc = fixture.CreateRandomE2ETestNamespaceWithCleanupFunc()

			iuFixture.CreateLocalGitRepo(ctx, k8sClient, ns.Name)

			By("waiting for local git repo to be ready")
			gitDepl := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: iuFixture.Name, Namespace: ns.Name}}
			Eventually(gitDepl).Should(k8sFixture.ExistByName())
			Eventually(gitDepl, "2m", "3s").Should(deplFixture.HaveReadyReplicas(1), "git repo server was not ready")

			By("creating CMP plugin ConfigMap")
			cmpConfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cmp-plugin",
					Namespace: ns.Name,
				},
				Data: map[string]string{
					"plugin.yaml": `apiVersion: argoproj.io/v1alpha1
kind: ConfigManagementPlugin
metadata:
  name: e2e-plugin
spec:
  generate:
    command: [sh, -c]
    args:
      - |
        cat <<EOF
        apiVersion: apps/v1
        kind: Deployment
        metadata:
          name: plugin-app
        spec:
          replicas: 1
          selector:
            matchLabels:
              app: plugin-app
          template:
            metadata:
              labels:
                app: plugin-app
            spec:
              containers:
              - name: test
                image: ${ARGOCD_ENV_IMAGE_NAME}:${ARGOCD_ENV_IMAGE_TAG}
        EOF
  discover:
    fileName: "plugin-app.yaml"
`,
				},
			}
			Expect(k8sClient.Create(ctx, cmpConfigMap)).To(Succeed())

			By("creating Argo CD instance with image updater and CMP sidecar")
			argoCD = &argov1beta1api.ArgoCD{
				ObjectMeta: metav1.ObjectMeta{Name: "argocd", Namespace: ns.Name},
				Spec: argov1beta1api.ArgoCDSpec{
					Repo: argov1beta1api.ArgoCDRepoSpec{
						SidecarContainers: []corev1.Container{
							{
								Name:    "e2e-plugin",
								Command: []string{"/var/run/argocd/argocd-cmp-server"},
								Image:   "busybox",
								SecurityContext: &corev1.SecurityContext{
									RunAsNonRoot: new(true),
									RunAsUser:    new(int64(999)),
								},
								VolumeMounts: []corev1.VolumeMount{
									{Name: "var-files", MountPath: "/var/run/argocd"},
									{Name: "plugins", MountPath: "/home/argocd/cmp-server/plugins"},
									{Name: "cmp-tmp", MountPath: "/tmp"},
									{Name: "cmp-plugin", MountPath: "/home/argocd/cmp-server/config/plugin.yaml", SubPath: "plugin.yaml"},
								},
							},
						},
						Volumes: []corev1.Volume{
							{
								Name: "cmp-plugin",
								VolumeSource: corev1.VolumeSource{
									ConfigMap: &corev1.ConfigMapVolumeSource{
										LocalObjectReference: corev1.LocalObjectReference{Name: "cmp-plugin"},
									},
								},
							},
							{
								Name:         "cmp-tmp",
								VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
							},
						},
					},
					ImageUpdater: argov1beta1api.ArgoCDImageUpdaterSpec{
						Env: []corev1.EnvVar{
							{Name: "IMAGE_UPDATER_LOGLEVEL", Value: "trace"},
							{Name: "IMAGE_UPDATER_INTERVAL", Value: "0"},
						},
						Enabled: true,
					},
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

			By("creating plugin-type Application")
			pluginEnvName := "IMAGE_NAME"
			pluginEnvTag := "IMAGE_TAG"
			gitRepoURL := fmt.Sprintf("https://%s.%s.svc.cluster.local:8081/testdata.git", iuFixture.Name, ns.Name)
			app := &appv1alpha1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "plugin-app",
					Namespace: ns.Name,
				},
				Spec: appv1alpha1.ApplicationSpec{
					Project: "default",
					Source: &appv1alpha1.ApplicationSource{
						RepoURL:        gitRepoURL,
						Path:           "1-011-plugin-argocd-write-back-test",
						TargetRevision: "HEAD",
						Plugin: &appv1alpha1.ApplicationSourcePlugin{
							Env: appv1alpha1.Env{
								&appv1alpha1.EnvEntry{Name: pluginEnvName, Value: "quay.io/dkarpele/my-guestbook"},
								&appv1alpha1.EnvEntry{Name: pluginEnvTag, Value: "1.0.0"},
							},
						},
					},
					Destination: appv1alpha1.ApplicationDestination{
						Server:    "https://kubernetes.default.svc",
						Namespace: ns.Name,
					},
					SyncPolicy: &appv1alpha1.SyncPolicy{
						Automated: &appv1alpha1.SyncPolicyAutomated{
							Prune: new(true),
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, app)).To(Succeed())

			By("verifying deploying the Application succeeded")
			Eventually(app, "4m", "3s").Should(applicationFixture.HaveHealthStatusCode(health.HealthStatusHealthy))
			Eventually(app, "4m", "3s").Should(applicationFixture.HaveSyncStatusCode(appv1alpha1.SyncStatusCodeSynced))

			By("creating ImageUpdater CR with manifestTargets.plugin")
			updateStrategy := "semver"

			imageUpdater = &imageUpdaterApi.ImageUpdater{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "image-updater",
					Namespace: ns.Name,
				},
				Spec: imageUpdaterApi.ImageUpdaterSpec{
					CommonUpdateSettings: &imageUpdaterApi.CommonUpdateSettings{
						UpdateStrategy: &updateStrategy,
					},
					ApplicationRefs: []imageUpdaterApi.ApplicationRef{
						{
							NamePattern: "plugin-*",
							Images: []imageUpdaterApi.ImageConfig{
								{
									Alias:     "guestbook",
									ImageName: "quay.io/dkarpele/my-guestbook:~29437546.0",
									ManifestTarget: &imageUpdaterApi.ManifestTarget{
										Plugin: &imageUpdaterApi.PluginTarget{
											Name: &pluginEnvName,
											Tag:  &pluginEnvTag,
										},
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, imageUpdater)).To(Succeed())

			By("ensuring that the Application plugin env vars and status image are updated")
			triggerRefresh := iuFixture.TriggerArgoCDRefresh(ctx, k8sClient, app)
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(app), app)
				g.Expect(err).NotTo(HaveOccurred())

				triggerRefresh()

				g.Expect(app.Spec.Source).NotTo(BeNil())
				g.Expect(app.Spec.Source.Plugin).NotTo(BeNil())
				var tagValue string
				for _, e := range app.Spec.Source.Plugin.Env {
					if e != nil && e.Name == pluginEnvTag {
						tagValue = e.Value
					}
				}
				g.Expect(tagValue).To(Equal("29437546.0"))

				g.Expect(app.Status.Summary.Images).NotTo(BeEmpty())
				g.Expect(app.Status.Summary.Images[0]).To(Equal("quay.io/dkarpele/my-guestbook:29437546.0"))
			}, "5m", "3s").Should(Succeed())
		})
	})
})
