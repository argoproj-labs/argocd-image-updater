This test case verifies applications in any namespace feature.

This test case uses image from public container registry and application source from public GitHub repo.

To run this individual test case,

* make sure both docker daemon and k8s cluster is running
* `cd $HOME/go/src/image-updater/test/e2e`
* `SRC_DIR=$HOME/go/src/image-updater kubectl kuttl test --namespace argocd-image-updater-e2e --timeout 120 --test 006-apps-in-any-namespace`

Test output:
```bash
=== RUN   kuttl
    harness.go:464: starting setup
    harness.go:255: running tests using configured kubeconfig.
    harness.go:278: Successful connection to cluster at: https://127.0.0.1:6443
    harness.go:363: running tests
    harness.go:75: going to run test suite with timeout of 120 seconds for each step
    harness.go:375: testsuite: ./suite has 7 tests
=== RUN   kuttl/harness
=== RUN   kuttl/harness/006-apps-in-any-namespace
=== PAUSE kuttl/harness/006-apps-in-any-namespace
=== CONT  kuttl/harness/006-apps-in-any-namespace
    logger.go:42: 12:34:29 | 006-apps-in-any-namespace | Ignoring README.md as it does not match file name regexp: ^(\d+)-(?:[^\.]+)(?:\.yaml)?$
    logger.go:42: 12:34:29 | 006-apps-in-any-namespace | Skipping creation of user-supplied namespace: argocd-image-updater-e2e
    logger.go:42: 12:34:29 | 006-apps-in-any-namespace/1-install | starting test step 1-install
    logger.go:42: 12:34:29 | 006-apps-in-any-namespace/1-install | running command: [sh -c kubectl rollout restart -n argocd-image-updater-e2e deployment argocd-server
        kubectl rollout restart -n argocd-image-updater-e2e statefulset argocd-application-controller
        sleep 30
        ]
    logger.go:42: 12:34:29 | 006-apps-in-any-namespace/1-install | deployment.apps/argocd-server restarted
    logger.go:42: 12:34:29 | 006-apps-in-any-namespace/1-install | statefulset.apps/argocd-application-controller restarted
[controller-runtime] log.SetLogger(...) was never called; logs will not be displayed.
Detected at:
        >  goroutine 22 [running]:
        >  runtime/debug.Stack()
        >       runtime/debug/stack.go:24 +0x64
        >  sigs.k8s.io/controller-runtime/pkg/log.eventuallyFulfillRoot()
        >       sigs.k8s.io/controller-runtime@v0.18.4/pkg/log/log.go:60 +0xf4
        >  sigs.k8s.io/controller-runtime/pkg/log.(*delegatingLogSink).WithName(0x140002fc440, {0x10115bbef, 0x14})
        >       sigs.k8s.io/controller-runtime@v0.18.4/pkg/log/deleg.go:147 +0x34
        >  github.com/go-logr/logr.Logger.WithName({{0x1017508a0, 0x140002fc440}, 0x0}, {0x10115bbef?, 0x140005754a8?})
        >       github.com/go-logr/logr@v1.4.1/logr.go:345 +0x40
        >  sigs.k8s.io/controller-runtime/pkg/client.newClient(0x14000575658?, {0x0, 0x1400042c8c0, {0x101751f80, 0x1400030e0f0}, 0x0, {0x0, 0x0}, 0x0})
        >       sigs.k8s.io/controller-runtime@v0.18.4/pkg/client/client.go:129 +0xb4
        >  sigs.k8s.io/controller-runtime/pkg/client.New(0x1400011db08?, {0x0, 0x1400042c8c0, {0x101751f80, 0x1400030e0f0}, 0x0, {0x0, 0x0}, 0x0})
        >       sigs.k8s.io/controller-runtime@v0.18.4/pkg/client/client.go:110 +0x54
        >  github.com/kudobuilder/kuttl/pkg/test/utils.NewRetryClient(0x1400011db08, {0x0, 0x1400042c8c0, {0x101751f80, 0x1400030e0f0}, 0x0, {0x0, 0x0}, 0x0})
        >       github.com/kudobuilder/kuttl/pkg/test/utils/kubernetes.go:177 +0xac
        >  github.com/kudobuilder/kuttl/pkg/test.(*Harness).Client(0x140000d6608, 0xf0?)
        >       github.com/kudobuilder/kuttl/pkg/test/harness.go:323 +0x15c
        >  github.com/kudobuilder/kuttl/pkg/test.(*Step).Create(0x14000444b60, 0x14000330d00, {0x16fd9f5a1, 0x18})
        >       github.com/kudobuilder/kuttl/pkg/test/step.go:178 +0x48
        >  github.com/kudobuilder/kuttl/pkg/test.(*Step).Run(0x14000444b60, 0x14000330d00, {0x16fd9f5a1, 0x18})
        >       github.com/kudobuilder/kuttl/pkg/test/step.go:458 +0x1d0
        >  github.com/kudobuilder/kuttl/pkg/test.(*Case).Run(0x1400043f720, 0x14000330d00, 0x140001fe990)
        >       github.com/kudobuilder/kuttl/pkg/test/case.go:392 +0xc90
        >  github.com/kudobuilder/kuttl/pkg/test.(*Harness).RunTests.func1.1(0x14000330d00)
        >       github.com/kudobuilder/kuttl/pkg/test/harness.go:401 +0x128
        >  testing.tRunner(0x14000330d00, 0x1400064e8d0)
        >       testing/testing.go:1689 +0xec
        >  created by testing.(*T).Run in goroutine 21
        >       testing/testing.go:1742 +0x318
    logger.go:42: 12:34:59 | 006-apps-in-any-namespace/1-install | Namespace:/image-updater-e2e-006 created
    logger.go:42: 12:34:59 | 006-apps-in-any-namespace/1-install | Namespace:/image-updater-e2e-006-01 created
    logger.go:42: 12:34:59 | 006-apps-in-any-namespace/1-install | ConfigMap:argocd-image-updater-e2e/argocd-cmd-params-cm updated
    logger.go:42: 12:34:59 | 006-apps-in-any-namespace/1-install | AppProject:argocd-image-updater-e2e/project-one created
    logger.go:42: 12:34:59 | 006-apps-in-any-namespace/1-install | Application:argocd-image-updater-e2e/image-updater-006 created
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/1-install | test step completed 1-install
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | starting test step 2-run-updater
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | running command: [sh -c ${SRC_DIR}/dist/argocd-image-updater run --once \
          --argocd-namespace argocd-image-updater-e2e \
          --loglevel trace
        ]
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=info msg="argocd-image-updater v99.9.9+43dbd63 starting [loglevel:TRACE, interval:once, healthport:off]"
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=warning msg="commit message template at /app/config/commit.template does not exist, using default"
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=debug msg="Successfully parsed commit message template"
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=warning msg="Registry configuration at /app/config/registries.conf could not be read: stat /app/config/registries.conf: no such file or directory -- using default configuration"
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=debug msg="Creating in-cluster Kubernetes client"
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=info msg="ArgoCD configuration: [apiKind=kubernetes, server=argocd-server.argocd-image-updater-e2e, auth_token=false, insecure=false, grpc_web=false, plaintext=false]"
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=info msg="Starting metrics server on TCP port=8081"
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=info msg="Warming up image cache"
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=trace msg="processing app 'argocd-image-updater-e2e/image-updater-006' of type 'Kustomize'" application=image-updater-006 namespace=argocd-image-updater-e2e
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=debug msg="Processing application argocd-image-updater-e2e/image-updater-006"
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=debug msg="Considering this image for update" alias=guestbook application=image-updater-006 image_name=heptio-images/ks-guestbook-demo image_tag=0.1 registry=gcr.io
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=debug msg="setting rate limit to 20 requests per second" prefix=gcr.io registry="https://gcr.io"
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=debug msg="Inferred registry from prefix gcr.io to use API https://gcr.io"
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=debug msg="Using version constraint '~0' when looking for a new tag" alias=guestbook application=image-updater-006 image_name=heptio-images/ks-guestbook-demo image_tag=0.1 registry=gcr.io
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=trace msg="Found update strategy semver" image_alias=guestbook image_digest= image_name=gcr.io/heptio-images/ks-guestbook-demo image_tag="~0" registry_url=gcr.io
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=trace msg="No match annotation found" image_alias=guestbook image_digest= image_name=gcr.io/heptio-images/ks-guestbook-demo image_tag="~0" registry_url=gcr.io
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=trace msg="No ignore-tags annotation found" image_alias=guestbook image_digest= image_name=gcr.io/heptio-images/ks-guestbook-demo image_tag="~0" registry_url=gcr.io
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=trace msg="Using runtime platform constraint darwin/arm64" image_alias=guestbook image_digest= image_name=gcr.io/heptio-images/ks-guestbook-demo image_tag="~0" registry_url=gcr.io
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=trace msg="No pull-secret annotation found" image_alias=guestbook image_digest= image_name=gcr.io/heptio-images/ks-guestbook-demo image_tag="~0" registry_url=gcr.io
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=trace msg="Performing HTTP GET https://gcr.io/v2/heptio-images/ks-guestbook-demo/tags/list"
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=trace msg="List of available tags found: [0.2 0.1]" alias=guestbook application=image-updater-006 image_name=heptio-images/ks-guestbook-demo image_tag=0.1 registry=gcr.io
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=trace msg="Finding out whether to consider 0.1 for being updateable" image="gcr.io/heptio-images/ks-guestbook-demo:0.1"
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=trace msg="Finding out whether to consider 0.2 for being updateable" image="gcr.io/heptio-images/ks-guestbook-demo:0.1"
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=debug msg="found 2 from 2 tags eligible for consideration" image="gcr.io/heptio-images/ks-guestbook-demo:0.1"
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=info msg="Setting new image to gcr.io/heptio-images/ks-guestbook-demo:0.2" alias=guestbook application=image-updater-006 image_name=heptio-images/ks-guestbook-demo image_tag=0.1 registry=gcr.io
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=trace msg="Setting Kustomize parameter gcr.io/heptio-images/ks-guestbook-demo:0.2" application=image-updater-006
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=info msg="Successfully updated image 'gcr.io/heptio-images/ks-guestbook-demo:0.1' to 'gcr.io/heptio-images/ks-guestbook-demo:0.2', but pending spec update (dry run=true)" alias=guestbook application=image-updater-006 image_name=heptio-images/ks-guestbook-demo image_tag=0.1 registry=gcr.io
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=debug msg="Using commit message: "
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=info msg="Dry run - not committing 1 changes to application" application=image-updater-006
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=info msg="Finished cache warm-up, pre-loaded 0 meta data entries from 2 registries"
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=debug msg="Starting askpass server"
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=trace msg="processing app 'argocd-image-updater-e2e/image-updater-006' of type 'Kustomize'" application=image-updater-006 namespace=argocd-image-updater-e2e
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=info msg="Starting image update cycle, considering 1 annotated application(s) for update"
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=debug msg="Processing application argocd-image-updater-e2e/image-updater-006"
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=debug msg="Considering this image for update" alias=guestbook application=image-updater-006 image_name=heptio-images/ks-guestbook-demo image_tag=0.1 registry=gcr.io
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=debug msg="Using version constraint '~0' when looking for a new tag" alias=guestbook application=image-updater-006 image_name=heptio-images/ks-guestbook-demo image_tag=0.1 registry=gcr.io
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=trace msg="Found update strategy semver" image_alias=guestbook image_digest= image_name=gcr.io/heptio-images/ks-guestbook-demo image_tag="~0" registry_url=gcr.io
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=trace msg="No match annotation found" image_alias=guestbook image_digest= image_name=gcr.io/heptio-images/ks-guestbook-demo image_tag="~0" registry_url=gcr.io
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=trace msg="No ignore-tags annotation found" image_alias=guestbook image_digest= image_name=gcr.io/heptio-images/ks-guestbook-demo image_tag="~0" registry_url=gcr.io
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=trace msg="Using runtime platform constraint darwin/arm64" image_alias=guestbook image_digest= image_name=gcr.io/heptio-images/ks-guestbook-demo image_tag="~0" registry_url=gcr.io
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=trace msg="No pull-secret annotation found" image_alias=guestbook image_digest= image_name=gcr.io/heptio-images/ks-guestbook-demo image_tag="~0" registry_url=gcr.io
    logger.go:42: 12:35:03 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:03-04:00" level=trace msg="Performing HTTP GET https://gcr.io/v2/heptio-images/ks-guestbook-demo/tags/list"
    logger.go:42: 12:35:04 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:04-04:00" level=trace msg="List of available tags found: [0.1 0.2]" alias=guestbook application=image-updater-006 image_name=heptio-images/ks-guestbook-demo image_tag=0.1 registry=gcr.io
    logger.go:42: 12:35:04 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:04-04:00" level=trace msg="Finding out whether to consider 0.1 for being updateable" image="gcr.io/heptio-images/ks-guestbook-demo:0.1"
    logger.go:42: 12:35:04 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:04-04:00" level=trace msg="Finding out whether to consider 0.2 for being updateable" image="gcr.io/heptio-images/ks-guestbook-demo:0.1"
    logger.go:42: 12:35:04 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:04-04:00" level=debug msg="found 2 from 2 tags eligible for consideration" image="gcr.io/heptio-images/ks-guestbook-demo:0.1"
    logger.go:42: 12:35:04 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:04-04:00" level=info msg="Setting new image to gcr.io/heptio-images/ks-guestbook-demo:0.2" alias=guestbook application=image-updater-006 image_name=heptio-images/ks-guestbook-demo image_tag=0.1 registry=gcr.io
    logger.go:42: 12:35:04 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:04-04:00" level=trace msg="Setting Kustomize parameter gcr.io/heptio-images/ks-guestbook-demo:0.2" application=image-updater-006
    logger.go:42: 12:35:04 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:04-04:00" level=info msg="Successfully updated image 'gcr.io/heptio-images/ks-guestbook-demo:0.1' to 'gcr.io/heptio-images/ks-guestbook-demo:0.2', but pending spec update (dry run=false)" alias=guestbook application=image-updater-006 image_name=heptio-images/ks-guestbook-demo image_tag=0.1 registry=gcr.io
    logger.go:42: 12:35:04 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:04-04:00" level=debug msg="Using commit message: "
    logger.go:42: 12:35:04 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:04-04:00" level=info msg="Committing 1 parameter update(s) for application image-updater-006" application=image-updater-006
    logger.go:42: 12:35:04 | 006-apps-in-any-namespace/2-run-updater | W0828 12:35:04.161436   23065 warnings.go:70] unknown field "status.history[0].initiatedBy"
    logger.go:42: 12:35:04 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:04-04:00" level=info msg="Successfully updated the live application spec" application=image-updater-006
    logger.go:42: 12:35:04 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:04-04:00" level=info msg="Processing results: applications=1 images_considered=1 images_skipped=0 images_updated=1 errors=0"
    logger.go:42: 12:35:04 | 006-apps-in-any-namespace/2-run-updater | time="2024-08-28T12:35:04-04:00" level=info msg=Finished.
    logger.go:42: 12:35:07 | 006-apps-in-any-namespace/2-run-updater | test step completed 2-run-updater
    logger.go:42: 12:35:07 | 006-apps-in-any-namespace/99-delete | starting test step 99-delete
    logger.go:42: 12:35:49 | 006-apps-in-any-namespace/99-delete | test step completed 99-delete
    logger.go:42: 12:35:49 | 006-apps-in-any-namespace | skipping kubernetes event logging
=== NAME  kuttl
    harness.go:407: run tests finished
    harness.go:515: cleaning up
    harness.go:572: removing temp folder: ""
--- PASS: kuttl (80.42s)
    --- PASS: kuttl/harness (0.00s)
        --- PASS: kuttl/harness/006-apps-in-any-namespace (80.41s)
PASS
```