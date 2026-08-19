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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"

	applicationFixture "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/application"
	"github.com/argoproj/argo-cd/gitops-engine/pkg/health"
	synccommon "github.com/argoproj/argo-cd/gitops-engine/pkg/sync/common"
	appv1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
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

// generateTestTLSCert creates a self-signed CA and a server certificate with SANs
// matching the git server's in-cluster DNS names. Returns PEM-encoded cert, key, and CA cert.
func generateTestTLSCert(serviceName, namespace string) (certPEM, keyPEM, caCertPEM []byte) {
	// Generate CA key and certificate
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).NotTo(HaveOccurred())

	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	Expect(err).NotTo(HaveOccurred())
	caCert, err := x509.ParseCertificate(caCertDER)
	Expect(err).NotTo(HaveOccurred())

	// Generate server key and certificate
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).NotTo(HaveOccurred())

	clusterDomain := fmt.Sprintf("%s.%s.svc.cluster.local", serviceName, namespace)
	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: clusterDomain},
		DNSNames: []string{
			"localhost",
			serviceName,
			fmt.Sprintf("%s.%s", serviceName, namespace),
			fmt.Sprintf("%s.%s.svc", serviceName, namespace),
			clusterDomain,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	serverCertDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	Expect(err).NotTo(HaveOccurred())

	// Encode to PEM
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverCertDER})
	serverKeyDER, err := x509.MarshalECPrivateKey(serverKey)
	Expect(err).NotTo(HaveOccurred())
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: serverKeyDER})
	caCertPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCertDER})
	return
}

