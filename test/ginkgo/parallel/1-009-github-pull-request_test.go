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
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/go-github/v69/github"

	appv1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/argoproj/gitops-engine/pkg/health"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	imageUpdaterApi "github.com/argoproj-labs/argocd-image-updater/api/v1alpha1"
	argov1beta1api "github.com/argoproj-labs/argocd-operator/api/v1beta1"

	"github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture"
	applicationFixture "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/application"
	argocdFixture "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/argocd"
	deplFixture "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/deployment"
	iuFixture "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/imageupdater"
	k8sFixture "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/k8s"
	ssFixture "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/statefulset"
	fixtureUtils "github.com/argoproj-labs/argocd-image-updater/test/ginkgo/fixture/utils"
)

// Environment variables that supply GitHub credentials and repository info for this test.
// All three are required; E2E_GITHUB_BRANCH defaults to "main" when omitted.
//
//	E2E_GITHUB_TOKEN  — a GitHub Personal Access Token (PAT) with repo + pull_request scopes,
//	                    or a GitHub App installation token with Contents: read/write and
//	                    Pull requests: read/write permissions.
//	E2E_GITHUB_OWNER  — GitHub user or organisation that owns the target repository.
//	E2E_GITHUB_REPO   — Repository name (not the full URL).
//	E2E_GITHUB_BRANCH — Base branch the PR is opened against (default: "main").
const (
	envGitHubToken  = "E2E_GITHUB_TOKEN"
	envGitHubOwner  = "E2E_GITHUB_OWNER"
	envGitHubRepo   = "E2E_GITHUB_REPO"
	envGitHubBranch = "E2E_GITHUB_BRANCH"

	githubCredsSecretName = "github-creds"
)

