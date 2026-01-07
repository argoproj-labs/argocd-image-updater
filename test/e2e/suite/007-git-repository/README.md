This test case verifies [possibility to specify write-back GIT Kustomize repository as annotation](https://argocd-image-updater.readthedocs.io/en/release-0.18/basics/update-methods/#specifying-a-repository-when-using-a-helm-repository-in-repourl).

This test case performs the following steps:
* install ArgoCD Application to the test cluster with GIT write-back method
* run argocd-image-updater
* assert that the expected image was installed
* revert the changes made to git repository 
* uninstall Application from the test cluster

To run this individual test case,

* make sure both docker daemon and k8s cluster is running
* `cd $HOME/go/src/image-updater/test/e2e`
* `SRC_DIR=$HOME/go/src/argocd-image-updater kubectl kuttl test --namespace argocd-image-updater-e2e --timeout 120 --test 007-git-repository`

Test output:
```bash
=== RUN   kuttl
    harness.go:459: starting setup
    harness.go:254: running tests using configured kubeconfig.
    harness.go:277: Successful connection to cluster at: https://127.0.0.1:6443
    harness.go:362: running tests
    harness.go:74: going to run test suite with timeout of 120 seconds for each step
    harness.go:374: testsuite: ./suite has 10 tests
=== RUN   kuttl/harness
=== RUN   kuttl/harness/007-git-repository
=== PAUSE kuttl/harness/007-git-repository
=== CONT  kuttl/harness/007-git-repository
    logger.go:42: 19:08:16 | 007-git-repository | Ignoring README.md as it does not match file name regexp: ^(\d+)-(?:[^\.]+)(?:\.yaml)?$
    logger.go:42: 19:08:16 | 007-git-repository | Ignoring prepare_assert.sh as it does not match file name regexp: ^(\d+)-(?:[^\.]+)(?:\.yaml)?$
    logger.go:42: 19:08:16 | 007-git-repository | Ignoring revert_commit.sh as it does not match file name regexp: ^(\d+)-(?:[^\.]+)(?:\.yaml)?$
    logger.go:42: 19:08:16 | 007-git-repository | Skipping creation of user-supplied namespace: argocd-image-updater-e2e
    logger.go:42: 19:08:16 | 007-git-repository/1-install | starting test step 1-install
    logger.go:42: 19:08:16 | 007-git-repository/1-install | Namespace:/image-updater-e2e-007 created
Warning: metadata.finalizers: "resources-finalizer.argocd.argoproj.io": prefer a domain-qualified finalizer name to avoid accidental conflicts with other finalizer writers
    logger.go:42: 19:08:16 | 007-git-repository/1-install | Application:argocd-image-updater-e2e/image-updater-007 created
    logger.go:42: 19:10:16 | 007-git-repository/1-install | test step failed 1-install
    case.go:396: failed in step 1-install
    case.go:398: --- Application:argocd-image-updater-e2e/image-updater-007
        +++ Application:argocd-image-updater-e2e/image-updater-007
        @@ -1,19 +1,62 @@
         apiVersion: argoproj.io/v1alpha1
         kind: Application
         metadata:
        +  annotations:
        +    argocd-image-updater.argoproj.io/force-update: "false"
        +    argocd-image-updater.argoproj.io/git-repository: https://10.42.0.1:30003/testdata.git
        +    argocd-image-updater.argoproj.io/image-list: test=10.42.0.1:30000/test-image:1.X.X
        +    argocd-image-updater.argoproj.io/update-strategy: semver
        +    argocd-image-updater.argoproj.io/write-back-method: git
        +    argocd-image-updater.argoproj.io/write-back-target: kustomization
        +  finalizers:
        +  - resources-finalizer.argocd.argoproj.io
        +  managedFields: '[... elided field over 10 lines long ...]'
           name: image-updater-007
           namespace: argocd-image-updater-e2e
         spec:
        +  destination:
        +    namespace: image-updater-e2e-007
        +    server: https://kubernetes.default.svc
        +  project: default
           source:
             path: ./001-simple-kustomize-app
             repoURL: https://10.42.0.1:30003/testdata.git
             targetRevision: master
        +  syncPolicy:
        +    automated: {}
        +    retry:
        +      limit: 2
         status:
        +  controllerNamespace: argocd-image-updater-e2e
           health:
        +    lastTransitionTime: "2025-03-17T18:08:17Z"
             status: Healthy
        +  history: '[... elided field over 10 lines long ...]'
        +  operationState: '[... elided field over 10 lines long ...]'
        +  reconciledAt: "2025-03-17T18:08:16Z"
        +  resources:
        +  - group: apps
        +    health:
        +      status: Healthy
        +    kind: Deployment
        +    name: e2e-registry
        +    namespace: image-updater-e2e-007
        +    status: Synced
        +    version: v1
        +  sourceHydrator: {}
        +  sourceType: Kustomize
           summary:
             images:
        -    - 10.42.0.1:30000/test-image:1.0.0
        +    - 10.42.0.1:30000/test-image:1.0.2
           sync:
        +    comparedTo:
        +      destination:
        +        namespace: image-updater-e2e-007
        +        server: https://kubernetes.default.svc
        +      source:
        +        path: ./001-simple-kustomize-app
        +        repoURL: https://10.42.0.1:30003/testdata.git
        +        targetRevision: master
        +    revision: 716acd062bb139a3d4d31c2570520f928be63eff
             status: Synced
         
        
    case.go:398: resource Application:argocd-image-updater-e2e/image-updater-007: .status.summary.images: value mismatch, expected: 10.42.0.1:30000/test-image:1.0.0 != actual: 10.42.0.1:30000/test-image:1.0.2
    logger.go:42: 19:10:16 | 007-git-repository | skipping kubernetes event logging
=== NAME  kuttl
    harness.go:403: run tests finished
    harness.go:510: cleaning up
    harness.go:567: removing temp folder: ""
--- FAIL: kuttl (120.49s)
    --- FAIL: kuttl/harness (0.00s)
        --- FAIL: kuttl/harness/007-git-repository (120.47s)
FAIL
dkarpele@fedora:~/go/src/argocd-image-updater/test/e2e$ SRC_DIR=$HOME/go/src/argocd-image-updater kubectl kuttl test --namespace argocd-image-updater-e2e --timeout 120 --test 007-git-repository
=== RUN   kuttl
    harness.go:459: starting setup
    harness.go:254: running tests using configured kubeconfig.
    harness.go:277: Successful connection to cluster at: https://127.0.0.1:6443
    harness.go:362: running tests
    harness.go:74: going to run test suite with timeout of 120 seconds for each step
    harness.go:374: testsuite: ./suite has 10 tests
=== RUN   kuttl/harness
=== RUN   kuttl/harness/007-git-repository
=== PAUSE kuttl/harness/007-git-repository
=== CONT  kuttl/harness/007-git-repository
    logger.go:42: 19:12:07 | 007-git-repository | Ignoring README.md as it does not match file name regexp: ^(\d+)-(?:[^\.]+)(?:\.yaml)?$
    logger.go:42: 19:12:07 | 007-git-repository | Ignoring prepare_assert.sh as it does not match file name regexp: ^(\d+)-(?:[^\.]+)(?:\.yaml)?$
    logger.go:42: 19:12:07 | 007-git-repository | Ignoring revert_commit.sh as it does not match file name regexp: ^(\d+)-(?:[^\.]+)(?:\.yaml)?$
    logger.go:42: 19:12:07 | 007-git-repository | Skipping creation of user-supplied namespace: argocd-image-updater-e2e
    logger.go:42: 19:12:07 | 007-git-repository/1-install | starting test step 1-install
    logger.go:42: 19:12:07 | 007-git-repository/1-install | Namespace:/image-updater-e2e-007 created
Warning: metadata.finalizers: "resources-finalizer.argocd.argoproj.io": prefer a domain-qualified finalizer name to avoid accidental conflicts with other finalizer writers
    logger.go:42: 19:12:07 | 007-git-repository/1-install | Application:argocd-image-updater-e2e/image-updater-007 created
    logger.go:42: 19:12:09 | 007-git-repository/1-install | test step completed 1-install
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | starting test step 2-run-updater
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | running command: [sh -c ${SRC_DIR}/dist/argocd-image-updater run --once \
          --argocd-namespace argocd-image-updater-e2e \
          --registries-conf-path ${SRC_DIR}/test/e2e/assets/registries.conf \
          --loglevel trace
        ]
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=info msg="argocd-image-updater v99.9.9+d6a78eb starting [loglevel:TRACE, interval:once, healthport:off]"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=warning msg="commit message template at /app/config/commit.template does not exist, using default"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=debug msg="Successfully parsed commit message template"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=debug msg="rate limiting is disabled" prefix="10.42.0.1:30000" registry="https://10.42.0.1:30000"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=info msg="Loaded 1 registry configurations from /home/dkarpele/go/src/argocd-image-updater/test/e2e/assets/registries.conf"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=info msg="ArgoCD configuration: [apiKind=kubernetes, server=argocd-server.argocd-image-updater-e2e, auth_token=false, insecure=false, grpc_web=false, plaintext=false]"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=info msg="Starting metrics server on TCP port=8081"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=info msg="Warming up image cache"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=debug msg="Applications listed: 1"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=trace msg="processing app 'argocd-image-updater-e2e/image-updater-007' of type 'Kustomize'" application=image-updater-007 namespace=argocd-image-updater-e2e
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=debug msg="Processing application argocd-image-updater-e2e/image-updater-007"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=debug msg="Considering this image for update" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.0 registry="10.42.0.1:30000"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=debug msg="Using version constraint '1.X.X' when looking for a new tag" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.0 registry="10.42.0.1:30000"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=trace msg="Found update strategy semver" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.X.X registry_url="10.42.0.1:30000"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=trace msg="No match annotation found" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.X.X registry_url="10.42.0.1:30000"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=trace msg="No ignore-tags annotation found" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.X.X registry_url="10.42.0.1:30000"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=trace msg="Using runtime platform constraint linux/amd64" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.X.X registry_url="10.42.0.1:30000"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=trace msg="No pull-secret annotation found" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.X.X registry_url="10.42.0.1:30000"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=trace msg="Performing HTTP GET https://10.42.0.1:30000/v2/test-image/tags/list"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=trace msg="List of available tags found: [1.0.0 1.0.1 1.0.2 latest]" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.0 registry="10.42.0.1:30000"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=debug msg="could not parse input tag latest as semver: Invalid Semantic Version"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=trace msg="Finding out whether to consider 1.0.0 for being updateable" image="10.42.0.1:30000/test-image:1.0.0"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=trace msg="Finding out whether to consider 1.0.1 for being updateable" image="10.42.0.1:30000/test-image:1.0.0"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=trace msg="Finding out whether to consider 1.0.2 for being updateable" image="10.42.0.1:30000/test-image:1.0.0"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=debug msg="found 3 from 3 tags eligible for consideration" image="10.42.0.1:30000/test-image:1.0.0"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=info msg="Setting new image to 10.42.0.1:30000/test-image:1.0.2" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.0 registry="10.42.0.1:30000"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=trace msg="Setting Kustomize parameter 10.42.0.1:30000/test-image:1.0.2" application=image-updater-007
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=info msg="Successfully updated image '10.42.0.1:30000/test-image:1.0.0' to '10.42.0.1:30000/test-image:1.0.2', but pending spec update (dry run=true)" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.0 registry="10.42.0.1:30000"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=debug msg="Using commit message: build: automatic update of image-updater-007\n\nupdates image test-image tag '1.0.0' to '1.0.2'\n"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=info msg="Dry run - not committing 1 changes to application" application=image-updater-007
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=info msg="Finished cache warm-up, pre-loaded 0 meta data entries from 2 registries"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=debug msg="Starting askpass server"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=debug msg="Applications listed: 1"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=trace msg="processing app 'argocd-image-updater-e2e/image-updater-007' of type 'Kustomize'" application=image-updater-007 namespace=argocd-image-updater-e2e
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=info msg="Starting image update cycle, considering 1 annotated application(s) for update"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=debug msg="Processing application argocd-image-updater-e2e/image-updater-007"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=debug msg="Considering this image for update" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.0 registry="10.42.0.1:30000"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=debug msg="Using version constraint '1.X.X' when looking for a new tag" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.0 registry="10.42.0.1:30000"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=trace msg="Found update strategy semver" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.X.X registry_url="10.42.0.1:30000"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=trace msg="No match annotation found" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.X.X registry_url="10.42.0.1:30000"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=trace msg="No ignore-tags annotation found" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.X.X registry_url="10.42.0.1:30000"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=trace msg="Using runtime platform constraint linux/amd64" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.X.X registry_url="10.42.0.1:30000"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=trace msg="No pull-secret annotation found" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.X.X registry_url="10.42.0.1:30000"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=trace msg="Performing HTTP GET https://10.42.0.1:30000/v2/test-image/tags/list"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=trace msg="List of available tags found: [1.0.0 1.0.1 1.0.2 latest]" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.0 registry="10.42.0.1:30000"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=debug msg="could not parse input tag latest as semver: Invalid Semantic Version"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=trace msg="Finding out whether to consider 1.0.0 for being updateable" image="10.42.0.1:30000/test-image:1.0.0"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=trace msg="Finding out whether to consider 1.0.1 for being updateable" image="10.42.0.1:30000/test-image:1.0.0"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=trace msg="Finding out whether to consider 1.0.2 for being updateable" image="10.42.0.1:30000/test-image:1.0.0"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=debug msg="found 3 from 3 tags eligible for consideration" image="10.42.0.1:30000/test-image:1.0.0"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=info msg="Setting new image to 10.42.0.1:30000/test-image:1.0.2" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.0 registry="10.42.0.1:30000"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=trace msg="Setting Kustomize parameter 10.42.0.1:30000/test-image:1.0.2" application=image-updater-007
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=info msg="Successfully updated image '10.42.0.1:30000/test-image:1.0.0' to '10.42.0.1:30000/test-image:1.0.2', but pending spec update (dry run=false)" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.0 registry="10.42.0.1:30000"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=debug msg="Using commit message: build: automatic update of image-updater-007\n\nupdates image test-image tag '1.0.0' to '1.0.2'\n"
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=info msg="Committing 1 parameter update(s) for application image-updater-007" application=image-updater-007
    logger.go:42: 19:12:09 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:09+01:00" level=info msg="Starting configmap/secret informers"
    logger.go:42: 19:12:10 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:10+01:00" level=info msg="Configmap/secret informer synced"
    logger.go:42: 19:12:10 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:10+01:00" level=info msg="Initializing https://10.42.0.1:30003/testdata.git to /tmp/git-image-updater-0072868329450"
    logger.go:42: 19:12:10 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:10+01:00" level=info msg="configmap informer cancelled"
    logger.go:42: 19:12:10 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:10+01:00" level=info msg="secrets informer cancelled"
    logger.go:42: 19:12:10 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:10+01:00" level=trace msg="targetRevision for update is 'master'" application=image-updater-007
    logger.go:42: 19:12:10 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:10+01:00" level=info msg="git fetch origin master --force --prune --depth 1" dir=/tmp/git-image-updater-0072868329450 execID=02156
    logger.go:42: 19:12:10 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:10+01:00" level=info msg=Trace args="[git fetch origin master --force --prune --depth 1]" dir=/tmp/git-image-updater-0072868329450 operation_name="exec git" time_ms=189.47612
    logger.go:42: 19:12:10 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:10+01:00" level=info msg="git checkout --force master" dir=/tmp/git-image-updater-0072868329450 execID=84375
    logger.go:42: 19:12:10 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:10+01:00" level=info msg=Trace args="[git checkout --force master]" dir=/tmp/git-image-updater-0072868329450 operation_name="exec git" time_ms=10.715367
    logger.go:42: 19:12:10 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:10+01:00" level=info msg="git clean -ffdx" dir=/tmp/git-image-updater-0072868329450 execID=0f0af
    logger.go:42: 19:12:10 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:10+01:00" level=info msg=Trace args="[git clean -ffdx]" dir=/tmp/git-image-updater-0072868329450 operation_name="exec git" time_ms=2.487301
    logger.go:42: 19:12:10 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:10+01:00" level=info msg="updating base /tmp/git-image-updater-0072868329450/001-simple-kustomize-app" application=image-updater-007
    logger.go:42: 19:12:10 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:10+01:00" level=debug msg="Writing commit message to /tmp/image-updater-commit-msg1661399210" application=image-updater-007
    logger.go:42: 19:12:10 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:10+01:00" level=info msg="git config user.name argocd-image-updater" dir=/tmp/git-image-updater-0072868329450 execID=68436
    logger.go:42: 19:12:10 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:10+01:00" level=info msg=Trace args="[git config user.name argocd-image-updater]" dir=/tmp/git-image-updater-0072868329450 operation_name="exec git" time_ms=3.293516
    logger.go:42: 19:12:10 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:10+01:00" level=info msg="git config user.email noreply@argoproj.io" dir=/tmp/git-image-updater-0072868329450 execID=04f05
    logger.go:42: 19:12:10 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:10+01:00" level=info msg=Trace args="[git config user.email noreply@argoproj.io]" dir=/tmp/git-image-updater-0072868329450 operation_name="exec git" time_ms=2.263721
    logger.go:42: 19:12:10 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:10+01:00" level=info msg="git -c gpg.format=openpgp commit -a -F /tmp/image-updater-commit-msg1661399210" dir=/tmp/git-image-updater-0072868329450 execID=1b52b
    logger.go:42: 19:12:10 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:10+01:00" level=info msg=Trace args="[git -c gpg.format=openpgp commit -a -F /tmp/image-updater-commit-msg1661399210]" dir=/tmp/git-image-updater-0072868329450 operation_name="exec git" time_ms=8.320400000000001
    logger.go:42: 19:12:10 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:10+01:00" level=info msg="git push origin master" dir=/tmp/git-image-updater-0072868329450 execID=04ab2
    logger.go:42: 19:12:10 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:10+01:00" level=info msg=Trace args="[git push origin master]" dir=/tmp/git-image-updater-0072868329450 operation_name="exec git" time_ms=211.18369
    logger.go:42: 19:12:10 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:10+01:00" level=info msg="Successfully updated the live application spec" application=image-updater-007
    logger.go:42: 19:12:10 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:10+01:00" level=info msg="Processing results: applications=1 images_considered=1 images_skipped=0 images_updated=1 errors=0"
    logger.go:42: 19:12:10 | 007-git-repository/2-run-updater | time="2025-03-17T19:12:10+01:00" level=info msg=Finished.
    logger.go:42: 19:12:50 | 007-git-repository/2-run-updater | test step completed 2-run-updater
    logger.go:42: 19:12:50 | 007-git-repository/3-run-updater-revert | starting test step 3-run-updater-revert
    logger.go:42: 19:12:50 | 007-git-repository/3-run-updater-revert | running command: [sh -c kubectl patch -n $NAMESPACE application image-updater-007 \
          --type=merge \
          --patch='{"metadata": {"annotations": {"argocd-image-updater.argoproj.io/image-list": "test=10.42.0.1:30000/test-image:1.0.0"}}}'
        ]
    logger.go:42: 19:12:50 | 007-git-repository/3-run-updater-revert | application.argoproj.io/image-updater-007 patched
    logger.go:42: 19:12:50 | 007-git-repository/3-run-updater-revert | running command: [sh -c sleep 2
        ]
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | running command: [sh -c ${SRC_DIR}/dist/argocd-image-updater run --once \
          --argocd-namespace argocd-image-updater-e2e \
          --registries-conf-path ${SRC_DIR}/test/e2e/assets/registries.conf \
          --loglevel trace
        ]
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=info msg="argocd-image-updater v99.9.9+d6a78eb starting [loglevel:TRACE, interval:once, healthport:off]"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=warning msg="commit message template at /app/config/commit.template does not exist, using default"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=debug msg="Successfully parsed commit message template"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=debug msg="rate limiting is disabled" prefix="10.42.0.1:30000" registry="https://10.42.0.1:30000"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=info msg="Loaded 1 registry configurations from /home/dkarpele/go/src/argocd-image-updater/test/e2e/assets/registries.conf"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=info msg="ArgoCD configuration: [apiKind=kubernetes, server=argocd-server.argocd-image-updater-e2e, auth_token=false, insecure=false, grpc_web=false, plaintext=false]"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=info msg="Starting metrics server on TCP port=8081"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=info msg="Warming up image cache"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=debug msg="Applications listed: 1"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=trace msg="processing app 'argocd-image-updater-e2e/image-updater-007' of type 'Kustomize'" application=image-updater-007 namespace=argocd-image-updater-e2e
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=debug msg="Processing application argocd-image-updater-e2e/image-updater-007"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=debug msg="Considering this image for update" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.2 registry="10.42.0.1:30000"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=debug msg="Using version constraint '1.0.0' when looking for a new tag" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.2 registry="10.42.0.1:30000"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=trace msg="Found update strategy semver" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.0.0 registry_url="10.42.0.1:30000"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=trace msg="No match annotation found" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.0.0 registry_url="10.42.0.1:30000"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=trace msg="No ignore-tags annotation found" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.0.0 registry_url="10.42.0.1:30000"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=trace msg="Using runtime platform constraint linux/amd64" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.0.0 registry_url="10.42.0.1:30000"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=trace msg="No pull-secret annotation found" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.0.0 registry_url="10.42.0.1:30000"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=trace msg="Performing HTTP GET https://10.42.0.1:30000/v2/test-image/tags/list"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=trace msg="List of available tags found: [latest 1.0.0 1.0.1 1.0.2]" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.2 registry="10.42.0.1:30000"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=debug msg="could not parse input tag latest as semver: Invalid Semantic Version"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=trace msg="Finding out whether to consider 1.0.0 for being updateable" image="10.42.0.1:30000/test-image:1.0.2"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=trace msg="Finding out whether to consider 1.0.1 for being updateable" image="10.42.0.1:30000/test-image:1.0.2"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=trace msg="1.0.1 did not match constraint 1.0.0" image="10.42.0.1:30000/test-image:1.0.2"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=trace msg="Finding out whether to consider 1.0.2 for being updateable" image="10.42.0.1:30000/test-image:1.0.2"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=trace msg="1.0.2 did not match constraint 1.0.0" image="10.42.0.1:30000/test-image:1.0.2"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=debug msg="found 1 from 3 tags eligible for consideration" image="10.42.0.1:30000/test-image:1.0.2"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=info msg="Setting new image to 10.42.0.1:30000/test-image:1.0.0" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.2 registry="10.42.0.1:30000"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=trace msg="Setting Kustomize parameter 10.42.0.1:30000/test-image:1.0.0" application=image-updater-007
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=info msg="Successfully updated image '10.42.0.1:30000/test-image:1.0.2' to '10.42.0.1:30000/test-image:1.0.0', but pending spec update (dry run=true)" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.2 registry="10.42.0.1:30000"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=debug msg="Using commit message: build: automatic update of image-updater-007\n\nupdates image test-image tag '1.0.2' to '1.0.0'\n"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=info msg="Dry run - not committing 1 changes to application" application=image-updater-007
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=info msg="Finished cache warm-up, pre-loaded 0 meta data entries from 2 registries"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=debug msg="Starting askpass server"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=debug msg="Applications listed: 1"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=trace msg="processing app 'argocd-image-updater-e2e/image-updater-007' of type 'Kustomize'" application=image-updater-007 namespace=argocd-image-updater-e2e
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=info msg="Starting image update cycle, considering 1 annotated application(s) for update"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=debug msg="Processing application argocd-image-updater-e2e/image-updater-007"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=debug msg="Considering this image for update" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.2 registry="10.42.0.1:30000"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=debug msg="Using version constraint '1.0.0' when looking for a new tag" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.2 registry="10.42.0.1:30000"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=trace msg="Found update strategy semver" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.0.0 registry_url="10.42.0.1:30000"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=trace msg="No match annotation found" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.0.0 registry_url="10.42.0.1:30000"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=trace msg="No ignore-tags annotation found" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.0.0 registry_url="10.42.0.1:30000"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=trace msg="Using runtime platform constraint linux/amd64" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.0.0 registry_url="10.42.0.1:30000"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=trace msg="No pull-secret annotation found" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.0.0 registry_url="10.42.0.1:30000"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=trace msg="Performing HTTP GET https://10.42.0.1:30000/v2/test-image/tags/list"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=trace msg="List of available tags found: [1.0.0 1.0.1 1.0.2 latest]" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.2 registry="10.42.0.1:30000"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=debug msg="could not parse input tag latest as semver: Invalid Semantic Version"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=trace msg="Finding out whether to consider 1.0.0 for being updateable" image="10.42.0.1:30000/test-image:1.0.2"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=trace msg="Finding out whether to consider 1.0.1 for being updateable" image="10.42.0.1:30000/test-image:1.0.2"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=trace msg="1.0.1 did not match constraint 1.0.0" image="10.42.0.1:30000/test-image:1.0.2"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=trace msg="Finding out whether to consider 1.0.2 for being updateable" image="10.42.0.1:30000/test-image:1.0.2"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=trace msg="1.0.2 did not match constraint 1.0.0" image="10.42.0.1:30000/test-image:1.0.2"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=debug msg="found 1 from 3 tags eligible for consideration" image="10.42.0.1:30000/test-image:1.0.2"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=info msg="Setting new image to 10.42.0.1:30000/test-image:1.0.0" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.2 registry="10.42.0.1:30000"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=trace msg="Setting Kustomize parameter 10.42.0.1:30000/test-image:1.0.0" application=image-updater-007
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=info msg="Successfully updated image '10.42.0.1:30000/test-image:1.0.2' to '10.42.0.1:30000/test-image:1.0.0', but pending spec update (dry run=false)" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.2 registry="10.42.0.1:30000"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=debug msg="Using commit message: build: automatic update of image-updater-007\n\nupdates image test-image tag '1.0.2' to '1.0.0'\n"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=info msg="Committing 1 parameter update(s) for application image-updater-007" application=image-updater-007
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=info msg="Starting configmap/secret informers"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=info msg="Configmap/secret informer synced"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=info msg="Initializing https://10.42.0.1:30003/testdata.git to /tmp/git-image-updater-0073285759338"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=info msg="configmap informer cancelled"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=info msg="secrets informer cancelled"
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=trace msg="targetRevision for update is 'master'" application=image-updater-007
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=info msg="git fetch origin master --force --prune --depth 1" dir=/tmp/git-image-updater-0073285759338 execID=55620
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=info msg=Trace args="[git fetch origin master --force --prune --depth 1]" dir=/tmp/git-image-updater-0073285759338 operation_name="exec git" time_ms=231.314844
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=info msg="git checkout --force master" dir=/tmp/git-image-updater-0073285759338 execID=8ec63
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=info msg=Trace args="[git checkout --force master]" dir=/tmp/git-image-updater-0073285759338 operation_name="exec git" time_ms=13.073202
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=info msg="git clean -ffdx" dir=/tmp/git-image-updater-0073285759338 execID=954a6
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=info msg=Trace args="[git clean -ffdx]" dir=/tmp/git-image-updater-0073285759338 operation_name="exec git" time_ms=2.999574
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=info msg="updating base /tmp/git-image-updater-0073285759338/001-simple-kustomize-app" application=image-updater-007
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=debug msg="Writing commit message to /tmp/image-updater-commit-msg2957814661" application=image-updater-007
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=info msg="git config user.name argocd-image-updater" dir=/tmp/git-image-updater-0073285759338 execID=c7cd6
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=info msg=Trace args="[git config user.name argocd-image-updater]" dir=/tmp/git-image-updater-0073285759338 operation_name="exec git" time_ms=2.2128959999999998
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=info msg="git config user.email noreply@argoproj.io" dir=/tmp/git-image-updater-0073285759338 execID=4b9f1
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=info msg=Trace args="[git config user.email noreply@argoproj.io]" dir=/tmp/git-image-updater-0073285759338 operation_name="exec git" time_ms=2.219835
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=info msg="git -c gpg.format=openpgp commit -a -F /tmp/image-updater-commit-msg2957814661" dir=/tmp/git-image-updater-0073285759338 execID=c827a
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=info msg=Trace args="[git -c gpg.format=openpgp commit -a -F /tmp/image-updater-commit-msg2957814661]" dir=/tmp/git-image-updater-0073285759338 operation_name="exec git" time_ms=8.51178
    logger.go:42: 19:12:52 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:52+01:00" level=info msg="git push origin master" dir=/tmp/git-image-updater-0073285759338 execID=d8d5c
    logger.go:42: 19:12:53 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:53+01:00" level=info msg=Trace args="[git push origin master]" dir=/tmp/git-image-updater-0073285759338 operation_name="exec git" time_ms=212.315548
    logger.go:42: 19:12:53 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:53+01:00" level=info msg="Successfully updated the live application spec" application=image-updater-007
    logger.go:42: 19:12:53 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:53+01:00" level=info msg="Processing results: applications=1 images_considered=1 images_skipped=0 images_updated=1 errors=0"
    logger.go:42: 19:12:53 | 007-git-repository/3-run-updater-revert | time="2025-03-17T19:12:53+01:00" level=info msg=Finished.
    logger.go:42: 19:14:53 | 007-git-repository/3-run-updater-revert | test step failed 3-run-updater-revert
    case.go:396: failed in step 3-run-updater-revert
    case.go:398: --- Application:argocd-image-updater-e2e/image-updater-007
        +++ Application:argocd-image-updater-e2e/image-updater-007
        @@ -1,19 +1,62 @@
         apiVersion: argoproj.io/v1alpha1
         kind: Application
         metadata:
        +  annotations:
        +    argocd-image-updater.argoproj.io/force-update: "false"
        +    argocd-image-updater.argoproj.io/git-repository: https://10.42.0.1:30003/testdata.git
        +    argocd-image-updater.argoproj.io/image-list: test=10.42.0.1:30000/test-image:1.0.0
        +    argocd-image-updater.argoproj.io/update-strategy: semver
        +    argocd-image-updater.argoproj.io/write-back-method: git
        +    argocd-image-updater.argoproj.io/write-back-target: kustomization
        +  finalizers:
        +  - resources-finalizer.argocd.argoproj.io
        +  managedFields: '[... elided field over 10 lines long ...]'
           name: image-updater-007
           namespace: argocd-image-updater-e2e
         spec:
        +  destination:
        +    namespace: image-updater-e2e-007
        +    server: https://kubernetes.default.svc
        +  project: default
           source:
             path: ./001-simple-kustomize-app
             repoURL: https://10.42.0.1:30003/testdata.git
             targetRevision: master
        +  syncPolicy:
        +    automated: {}
        +    retry:
        +      limit: 2
         status:
        +  controllerNamespace: argocd-image-updater-e2e
           health:
        +    lastTransitionTime: "2025-03-17T18:12:19Z"
             status: Healthy
        +  history: '[... elided field over 10 lines long ...]'
        +  operationState: '[... elided field over 10 lines long ...]'
        +  reconciledAt: "2025-03-17T18:12:17Z"
        +  resources:
        +  - group: apps
        +    health:
        +      status: Healthy
        +    kind: Deployment
        +    name: e2e-registry
        +    namespace: image-updater-e2e-007
        +    status: Synced
        +    version: v1
        +  sourceHydrator: {}
        +  sourceType: Kustomize
           summary:
             images:
        -    - 10.42.0.1:30000/test-image:1.0.0
        +    - 10.42.0.1:30000/test-image:1.0.2
           sync:
        +    comparedTo:
        +      destination:
        +        namespace: image-updater-e2e-007
        +        server: https://kubernetes.default.svc
        +      source:
        +        path: ./001-simple-kustomize-app
        +        repoURL: https://10.42.0.1:30003/testdata.git
        +        targetRevision: master
        +    revision: eb09d7089e88c736f54f13ec6fa513355aababd1
             status: Synced
         
        
    case.go:398: resource Application:argocd-image-updater-e2e/image-updater-007: .status.summary.images: value mismatch, expected: 10.42.0.1:30000/test-image:1.0.0 != actual: 10.42.0.1:30000/test-image:1.0.2
    logger.go:42: 19:14:53 | 007-git-repository | skipping kubernetes event logging
=== NAME  kuttl
    harness.go:403: run tests finished
    harness.go:510: cleaning up
    harness.go:567: removing temp folder: ""
--- FAIL: kuttl (166.07s)
    --- FAIL: kuttl/harness (0.00s)
        --- FAIL: kuttl/harness/007-git-repository (166.05s)
FAIL
dkarpele@fedora:~/go/src/argocd-image-updater/test/e2e$ SRC_DIR=$HOME/go/src/argocd-image-updater kubectl kuttl test --namespace argocd-image-updater-e2e --timeout 120 --test 007-git-repository
=== RUN   kuttl
    harness.go:459: starting setup
    harness.go:254: running tests using configured kubeconfig.
    harness.go:277: Successful connection to cluster at: https://127.0.0.1:6443
    harness.go:362: running tests
    harness.go:74: going to run test suite with timeout of 120 seconds for each step
    harness.go:374: testsuite: ./suite has 10 tests
=== RUN   kuttl/harness
=== RUN   kuttl/harness/007-git-repository
=== PAUSE kuttl/harness/007-git-repository
=== CONT  kuttl/harness/007-git-repository
    logger.go:42: 19:59:38 | 007-git-repository | Ignoring README.md as it does not match file name regexp: ^(\d+)-(?:[^\.]+)(?:\.yaml)?$
    logger.go:42: 19:59:38 | 007-git-repository | Ignoring prepare_assert.sh as it does not match file name regexp: ^(\d+)-(?:[^\.]+)(?:\.yaml)?$
    logger.go:42: 19:59:38 | 007-git-repository | Ignoring revert_commit.sh as it does not match file name regexp: ^(\d+)-(?:[^\.]+)(?:\.yaml)?$
    logger.go:42: 19:59:38 | 007-git-repository | Skipping creation of user-supplied namespace: argocd-image-updater-e2e
    logger.go:42: 19:59:38 | 007-git-repository/1-install | starting test step 1-install
    logger.go:42: 19:59:38 | 007-git-repository/1-install | Namespace:/image-updater-e2e-007 created
Warning: metadata.finalizers: "resources-finalizer.argocd.argoproj.io": prefer a domain-qualified finalizer name to avoid accidental conflicts with other finalizer writers
    logger.go:42: 19:59:38 | 007-git-repository/1-install | Application:argocd-image-updater-e2e/image-updater-007 created
    logger.go:42: 19:59:40 | 007-git-repository/1-install | test step completed 1-install
    logger.go:42: 19:59:40 | 007-git-repository/2-run-updater | starting test step 2-run-updater
    logger.go:42: 19:59:40 | 007-git-repository/2-run-updater | running command: [sh -c ${SRC_DIR}/dist/argocd-image-updater run --once \
          --argocd-namespace argocd-image-updater-e2e \
          --registries-conf-path ${SRC_DIR}/test/e2e/assets/registries.conf \
          --loglevel trace
        ]
    logger.go:42: 19:59:40 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:40+01:00" level=info msg="argocd-image-updater v99.9.9+d6a78eb starting [loglevel:TRACE, interval:once, healthport:off]"
    logger.go:42: 19:59:40 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:40+01:00" level=warning msg="commit message template at /app/config/commit.template does not exist, using default"
    logger.go:42: 19:59:40 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:40+01:00" level=debug msg="Successfully parsed commit message template"
    logger.go:42: 19:59:40 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:40+01:00" level=debug msg="rate limiting is disabled" prefix="10.42.0.1:30000" registry="https://10.42.0.1:30000"
    logger.go:42: 19:59:40 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:40+01:00" level=info msg="Loaded 1 registry configurations from /home/dkarpele/go/src/argocd-image-updater/test/e2e/assets/registries.conf"
    logger.go:42: 19:59:40 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:40+01:00" level=info msg="ArgoCD configuration: [apiKind=kubernetes, server=argocd-server.argocd-image-updater-e2e, auth_token=false, insecure=false, grpc_web=false, plaintext=false]"
    logger.go:42: 19:59:40 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:40+01:00" level=info msg="Starting metrics server on TCP port=8081"
    logger.go:42: 19:59:40 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:40+01:00" level=info msg="Warming up image cache"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=debug msg="Applications listed: 1"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=trace msg="processing app 'argocd-image-updater-e2e/image-updater-007' of type 'Kustomize'" application=image-updater-007 namespace=argocd-image-updater-e2e
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=debug msg="Processing application argocd-image-updater-e2e/image-updater-007"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=debug msg="Considering this image for update" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.0 registry="10.42.0.1:30000"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=debug msg="Using version constraint '1.X.X' when looking for a new tag" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.0 registry="10.42.0.1:30000"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=trace msg="Found update strategy semver" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.X.X registry_url="10.42.0.1:30000"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=trace msg="No match annotation found" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.X.X registry_url="10.42.0.1:30000"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=trace msg="No ignore-tags annotation found" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.X.X registry_url="10.42.0.1:30000"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=trace msg="Using runtime platform constraint linux/amd64" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.X.X registry_url="10.42.0.1:30000"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=trace msg="No pull-secret annotation found" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.X.X registry_url="10.42.0.1:30000"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=trace msg="Performing HTTP GET https://10.42.0.1:30000/v2/test-image/tags/list"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=trace msg="List of available tags found: [1.0.2 latest 1.0.0 1.0.1]" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.0 registry="10.42.0.1:30000"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=debug msg="could not parse input tag latest as semver: Invalid Semantic Version"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=trace msg="Finding out whether to consider 1.0.0 for being updateable" image="10.42.0.1:30000/test-image:1.0.0"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=trace msg="Finding out whether to consider 1.0.1 for being updateable" image="10.42.0.1:30000/test-image:1.0.0"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=trace msg="Finding out whether to consider 1.0.2 for being updateable" image="10.42.0.1:30000/test-image:1.0.0"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=debug msg="found 3 from 3 tags eligible for consideration" image="10.42.0.1:30000/test-image:1.0.0"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=info msg="Setting new image to 10.42.0.1:30000/test-image:1.0.2" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.0 registry="10.42.0.1:30000"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=trace msg="Setting Kustomize parameter 10.42.0.1:30000/test-image:1.0.2" application=image-updater-007
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=info msg="Successfully updated image '10.42.0.1:30000/test-image:1.0.0' to '10.42.0.1:30000/test-image:1.0.2', but pending spec update (dry run=true)" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.0 registry="10.42.0.1:30000"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=debug msg="Using commit message: build: automatic update of image-updater-007\n\nupdates image test-image tag '1.0.0' to '1.0.2'\n"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=info msg="Dry run - not committing 1 changes to application" application=image-updater-007
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=info msg="Finished cache warm-up, pre-loaded 0 meta data entries from 2 registries"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=debug msg="Starting askpass server"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=debug msg="Applications listed: 1"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=trace msg="processing app 'argocd-image-updater-e2e/image-updater-007' of type 'Kustomize'" application=image-updater-007 namespace=argocd-image-updater-e2e
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=info msg="Starting image update cycle, considering 1 annotated application(s) for update"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=debug msg="Processing application argocd-image-updater-e2e/image-updater-007"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=debug msg="Considering this image for update" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.0 registry="10.42.0.1:30000"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=debug msg="Using version constraint '1.X.X' when looking for a new tag" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.0 registry="10.42.0.1:30000"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=trace msg="Found update strategy semver" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.X.X registry_url="10.42.0.1:30000"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=trace msg="No match annotation found" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.X.X registry_url="10.42.0.1:30000"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=trace msg="No ignore-tags annotation found" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.X.X registry_url="10.42.0.1:30000"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=trace msg="Using runtime platform constraint linux/amd64" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.X.X registry_url="10.42.0.1:30000"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=trace msg="No pull-secret annotation found" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.X.X registry_url="10.42.0.1:30000"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=trace msg="Performing HTTP GET https://10.42.0.1:30000/v2/test-image/tags/list"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=trace msg="List of available tags found: [1.0.0 1.0.1 1.0.2 latest]" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.0 registry="10.42.0.1:30000"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=debug msg="could not parse input tag latest as semver: Invalid Semantic Version"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=trace msg="Finding out whether to consider 1.0.0 for being updateable" image="10.42.0.1:30000/test-image:1.0.0"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=trace msg="Finding out whether to consider 1.0.1 for being updateable" image="10.42.0.1:30000/test-image:1.0.0"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=trace msg="Finding out whether to consider 1.0.2 for being updateable" image="10.42.0.1:30000/test-image:1.0.0"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=debug msg="found 3 from 3 tags eligible for consideration" image="10.42.0.1:30000/test-image:1.0.0"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=info msg="Setting new image to 10.42.0.1:30000/test-image:1.0.2" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.0 registry="10.42.0.1:30000"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=trace msg="Setting Kustomize parameter 10.42.0.1:30000/test-image:1.0.2" application=image-updater-007
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=info msg="Successfully updated image '10.42.0.1:30000/test-image:1.0.0' to '10.42.0.1:30000/test-image:1.0.2', but pending spec update (dry run=false)" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.0 registry="10.42.0.1:30000"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=debug msg="Using commit message: build: automatic update of image-updater-007\n\nupdates image test-image tag '1.0.0' to '1.0.2'\n"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=info msg="Committing 1 parameter update(s) for application image-updater-007" application=image-updater-007
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=info msg="Starting configmap/secret informers"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=info msg="Configmap/secret informer synced"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=info msg="Initializing https://10.42.0.1:30003/testdata.git to /tmp/git-image-updater-007323471414"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=info msg="configmap informer cancelled"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=info msg="secrets informer cancelled"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=trace msg="targetRevision for update is 'master'" application=image-updater-007
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=info msg="git fetch origin master --force --prune --depth 1" dir=/tmp/git-image-updater-007323471414 execID=28995
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=info msg=Trace args="[git fetch origin master --force --prune --depth 1]" dir=/tmp/git-image-updater-007323471414 operation_name="exec git" time_ms=102.52887600000001
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=info msg="git checkout --force master" dir=/tmp/git-image-updater-007323471414 execID=5b5db
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=info msg=Trace args="[git checkout --force master]" dir=/tmp/git-image-updater-007323471414 operation_name="exec git" time_ms=3.726756
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=info msg="git clean -ffdx" dir=/tmp/git-image-updater-007323471414 execID=2f951
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=info msg=Trace args="[git clean -ffdx]" dir=/tmp/git-image-updater-007323471414 operation_name="exec git" time_ms=0.873878
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=info msg="updating base /tmp/git-image-updater-007323471414/001-simple-kustomize-app" application=image-updater-007
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=debug msg="Writing commit message to /tmp/image-updater-commit-msg3054864962" application=image-updater-007
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=info msg="git config user.name argocd-image-updater" dir=/tmp/git-image-updater-007323471414 execID=7a373
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=info msg=Trace args="[git config user.name argocd-image-updater]" dir=/tmp/git-image-updater-007323471414 operation_name="exec git" time_ms=0.8349
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=info msg="git config user.email noreply@argoproj.io" dir=/tmp/git-image-updater-007323471414 execID=66def
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=info msg=Trace args="[git config user.email noreply@argoproj.io]" dir=/tmp/git-image-updater-007323471414 operation_name="exec git" time_ms=0.706868
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=info msg="git -c gpg.format=openpgp commit -a -F /tmp/image-updater-commit-msg3054864962" dir=/tmp/git-image-updater-007323471414 execID=f4c32
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=info msg=Trace args="[git -c gpg.format=openpgp commit -a -F /tmp/image-updater-commit-msg3054864962]" dir=/tmp/git-image-updater-007323471414 operation_name="exec git" time_ms=2.919998
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=info msg="git push origin master" dir=/tmp/git-image-updater-007323471414 execID=27013
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=info msg=Trace args="[git push origin master]" dir=/tmp/git-image-updater-007323471414 operation_name="exec git" time_ms=80.777671
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=info msg="Successfully updated the live application spec" application=image-updater-007
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=info msg="Processing results: applications=1 images_considered=1 images_skipped=0 images_updated=1 errors=0"
    logger.go:42: 19:59:41 | 007-git-repository/2-run-updater | time="2025-03-17T19:59:41+01:00" level=info msg=Finished.
    logger.go:42: 20:00:22 | 007-git-repository/2-run-updater | test step completed 2-run-updater
    logger.go:42: 20:00:22 | 007-git-repository/3-run-updater-revert | starting test step 3-run-updater-revert
    logger.go:42: 20:00:22 | 007-git-repository/3-run-updater-revert | running command: [sh -c kubectl patch -n $NAMESPACE application image-updater-007 \
          --type=merge \
          --patch='{"metadata": {"annotations": {"argocd-image-updater.argoproj.io/image-list": "test=10.42.0.1:30000/test-image:1.0.0"}}}'
        ]
    logger.go:42: 20:00:22 | 007-git-repository/3-run-updater-revert | application.argoproj.io/image-updater-007 patched
    logger.go:42: 20:00:22 | 007-git-repository/3-run-updater-revert | running command: [sh -c sleep 2
        ]
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | running command: [sh -c ${SRC_DIR}/dist/argocd-image-updater run --once \
          --argocd-namespace argocd-image-updater-e2e \
          --registries-conf-path ${SRC_DIR}/test/e2e/assets/registries.conf \
          --loglevel trace
        ]
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=info msg="argocd-image-updater v99.9.9+d6a78eb starting [loglevel:TRACE, interval:once, healthport:off]"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=warning msg="commit message template at /app/config/commit.template does not exist, using default"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=debug msg="Successfully parsed commit message template"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=debug msg="rate limiting is disabled" prefix="10.42.0.1:30000" registry="https://10.42.0.1:30000"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=info msg="Loaded 1 registry configurations from /home/dkarpele/go/src/argocd-image-updater/test/e2e/assets/registries.conf"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=info msg="ArgoCD configuration: [apiKind=kubernetes, server=argocd-server.argocd-image-updater-e2e, auth_token=false, insecure=false, grpc_web=false, plaintext=false]"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=info msg="Starting metrics server on TCP port=8081"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=info msg="Warming up image cache"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=debug msg="Applications listed: 1"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=trace msg="processing app 'argocd-image-updater-e2e/image-updater-007' of type 'Kustomize'" application=image-updater-007 namespace=argocd-image-updater-e2e
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=debug msg="Processing application argocd-image-updater-e2e/image-updater-007"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=debug msg="Considering this image for update" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.2 registry="10.42.0.1:30000"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=debug msg="Using version constraint '1.0.0' when looking for a new tag" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.2 registry="10.42.0.1:30000"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=trace msg="Found update strategy semver" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.0.0 registry_url="10.42.0.1:30000"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=trace msg="No match annotation found" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.0.0 registry_url="10.42.0.1:30000"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=trace msg="No ignore-tags annotation found" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.0.0 registry_url="10.42.0.1:30000"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=trace msg="Using runtime platform constraint linux/amd64" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.0.0 registry_url="10.42.0.1:30000"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=trace msg="No pull-secret annotation found" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.0.0 registry_url="10.42.0.1:30000"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=trace msg="Performing HTTP GET https://10.42.0.1:30000/v2/test-image/tags/list"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=trace msg="List of available tags found: [latest 1.0.0 1.0.1 1.0.2]" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.2 registry="10.42.0.1:30000"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=debug msg="could not parse input tag latest as semver: Invalid Semantic Version"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=trace msg="Finding out whether to consider 1.0.0 for being updateable" image="10.42.0.1:30000/test-image:1.0.2"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=trace msg="Finding out whether to consider 1.0.1 for being updateable" image="10.42.0.1:30000/test-image:1.0.2"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=trace msg="1.0.1 did not match constraint 1.0.0" image="10.42.0.1:30000/test-image:1.0.2"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=trace msg="Finding out whether to consider 1.0.2 for being updateable" image="10.42.0.1:30000/test-image:1.0.2"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=trace msg="1.0.2 did not match constraint 1.0.0" image="10.42.0.1:30000/test-image:1.0.2"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=debug msg="found 1 from 3 tags eligible for consideration" image="10.42.0.1:30000/test-image:1.0.2"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=info msg="Setting new image to 10.42.0.1:30000/test-image:1.0.0" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.2 registry="10.42.0.1:30000"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=trace msg="Setting Kustomize parameter 10.42.0.1:30000/test-image:1.0.0" application=image-updater-007
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=info msg="Successfully updated image '10.42.0.1:30000/test-image:1.0.2' to '10.42.0.1:30000/test-image:1.0.0', but pending spec update (dry run=true)" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.2 registry="10.42.0.1:30000"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=debug msg="Using commit message: build: automatic update of image-updater-007\n\nupdates image test-image tag '1.0.2' to '1.0.0'\n"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=info msg="Dry run - not committing 1 changes to application" application=image-updater-007
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=info msg="Finished cache warm-up, pre-loaded 0 meta data entries from 2 registries"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=debug msg="Starting askpass server"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=debug msg="Applications listed: 1"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=trace msg="processing app 'argocd-image-updater-e2e/image-updater-007' of type 'Kustomize'" application=image-updater-007 namespace=argocd-image-updater-e2e
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=info msg="Starting image update cycle, considering 1 annotated application(s) for update"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=debug msg="Processing application argocd-image-updater-e2e/image-updater-007"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=debug msg="Considering this image for update" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.2 registry="10.42.0.1:30000"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=debug msg="Using version constraint '1.0.0' when looking for a new tag" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.2 registry="10.42.0.1:30000"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=trace msg="Found update strategy semver" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.0.0 registry_url="10.42.0.1:30000"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=trace msg="No match annotation found" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.0.0 registry_url="10.42.0.1:30000"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=trace msg="No ignore-tags annotation found" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.0.0 registry_url="10.42.0.1:30000"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=trace msg="Using runtime platform constraint linux/amd64" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.0.0 registry_url="10.42.0.1:30000"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=trace msg="No pull-secret annotation found" image_alias=test image_digest= image_name="10.42.0.1:30000/test-image" image_tag=1.0.0 registry_url="10.42.0.1:30000"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=trace msg="Performing HTTP GET https://10.42.0.1:30000/v2/test-image/tags/list"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=trace msg="List of available tags found: [latest 1.0.0 1.0.1 1.0.2]" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.2 registry="10.42.0.1:30000"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=debug msg="could not parse input tag latest as semver: Invalid Semantic Version"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=trace msg="Finding out whether to consider 1.0.0 for being updateable" image="10.42.0.1:30000/test-image:1.0.2"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=trace msg="Finding out whether to consider 1.0.1 for being updateable" image="10.42.0.1:30000/test-image:1.0.2"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=trace msg="1.0.1 did not match constraint 1.0.0" image="10.42.0.1:30000/test-image:1.0.2"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=trace msg="Finding out whether to consider 1.0.2 for being updateable" image="10.42.0.1:30000/test-image:1.0.2"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=trace msg="1.0.2 did not match constraint 1.0.0" image="10.42.0.1:30000/test-image:1.0.2"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=debug msg="found 1 from 3 tags eligible for consideration" image="10.42.0.1:30000/test-image:1.0.2"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=info msg="Setting new image to 10.42.0.1:30000/test-image:1.0.0" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.2 registry="10.42.0.1:30000"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=trace msg="Setting Kustomize parameter 10.42.0.1:30000/test-image:1.0.0" application=image-updater-007
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=info msg="Successfully updated image '10.42.0.1:30000/test-image:1.0.2' to '10.42.0.1:30000/test-image:1.0.0', but pending spec update (dry run=false)" alias=test application=image-updater-007 image_name=test-image image_tag=1.0.2 registry="10.42.0.1:30000"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=debug msg="Using commit message: build: automatic update of image-updater-007\n\nupdates image test-image tag '1.0.2' to '1.0.0'\n"
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=info msg="Committing 1 parameter update(s) for application image-updater-007" application=image-updater-007
    logger.go:42: 20:00:24 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:24+01:00" level=info msg="Starting configmap/secret informers"
    logger.go:42: 20:00:25 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:25+01:00" level=info msg="Configmap/secret informer synced"
    logger.go:42: 20:00:25 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:25+01:00" level=info msg="Initializing https://10.42.0.1:30003/testdata.git to /tmp/git-image-updater-0071808259276"
    logger.go:42: 20:00:25 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:25+01:00" level=info msg="configmap informer cancelled"
    logger.go:42: 20:00:25 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:25+01:00" level=info msg="secrets informer cancelled"
    logger.go:42: 20:00:25 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:25+01:00" level=trace msg="targetRevision for update is 'master'" application=image-updater-007
    logger.go:42: 20:00:25 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:25+01:00" level=info msg="git fetch origin master --force --prune --depth 1" dir=/tmp/git-image-updater-0071808259276 execID=8d968
    logger.go:42: 20:00:25 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:25+01:00" level=info msg=Trace args="[git fetch origin master --force --prune --depth 1]" dir=/tmp/git-image-updater-0071808259276 operation_name="exec git" time_ms=93.661819
    logger.go:42: 20:00:25 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:25+01:00" level=info msg="git checkout --force master" dir=/tmp/git-image-updater-0071808259276 execID=e309c
    logger.go:42: 20:00:25 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:25+01:00" level=info msg=Trace args="[git checkout --force master]" dir=/tmp/git-image-updater-0071808259276 operation_name="exec git" time_ms=3.694852
    logger.go:42: 20:00:25 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:25+01:00" level=info msg="git clean -ffdx" dir=/tmp/git-image-updater-0071808259276 execID=cd668
    logger.go:42: 20:00:25 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:25+01:00" level=info msg=Trace args="[git clean -ffdx]" dir=/tmp/git-image-updater-0071808259276 operation_name="exec git" time_ms=0.828483
    logger.go:42: 20:00:25 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:25+01:00" level=info msg="updating base /tmp/git-image-updater-0071808259276/001-simple-kustomize-app" application=image-updater-007
    logger.go:42: 20:00:25 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:25+01:00" level=debug msg="Writing commit message to /tmp/image-updater-commit-msg3484888431" application=image-updater-007
    logger.go:42: 20:00:25 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:25+01:00" level=info msg="git config user.name argocd-image-updater" dir=/tmp/git-image-updater-0071808259276 execID=85ed8
    logger.go:42: 20:00:25 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:25+01:00" level=info msg=Trace args="[git config user.name argocd-image-updater]" dir=/tmp/git-image-updater-0071808259276 operation_name="exec git" time_ms=0.7378429999999999
    logger.go:42: 20:00:25 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:25+01:00" level=info msg="git config user.email noreply@argoproj.io" dir=/tmp/git-image-updater-0071808259276 execID=b6933
    logger.go:42: 20:00:25 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:25+01:00" level=info msg=Trace args="[git config user.email noreply@argoproj.io]" dir=/tmp/git-image-updater-0071808259276 operation_name="exec git" time_ms=0.689483
    logger.go:42: 20:00:25 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:25+01:00" level=info msg="git -c gpg.format=openpgp commit -a -F /tmp/image-updater-commit-msg3484888431" dir=/tmp/git-image-updater-0071808259276 execID=9c475
    logger.go:42: 20:00:25 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:25+01:00" level=info msg=Trace args="[git -c gpg.format=openpgp commit -a -F /tmp/image-updater-commit-msg3484888431]" dir=/tmp/git-image-updater-0071808259276 operation_name="exec git" time_ms=2.878824
    logger.go:42: 20:00:25 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:25+01:00" level=info msg="git push origin master" dir=/tmp/git-image-updater-0071808259276 execID=3276e
    logger.go:42: 20:00:25 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:25+01:00" level=info msg=Trace args="[git push origin master]" dir=/tmp/git-image-updater-0071808259276 operation_name="exec git" time_ms=80.467038
    logger.go:42: 20:00:25 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:25+01:00" level=info msg="Successfully updated the live application spec" application=image-updater-007
    logger.go:42: 20:00:25 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:25+01:00" level=info msg="Processing results: applications=1 images_considered=1 images_skipped=0 images_updated=1 errors=0"
    logger.go:42: 20:00:25 | 007-git-repository/3-run-updater-revert | time="2025-03-17T20:00:25+01:00" level=info msg=Finished.
    logger.go:42: 20:02:01 | 007-git-repository/3-run-updater-revert | test step completed 3-run-updater-revert
    logger.go:42: 20:02:01 | 007-git-repository/99-delete | starting test step 99-delete
    logger.go:42: 20:02:44 | 007-git-repository/99-delete | test step completed 99-delete
    logger.go:42: 20:02:44 | 007-git-repository | skipping kubernetes event logging
=== NAME  kuttl
    harness.go:403: run tests finished
    harness.go:510: cleaning up
    harness.go:567: removing temp folder: ""
--- PASS: kuttl (185.74s)
    --- PASS: kuttl/harness (0.00s)
        --- PASS: kuttl/harness/007-git-repository (185.73s)
PASS
```