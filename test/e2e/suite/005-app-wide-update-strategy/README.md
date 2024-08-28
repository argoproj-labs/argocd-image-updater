This test case verifies [app wide update strategy]() with `--match-application-label` command line options

This test case uses image from public container registry and application source from public GitHub repo.

To run this individual test case,

* make sure both docker daemon and k8s cluster is running
* `cd $HOME/go/src/image-updater/test/e2e`
* `SRC_DIR=$HOME/go/src/image-updater kubectl kuttl test --namespace argocd-image-updater-e2e --timeout 120 --test 005-app-wide-update-strategy`

Test output:
```bash
=== RUN   kuttl
    harness.go:464: starting setup
    harness.go:255: running tests using configured kubeconfig.
    harness.go:278: Successful connection to cluster at: https://127.0.0.1:6443
    harness.go:363: running tests
    harness.go:75: going to run test suite with timeout of 120 seconds for each step
    harness.go:375: testsuite: ./suite has 8 tests
=== RUN   kuttl/harness
=== RUN   kuttl/harness/005-app-wide-update-strategy
=== PAUSE kuttl/harness/005-app-wide-update-strategy
=== CONT  kuttl/harness/005-app-wide-update-strategy
    logger.go:42: 20:21:30 | 005-app-wide-update-strategy | Ignoring README.md as it does not match file name regexp: ^(\d+)-(?:[^\.]+)(?:\.yaml)?$
    logger.go:42: 20:21:30 | 005-app-wide-update-strategy | Skipping creation of user-supplied namespace: argocd-image-updater-e2e
    logger.go:42: 20:21:30 | 005-app-wide-update-strategy/1-install | starting test step 1-install
    logger.go:42: 20:21:30 | 005-app-wide-update-strategy/1-install | Namespace:/image-updater-e2e-005-01 created
    logger.go:42: 20:21:30 | 005-app-wide-update-strategy/1-install | Application:argocd-image-updater-e2e/image-updater-005-01 created
    logger.go:42: 20:21:30 | 005-app-wide-update-strategy/1-install | Namespace:/image-updater-e2e-005-02 created
    logger.go:42: 20:21:30 | 005-app-wide-update-strategy/1-install | Application:argocd-image-updater-e2e/image-updater-005-02 created
    logger.go:42: 20:21:34 | 005-app-wide-update-strategy/1-install | test step completed 1-install
    logger.go:42: 20:21:34 | 005-app-wide-update-strategy/2-run-updater | starting test step 2-run-updater
    logger.go:42: 20:21:34 | 005-app-wide-update-strategy/2-run-updater | running command: [sh -c ${SRC_DIR}/dist/argocd-image-updater run --once \
          --argocd-namespace argocd-image-updater-e2e \
          --loglevel info
        ]
    logger.go:42: 20:21:34 | 005-app-wide-update-strategy/2-run-updater | time="2024-08-27T20:21:34-04:00" level=info msg="argocd-image-updater v99.9.9+43dbd63 starting [loglevel:INFO, interval:once, healthport:off]"
    logger.go:42: 20:21:34 | 005-app-wide-update-strategy/2-run-updater | time="2024-08-27T20:21:34-04:00" level=warning msg="commit message template at /app/config/commit.template does not exist, using default"
    logger.go:42: 20:21:34 | 005-app-wide-update-strategy/2-run-updater | time="2024-08-27T20:21:34-04:00" level=warning msg="Registry configuration at /app/config/registries.conf could not be read: stat /app/config/registries.conf: no such file or directory -- using default configuration"
    logger.go:42: 20:21:34 | 005-app-wide-update-strategy/2-run-updater | time="2024-08-27T20:21:34-04:00" level=info msg="ArgoCD configuration: [apiKind=kubernetes, server=argocd-server.argocd-image-updater-e2e, auth_token=false, insecure=false, grpc_web=false, plaintext=false]"
    logger.go:42: 20:21:34 | 005-app-wide-update-strategy/2-run-updater | time="2024-08-27T20:21:34-04:00" level=info msg="Starting metrics server on TCP port=8081"
    logger.go:42: 20:21:34 | 005-app-wide-update-strategy/2-run-updater | time="2024-08-27T20:21:34-04:00" level=info msg="Warming up image cache"
    logger.go:42: 20:21:35 | 005-app-wide-update-strategy/2-run-updater | time="2024-08-27T20:21:35-04:00" level=info msg="Setting new image to gcr.io/heptio-images/ks-guestbook-demo:0.2" alias=guestbook application=image-updater-005-01 image_name=heptio-images/ks-guestbook-demo image_tag=0.1 registry=gcr.io
    logger.go:42: 20:21:35 | 005-app-wide-update-strategy/2-run-updater | time="2024-08-27T20:21:35-04:00" level=info msg="Successfully updated image 'gcr.io/heptio-images/ks-guestbook-demo:0.1' to 'gcr.io/heptio-images/ks-guestbook-demo:0.2', but pending spec update (dry run=true)" alias=guestbook application=image-updater-005-01 image_name=heptio-images/ks-guestbook-demo image_tag=0.1 registry=gcr.io
    logger.go:42: 20:21:35 | 005-app-wide-update-strategy/2-run-updater | time="2024-08-27T20:21:35-04:00" level=info msg="Dry run - not committing 1 changes to application" application=image-updater-005-01
    logger.go:42: 20:21:35 | 005-app-wide-update-strategy/2-run-updater | time="2024-08-27T20:21:35-04:00" level=warning msg="\"latest\" strategy has been renamed to \"newest-build\". Please switch to the new convention as support for the old naming convention will be removed in future versions." image_alias=guestbook image_digest= image_name=gcr.io/heptio-images/ks-guestbook-demo image_tag=latest registry_url=gcr.io
    logger.go:42: 20:21:36 | 005-app-wide-update-strategy/2-run-updater | time="2024-08-27T20:21:36-04:00" level=info msg="Finished cache warm-up, pre-loaded 0 meta data entries from 2 registries"
    logger.go:42: 20:21:36 | 005-app-wide-update-strategy/2-run-updater | time="2024-08-27T20:21:36-04:00" level=info msg="Starting image update cycle, considering 2 annotated application(s) for update"
    logger.go:42: 20:21:36 | 005-app-wide-update-strategy/2-run-updater | time="2024-08-27T20:21:36-04:00" level=warning msg="\"latest\" strategy has been renamed to \"newest-build\". Please switch to the new convention as support for the old naming convention will be removed in future versions." image_alias=guestbook image_digest= image_name=gcr.io/heptio-images/ks-guestbook-demo image_tag=latest registry_url=gcr.io
    logger.go:42: 20:21:36 | 005-app-wide-update-strategy/2-run-updater | time="2024-08-27T20:21:36-04:00" level=info msg="Setting new image to gcr.io/heptio-images/ks-guestbook-demo:0.2" alias=guestbook application=image-updater-005-01 image_name=heptio-images/ks-guestbook-demo image_tag=0.1 registry=gcr.io
    logger.go:42: 20:21:36 | 005-app-wide-update-strategy/2-run-updater | time="2024-08-27T20:21:36-04:00" level=info msg="Successfully updated image 'gcr.io/heptio-images/ks-guestbook-demo:0.1' to 'gcr.io/heptio-images/ks-guestbook-demo:0.2', but pending spec update (dry run=false)" alias=guestbook application=image-updater-005-01 image_name=heptio-images/ks-guestbook-demo image_tag=0.1 registry=gcr.io
    logger.go:42: 20:21:36 | 005-app-wide-update-strategy/2-run-updater | time="2024-08-27T20:21:36-04:00" level=info msg="Committing 1 parameter update(s) for application image-updater-005-01" application=image-updater-005-01
    logger.go:42: 20:21:36 | 005-app-wide-update-strategy/2-run-updater | W0827 20:21:36.756023    7062 warnings.go:70] unknown field "status.history[0].initiatedBy"
    logger.go:42: 20:21:36 | 005-app-wide-update-strategy/2-run-updater | time="2024-08-27T20:21:36-04:00" level=info msg="Successfully updated the live application spec" application=image-updater-005-01
    logger.go:42: 20:21:36 | 005-app-wide-update-strategy/2-run-updater | time="2024-08-27T20:21:36-04:00" level=info msg="Processing results: applications=2 images_considered=2 images_skipped=0 images_updated=1 errors=0"
    logger.go:42: 20:21:36 | 005-app-wide-update-strategy/2-run-updater | time="2024-08-27T20:21:36-04:00" level=info msg=Finished.
    logger.go:42: 20:21:36 | 005-app-wide-update-strategy/2-run-updater | test step completed 2-run-updater
    logger.go:42: 20:21:36 | 005-app-wide-update-strategy/99-delete | starting test step 99-delete
    logger.go:42: 20:21:36 | 005-app-wide-update-strategy/99-delete | test step completed 99-delete
    logger.go:42: 20:21:36 | 005-app-wide-update-strategy | skipping kubernetes event logging
=== NAME  kuttl
    harness.go:407: run tests finished
    harness.go:515: cleaning up
    harness.go:572: removing temp folder: ""
--- PASS: kuttl (6.16s)
    --- PASS: kuttl/harness (0.00s)
        --- PASS: kuttl/harness/005-app-wide-update-strategy (6.14s)
PASS
```