This test case verifies an Application with multiple images in one Deployment.

This test case uses images from public container registry (`nginx` and `memcached`) and application source from public GitHub repo.

To run this individual test case,

* make sure both docker daemon and k8s cluster is running
* `cd $HOME/go/src/image-updater/test/e2e`
* `SRC_DIR=$HOME/go/src/argocd-image-updater kubectl kuttl test --namespace argocd-image-updater-e2e --timeout 120 --test 004-multiple-images`

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
=== RUN   kuttl/harness/004-multiple-images
=== PAUSE kuttl/harness/004-multiple-images
=== CONT  kuttl/harness/004-multiple-images
    logger.go:42: 15:38:10 | 004-multiple-images | Ignoring README.md as it does not match file name regexp: ^(\d+)-(?:[^\.]+)(?:\.yaml)?$
    logger.go:42: 15:38:10 | 004-multiple-images | Skipping creation of user-supplied namespace: argocd-image-updater-e2e
    logger.go:42: 15:38:10 | 004-multiple-images/1-install | starting test step 1-install
    logger.go:42: 15:38:10 | 004-multiple-images/1-install | Namespace:/image-updater-e2e-004 created
Warning: metadata.finalizers: "resources-finalizer.argocd.argoproj.io": prefer a domain-qualified finalizer name to avoid accidental conflicts with other finalizer writers
    logger.go:42: 15:38:10 | 004-multiple-images/1-install | Application:argocd-image-updater-e2e/image-updater-004 created
    logger.go:42: 15:38:12 | 004-multiple-images/1-install | test step completed 1-install
    logger.go:42: 15:38:12 | 004-multiple-images/2-run-updater | starting test step 2-run-updater
    logger.go:42: 15:38:12 | 004-multiple-images/2-run-updater | running command: [sh -c ${SRC_DIR}/dist/argocd-image-updater run --once \
          --argocd-namespace argocd-image-updater-e2e \
          --registries-conf-path ${SRC_DIR}/test/e2e/assets/registries.conf \
          --loglevel debug
        ]
    logger.go:42: 15:38:12 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:12+01:00" level=info msg="argocd-image-updater v99.9.9+d6a78eb starting [loglevel:DEBUG, interval:once, healthport:off]"
    logger.go:42: 15:38:12 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:12+01:00" level=warning msg="commit message template at /app/config/commit.template does not exist, using default"
    logger.go:42: 15:38:12 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:12+01:00" level=debug msg="Successfully parsed commit message template"
    logger.go:42: 15:38:12 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:12+01:00" level=debug msg="rate limiting is disabled" prefix="10.42.0.1:30000" registry="https://10.42.0.1:30000"
    logger.go:42: 15:38:12 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:12+01:00" level=info msg="Loaded 1 registry configurations from /home/dkarpele/go/src/argocd-image-updater/test/e2e/assets/registries.conf"
    logger.go:42: 15:38:12 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:12+01:00" level=info msg="ArgoCD configuration: [apiKind=kubernetes, server=argocd-server.argocd-image-updater-e2e, auth_token=false, insecure=false, grpc_web=false, plaintext=false]"
    logger.go:42: 15:38:12 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:12+01:00" level=info msg="Starting metrics server on TCP port=8081"
    logger.go:42: 15:38:12 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:12+01:00" level=info msg="Warming up image cache"
    logger.go:42: 15:38:12 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:12+01:00" level=debug msg="Applications listed: 1"
    logger.go:42: 15:38:12 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:12+01:00" level=debug msg="Processing application argocd-image-updater-e2e/image-updater-004"
    logger.go:42: 15:38:12 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:12+01:00" level=debug msg="Considering this image for update" alias=test-nginx application=image-updater-004 image_name=nginx image_tag=1.17.0 registry=
    logger.go:42: 15:38:12 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:12+01:00" level=debug msg="Using version constraint '1.17.10' when looking for a new tag" alias=test-nginx application=image-updater-004 image_name=nginx image_tag=1.17.0 registry=
    logger.go:42: 15:38:12 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:12+01:00" level=debug msg="Using canonical image name 'library/nginx' for image 'nginx'" alias=test-nginx application=image-updater-004 image_name=nginx image_tag=1.17.0 registry=
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag mainline-bullseye as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag bookworm-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag alpine3.20-slim as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag stable-alpine3.17-slim as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag alpine3.21-slim as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag mainline-bullseye-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag stable-alpine3.19-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag stable-alpine3.19-slim as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag alpine3.19-slim as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag alpine3.19-otel as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag mainline-alpine3.21-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag mainline-alpine3.17-slim as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag alpine3.21-otel as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag alpine as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag mainline-alpine3.17-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag alpine3.17-slim as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag mainline-alpine3.19-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag alpine3.21-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag mainline-alpine3.19-slim as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag stable-alpine3.20 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag mainline-alpine3.20-slim as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag mainline-alpine3.20-otel as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag mainline-otel as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag alpine3.20 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag stable-otel as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag stable-alpine3.20-slim as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag alpine3.19 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag mainline-alpine3.20 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag mainline-alpine-otel as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag alpine3.18-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag mainline-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag stable-bookworm-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag alpine3.20-otel as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag stable-bookworm-otel as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag stable-bullseye-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag bullseye as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag alpine-slim as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag mainline-bookworm-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag stable-bookworm as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag alpine3.17-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag stable-alpine as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag mainline-alpine3.19 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag stable-alpine3.20-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag stable-bullseye as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag alpine3.18-otel as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag alpine3.20-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag stable-alpine3.20-otel as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag mainline-alpine3.19-otel as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag alpine3.18 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag stable-alpine3.17-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag mainline as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag alpine3.17 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag mainline-alpine3.21-otel as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag mainline-alpine3.20-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag stable-alpine-otel as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag stable-alpine-slim as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag otel as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag mainline-alpine3.21 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag mainline-alpine3.18 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag mainline-bookworm as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag bullseye-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag bookworm-otel as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag mainline-alpine3.18-otel as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag bookworm as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag latest as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag alpine3.21 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag stable-alpine-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag stable-alpine3.17 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag mainline-alpine3.21-slim as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag alpine-otel as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag alpine3.19-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag mainline-alpine-slim as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag mainline-alpine as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag mainline-alpine3.18-slim as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag mainline-alpine-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag mainline-alpine3.17 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag stable as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag stable-alpine3.19-otel as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag alpine-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag stable-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag stable-alpine3.19 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag mainline-alpine3.18-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag alpine3.18-slim as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="could not parse input tag mainline-bookworm-otel as semver: Invalid Semantic Version"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="found 1 from 738 tags eligible for consideration" image="nginx:1.17.0"
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=info msg="Setting new image to nginx:1.17.10" alias=test-nginx application=image-updater-004 image_name=nginx image_tag=1.17.0 registry=
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=info msg="Successfully updated image 'nginx:1.17.0' to 'nginx:1.17.10', but pending spec update (dry run=true)" alias=test-nginx application=image-updater-004 image_name=nginx image_tag=1.17.0 registry=
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="Considering this image for update" alias=test-memcached application=image-updater-004 image_name=memcached image_tag=1.6.0 registry=
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="Using version constraint '1.6.10' when looking for a new tag" alias=test-memcached application=image-updater-004 image_name=memcached image_tag=1.6.0 registry=
    logger.go:42: 15:38:14 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:14+01:00" level=debug msg="Using canonical image name 'library/memcached' for image 'memcached'" alias=test-memcached application=image-updater-004 image_name=memcached image_tag=1.6.0 registry=
    logger.go:42: 15:38:15 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:15+01:00" level=debug msg="could not parse input tag alpine3.14 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:15 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:15+01:00" level=debug msg="could not parse input tag alpine as semver: Invalid Semantic Version"
    logger.go:42: 15:38:15 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:15+01:00" level=debug msg="could not parse input tag alpine3.16 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:15 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:15+01:00" level=debug msg="could not parse input tag latest as semver: Invalid Semantic Version"
    logger.go:42: 15:38:15 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:15+01:00" level=debug msg="could not parse input tag bookworm as semver: Invalid Semantic Version"
    logger.go:42: 15:38:15 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:15+01:00" level=debug msg="could not parse input tag bullseye as semver: Invalid Semantic Version"
    logger.go:42: 15:38:15 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:15+01:00" level=debug msg="could not parse input tag alpine3.15 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:15 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:15+01:00" level=debug msg="could not parse input tag alpine3.17 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:15 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:15+01:00" level=debug msg="could not parse input tag alpine3.19 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:15 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:15+01:00" level=debug msg="could not parse input tag alpine3.21 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:15 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:15+01:00" level=debug msg="could not parse input tag alpine3.13 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:15 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:15+01:00" level=debug msg="could not parse input tag alpine3.18 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:15 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:15+01:00" level=debug msg="could not parse input tag alpine3.20 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:15 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:15+01:00" level=debug msg="could not parse input tag buster as semver: Invalid Semantic Version"
    logger.go:42: 15:38:15 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:15+01:00" level=debug msg="found 1 from 244 tags eligible for consideration" image="memcached:1.6.0"
    logger.go:42: 15:38:15 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:15+01:00" level=info msg="Setting new image to memcached:1.6.10" alias=test-memcached application=image-updater-004 image_name=memcached image_tag=1.6.0 registry=
    logger.go:42: 15:38:15 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:15+01:00" level=info msg="Successfully updated image 'memcached:1.6.0' to 'memcached:1.6.10', but pending spec update (dry run=true)" alias=test-memcached application=image-updater-004 image_name=memcached image_tag=1.6.0 registry=
    logger.go:42: 15:38:15 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:15+01:00" level=debug msg="Using commit message: "
    logger.go:42: 15:38:15 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:15+01:00" level=info msg="Dry run - not committing 2 changes to application" application=image-updater-004
    logger.go:42: 15:38:15 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:15+01:00" level=info msg="Finished cache warm-up, pre-loaded 0 meta data entries from 2 registries"
    logger.go:42: 15:38:15 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:15+01:00" level=debug msg="Starting askpass server"
    logger.go:42: 15:38:15 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:15+01:00" level=debug msg="Applications listed: 1"
    logger.go:42: 15:38:15 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:15+01:00" level=info msg="Starting image update cycle, considering 1 annotated application(s) for update"
    logger.go:42: 15:38:15 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:15+01:00" level=debug msg="Processing application argocd-image-updater-e2e/image-updater-004"
    logger.go:42: 15:38:15 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:15+01:00" level=debug msg="Considering this image for update" alias=test-nginx application=image-updater-004 image_name=nginx image_tag=1.17.0 registry=
    logger.go:42: 15:38:15 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:15+01:00" level=debug msg="Using version constraint '1.17.10' when looking for a new tag" alias=test-nginx application=image-updater-004 image_name=nginx image_tag=1.17.0 registry=
    logger.go:42: 15:38:15 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:15+01:00" level=debug msg="Using canonical image name 'library/nginx' for image 'nginx'" alias=test-nginx application=image-updater-004 image_name=nginx image_tag=1.17.0 registry=
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag alpine3.20-slim as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag alpine3.18-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag alpine3.19-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag alpine3.20 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag mainline-alpine-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag stable-alpine as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag mainline-alpine3.21 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag alpine3.17-slim as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag alpine3.19 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag alpine3.19-otel as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag stable-alpine3.19-otel as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag alpine3.18-slim as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag alpine as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag alpine-otel as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag stable as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag mainline-alpine-slim as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag bullseye-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag mainline-alpine3.17-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag mainline-alpine3.19-slim as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag mainline-alpine3.18-otel as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag stable-alpine3.17-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag mainline-otel as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag mainline-alpine as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag mainline-alpine3.21-slim as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag alpine3.18 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag alpine3.18-otel as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag stable-bookworm-otel as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag bookworm as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag mainline-alpine3.21-otel as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag stable-alpine3.19 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag alpine3.20-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag stable-alpine3.20 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag stable-bullseye as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag stable-alpine3.20-slim as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag mainline-alpine3.18 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag mainline-alpine3.17 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag mainline-alpine3.19 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag mainline-alpine-otel as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag mainline-alpine3.18-slim as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag mainline-alpine3.19-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag stable-otel as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag stable-bookworm-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag alpine3.21 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag mainline-alpine3.20 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag mainline-bullseye-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag latest as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag alpine3.21-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag bookworm-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag mainline-bullseye as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag alpine-slim as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag alpine3.21-otel as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag stable-alpine3.19-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag otel as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag stable-bookworm as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag bullseye as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag mainline-bookworm-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag stable-alpine-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag mainline as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag stable-alpine3.20-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag alpine3.20-otel as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag stable-alpine3.20-otel as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag mainline-alpine3.17-slim as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag stable-alpine3.17-slim as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag stable-bullseye-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag mainline-bookworm-otel as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag stable-alpine-slim as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag bookworm-otel as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag mainline-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag alpine-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag stable-alpine-otel as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag stable-alpine3.17 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag stable-alpine3.19-slim as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag alpine3.21-slim as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag mainline-alpine3.20-slim as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag alpine3.19-slim as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag mainline-alpine3.20-otel as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag mainline-bookworm as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag mainline-alpine3.18-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag alpine3.17-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag mainline-alpine3.19-otel as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag alpine3.17 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag stable-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag mainline-alpine3.21-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="could not parse input tag mainline-alpine3.20-perl as semver: Invalid Semantic Version"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="found 1 from 738 tags eligible for consideration" image="nginx:1.17.0"
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=info msg="Setting new image to nginx:1.17.10" alias=test-nginx application=image-updater-004 image_name=nginx image_tag=1.17.0 registry=
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=info msg="Successfully updated image 'nginx:1.17.0' to 'nginx:1.17.10', but pending spec update (dry run=false)" alias=test-nginx application=image-updater-004 image_name=nginx image_tag=1.17.0 registry=
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="Considering this image for update" alias=test-memcached application=image-updater-004 image_name=memcached image_tag=1.6.0 registry=
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="Using version constraint '1.6.10' when looking for a new tag" alias=test-memcached application=image-updater-004 image_name=memcached image_tag=1.6.0 registry=
    logger.go:42: 15:38:16 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:16+01:00" level=debug msg="Using canonical image name 'library/memcached' for image 'memcached'" alias=test-memcached application=image-updater-004 image_name=memcached image_tag=1.6.0 registry=
    logger.go:42: 15:38:17 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:17+01:00" level=debug msg="could not parse input tag alpine3.20 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:17 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:17+01:00" level=debug msg="could not parse input tag buster as semver: Invalid Semantic Version"
    logger.go:42: 15:38:17 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:17+01:00" level=debug msg="could not parse input tag alpine3.19 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:17 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:17+01:00" level=debug msg="could not parse input tag alpine3.13 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:17 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:17+01:00" level=debug msg="could not parse input tag alpine3.21 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:17 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:17+01:00" level=debug msg="could not parse input tag bookworm as semver: Invalid Semantic Version"
    logger.go:42: 15:38:17 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:17+01:00" level=debug msg="could not parse input tag latest as semver: Invalid Semantic Version"
    logger.go:42: 15:38:17 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:17+01:00" level=debug msg="could not parse input tag alpine3.17 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:17 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:17+01:00" level=debug msg="could not parse input tag alpine as semver: Invalid Semantic Version"
    logger.go:42: 15:38:17 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:17+01:00" level=debug msg="could not parse input tag alpine3.16 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:17 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:17+01:00" level=debug msg="could not parse input tag bullseye as semver: Invalid Semantic Version"
    logger.go:42: 15:38:17 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:17+01:00" level=debug msg="could not parse input tag alpine3.15 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:17 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:17+01:00" level=debug msg="could not parse input tag alpine3.18 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:17 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:17+01:00" level=debug msg="could not parse input tag alpine3.14 as semver: Invalid Semantic Version"
    logger.go:42: 15:38:17 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:17+01:00" level=debug msg="found 1 from 244 tags eligible for consideration" image="memcached:1.6.0"
    logger.go:42: 15:38:17 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:17+01:00" level=info msg="Setting new image to memcached:1.6.10" alias=test-memcached application=image-updater-004 image_name=memcached image_tag=1.6.0 registry=
    logger.go:42: 15:38:17 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:17+01:00" level=info msg="Successfully updated image 'memcached:1.6.0' to 'memcached:1.6.10', but pending spec update (dry run=false)" alias=test-memcached application=image-updater-004 image_name=memcached image_tag=1.6.0 registry=
    logger.go:42: 15:38:17 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:17+01:00" level=debug msg="Using commit message: "
    logger.go:42: 15:38:17 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:17+01:00" level=info msg="Committing 2 parameter update(s) for application image-updater-004" application=image-updater-004
    logger.go:42: 15:38:17 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:17+01:00" level=debug msg="Applications listed: 1"
    logger.go:42: 15:38:17 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:17+01:00" level=debug msg="Found application: image-updater-004 in namespace argocd-image-updater-e2e"
    logger.go:42: 15:38:17 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:17+01:00" level=debug msg="Application image-updater-004 matches the pattern"
    logger.go:42: 15:38:17 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:17+01:00" level=info msg="Successfully updated the live application spec" application=image-updater-004
    logger.go:42: 15:38:17 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:17+01:00" level=info msg="Processing results: applications=1 images_considered=2 images_skipped=0 images_updated=2 errors=0"
    logger.go:42: 15:38:17 | 004-multiple-images/2-run-updater | time="2025-03-17T15:38:17+01:00" level=info msg=Finished.
    logger.go:42: 15:38:19 | 004-multiple-images/2-run-updater | test step completed 2-run-updater
    logger.go:42: 15:38:19 | 004-multiple-images/99-delete | starting test step 99-delete
    logger.go:42: 15:38:25 | 004-multiple-images/99-delete | test step completed 99-delete
    logger.go:42: 15:38:25 | 004-multiple-images | skipping kubernetes event logging
=== NAME  kuttl
    harness.go:403: run tests finished
    harness.go:510: cleaning up
    harness.go:567: removing temp folder: ""
--- PASS: kuttl (14.45s)
    --- PASS: kuttl/harness (0.00s)
        --- PASS: kuttl/harness/004-multiple-images (14.44s)
PASS
```