var _ = Describe("ArgoCD Image Updater Parallel E2E Tests", func() {

	Context("1-009-github-pull-request_test", func() {

		var (
			k8sClient    client.Client
			ctx          context.Context
			ns           *corev1.Namespace
			cleanupFunc  func()
			imageUpdater *imageUpdaterApi.ImageUpdater
			argoCD       *argov1beta1api.ArgoCD

			// GitHub client and PR tracking for AfterEach cleanup.
			ghClient     *github.Client
			ghOwner      string
			ghRepo       string
			prNumber     int
			prHeadBranch string
		)

		BeforeEach(func() {
			fixture.EnsureParallelCleanSlate()

			k8sClient, _ = fixtureUtils.GetE2ETestKubeClient()
			ctx = context.Background()

			ghClient = nil
			prNumber = 0
			prHeadBranch = ""
		})

		AfterEach(func() {
			// Stop the updater before touching the remote repo so that a
			// final reconcile cannot recreate the branch or PR after cleanup.
			if imageUpdater != nil {
				By("deleting ImageUpdater CR")
				_ = k8sClient.Delete(ctx, imageUpdater)
				Eventually(imageUpdater, "2m", "3s").Should(k8sFixture.NotExistByName())
			}

			// Close the GitHub PR and delete its head branch only after the
			// updater is gone, so cleanup cannot be undone by another reconcile.
			if ghClient != nil && prNumber > 0 {
				By("closing GitHub PR and deleting its head branch")
				closed := "closed"
				if _, _, err := ghClient.PullRequests.Edit(ctx, ghOwner, ghRepo, prNumber,
					&github.PullRequest{State: &closed}); err != nil {
					GinkgoWriter.Println("warning: could not close PR:", err)
				}
				if prHeadBranch != "" {
					if _, err := ghClient.Git.DeleteRef(ctx, ghOwner, ghRepo,
						"heads/"+prHeadBranch); err != nil {
						GinkgoWriter.Println("warning: could not delete head branch:", err)
					}
				}
			}

			if argoCD != nil {
				By("deleting ArgoCD CR")
				_ = k8sClient.Delete(ctx, argoCD)
			}

			// Capture debug output before cleanupFunc removes the namespace.
			fixture.OutputDebugOnFail(ns)

			if cleanupFunc != nil {
				cleanupFunc()
			}
		})

		It("ensures that Image Updater creates a GitHub PR when pull request mode is enabled", func() {

			// ---------------------------------------------------------------
			// 1. Resolve credentials from environment — skip if not provided.
			// ---------------------------------------------------------------
			githubToken := os.Getenv(envGitHubToken)
			if githubToken == "" {
				Skip(fmt.Sprintf("skipping: %s is not set — provide %s, %s, %s (and optionally %s) to run this test",
					envGitHubToken, envGitHubToken, envGitHubOwner, envGitHubRepo, envGitHubBranch))
			}
			ghOwner = os.Getenv(envGitHubOwner)
			if ghOwner == "" {
				Skip(fmt.Sprintf("skipping: %s is not set", envGitHubOwner))
			}
			ghRepo = os.Getenv(envGitHubRepo)
			if ghRepo == "" {
				Skip(fmt.Sprintf("skipping: %s is not set", envGitHubRepo))
			}
			githubBranch := os.Getenv(envGitHubBranch)
			if githubBranch == "" {
				githubBranch = "main"
			}

			githubRepoURL := fmt.Sprintf("https://github.com/%s/%s.git", ghOwner, ghRepo)

			// Initialise the GitHub API client used for PR verification and cleanup.
			// Use a 30 s timeout so stalled API calls fail within the Eventually window.
			ghClient = github.NewClient(&http.Client{
				Timeout: 30 * time.Second,
			}).WithAuthToken(githubToken)

			// ---------------------------------------------------------------
			// 2. Spin up a namespace-scoped ArgoCD instance.
			// ---------------------------------------------------------------
			By("creating simple namespace-scoped Argo CD instance with image updater enabled")
			ns, cleanupFunc = fixture.CreateRandomE2ETestNamespaceWithCleanupFunc()

			By("creating local git repo")
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
								Value: "0",
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
			for _, deplName := range []string{
				"argocd-redis",
				"argocd-server",
				"argocd-repo-server",
				"argocd-argocd-image-updater-controller",
			} {
				depl := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: deplName, Namespace: ns.Name}}
				Eventually(depl).Should(k8sFixture.ExistByName())
				Eventually(depl).Should(deplFixture.HaveReplicas(1))
				Eventually(depl, "3m", "3s").Should(deplFixture.HaveReadyReplicas(1), deplName+" was not ready")
			}

			statefulSet := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "argocd-application-controller", Namespace: ns.Name}}
			Eventually(statefulSet).Should(k8sFixture.ExistByName())
			Eventually(statefulSet).Should(ssFixture.HaveReplicas(1))
			Eventually(statefulSet, "3m", "3s").Should(ssFixture.HaveReadyReplicas(1))

			// ---------------------------------------------------------------
			// 3. Create the ArgoCD Application (source: local git repo).
			// ---------------------------------------------------------------
			By("creating Application")
			app := &appv1alpha1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "app-01",
					Namespace: ns.Name,
				},
				Spec: appv1alpha1.ApplicationSpec{
					Project: "default",
					Source: &appv1alpha1.ApplicationSource{
						RepoURL:        fmt.Sprintf("https://%s.%s.svc.cluster.local:8081/testdata.git", iuFixture.Name, ns.Name),
						Path:           "1-009-git-pull-request",
						TargetRevision: "HEAD",
					},
					Destination: appv1alpha1.ApplicationDestination{
						Server:    "https://kubernetes.default.svc",
						Namespace: ns.Name,
					},
					SyncPolicy: &appv1alpha1.SyncPolicy{
						Automated: &appv1alpha1.SyncPolicyAutomated{
							Prune: true,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, app)).To(Succeed())

			By("verifying Application deployed successfully")
			Eventually(app, "4m", "3s").Should(applicationFixture.HaveHealthStatusCode(health.HealthStatusHealthy))
			Eventually(app, "4m", "3s").Should(applicationFixture.HaveSyncStatusCode(appv1alpha1.SyncStatusCodeSynced))

			// ---------------------------------------------------------------
			// 4. Store GitHub credentials so the image updater can push and
			//    call the GitHub API.
			// ---------------------------------------------------------------
			By("creating GitHub credentials secret")
			Expect(k8sClient.Create(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      githubCredsSecretName,
					Namespace: ns.Name,
				},
				StringData: map[string]string{
					// GitHub accepts any non-empty username when authenticating
					// with a PAT; "x-access-token" is the canonical choice.
					"username": "x-access-token",
					"password": githubToken,
				},
			})).To(Succeed())

			// ---------------------------------------------------------------
			// 5. Create the ImageUpdater CR in GitHub pull-request mode.
			//    Write-back: push a head branch to GitHub and open a PR.
			//    Application source stays on the local git server — the PR
			//    target repo is fully independent.
			// ---------------------------------------------------------------
			By("creating ImageUpdater CR with GitHub pull-request write-back mode")
			updateStrategy := "semver"
			forceUpdate := false
			method := fmt.Sprintf("git:secret:%s/%s", ns.Name, githubCredsSecretName)

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
							Branch:     &githubBranch,
							Repository: &githubRepoURL,
							PullRequest: &imageUpdaterApi.PullRequest{
								GitHub: &imageUpdaterApi.PullRequestGitHub{},
							},
						},
					},
					ApplicationRefs: []imageUpdaterApi.ApplicationRef{
						{
							NamePattern: "app*",
							Images: []imageUpdaterApi.ImageConfig{
								{
									Alias:     "test",
									ImageName: "quay.io/dkarpele/my-guestbook:29437546.X",
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, imageUpdater)).To(Succeed())

			// ---------------------------------------------------------------
			// 6. Wait for the PR to appear on GitHub and capture its metadata.
			// ---------------------------------------------------------------
			By("waiting for the GitHub PR to be created")
			expectedTitle := "build: automatic update of app-01"
			expectedHeadPrefix := fmt.Sprintf("image-updater-%s-app-01-", ns.Name)
			expectedBody := "updates image dkarpele/my-guestbook tag '1.0.0' to '29437546.0'"

			triggerRefresh := iuFixture.TriggerArgoCDRefresh(ctx, k8sClient, app)

			var foundPR *github.PullRequest
			Eventually(func() bool {
				// Periodically nudge ArgoCD so the image updater reconciles.
				triggerRefresh()

				prs, _, err := ghClient.PullRequests.List(ctx, ghOwner, ghRepo,
					&github.PullRequestListOptions{
						State:       "open",
						Base:        githubBranch,
						ListOptions: github.ListOptions{PerPage: 100},
					})
				if err != nil {
					GinkgoWriter.Printf("error listing PRs: %v\n", err)
					return false
				}

				for _, pr := range prs {
					if pr.GetTitle() == expectedTitle &&
						strings.HasPrefix(pr.GetHead().GetRef(), expectedHeadPrefix) {
						foundPR = pr
						prNumber = pr.GetNumber()
						prHeadBranch = pr.GetHead().GetRef()
						return true
					}
				}
				return false
			}, "8m", "5s").Should(BeTrue(),
				"expected a GitHub PR with title %q targeting base branch %q to be created", expectedTitle, githubBranch)

			// ---------------------------------------------------------------
			// 7. Verify PR properties.
			// ---------------------------------------------------------------
			By("verifying PR title")
			Expect(foundPR.GetTitle()).To(Equal(expectedTitle))

			By("verifying PR body")
			Expect(foundPR.GetBody()).To(ContainSubstring(expectedBody))

			By("verifying PR base branch")
			Expect(foundPR.GetBase().GetRef()).To(Equal(githubBranch),
				"PR should target the configured base branch")

			By("verifying PR head branch naming convention")
			Expect(strings.HasPrefix(foundPR.GetHead().GetRef(), expectedHeadPrefix)).To(BeTrue(),
				"head branch %q should start with %q", foundPR.GetHead().GetRef(), expectedHeadPrefix)

			By("verifying PR head branch belongs to the configured repository owner")
			Expect(foundPR.GetHead().GetRepo().GetFullName()).To(Equal(ghOwner + "/" + ghRepo))

			By("verifying PR author (head repository owner matches the credentials owner)")
			Expect(foundPR.GetUser().GetLogin()).ToNot(BeEmpty(),
				"PR should have a non-empty author login")

			// ---------------------------------------------------------------
			// 8. Verify the commit on the head branch contains the update.
			// ---------------------------------------------------------------
			By("verifying the PR contains at least one commit")
			commits, _, err := ghClient.PullRequests.ListCommits(ctx, ghOwner, ghRepo, prNumber, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(commits).ToNot(BeEmpty(), "PR should contain at least one commit")

			By("verifying the commit message references the image update")
			latestCommit := commits[len(commits)-1]
			commitMsg := latestCommit.GetCommit().GetMessage()
			Expect(commitMsg).ToNot(BeEmpty(), "commit message should not be empty")
			GinkgoWriter.Printf("PR #%d head branch: %s\n", prNumber, prHeadBranch)
			GinkgoWriter.Printf("PR commit message: %s\n", commitMsg)

			By("verifying the commit modifies the argocd-source override file for app-01")
			files, _, err := ghClient.PullRequests.ListFiles(ctx, ghOwner, ghRepo, prNumber, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(files).ToNot(BeEmpty(), "PR should modify at least one file")

			var hasSourceFile bool
			for _, f := range files {
				if strings.HasSuffix(f.GetFilename(), ".argocd-source-app-01.yaml") {
					hasSourceFile = true
					GinkgoWriter.Printf("modified file: %s\n", f.GetFilename())
					break
				}
			}
			Expect(hasSourceFile).To(BeTrue(),
				"PR should modify .argocd-source-app-01.yaml to store the image override")
		})
	})
})