var _ = Describe("ArgoCD Image Updater Parallel E2E Tests", func() {

	// This test verifies that Image Updater correctly handles SourceHydrator Applications
	// using git write-back with an external Helm values file.
	Context("1-012-source-hydrator-helm-test", func() {

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
			}

			if argoCD != nil {
				By("deleting ArgoCD CR")
				_ = k8sClient.Delete(ctx, argoCD)
			}

			if cleanupFunc != nil {
				cleanupFunc()
			}
		})

		It("ensures that Image Updater will update SourceHydrator Helm Application using git write-back policy", func() {

			By("creating simple namespace-scoped Argo CD instance with image updater enabled")
			ns, cleanupFunc = fixture.CreateRandomE2ETestNamespaceWithCleanupFunc()

			By("generating TLS certificate for the local git server")
			certPEM, keyPEM, caCertPEM := generateTestTLSCert(iuFixture.Name, ns.Name)

			By("creating TLS Secret for the git server")
			tlsSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      iuFixture.Name + "-tls",
					Namespace: ns.Name,
				},
				Data: map[string][]byte{
					"tls.crt": certPEM,
					"tls.key": keyPEM,
				},
			}
			Expect(k8sClient.Create(ctx, tlsSecret)).To(Succeed())

			By("creating local git repo with TLS certificate mounted")
			gitImageRef := fmt.Sprintf("%s/%s:%s", iuFixture.GitImageRepo, iuFixture.GitImageName, iuFixture.GitImageTag)
			depl := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      iuFixture.Name,
					Namespace: ns.Name,
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app":       iuFixture.Name,
							"component": ns.Name,
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app":       iuFixture.Name,
								"component": ns.Name,
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:            iuFixture.GitImageName,
									Image:           gitImageRef,
									ImagePullPolicy: corev1.PullAlways,
									Ports: []corev1.ContainerPort{
										{ContainerPort: 8080},
										{ContainerPort: 8081},
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "tls-cert",
											MountPath: "/etc/nginx/localhost.crt",
											SubPath:   "tls.crt",
											ReadOnly:  true,
										},
										{
											Name:      "tls-cert",
											MountPath: "/etc/nginx/localhost.key",
											SubPath:   "tls.key",
											ReadOnly:  true,
										},
									},
								},
							},
							Volumes: []corev1.Volume{
								{
									Name: "tls-cert",
									VolumeSource: corev1.VolumeSource{
										Secret: &corev1.SecretVolumeSource{
											SecretName: tlsSecret.Name,
										},
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, depl)).To(Succeed())

			By("creating local git repo Service")
			service := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      iuFixture.Name,
					Namespace: ns.Name,
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeNodePort,
					Selector: map[string]string{
						"app":       iuFixture.Name,
						"component": ns.Name,
					},
					Ports: []corev1.ServicePort{
						{
							Name:       "unauth",
							Protocol:   corev1.ProtocolTCP,
							Port:       8080,
							TargetPort: intstr.FromInt32(8080),
						},
						{
							Name:       "auth",
							Protocol:   corev1.ProtocolTCP,
							Port:       8081,
							TargetPort: intstr.FromInt32(8081),
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, service)).To(Succeed())

			gitRepoURL := fmt.Sprintf("https://%s.%s.svc.cluster.local:8081/testdata.git", iuFixture.Name, ns.Name)

			By("creating ArgoCD repository credential Secret")
			repoSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      iuFixture.Name,
					Namespace: ns.Name,
					Labels: map[string]string{
						"argocd.argoproj.io/secret-type": "repository",
						"component":                      ns.Name,
					},
				},
				StringData: map[string]string{
					"url":      gitRepoURL,
					"type":     "git",
					"password": "git",
					"username": "git",
				},
			}
			Expect(k8sClient.Create(ctx, repoSecret)).To(Succeed())

			By("creating repo-write-creds Secret for the commit-server")
			repoWriteCredsURL := fmt.Sprintf("https://%s.%s.svc.cluster.local:8081/", iuFixture.Name, ns.Name)
			writeCredsSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      iuFixture.Name + "-write-creds",
					Namespace: ns.Name,
					Labels: map[string]string{
						"argocd.argoproj.io/secret-type": "repo-write-creds",
					},
				},
				StringData: map[string]string{
					"type":     "git",
					"url":      repoWriteCredsURL,
					"password": "git",
					"username": "git",
				},
			}
			Expect(k8sClient.Create(ctx, writeCredsSecret)).To(Succeed())

			By("waiting for local git repo to be ready")
			gitDepl := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: iuFixture.Name, Namespace: ns.Name}}
			Eventually(gitDepl).Should(k8sFixture.ExistByName())
			Eventually(gitDepl, "2m", "3s").Should(deplFixture.HaveReadyReplicas(1), "git repo server was not ready")

			tlsHostKey := fmt.Sprintf("%s.%s.svc.cluster.local", iuFixture.Name, ns.Name)
			argoCD = &argov1beta1api.ArgoCD{
				ObjectMeta: metav1.ObjectMeta{Name: "argocd", Namespace: ns.Name},
				Spec: argov1beta1api.ArgoCDSpec{
					TLS: argov1beta1api.ArgoCDTLSSpec{
						InitialCerts: map[string]string{
							tlsHostKey: string(caCertPEM),
						},
					},
					SourceHydrator: argov1beta1api.ArgoCDSourceHydratorSpec{
						Enabled: ptr.To(true),
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
			deploymentsShouldExist := []string{"argocd-redis", "argocd-server", "argocd-repo-server", "argocd-commit-server", "argocd-argocd-image-updater-controller"}
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

			By("creating SourceHydrator Helm Application")
			hydrateBranch := "environments/hydrator-test"
			app := &appv1alpha1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "hydrator-helm-app",
					Namespace: ns.Name,
				},
				Spec: appv1alpha1.ApplicationSpec{
					Project: "default",
					SourceHydrator: &appv1alpha1.SourceHydrator{
						DrySource: appv1alpha1.DrySource{
							RepoURL:        gitRepoURL,
							Path:           "1-012-source-hydrator-helm-test/helm",
							TargetRevision: "HEAD",
							Helm: &appv1alpha1.ApplicationSourceHelm{
								ValueFiles: []string{"values.yaml"},
							},
						},
						SyncSource: appv1alpha1.SyncSource{
							TargetBranch: hydrateBranch,
							Path:         "manifests",
						},
					},
					Destination: appv1alpha1.ApplicationDestination{
						Server:    "https://kubernetes.default.svc",
						Namespace: ns.Name,
					},
				},
			}
			Expect(k8sClient.Create(ctx, app)).To(Succeed())

			By("waiting for Source Hydrator to hydrate the application")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(app), app)).To(Succeed())
				g.Expect(app.Status.SourceHydrator.CurrentOperation).NotTo(BeNil())
				g.Expect(app.Status.SourceHydrator.CurrentOperation.Phase).To(
					Equal(appv1alpha1.HydrateOperationPhaseHydrated),
				)
			}, "2m", "5s").Should(Succeed())

			By("syncing hydrated manifests to the cluster")
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(app), app)).To(Succeed())
			app.Operation = &appv1alpha1.Operation{
				Sync: &appv1alpha1.SyncOperation{},
			}
			Expect(k8sClient.Update(ctx, app)).To(Succeed())
			Eventually(app, "2m", "5s").Should(applicationFixture.HaveOperationStatePhase(synccommon.OperationSucceeded))
			Expect(app).Should(applicationFixture.HaveSyncStatusCode(appv1alpha1.SyncStatusCodeSynced))
			Expect(app).Should(applicationFixture.HaveHealthStatusCode(health.HealthStatusHealthy))

			By("creating ImageUpdater CR with git write-back targeting the Helm values file")
			updateStrategy := "semver"
			forceUpdate := false
			method := fmt.Sprintf("git:secret:%s/%s", ns.Name, iuFixture.Name)
			branch := "master"
			repository := gitRepoURL
			writeBackTarget := "helmvalues:/1-012-source-hydrator-helm-test/helm/values.yaml"
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
							NamePattern: "hydrator-helm-*",
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
