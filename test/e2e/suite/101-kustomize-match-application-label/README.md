This test case verifies [filtering applications by labels](https://argocd-image-updater.readthedocs.io/en/stable/install/reference/#flags) with `--match-application-label` command line options

This test case uses image from public container registry and application source from public GitHub repo.

To run this individual test case,

* make sure both docker daemon and k8s cluster is running
* `cd $HOME/go/src/image-updater/test/e2e`
* `SRC_DIR=$HOME/go/src/argocd-image-updater kubectl kuttl test --namespace argocd-image-updater-e2e --timeout 120 --test 101-kustomize-match-application-label`

Test output:
```bash
=== RUN   kuttl
    harness.go:464: starting setup
    harness.go:255: running tests using configured kubeconfig.
    harness.go:278: Successful connection to cluster at: https://0.0.0.0:55975
    harness.go:363: running tests
    harness.go:75: going to run test suite with timeout of 120 seconds for each step
    harness.go:375: testsuite: ./suite has 5 tests
=== RUN   kuttl/harness
=== RUN   kuttl/harness/101-kustomize-match-application-label
=== PAUSE kuttl/harness/101-kustomize-match-application-label
=== CONT  kuttl/harness/101-kustomize-match-application-label
    logger.go:42: 11:56:07 | 101-kustomize-match-application-label | Ignoring README.md as it does not match file name regexp: ^(\d+)-(?:[^\.]+)(?:\.yaml)?$
    logger.go:42: 11:56:07 | 101-kustomize-match-application-label | Skipping creation of user-supplied namespace: argocd-image-updater-e2e
    logger.go:42: 11:56:07 | 101-kustomize-match-application-label/1-install | starting test step 1-install
    logger.go:42: 11:56:07 | 101-kustomize-match-application-label/1-install | Namespace:/image-updater-e2e-101-0 created
    logger.go:42: 11:56:07 | 101-kustomize-match-application-label/1-install | Application:argocd-image-updater-e2e/image-updater-101-0 created
    logger.go:42: 11:56:07 | 101-kustomize-match-application-label/1-install | Namespace:/image-updater-e2e-101-1 created
    logger.go:42: 11:56:07 | 101-kustomize-match-application-label/1-install | Application:argocd-image-updater-e2e/image-updater-101-1 created
    logger.go:42: 11:56:07 | 101-kustomize-match-application-label/1-install | Namespace:/image-updater-e2e-101-2 created
    logger.go:42: 11:56:07 | 101-kustomize-match-application-label/1-install | Application:argocd-image-updater-e2e/image-updater-101-2 created
    logger.go:42: 11:56:11 | 101-kustomize-match-application-label/1-install | test step completed 1-install
    logger.go:42: 11:56:11 | 101-kustomize-match-application-label/2-run-updater | starting test step 2-run-updater
    logger.go:42: 11:56:11 | 101-kustomize-match-application-label/2-run-updater | running command: [sh -c ${SRC_DIR}/dist/argocd-image-updater run --once \
          --argocd-namespace argocd-image-updater-e2e \
          --match-application-label app.index=0 \
          --loglevel trace
        ]
    logger.go:42: 11:56:11 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:11-04:00" level=info msg="argocd-image-updater v99.9.9+2bf4b0a starting [loglevel:TRACE, interval:once, healthport:off]"
    logger.go:42: 11:56:11 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:11-04:00" level=warning msg="commit message template at /app/config/commit.template does not exist, using default"
    logger.go:42: 11:56:11 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:11-04:00" level=debug msg="Successfully parsed commit message template"
    logger.go:42: 11:56:11 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:11-04:00" level=warning msg="Registry configuration at /app/config/registries.conf could not be read: stat /app/config/registries.conf: no such file or directory -- using default configuration"
    logger.go:42: 11:56:11 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:11-04:00" level=debug msg="Creating in-cluster Kubernetes client"
    logger.go:42: 11:56:11 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:11-04:00" level=info msg="ArgoCD configuration: [apiKind=kubernetes, server=argocd-server.argocd-image-updater-e2e, auth_token=false, insecure=false, grpc_web=false, plaintext=false]"
    logger.go:42: 11:56:11 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:11-04:00" level=info msg="Starting metrics server on TCP port=8081"
    logger.go:42: 11:56:11 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:11-04:00" level=info msg="Warming up image cache"
    logger.go:42: 11:56:11 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:11-04:00" level=trace msg="Matching application name image-updater-101-0 against label app.index=0"
    logger.go:42: 11:56:11 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:11-04:00" level=trace msg="processing app 'argocd-image-updater-e2e/image-updater-101-0' of type 'Kustomize'" application=image-updater-101-0 namespace=argocd-image-updater-e2e
    logger.go:42: 11:56:11 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:11-04:00" level=trace msg="Matching application name image-updater-101-1 against label app.index=0"
    logger.go:42: 11:56:11 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:11-04:00" level=debug msg="Skipping app 'argocd-image-updater-e2e/image-updater-101-1' because it does not carry requested label" application=image-updater-101-1 namespace=argocd-image-updater-e2e
    logger.go:42: 11:56:11 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:11-04:00" level=trace msg="Matching application name image-updater-101-2 against label app.index=0"
    logger.go:42: 11:56:11 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:11-04:00" level=debug msg="Skipping app 'argocd-image-updater-e2e/image-updater-101-2' because it does not carry requested label" application=image-updater-101-2 namespace=argocd-image-updater-e2e
    logger.go:42: 11:56:11 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:11-04:00" level=debug msg="Processing application argocd-image-updater-e2e/image-updater-101-0"
    logger.go:42: 11:56:11 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:11-04:00" level=debug msg="Considering this image for update" alias=guestbook application=image-updater-101-0 image_name=heptio-images/ks-guestbook-demo image_tag=0.1 registry=gcr.io
    logger.go:42: 11:56:11 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:11-04:00" level=debug msg="setting rate limit to 20 requests per second" prefix=gcr.io registry="https://gcr.io"
    logger.go:42: 11:56:11 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:11-04:00" level=debug msg="Inferred registry from prefix gcr.io to use API https://gcr.io"
    logger.go:42: 11:56:11 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:11-04:00" level=debug msg="Using version constraint '~0' when looking for a new tag" alias=guestbook application=image-updater-101-0 image_name=heptio-images/ks-guestbook-demo image_tag=0.1 registry=gcr.io
    logger.go:42: 11:56:11 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:11-04:00" level=trace msg="No sort option found" image_alias=guestbook image_digest= image_name=gcr.io/heptio-images/ks-guestbook-demo image_tag="~0" registry_url=gcr.io
    logger.go:42: 11:56:11 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:11-04:00" level=trace msg="No match annotation found" image_alias=guestbook image_digest= image_name=gcr.io/heptio-images/ks-guestbook-demo image_tag="~0" registry_url=gcr.io
    logger.go:42: 11:56:11 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:11-04:00" level=trace msg="No ignore-tags annotation found" image_alias=guestbook image_digest= image_name=gcr.io/heptio-images/ks-guestbook-demo image_tag="~0" registry_url=gcr.io
    logger.go:42: 11:56:11 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:11-04:00" level=trace msg="Using runtime platform constraint darwin/arm64" image_alias=guestbook image_digest= image_name=gcr.io/heptio-images/ks-guestbook-demo image_tag="~0" registry_url=gcr.io
    logger.go:42: 11:56:11 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:11-04:00" level=trace msg="No pull-secret annotation found" image_alias=guestbook image_digest= image_name=gcr.io/heptio-images/ks-guestbook-demo image_tag="~0" registry_url=gcr.io
    logger.go:42: 11:56:11 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:11-04:00" level=trace msg="Performing HTTP GET https://gcr.io/v2/heptio-images/ks-guestbook-demo/tags/list"
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=trace msg="List of available tags found: [0.1 0.2]" alias=guestbook application=image-updater-101-0 image_name=heptio-images/ks-guestbook-demo image_tag=0.1 registry=gcr.io
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=trace msg="Finding out whether to consider 0.1 for being updateable" image="gcr.io/heptio-images/ks-guestbook-demo:0.1"
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=trace msg="Finding out whether to consider 0.2 for being updateable" image="gcr.io/heptio-images/ks-guestbook-demo:0.1"
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=debug msg="found 2 from 2 tags eligible for consideration" image="gcr.io/heptio-images/ks-guestbook-demo:0.1"
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=info msg="Setting new image to gcr.io/heptio-images/ks-guestbook-demo:0.2" alias=guestbook application=image-updater-101-0 image_name=heptio-images/ks-guestbook-demo image_tag=0.1 registry=gcr.io
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=trace msg="Setting Kustomize parameter gcr.io/heptio-images/ks-guestbook-demo:0.2" application=image-updater-101-0
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=info msg="Successfully updated image 'gcr.io/heptio-images/ks-guestbook-demo:0.1' to 'gcr.io/heptio-images/ks-guestbook-demo:0.2', but pending spec update (dry run=true)" alias=guestbook application=image-updater-101-0 image_name=heptio-images/ks-guestbook-demo image_tag=0.1 registry=gcr.io
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=debug msg="Using commit message: "
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=info msg="Dry run - not committing 1 changes to application" application=image-updater-101-0
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=info msg="Finished cache warm-up, pre-loaded 0 meta data entries from 2 registries"
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=debug msg="Starting askpass server"
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=trace msg="Matching application name image-updater-101-0 against label app.index=0"
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=trace msg="processing app 'argocd-image-updater-e2e/image-updater-101-0' of type 'Kustomize'" application=image-updater-101-0 namespace=argocd-image-updater-e2e
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=trace msg="Matching application name image-updater-101-1 against label app.index=0"
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=debug msg="Skipping app 'argocd-image-updater-e2e/image-updater-101-1' because it does not carry requested label" application=image-updater-101-1 namespace=argocd-image-updater-e2e
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=trace msg="Matching application name image-updater-101-2 against label app.index=0"
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=debug msg="Skipping app 'argocd-image-updater-e2e/image-updater-101-2' because it does not carry requested label" application=image-updater-101-2 namespace=argocd-image-updater-e2e
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=info msg="Starting image update cycle, considering 1 annotated application(s) for update"
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=debug msg="Processing application argocd-image-updater-e2e/image-updater-101-0"
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=debug msg="Considering this image for update" alias=guestbook application=image-updater-101-0 image_name=heptio-images/ks-guestbook-demo image_tag=0.1 registry=gcr.io
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=debug msg="Using version constraint '~0' when looking for a new tag" alias=guestbook application=image-updater-101-0 image_name=heptio-images/ks-guestbook-demo image_tag=0.1 registry=gcr.io
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=trace msg="No sort option found" image_alias=guestbook image_digest= image_name=gcr.io/heptio-images/ks-guestbook-demo image_tag="~0" registry_url=gcr.io
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=trace msg="No match annotation found" image_alias=guestbook image_digest= image_name=gcr.io/heptio-images/ks-guestbook-demo image_tag="~0" registry_url=gcr.io
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=trace msg="No ignore-tags annotation found" image_alias=guestbook image_digest= image_name=gcr.io/heptio-images/ks-guestbook-demo image_tag="~0" registry_url=gcr.io
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=trace msg="Using runtime platform constraint darwin/arm64" image_alias=guestbook image_digest= image_name=gcr.io/heptio-images/ks-guestbook-demo image_tag="~0" registry_url=gcr.io
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=trace msg="No pull-secret annotation found" image_alias=guestbook image_digest= image_name=gcr.io/heptio-images/ks-guestbook-demo image_tag="~0" registry_url=gcr.io
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=trace msg="Performing HTTP GET https://gcr.io/v2/heptio-images/ks-guestbook-demo/tags/list"
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=trace msg="List of available tags found: [0.1 0.2]" alias=guestbook application=image-updater-101-0 image_name=heptio-images/ks-guestbook-demo image_tag=0.1 registry=gcr.io
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=trace msg="Finding out whether to consider 0.1 for being updateable" image="gcr.io/heptio-images/ks-guestbook-demo:0.1"
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=trace msg="Finding out whether to consider 0.2 for being updateable" image="gcr.io/heptio-images/ks-guestbook-demo:0.1"
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=debug msg="found 2 from 2 tags eligible for consideration" image="gcr.io/heptio-images/ks-guestbook-demo:0.1"
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=info msg="Setting new image to gcr.io/heptio-images/ks-guestbook-demo:0.2" alias=guestbook application=image-updater-101-0 image_name=heptio-images/ks-guestbook-demo image_tag=0.1 registry=gcr.io
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=trace msg="Setting Kustomize parameter gcr.io/heptio-images/ks-guestbook-demo:0.2" application=image-updater-101-0
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=info msg="Successfully updated image 'gcr.io/heptio-images/ks-guestbook-demo:0.1' to 'gcr.io/heptio-images/ks-guestbook-demo:0.2', but pending spec update (dry run=false)" alias=guestbook application=image-updater-101-0 image_name=heptio-images/ks-guestbook-demo image_tag=0.1 registry=gcr.io
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=debug msg="Using commit message: "
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=info msg="Committing 1 parameter update(s) for application image-updater-101-0" application=image-updater-101-0
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | W0820 11:56:12.643735   31180 warnings.go:70] unknown field "status.history[0].initiatedBy"
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=info msg="Successfully updated the live application spec" application=image-updater-101-0
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=info msg="Processing results: applications=1 images_considered=1 images_skipped=0 images_updated=1 errors=0"
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | time="2024-08-20T11:56:12-04:00" level=info msg=Finished.
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/2-run-updater | test step completed 2-run-updater
    logger.go:42: 11:56:12 | 101-kustomize-match-application-label/99-delete | starting test step 99-delete
    logger.go:42: 11:56:55 | 101-kustomize-match-application-label/99-delete | test step completed 99-delete
    logger.go:42: 11:56:55 | 101-kustomize-match-application-label | skipping kubernetes event logging
=== NAME  kuttl
    harness.go:407: run tests finished
    harness.go:515: cleaning up
    harness.go:572: removing temp folder: ""
--- PASS: kuttl (47.84s)
    --- PASS: kuttl/harness (0.00s)
        --- PASS: kuttl/harness/101-kustomize-match-application-label (47.84s)
PASS
```