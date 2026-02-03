// Assisted-by: Claude AI

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

package v1alpha1

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var (
	cfg           *rest.Config
	k8sClient     client.Client
	testEnv       *envtest.Environment
	runtimeScheme *runtime.Scheme
)

func TestImageUpdaterTypes(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ImageUpdater Types Suite")
}

var _ = BeforeSuite(func() {
	By("bootstrapping test environment")

	// Get the project root directory
	wd, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred())

	// Find project root by looking for go.mod or config/crd directory
	var projectRoot string
	currentDir := wd
	for {
		if _, err := os.Stat(filepath.Join(currentDir, "go.mod")); err == nil {
			projectRoot = currentDir
			break
		}
		if _, err := os.Stat(filepath.Join(currentDir, "config", "crd")); err == nil {
			projectRoot = currentDir
			break
		}
		parent := filepath.Dir(currentDir)
		if parent == currentDir {
			// Reached filesystem root
			projectRoot = wd
			break
		}
		currentDir = parent
	}

	crdPath := filepath.Join(projectRoot, "config/crd/bases")
	crdAbsPath, err := filepath.Abs(crdPath)
	Expect(err).NotTo(HaveOccurred())

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{crdAbsPath},
	}

	// Try to find kubebuilder binaries
	var assetsPath string

	// First, check KUBEBUILDER_ASSETS env var (maybe relative or absolute)
	if envAssets := os.Getenv("KUBEBUILDER_ASSETS"); envAssets != "" {
		// If relative, make it absolute relative to project root
		if !filepath.IsAbs(envAssets) {
			absPath, err := filepath.Abs(filepath.Join(projectRoot, envAssets))
			if err == nil {
				assetsPath = absPath
			}
		} else {
			assetsPath = envAssets
		}
		// Verify it exists
		if _, err := os.Stat(filepath.Join(assetsPath, "etcd")); err == nil {
			testEnv.BinaryAssetsDirectory = assetsPath
		} else {
			assetsPath = "" // Reset if not found
		}
	}

	// If not found, try common locations
	if assetsPath == "" {
		// Check project's bin directory (where make test puts them)
		projectBinPaths := []string{
			filepath.Join(projectRoot, "bin/k8s/1.31.0-darwin-arm64"),
			filepath.Join(projectRoot, "bin/k8s/1.31.0-darwin-amd64"),
			filepath.Join(projectRoot, "bin/k8s/1.31.0-linux-amd64"),
		}

		// Also check for any version in bin/k8s
		if entries, err := os.ReadDir(filepath.Join(projectRoot, "bin/k8s")); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					projectBinPaths = append(projectBinPaths, filepath.Join(projectRoot, "bin/k8s", entry.Name()))
				}
			}
		}

		// Check default system locations
		defaultPaths := []string{
			"/usr/local/kubebuilder/bin",
			filepath.Join(os.Getenv("HOME"), ".local/share/kubebuilder-envtest"),
		}

		allPaths := append(projectBinPaths, defaultPaths...)

		for _, path := range allPaths {
			if _, err := os.Stat(filepath.Join(path, "etcd")); err == nil {
				testEnv.BinaryAssetsDirectory = path
				break
			}
		}
	}

	// If still not found, let envtest try to download automatically (requires network)
	// This will work if network is available
	// Don't set BinaryAssetsDirectory - let envtest handle it automatically
	// This requires network access but works without manual setup

	cfg, err = testEnv.Start()
	if err != nil {
		Skip("Skipping validation tests: kubebuilder binaries not found. " +
			"To fix this:\n" +
			"  1. Run 'make envtest' to install setup-envtest\n" +
			"  2. Run: export KUBEBUILDER_ASSETS=$(./bin/setup-envtest use 1.31.0 --bin-dir ./bin -p path)\n" +
			"  3. Or ensure network access for automatic download\n" +
			"Error: " + err.Error())
	}
	Expect(cfg).NotTo(BeNil())

	runtimeScheme = runtime.NewScheme()
	err = AddToScheme(runtimeScheme)
	Expect(err).NotTo(HaveOccurred())

	// Add core API types (for Namespace)
	err = corev1.AddToScheme(runtimeScheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: runtimeScheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// Create the argocd namespace that tests will use
	argocdNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "argocd",
		},
	}
	err = k8sClient.Create(context.Background(), argocdNS)
	// Ignore error if namespace already exists
	if err != nil && !apierrors.IsAlreadyExists(err) {
		Expect(err).NotTo(HaveOccurred())
	}
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	if testEnv == nil || cfg == nil {
		return // Environment was never started
	}
	err := testEnv.Stop()
	if err != nil {
		// Log but don't fail - environment may not have been fully started
		GinkgoWriter.Printf("Warning: failed to stop test environment: %v\n", err)
	}
})

