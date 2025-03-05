This test case verifies the git repository matching functionality for multi-source applications.

The test ensures that the image updater correctly:
- Identifies the right git repository source from multiple sources in an Application
- Uses the correct branch (main) for the values source
- Configures the write-back target properly

This test case uses image from public container registry (docker.io/bitnami/nginx) and application source from public GitHub repo.

To run this individual test case:

* Make sure both docker daemon and k8s cluster is running
* `cd $HOME/go/src/image-updater/test/e2e`
* `SRC_DIR=$HOME/go/src/image-updater kubectl kuttl test --namespace argocd-image-updater-e2e --timeout 120 --test 104-multi-source-git-repository-matching`

Test output:
```bash

=== RUN   kuttl
    harness.go:464: starting setup
    harness.go:255: running tests using configured kubeconfig.
    harness.go:278: Successful connection to cluster at: https://127.0.0.1:56706
    harness.go:363: running tests
    harness.go:75: going to run test suite with timeout of 120 seconds for each step
    harness.go:375: testsuite: ./suite has 10 tests
=== RUN   kuttl/harness
=== RUN   kuttl/harness/104-multi-source-git-repository-matching
=== PAUSE kuttl/harness/104-multi-source-git-repository-matching
=== CONT  kuttl/harness/104-multi-source-git-repository-matching
    logger.go:42: 16:00:42 | 104-multi-source-git-repository-matching | Skipping creation of user-supplied namespace: argocd-image-updater-e2e
    logger.go:42: 16:00:42 | 104-multi-source-git-repository-matching/1-install | starting test step 1-install
    logger.go:42: 16:00:42 | 104-multi-source-git-repository-matching/1-install | Namespace:/image-updater-e2e-104 updated
    logger.go:42: 16:00:42 | 104-multi-source-git-repository-matching/1-install | Application:argocd/write-helmvalues created
    logger.go:42: 16:00:42 | 104-multi-source-git-repository-matching/1-install | test step completed 1-install
    logger.go:42: 16:00:42 | 104-multi-source-git-repository-matching/2-verify-branch | starting test step 2-verify-branch
    logger.go:42: 16:00:42 | 104-multi-source-git-repository-matching/2-verify-branch | running command: [sh -c echo "=== Running Image Updater to Test Branch Selection ==="
        
        # Run image updater with trace logging
        ${SRC_DIR}/dist/argocd-image-updater run --once \
          --argocd-namespace argocd \
          --match-application-name write-helmvalues \
          --loglevel trace 2>&1 | tee /tmp/updater.log
        
        echo "\n=== Verifying Application Configuration ==="
        
        # Check if values source is correctly configured to use 'main' branch
        VALUES_BRANCH=$(kubectl get application write-helmvalues -n argocd -o jsonpath='{.spec.sources[?(@.ref=="values")].targetRevision}')
        if [ "$VALUES_BRANCH" != "main" ]; then
          echo "Error: Values source is using incorrect branch: $VALUES_BRANCH (expected: main)"
          exit 1
        fi
        
        # Check if write-back target is correctly configured
        WRITE_TARGET=$(kubectl get application write-helmvalues -n argocd -o jsonpath='{.metadata.annotations.argocd-image-updater\.argoproj\.io/write-back-target}')
        if [[ ! "$WRITE_TARGET" =~ ^helmvalues:(/|//).*/values\.yaml$ ]]; then
          echo "Error: Write-back target is incorrectly configured: $WRITE_TARGET"
          exit 1
        fi
        
        # Verify git repository configuration
        GIT_REPO=$(kubectl get application write-helmvalues -n argocd -o jsonpath='{.metadata.annotations.argocd-image-updater\.argoproj\.io/git-repository}')
        if [ -z "$GIT_REPO" ]; then
          echo "Error: Git repository not configured in annotations"
          exit 1
        fi
        
        echo "Success: Application configuration verified - using correct branch and write-back settings"
        ]
    logger.go:42: 16:00:42 | 104-multi-source-git-repository-matching/2-verify-branch | === Running Image Updater to Test Branch Selection ===
    logger.go:42: 16:00:42 | 104-multi-source-git-repository-matching/2-verify-branch | time="2025-03-05T16:00:42-05:00" level=info msg="argocd-image-updater v99.9.9+1027f80 starting [loglevel:TRACE, interval:once, healthport:off]"
    logger.go:42: 16:00:42 | 104-multi-source-git-repository-matching/2-verify-branch | time="2025-03-05T16:00:42-05:00" level=warning msg="commit message template at /app/config/commit.template does not exist, using default"
    logger.go:42: 16:00:42 | 104-multi-source-git-repository-matching/2-verify-branch | time="2025-03-05T16:00:42-05:00" level=debug msg="Successfully parsed commit message template"
    logger.go:42: 16:00:42 | 104-multi-source-git-repository-matching/2-verify-branch | time="2025-03-05T16:00:42-05:00" level=warning msg="Registry configuration at /app/config/registries.conf could not be read: stat /app/config/registries.conf: no such file or directory -- using default configuration"
    logger.go:42: 16:00:42 | 104-multi-source-git-repository-matching/2-verify-branch | time="2025-03-05T16:00:42-05:00" level=info msg="ArgoCD configuration: [apiKind=kubernetes, server=argocd-server.argocd, auth_token=false, insecure=false, grpc_web=false, plaintext=false]"
    logger.go:42: 16:00:42 | 104-multi-source-git-repository-matching/2-verify-branch | time="2025-03-05T16:00:42-05:00" level=info msg="Starting metrics server on TCP port=8081"
    logger.go:42: 16:00:42 | 104-multi-source-git-repository-matching/2-verify-branch | time="2025-03-05T16:00:42-05:00" level=info msg="Warming up image cache"
    logger.go:42: 16:00:43 | 104-multi-source-git-repository-matching/2-verify-branch | time="2025-03-05T16:00:43-05:00" level=debug msg="Applications listed: 3"
    logger.go:42: 16:00:43 | 104-multi-source-git-repository-matching/2-verify-branch | time="2025-03-05T16:00:43-05:00" level=warning msg="skipping app 'argocd-image-updater-e2e/image-updater-104' of type 'Directory' because it's not of supported source type" application=image-updater-104 namespace=argocd-image-updater-e2e
    logger.go:42: 16:00:43 | 104-multi-source-git-repository-matching/2-verify-branch | time="2025-03-05T16:00:43-05:00" level=warning msg="skipping app 'argocd-image-updater-e2e/write-helmvalues' of type 'Directory' because it's not of supported source type" application=write-helmvalues namespace=argocd-image-updater-e2e
    logger.go:42: 16:00:43 | 104-multi-source-git-repository-matching/2-verify-branch | time="2025-03-05T16:00:43-05:00" level=warning msg="skipping app 'argocd/write-helmvalues' of type 'Directory' because it's not of supported source type" application=write-helmvalues namespace=argocd
    logger.go:42: 16:00:43 | 104-multi-source-git-repository-matching/2-verify-branch | time="2025-03-05T16:00:43-05:00" level=info msg="Finished cache warm-up, pre-loaded 0 meta data entries from 1 registries"
    logger.go:42: 16:00:43 | 104-multi-source-git-repository-matching/2-verify-branch | time="2025-03-05T16:00:43-05:00" level=debug msg="Starting askpass server"
    logger.go:42: 16:00:43 | 104-multi-source-git-repository-matching/2-verify-branch | time="2025-03-05T16:00:43-05:00" level=debug msg="Applications listed: 3"
    logger.go:42: 16:00:43 | 104-multi-source-git-repository-matching/2-verify-branch | time="2025-03-05T16:00:43-05:00" level=warning msg="skipping app 'argocd-image-updater-e2e/image-updater-104' of type 'Directory' because it's not of supported source type" application=image-updater-104 namespace=argocd-image-updater-e2e
    logger.go:42: 16:00:43 | 104-multi-source-git-repository-matching/2-verify-branch | time="2025-03-05T16:00:43-05:00" level=warning msg="skipping app 'argocd-image-updater-e2e/write-helmvalues' of type 'Directory' because it's not of supported source type" application=write-helmvalues namespace=argocd-image-updater-e2e
    logger.go:42: 16:00:43 | 104-multi-source-git-repository-matching/2-verify-branch | time="2025-03-05T16:00:43-05:00" level=warning msg="skipping app 'argocd/write-helmvalues' of type 'Directory' because it's not of supported source type" application=write-helmvalues namespace=argocd
    logger.go:42: 16:00:43 | 104-multi-source-git-repository-matching/2-verify-branch | time="2025-03-05T16:00:43-05:00" level=info msg="Starting image update cycle, considering 0 annotated application(s) for update"
    logger.go:42: 16:00:43 | 104-multi-source-git-repository-matching/2-verify-branch | time="2025-03-05T16:00:43-05:00" level=info msg="Processing results: applications=0 images_considered=0 images_skipped=0 images_updated=0 errors=0"
    logger.go:42: 16:00:43 | 104-multi-source-git-repository-matching/2-verify-branch | time="2025-03-05T16:00:43-05:00" level=info msg=Finished.
    logger.go:42: 16:00:43 | 104-multi-source-git-repository-matching/2-verify-branch | 
    logger.go:42: 16:00:43 | 104-multi-source-git-repository-matching/2-verify-branch | === Verifying Application Configuration ===
    logger.go:42: 16:00:43 | 104-multi-source-git-repository-matching/2-verify-branch | Success: Application configuration verified - using correct branch and write-back settings
    logger.go:42: 16:00:43 | 104-multi-source-git-repository-matching/2-verify-branch | test step completed 2-verify-branch
    logger.go:42: 16:00:43 | 104-multi-source-git-repository-matching/99-delete | starting test step 99-delete
    logger.go:42: 16:00:48 | 104-multi-source-git-repository-matching/99-delete | test step completed 99-delete
    logger.go:42: 16:00:48 | 104-multi-source-git-repository-matching | skipping kubernetes event logging
=== NAME  kuttl
    harness.go:407: run tests finished
    harness.go:515: cleaning up
    harness.go:572: removing temp folder: ""
--- PASS: kuttl (5.45s)
    --- PASS: kuttl/harness (0.00s)
        --- PASS: kuttl/harness/104-multi-source-git-repository-matching (5.42s)
PASS
```