package imageupdater

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	//lint:ignore ST1001 "This is a common practice in Gomega tests for readability."
	. "github.com/onsi/ginkgo/v2" //nolint:all
	//lint:ignore ST1001 "This is a common practice in Gomega tests for readability."
	. "github.com/onsi/gomega" //nolint:all

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	GitImageRepo = "127.0.0.1:30000"
	GitImageName = "git-http"
	GitImageTag  = "latest"
	Name         = "e2e-repository"
)

// createLocalGitRepoDeployment creates a Deployment for a local git repository server
// used in E2E tests. The deployment runs a git-http container that serves git repositories
// over HTTP on ports 8080 (unauthenticated) and 8081 (authenticated).
func createLocalGitRepoDeployment(ctx context.Context, k8sClient client.Client, namespace string) {
	By("creating local git repo Deployment")

	depl := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      Name,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":       Name,
					"component": namespace,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":       Name,
						"component": namespace,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            GitImageName,
							Image:           fmt.Sprintf("%s/%s:%s", GitImageRepo, GitImageName, GitImageTag), //"127.0.0.1:30000/git-http:latest",
							ImagePullPolicy: corev1.PullAlways,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8080,
								},
								{
									ContainerPort: 8081,
								},
							},
						},
					},
				},
			},
		},
	}
	Expect(k8sClient.Create(ctx, depl)).To(Succeed())
}

// createLocalGitRepoService creates a NodePort Service for the local git repository server.
// The service exposes ports 8080 (unauthenticated) and 8081 (authenticated) allowing external access to the git server.
func createLocalGitRepoService(ctx context.Context, k8sClient client.Client, namespace string) {
	By("creating local git repo Service")

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      Name,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort,
			Selector: map[string]string{
				"app":       Name,
				"component": namespace,
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
}

// createLocalGitRepoSecret creates a Kubernetes Secret with ArgoCD repository credentials
// for the local git repository. The secret is labeled with "argocd.argoproj.io/secret-type: repository"
// so that ArgoCD can discover and use it for git operations.
func createLocalGitRepoSecret(ctx context.Context, k8sClient client.Client, namespace string) {
	By("creating local git repo Secret")
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      Name,
			Namespace: namespace,
			Labels: map[string]string{
				"argocd.argoproj.io/secret-type": "repository",
				"component":                      namespace,
			},
		},
		StringData: map[string]string{
			"url":      fmt.Sprintf("https://%s.%s.svc.cluster.local:8081/testdata.git", Name, namespace), //"https://e2e-repository.argocd-operator-system.svc.cluster.local:8081/testdata.git",
			"type":     "git",
			"password": "git",
			"username": "git",
			"insecure": "true",
		},
	}
	Expect(k8sClient.Create(ctx, secret)).To(Succeed())
}

// CreateLocalGitRepo creates a complete local git repository setup for E2E tests.
// It creates a Deployment (git server), Service (NodePort for external access), and Secret
// (ArgoCD repository credentials). This provides an isolated git repository that can be used
// for testing git write-back functionality without requiring external git services.
func CreateLocalGitRepo(ctx context.Context, k8sClient client.Client, namespace string) {
	By("creating local git repo")
	createLocalGitRepoDeployment(ctx, k8sClient, namespace)
	createLocalGitRepoService(ctx, k8sClient, namespace)
	createLocalGitRepoSecret(ctx, k8sClient, namespace)
}

// TriggerArgoCDRefresh periodically sets the refresh annotation on an ArgoCD Application
// to force immediate git checks, bypassing ArgoCD's default 3-minute polling interval.
// This function returns a function that can be used in Eventually() loops.
// The refresh annotation is set every other call (every ~10 seconds when polling every 5s).
// app should be a pointer to an Application object that will be updated in place.
func TriggerArgoCDRefresh(ctx context.Context, k8sClient client.Client, app client.Object) func() {
	refreshCounter := 0
	return func() {
		refreshCounter++
		if refreshCounter%2 == 0 { // Every other call (every ~10 seconds when polling every 5s)
			// Re-fetch to get latest resource version
			if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(app), app); err != nil {
				GinkgoWriter.Println("TriggerArgoCDRefresh: failed to get app:", err)
				return
			}
			// Set refresh annotation to "hard" to force immediate git check and cache invalidation
			// ArgoCD removes the annotation after processing, so we need to keep setting it
			annotations := app.GetAnnotations()
			if annotations == nil {
				annotations = make(map[string]string)
			}
			annotations["argocd.argoproj.io/refresh"] = "hard"
			app.SetAnnotations(annotations)
			if err := k8sClient.Update(ctx, app); err != nil {
				GinkgoWriter.Println("TriggerArgoCDRefresh: failed to update app:", err)
			}
		}
	}
}
