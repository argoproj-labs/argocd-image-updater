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

	appv1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/argoproj/gitops-engine/pkg/health"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	imageUpdaterApi "github.com/argoproj-labs/argocd-image-updater/api/v1alpha1"
	applicationFixture "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/application"
	iuFixture "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/imageupdater"
)

// imageUpdaterManagerPolicyRules returns the RBAC policy rules required by the
// Image Updater manager role. Shared by both namespace-scoped (Role) and
// cluster-scoped (ClusterRole) installations.
func imageUpdaterManagerPolicyRules() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		{APIGroups: []string{""}, Resources: []string{"events"}, Verbs: []string{"create"}},
		{APIGroups: []string{"argocd-image-updater.argoproj.io"}, Resources: []string{"imageupdaters"}, Verbs: []string{"create", "delete", "get", "list", "patch", "update", "watch"}},
		{APIGroups: []string{"argocd-image-updater.argoproj.io"}, Resources: []string{"imageupdaters/finalizers"}, Verbs: []string{"update"}},
		{APIGroups: []string{"argocd-image-updater.argoproj.io"}, Resources: []string{"imageupdaters/status"}, Verbs: []string{"get", "patch", "update"}},
		{APIGroups: []string{"argoproj.io"}, Resources: []string{"applications"}, Verbs: []string{"get", "list", "patch", "update", "watch"}},
	}
}

// createImageUpdaterRoleAndBinding creates a namespace-scoped Role and RoleBinding
// in targetNamespace granting the Image Updater ServiceAccount in controllerNamespace
// the permissions it needs to manage ImageUpdater CRs and Applications.
//
// TODO: Remove after ArgoCD Operator fix https://github.com/argoproj-labs/argocd-operator/pull/2172
func createImageUpdaterRoleAndBinding(ctx context.Context, k8sClient client.Client, targetNamespace, controllerNamespace string) {
	By("creating Role and RoleBinding in " + targetNamespace + " for Image Updater ServiceAccount")
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "argocd-image-updater-manager-role",
			Namespace: targetNamespace,
		},
		Rules: imageUpdaterManagerPolicyRules(),
	}
	Expect(k8sClient.Create(ctx, role)).To(Succeed())

	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "argocd-image-updater-manager-rolebinding",
			Namespace: targetNamespace,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     "argocd-image-updater-manager-role",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "argocd-argocd-image-updater-controller",
				Namespace: controllerNamespace,
			},
		},
	}
	Expect(k8sClient.Create(ctx, roleBinding)).To(Succeed())
}

// createAppProject creates an ArgoCD AppProject named after sourceNamespace,
// hosted in argoCDNamespace, permitting all sources and destinations.
func createAppProject(ctx context.Context, k8sClient client.Client, sourceNamespace, argoCDNamespace string) {
	By("creating AppProject for " + sourceNamespace)
	appProject := &appv1alpha1.AppProject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sourceNamespace,
			Namespace: argoCDNamespace,
		},
		Spec: appv1alpha1.AppProjectSpec{
			SourceRepos:      []string{"*"},
			SourceNamespaces: []string{sourceNamespace},
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
}

// createGuestbookApp creates a public guestbook Application named "app-01" in
// appNamespace, waits for it to become healthy and synced, and returns the object.
func createGuestbookApp(ctx context.Context, k8sClient client.Client, appNamespace, projectName string) *appv1alpha1.Application {
	By("creating Application in " + appNamespace)
	app := &appv1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app-01",
			Namespace: appNamespace,
		},
		Spec: appv1alpha1.ApplicationSpec{
			Project: projectName,
			Source: &appv1alpha1.ApplicationSource{
				RepoURL:        "https://github.com/argoproj-labs/argocd-image-updater/",
				Path:           "test/e2e/testdata/005-public-guestbook",
				TargetRevision: "HEAD",
			},
			Destination: appv1alpha1.ApplicationDestination{
				Server:    "https://kubernetes.default.svc",
				Namespace: appNamespace,
			},
			SyncPolicy: &appv1alpha1.SyncPolicy{Automated: &appv1alpha1.SyncPolicyAutomated{}},
		},
	}
	Expect(k8sClient.Create(ctx, app)).To(Succeed())

	By("verifying Application in " + appNamespace + " is healthy and synced")
	Eventually(app, "4m", "3s").Should(applicationFixture.HaveHealthStatusCode(health.HealthStatusHealthy))
	Eventually(app, "4m", "3s").Should(applicationFixture.HaveSyncStatusCode(appv1alpha1.SyncStatusCodeSynced))
	return app
}

// createImageUpdaterAndVerify creates an ImageUpdater CR in namespace, waits for the
// guestbook image in app to be updated to the semver-resolved tag, and returns the CR.
func createImageUpdaterAndVerify(ctx context.Context, k8sClient client.Client, namespace string, app *appv1alpha1.Application) *imageUpdaterApi.ImageUpdater {
	By("creating ImageUpdater CR in " + namespace)
	updateStrategy := "semver"
	iu := &imageUpdaterApi.ImageUpdater{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "image-updater",
			Namespace: namespace,
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
	Expect(k8sClient.Create(ctx, iu)).To(Succeed())

	By("ensuring Application image in " + namespace + " has `29437546.0` version after update")
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

	return iu
}
