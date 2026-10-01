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

// Package parallel contains E2E tests for custom CA certificate support in registries.
// This test suite validates the three ways to provide custom CA certificates for
// registries with self-signed TLS certificates (feature from PR #1743):
//
// 1. ca_data: Inline PEM-encoded certificate directly in registries.conf
// 2. ca_file: Reference a mounted certificate file path in registries.conf
// 3. Auto-discovery: Use argocd-tls-certs-cm with hostname-based key naming
//
// A fourth scenario is a negative control: with none of the three configured, the
// update must not happen, and must not happen because the certificate was rejected.
//
// All tests use the e2e-registry-public test registry deployed in argocd-operator-system
// namespace with a self-signed certificate.
package parallel

import (
	"context"
	"strings"

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

var _ = Describe("ArgoCD Image Updater Custom CA Certificate E2E Tests", func() {

	Context("1-012-custom-ca-registry_test", func() {

		var (
			k8sClient client.Client
			ctx       context.Context
		)

		BeforeEach(func() {
			fixture.EnsureParallelCleanSlate()

			k8sClient, _ = fixtureUtils.GetE2ETestKubeClient()
			ctx = context.Background()
		})

		// Helper function to extract the CA certificate the test registry serves.
		// The registry is self-signed, so its serving certificate doubles as the
		// trust anchor; it lives in the kubernetes.io/tls secret mounted by the
		// registry deployment (see prereqs/assets/generate-registry-tls-secrets.sh).
		getRegistryCA := func() string {
			By("extracting CA certificate from registry TLS secret")
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "e2e-registry-public-tls",
					Namespace: "argocd-operator-system",
				},
			}
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(secret), secret)
			Expect(err).ToNot(HaveOccurred(), "registry TLS secret not found - ensure e2e prerequisites are deployed")

			caCert := string(secret.Data[corev1.TLSCertKey])
			Expect(caCert).ToNot(BeEmpty(), corev1.TLSCertKey+" not found in registry TLS secret")
			return caCert
		}

		// Common test setup for all scenarios
		type testContext struct {
			ns           *corev1.Namespace
			cleanupFunc  func()
			imageUpdater *imageUpdaterApi.ImageUpdater
			argoCD       *argov1beta1api.ArgoCD
			app          *appv1alpha1.Application
		}

		cleanupTest := func(tc *testContext) {
			// Collect debug info BEFORE cleanup so controller pod logs are still available.
			fixture.OutputDebugOnFail(tc.ns)

			if tc.imageUpdater != nil {
				By("deleting ImageUpdater CR")
				_ = k8sClient.Delete(ctx, tc.imageUpdater)
				Eventually(tc.imageUpdater, "2m", "3s").Should(k8sFixture.NotExistByName())
			}

			if tc.argoCD != nil {
				By("deleting ArgoCD CR")
				_ = k8sClient.Delete(ctx, tc.argoCD)
				Eventually(tc.argoCD, "2m", "3s").Should(k8sFixture.NotExistByName())
			}

			if tc.cleanupFunc != nil {
				tc.cleanupFunc()
			}
		}

		setupTest := func(registriesConf string, extraSetup func(*testContext)) *testContext {
			tc := &testContext{}

			tc.ns, tc.cleanupFunc = fixture.CreateRandomE2ETestNamespaceWithCleanupFunc()

			// Register cleanup immediately: every step below can fail, and without this
			// the namespace, ArgoCD CR and ImageUpdater CR would leak and
			// OutputDebugOnFail would never run for the failure we most need to debug.
			DeferCleanup(func() {
				cleanupTest(tc)
			})

			// extraSetup runs before the ArgoCD CR is created, and that ordering is
			// load-bearing: registries.conf and argocd-tls-certs-cm are both read once,
			// at controller startup. Creating them here means the operator has already
			// mounted them by the time it creates the image updater Deployment, so the
			// first pod starts with the certificate in place and no restart is needed.
			if extraSetup != nil {
				extraSetup(tc)
			}

			By("creating argocd-image-updater-config ConfigMap with registry configuration")
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "argocd-image-updater-config",
					Namespace: tc.ns.Name,
				},
				Data: map[string]string{
					"registries.conf": registriesConf,
				},
			}
			Expect(k8sClient.Create(ctx, configMap)).To(Succeed())

			By("creating simple namespace-scoped Argo CD instance with image updater enabled")
			tc.argoCD = &argov1beta1api.ArgoCD{
				ObjectMeta: metav1.ObjectMeta{Name: "argocd", Namespace: tc.ns.Name},
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
						Enabled: true,
					},
				},
			}
			Expect(k8sClient.Create(ctx, tc.argoCD)).To(Succeed())

			By("waiting for ArgoCD CR to be reconciled and the instance to be ready")
			Eventually(tc.argoCD, "5m", "3s").Should(argocdFixture.BeAvailable())

			By("verifying all workloads are started")
			deploymentsShouldExist := []string{"argocd-redis", "argocd-server", "argocd-repo-server", "argocd-argocd-image-updater-controller"}
			for _, depl := range deploymentsShouldExist {
				depl := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: depl, Namespace: tc.ns.Name}}
				Eventually(depl).Should(k8sFixture.ExistByName())
				Eventually(depl).Should(deplFixture.HaveReplicas(1))
				Eventually(depl, "3m", "3s").Should(deplFixture.HaveReadyReplicas(1), depl.Name+" was not ready")
			}

			statefulSet := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "argocd-application-controller", Namespace: tc.ns.Name}}
			Eventually(statefulSet).Should(k8sFixture.ExistByName())
			Eventually(statefulSet).Should(ssFixture.HaveReplicas(1))
			Eventually(statefulSet, "3m", "3s").Should(ssFixture.HaveReadyReplicas(1))

			By("creating Application")
			tc.app = &appv1alpha1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "app-01",
					Namespace: tc.ns.Name,
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
						Namespace: tc.ns.Name,
					},
					SyncPolicy: &appv1alpha1.SyncPolicy{Automated: &appv1alpha1.SyncPolicyAutomated{}},
				},
			}
			Expect(k8sClient.Create(ctx, tc.app)).To(Succeed())

			By("verifying deploying the Application succeeded")
			Eventually(tc.app, "4m", "3s").Should(applicationFixture.HaveHealthStatusCode(health.HealthStatusHealthy))
			Eventually(tc.app, "4m", "3s").Should(applicationFixture.HaveSyncStatusCode(appv1alpha1.SyncStatusCodeSynced))

			By("creating ImageUpdater CR")
			updateStrategy := "semver"
			// The guestbook Application does not run 127.0.0.1:30000/test-image, and
			// GetImagesAndAliasesFromApplication drops configured images that are not
			// live in .status.summary.images unless force-update is set.
			forceUpdate := true
			tc.imageUpdater = &imageUpdaterApi.ImageUpdater{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "image-updater",
					Namespace: tc.ns.Name,
				},
				Spec: imageUpdaterApi.ImageUpdaterSpec{
					ApplicationRefs: []imageUpdaterApi.ApplicationRef{
						{
							NamePattern: "app*",
							Images: []imageUpdaterApi.ImageConfig{
								{
									Alias:     "test",
									ImageName: "127.0.0.1:30000/test-image:~1.0",
									CommonUpdateSettings: &imageUpdaterApi.CommonUpdateSettings{
										UpdateStrategy: &updateStrategy,
										ForceUpdate:    &forceUpdate,
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, tc.imageUpdater)).To(Succeed())

			return tc
		}

		verifyImageUpdate := func(tc *testContext) {
			By("ensuring that the Application image has been updated")
			// The registry holds 1.0.0/1.0.1/1.0.2, so the ~1.0 semver constraint
			// resolves deterministically to 1.0.2.
			triggerRefresh := iuFixture.TriggerArgoCDRefresh(ctx, k8sClient, tc.app)
			Eventually(func() string {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(tc.app), tc.app)
				if err != nil {
					return ""
				}

				triggerRefresh()

				if tc.app.Spec.Source.Kustomize != nil && len(tc.app.Spec.Source.Kustomize.Images) > 0 {
					return string(tc.app.Spec.Source.Kustomize.Images[0])
				}

				return ""
			}, "2m", "3s").Should(Equal("127.0.0.1:30000/test-image:1.0.2"), "image should have been updated to the newest 1.0.x version")
		}

		It("should connect to registry with self-signed cert using ca_data (inline certificate)", func() {
			caCert := getRegistryCA()

			// Indent the certificate for YAML multiline string
			lines := strings.Split(strings.TrimSpace(caCert), "\n")
			indentedCert := strings.Join(lines, "\n    ")

			registriesConf := `registries:
- name: Local Registry with ca_data
  api_url: https://e2e-registry-public.argocd-operator-system.svc.cluster.local
  prefix: 127.0.0.1:30000
  ca_data: |
    ` + indentedCert

			tc := setupTest(registriesConf, nil)

			verifyImageUpdate(tc)
		})

		It("should connect to registry with self-signed cert using ca_file (mounted in argocd-tls-certs-cm)", func() {
			caCert := getRegistryCA()

			// We'll use argocd-tls-certs-cm which the operator automatically mounts to /app/config/tls
			// Instead of using the hostname-based key (auto-discovery), we'll use a custom filename
			// and reference it explicitly via ca_file in registries.conf
			registriesConf := `registries:
- name: Local Registry with ca_file
  api_url: https://e2e-registry-public.argocd-operator-system.svc.cluster.local
  prefix: 127.0.0.1:30000
  ca_file: /app/config/tls/custom-registry-ca.crt
`

			tc := setupTest(registriesConf, func(tc *testContext) {
				By("creating argocd-tls-certs-cm with custom-named CA certificate file")
				// The argocd-operator automatically mounts argocd-tls-certs-cm to /app/config/tls
				// We use a custom key name (not hostname-based) and reference it via ca_file
				tlsCertsConfigMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "argocd-tls-certs-cm",
						Namespace: tc.ns.Name,
					},
					Data: map[string]string{
						"custom-registry-ca.crt": caCert,
					},
				}
				Expect(k8sClient.Create(ctx, tlsCertsConfigMap)).To(Succeed())
			})

			verifyImageUpdate(tc)
		})

		It("should connect to registry with self-signed cert using auto-discovery (argocd-tls-certs-cm)", func() {
			caCert := getRegistryCA()

			// For auto-discovery, we don't specify ca_file or ca_data in registries.conf
			// The registry scanner will automatically look for /app/config/tls/<hostname>
			registriesConf := `registries:
- name: Local Registry with auto-discovery
  api_url: https://e2e-registry-public.argocd-operator-system.svc.cluster.local
  prefix: 127.0.0.1:30000
`

			tc := setupTest(registriesConf, func(tc *testContext) {
				By("creating argocd-tls-certs-cm ConfigMap for auto-discovery")
				// The argocd-operator automatically mounts argocd-tls-certs-cm to /app/config/tls
				// The key should be the hostname from api_url
				// When api_url is parsed, the hostname is: e2e-registry-public.argocd-operator-system.svc.cluster.local
				tlsCertsConfigMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "argocd-tls-certs-cm",
						Namespace: tc.ns.Name,
					},
					Data: map[string]string{
						"e2e-registry-public.argocd-operator-system.svc.cluster.local": caCert,
					},
				}
				Expect(k8sClient.Create(ctx, tlsCertsConfigMap)).To(Succeed())
			})

			verifyImageUpdate(tc)
		})

		It("should not update image when no CA certificate is configured for the registry", func() {
			// Negative control for the three scenarios above. Each of them asserts that
			// an update succeeds once a CA is configured, which proves the feature is
			// present but not that it is doing the work: a regression that silently
			// stopped verifying certificates altogether would leave all three green.
			// This scenario removes the only thing that differs - the CA - and asserts
			// both that no update happens and that it does not happen *because* the
			// registry's certificate was rejected.
			registriesConf := `registries:
- name: Local Registry without a CA
  api_url: https://e2e-registry-public.argocd-operator-system.svc.cluster.local
  prefix: 127.0.0.1:30000
`

			tc := setupTest(registriesConf, nil)

			// The reconcile interval is 0, so the controller only contacts the registry
			// in response to an event; the refresh is what gives it something to react to.
			triggerRefresh := iuFixture.TriggerArgoCDRefresh(ctx, k8sClient, tc.app)

			By("waiting for the image updater to reject the registry's self-signed certificate")
			Eventually(func() string {
				triggerRefresh()
				logs, err := fixture.GetPodLogs(tc.ns.Name, "argocd-image-updater-controller")
				if err != nil {
					GinkgoWriter.Println("unable to read image updater controller logs:", err)
					return ""
				}
				return logs
			}, "3m", "5s").Should(ContainSubstring("x509: certificate signed by unknown authority"),
				"the controller should have refused to trust the registry with no CA configured")

			By("ensuring that the Application image is never updated")
			Consistently(func() string {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(tc.app), tc.app); err != nil {
					return ""
				}

				triggerRefresh()

				if tc.app.Spec.Source.Kustomize != nil && len(tc.app.Spec.Source.Kustomize.Images) > 0 {
					return string(tc.app.Spec.Source.Kustomize.Images[0])
				}

				return ""
			}, "90s", "3s").Should(BeEmpty(), "image must not be updated while the registry's certificate is untrusted")
		})
	})
})