var _ = Describe("ApplicationRef UseAnnotations Validation", func() {
	Context("when useAnnotations is false", func() {
		It("should reject ApplicationRef without images", func() {
			cr := &ImageUpdater{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "image-updater",
					Namespace: "argocd",
				},
				Spec: ImageUpdaterSpec{
					Namespace: "argocd",
					ApplicationRefs: []ApplicationRef{
						{
							NamePattern:    "*",
							UseAnnotations: boolPtr(false),
							LabelSelectors: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app.kubernetes.io/part-of": "my-project-1",
								},
							},
							// No images provided - should fail validation
						},
					},
				},
			}

			err := k8sClient.Create(context.Background(), cr)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Either useAnnotations must be true, or images must be provided with at least one item"))
		})

		It("should accept ApplicationRef with images", func() {
			cr := &ImageUpdater{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "image-updater-valid",
					Namespace: "argocd",
				},
				Spec: ImageUpdaterSpec{
					Namespace: "argocd",
					ApplicationRefs: []ApplicationRef{
						{
							NamePattern:    "*",
							UseAnnotations: boolPtr(false),
							LabelSelectors: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app.kubernetes.io/part-of": "my-project-1",
								},
							},
							Images: []ImageConfig{
								{
									Alias:     "test",
									ImageName: "127.0.0.1:30000/test-image:1.X.X",
								},
							},
						},
					},
				},
			}

			err := k8sClient.Create(context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())

			// Cleanup
			err = k8sClient.Delete(context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("when useAnnotations is true", func() {
		It("should accept ApplicationRef without images", func() {
			cr := &ImageUpdater{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "image-updater-annotations-true",
					Namespace: "argocd",
				},
				Spec: ImageUpdaterSpec{
					Namespace: "argocd",
					ApplicationRefs: []ApplicationRef{
						{
							NamePattern:    "*",
							UseAnnotations: boolPtr(true),
							LabelSelectors: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app.kubernetes.io/part-of": "my-project-1",
								},
							},
							// No images provided - should be valid
						},
					},
				},
			}

			err := k8sClient.Create(context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())

			// Cleanup
			err = k8sClient.Delete(context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept ApplicationRef with images (images will be ignored)", func() {
			cr := &ImageUpdater{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "image-updater-annotations-true-with-images",
					Namespace: "argocd",
				},
				Spec: ImageUpdaterSpec{
					Namespace: "argocd",
					ApplicationRefs: []ApplicationRef{
						{
							NamePattern:    "*",
							UseAnnotations: boolPtr(true),
							LabelSelectors: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app.kubernetes.io/part-of": "my-project-1",
								},
							},
							Images: []ImageConfig{
								{
									Alias:     "test",
									ImageName: "127.0.0.1:30000/test-image:1.X.X",
								},
							},
						},
					},
				},
			}

			err := k8sClient.Create(context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())

			// Cleanup
			err = k8sClient.Delete(context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("when useAnnotations is nil (not set)", func() {
		It("should reject ApplicationRef without images", func() {
			cr := &ImageUpdater{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "image-updater-nil-no-images",
					Namespace: "argocd",
				},
				Spec: ImageUpdaterSpec{
					Namespace: "argocd",
					ApplicationRefs: []ApplicationRef{
						{
							NamePattern: "*",
							// UseAnnotations is nil (defaults to false)
							// No images provided - should fail validation
						},
					},
				},
			}

			err := k8sClient.Create(context.Background(), cr)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Either useAnnotations must be true, or images must be provided with at least one item"))
		})

		It("should accept ApplicationRef with images", func() {
			cr := &ImageUpdater{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "image-updater-nil-with-images",
					Namespace: "argocd",
				},
				Spec: ImageUpdaterSpec{
					Namespace: "argocd",
					ApplicationRefs: []ApplicationRef{
						{
							NamePattern: "*",
							// UseAnnotations is nil (defaults to false)
							Images: []ImageConfig{
								{
									Alias:     "nginx",
									ImageName: "nginx:1.21.0",
								},
							},
						},
					},
				},
			}

			err := k8sClient.Create(context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())

			// Cleanup
			err = k8sClient.Delete(context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("when ApplicationRef has empty images array", func() {
		It("should reject ApplicationRef with empty images array when useAnnotations is false", func() {
			cr := &ImageUpdater{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "image-updater-empty-images",
					Namespace: "argocd",
				},
				Spec: ImageUpdaterSpec{
					Namespace: "argocd",
					ApplicationRefs: []ApplicationRef{
						{
							NamePattern:    "*",
							UseAnnotations: boolPtr(false),
							Images:         []ImageConfig{}, // Empty array
						},
					},
				},
			}

			err := k8sClient.Create(context.Background(), cr)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Either useAnnotations must be true, or images must be provided with at least one item"))
		})
	})

	Context("when multiple ApplicationRefs are present", func() {
		It("should validate each ApplicationRef independently", func() {
			cr := &ImageUpdater{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "image-updater-multiple-refs",
					Namespace: "argocd",
				},
				Spec: ImageUpdaterSpec{
					Namespace: "argocd",
					ApplicationRefs: []ApplicationRef{
						{
							NamePattern:    "app-1",
							UseAnnotations: boolPtr(true),
							// Valid: useAnnotations is true
						},
						{
							NamePattern:    "app-2",
							UseAnnotations: boolPtr(false),
							// Invalid: useAnnotations is false but no images
						},
					},
				},
			}

			err := k8sClient.Create(context.Background(), cr)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Either useAnnotations must be true, or images must be provided with at least one item"))
		})

		It("should accept multiple valid ApplicationRefs", func() {
			cr := &ImageUpdater{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "image-updater-multiple-valid-refs",
					Namespace: "argocd",
				},
				Spec: ImageUpdaterSpec{
					Namespace: "argocd",
					ApplicationRefs: []ApplicationRef{
						{
							NamePattern:    "app-1",
							UseAnnotations: boolPtr(true),
						},
						{
							NamePattern:    "app-2",
							UseAnnotations: boolPtr(false),
							Images: []ImageConfig{
								{
									Alias:     "nginx",
									ImageName: "nginx:1.21.0",
								},
							},
						},
					},
				},
			}

			err := k8sClient.Create(context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())

			// Cleanup
			err = k8sClient.Delete(context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

var _ = Describe("HelmTarget Validation", func() {
	Context("when spec is set", func() {
		It("should accept HelmTarget with only spec", func() {
			cr := &ImageUpdater{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "image-updater-helm-spec-only",
					Namespace: "argocd",
				},
				Spec: ImageUpdaterSpec{
					Namespace: "argocd",
					ApplicationRefs: []ApplicationRef{
						{
							NamePattern: "*",
							Images: []ImageConfig{
								{
									Alias:     "test",
									ImageName: "nginx:1.21.0",
									ManifestTarget: &ManifestTarget{
										Helm: &HelmTarget{
											Spec: strPtr("image"),
										},
									},
								},
							},
						},
					},
				},
			}

			err := k8sClient.Create(context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())

			// Cleanup
			err = k8sClient.Delete(context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept HelmTarget with spec even when name and tag are also set", func() {
			cr := &ImageUpdater{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "image-updater-helm-spec-with-name-tag",
					Namespace: "argocd",
				},
				Spec: ImageUpdaterSpec{
					Namespace: "argocd",
					ApplicationRefs: []ApplicationRef{
						{
							NamePattern: "*",
							Images: []ImageConfig{
								{
									Alias:     "test",
									ImageName: "nginx:1.21.0",
									ManifestTarget: &ManifestTarget{
										Helm: &HelmTarget{
											Spec: strPtr("image"),
											Name: strPtr("image.repository"),
											Tag:  strPtr("image.tag"),
										},
									},
								},
							},
						},
					},
				},
			}

			err := k8sClient.Create(context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())

			// Cleanup
			err = k8sClient.Delete(context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("when spec is not set", func() {
		It("should accept HelmTarget with both name and tag", func() {
			cr := &ImageUpdater{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "image-updater-helm-name-tag",
					Namespace: "argocd",
				},
				Spec: ImageUpdaterSpec{
					Namespace: "argocd",
					ApplicationRefs: []ApplicationRef{
						{
							NamePattern: "*",
							Images: []ImageConfig{
								{
									Alias:     "test",
									ImageName: "nginx:1.21.0",
									ManifestTarget: &ManifestTarget{
										Helm: &HelmTarget{
											Name: strPtr("image.repository"),
											Tag:  strPtr("image.tag"),
										},
									},
								},
							},
						},
					},
				},
			}

			err := k8sClient.Create(context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())

			// Cleanup
			err = k8sClient.Delete(context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept HelmTarget with only name", func() {
			cr := &ImageUpdater{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "image-updater-helm-name-only",
					Namespace: "argocd",
				},
				Spec: ImageUpdaterSpec{
					Namespace: "argocd",
					ApplicationRefs: []ApplicationRef{
						{
							NamePattern: "*",
							Images: []ImageConfig{
								{
									Alias:     "test",
									ImageName: "nginx:1.21.0",
									ManifestTarget: &ManifestTarget{
										Helm: &HelmTarget{
											Name: strPtr("image.repository"),
										},
									},
								},
							},
						},
					},
				},
			}

			err := k8sClient.Create(context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())

			// Cleanup
			err = k8sClient.Delete(context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept HelmTarget with only tag", func() {
			cr := &ImageUpdater{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "image-updater-helm-tag-only",
					Namespace: "argocd",
				},
				Spec: ImageUpdaterSpec{
					Namespace: "argocd",
					ApplicationRefs: []ApplicationRef{
						{
							NamePattern: "*",
							Images: []ImageConfig{
								{
									Alias:     "test",
									ImageName: "nginx:1.21.0",
									ManifestTarget: &ManifestTarget{
										Helm: &HelmTarget{
											Tag: strPtr("image.tag"),
										},
									},
								},
							},
						},
					},
				},
			}

			err := k8sClient.Create(context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())

			// Cleanup
			err = k8sClient.Delete(context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept HelmTarget with neither spec nor name/tag (defaults will be used)", func() {
			cr := &ImageUpdater{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "image-updater-helm-empty",
					Namespace: "argocd",
				},
				Spec: ImageUpdaterSpec{
					Namespace: "argocd",
					ApplicationRefs: []ApplicationRef{
						{
							NamePattern: "*",
							Images: []ImageConfig{
								{
									Alias:     "test",
									ImageName: "nginx:1.21.0",
									ManifestTarget: &ManifestTarget{
										Helm: &HelmTarget{},
									},
								},
							},
						},
					},
				},
			}

			err := k8sClient.Create(context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())

			// Cleanup
			err = k8sClient.Delete(context.Background(), cr)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

// Helper function to create a bool pointer
func boolPtr(b bool) *bool {
	return &b
}

// Helper function to create a string pointer
func strPtr(s string) *string {
	return &s
}
