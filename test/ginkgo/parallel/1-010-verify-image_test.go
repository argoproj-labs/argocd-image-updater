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
	"os"

	"github.com/argoproj/argo-cd/gitops-engine/pkg/health"
	appv1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
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

	Context("1-010-verify-image_test", func() {

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
			// Collect debug info BEFORE cleanup so pod logs are still available.
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

		It("verifies that image update is skipped for unsigned images and succeeds for signed images", func() {

			By("creating simple namespace-scoped Argo CD instance with image updater enabled")
			ns, cleanupFunc = fixture.CreateRandomE2ETestNamespaceWithCleanupFunc()

			By("creating argocd-image-updater-config ConfigMap with local registry config")
			// The image-updater controller reads this ConfigMap at startup. Creating it before
			// the ArgoCD CR ensures the controller picks it up on first boot without a restart.
			// prefix: 127.0.0.1:30000 maps image names in CRs to the in-cluster registry API URL.
			// api_url uses the cluster-internal service DNS so the image-updater pod (running
			// inside k3d) can reach the registry on both Linux CI and macOS without relying on
			// host.docker.internal, which is not available in Linux k3d containers.
			// insecure: true skips TLS verification for the self-signed registry certificate.
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "argocd-image-updater-config",
					Namespace: ns.Name,
				},
				Data: map[string]string{
					"registries.conf": `registries:
- name: Local Registry
  api_url: https://e2e-registry-public.argocd-operator-system.svc.cluster.local
  prefix: 127.0.0.1:30000
  insecure: true
`,
				},
			}
			Expect(k8sClient.Create(ctx, configMap)).To(Succeed())

			By("reading cosign public key and creating Secret in test namespace")
			cosignPubKey, err := os.ReadFile("../prereqs/assets/cosign/cosign.pub")
			Expect(err).ToNot(HaveOccurred(), "cosign.pub not found — run 'make generate-cosign-keypair' first")
			cosignSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cosign-pubkey",
					Namespace: ns.Name,
				},
				Data: map[string][]byte{
					"cosign.pub": cosignPubKey,
				},
			}
			Expect(k8sClient.Create(ctx, cosignSecret)).To(Succeed())

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
								Value: "15s",
							},
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

			By("creating Application with initial basic image")
			app := &appv1alpha1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "app-01",
					Namespace: ns.Name,
				},
				Spec: appv1alpha1.ApplicationSpec{
					Project: "default",
					Source: &appv1alpha1.ApplicationSource{
						RepoURL:        fmt.Sprintf("https://%s.%s.svc.cluster.local:8081/testdata.git", iuFixture.Name, ns.Name),
						Path:           "1-010-verify-image-test",
						TargetRevision: "HEAD",
					},
					Destination: appv1alpha1.ApplicationDestination{
						Server:    "https://kubernetes.default.svc",
						Namespace: ns.Name,
					},
					SyncPolicy: &appv1alpha1.SyncPolicy{
						Automated: &appv1alpha1.SyncPolicyAutomated{
							Prune: ptr.To(true),
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, app)).To(Succeed())

			By("verifying the Application synced and is healthy with the basic image")
			Eventually(app, "4m", "3s").Should(applicationFixture.HaveHealthStatusCode(health.HealthStatusHealthy))
			Eventually(app, "4m", "3s").Should(applicationFixture.HaveSyncStatusCode(appv1alpha1.SyncStatusCodeSynced))

			// ---------------------------------------------------------------------------
			// Phase 1: unsigned image — signature verification must block the update
			// ---------------------------------------------------------------------------

			By("creating ImageUpdater CR targeting 1.0.1 (unsigned — verification must fail → no update)")
			// Semver constraint "1.0.1" matches exactly tag 1.0.1 (unsigned).
			// The sha256-<hex> cosign signature tag is not valid semver and is silently ignored.
			// Verification fails because 1.0.1 has no cosign signature → image stays at 1.0.0.
			updateStrategy := "semver"
			forceUpdate := false
			verificationEnabled := true
			method := fmt.Sprintf("git:secret:%s/%s", ns.Name, iuFixture.Name)
			branch := "master"
			repository := fmt.Sprintf("https://%s.%s.svc.cluster.local:8081/testdata.git", iuFixture.Name, ns.Name)

			imageUpdater = &imageUpdaterApi.ImageUpdater{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "image-updater",
					Namespace: ns.Name,
				},
				Spec: imageUpdaterApi.ImageUpdaterSpec{
					ImagesVerification: &imageUpdaterApi.ImagesVerification{
						Enabled: &verificationEnabled,
						CosignKey: &imageUpdaterApi.SecretRef{
							SecretName: "cosign-pubkey",
							Key:        "cosign.pub",
						},
					},
					CommonUpdateSettings: &imageUpdaterApi.CommonUpdateSettings{
						UpdateStrategy: &updateStrategy,
						ForceUpdate:    &forceUpdate,
					},
					WriteBackConfig: &imageUpdaterApi.WriteBackConfig{
						Method: &method,
						GitConfig: &imageUpdaterApi.GitConfig{
							Branch:     &branch,
							Repository: &repository,
						},
					},
					ApplicationRefs: []imageUpdaterApi.ApplicationRef{
						{
							NamePattern: "app*",
							Images: []imageUpdaterApi.ImageConfig{
								{
									Alias:     "test",
									ImageName: "127.0.0.1:30000/test-image:1.0.1",
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, imageUpdater)).To(Succeed())

			By("waiting for ImageUpdater controller to process the CR at least once")
			Eventually(func() int32 {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(imageUpdater), imageUpdater); err != nil {
					return 0
				}
				return imageUpdater.Status.ApplicationsMatched
			}, "2m", "3s").Should(BeNumerically(">", 0))

			By("ensuring image stays at '1.0.0' — unsigned 1.0.1 must not pass verification")
			triggerRefresh := iuFixture.TriggerArgoCDRefresh(ctx, k8sClient, app)
			Consistently(func() string {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(app), app); err != nil {
					return ""
				}
				triggerRefresh()
				if len(app.Status.Summary.Images) > 0 {
					return app.Status.Summary.Images[0]
				}
				return ""
			}, "30s", "5s").Should(Equal("127.0.0.1:30000/test-image:1.0.0"))

			By("deleting phase-1 ImageUpdater CR")
			Expect(k8sClient.Delete(ctx, imageUpdater)).To(Succeed())
			Eventually(imageUpdater, "2m", "3s").Should(k8sFixture.NotExistByName())
			imageUpdater = nil

			// ---------------------------------------------------------------------------
			// Phase 2: signed image — signature verification must allow the update
			// ---------------------------------------------------------------------------

			By("creating ImageUpdater CR targeting ~1.0 range (1.0.2 is signed — verification must succeed)")
			// Semver constraint "~1.0" allows patch updates: >=1.0.0, <1.1.0.
			// Available semver tags: 1.0.0, 1.0.1, 1.0.2  (sha256-<hex> is not valid semver, ignored).
			// Semver picks 1.0.2 as the highest. 1.0.2 is signed → verification passes → update to 1.0.2.

			imageUpdater = &imageUpdaterApi.ImageUpdater{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "image-updater",
					Namespace: ns.Name,
				},
				Spec: imageUpdaterApi.ImageUpdaterSpec{
					ImagesVerification: &imageUpdaterApi.ImagesVerification{
						Enabled: &verificationEnabled,
						CosignKey: &imageUpdaterApi.SecretRef{
							SecretName: "cosign-pubkey",
							Key:        "cosign.pub",
						},
					},
					CommonUpdateSettings: &imageUpdaterApi.CommonUpdateSettings{
						UpdateStrategy: &updateStrategy,
						ForceUpdate:    &forceUpdate,
					},
					WriteBackConfig: &imageUpdaterApi.WriteBackConfig{
						Method: &method,
						GitConfig: &imageUpdaterApi.GitConfig{
							Branch:     &branch,
							Repository: &repository,
						},
					},
					ApplicationRefs: []imageUpdaterApi.ApplicationRef{
						{
							NamePattern: "app*",
							Images: []imageUpdaterApi.ImageConfig{
								{
									Alias:     "test",
									ImageName: "127.0.0.1:30000/test-image:~1.0",
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, imageUpdater)).To(Succeed())

			By("waiting for ImageUpdater controller to process Phase-2 CR at least once")
			Eventually(func() int32 {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(imageUpdater), imageUpdater); err != nil {
					return 0
				}
				return imageUpdater.Status.ApplicationsMatched
			}, "2m", "3s").Should(BeNumerically(">", 0))

			By("ensuring that the Application image is updated to '1.0.2' after verification succeeds")
			triggerRefresh2 := iuFixture.TriggerArgoCDRefresh(ctx, k8sClient, app)
			Eventually(func() string {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(app), app); err != nil {
					return ""
				}
				triggerRefresh2()
				if len(app.Status.Summary.Images) > 0 {
					return app.Status.Summary.Images[0]
				}
				return ""
			}, "5m", "3s").Should(Equal("127.0.0.1:30000/test-image:1.0.2"))
		})
	})
})
