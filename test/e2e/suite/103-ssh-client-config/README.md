This test case verifies the support for configuring ssh client via the config map `argocd-image-updater-ssh-config`.

This test case performs the following steps:
* kustomize the default argocd-image-updater installation by adding custom ssh config data to the config map `argocd-image-updater-ssh-config`
* install the customized argocd-image-updater to the test cluster
* verify that the customized ssh config (config map and volume mount) are present 
* uninstall argocd-image-updater from the test cluster

To run this individual test case,

* make sure both docker daemon and k8s cluster is running
* `cd $HOME/go/src/image-updater/test/e2e`
* `SRC_DIR=$HOME/go/src/image-updater kubectl kuttl test --namespace argocd-image-updater-e2e --timeout 120 --test 103-ssh-client-config`

Test output:
```bash
    harness.go:278: Successful connection to cluster at: https://0.0.0.0:58961
    harness.go:363: running tests
    harness.go:75: going to run test suite with timeout of 120 seconds for each step
    harness.go:375: testsuite: ./suite has 8 tests
=== RUN   kuttl/harness
=== RUN   kuttl/harness/103-ssh-client-config
=== PAUSE kuttl/harness/103-ssh-client-config
=== CONT  kuttl/harness/103-ssh-client-config
    logger.go:42: 19:47:52 | 103-ssh-client-config/1-install | starting test step 1-install
    logger.go:42: 19:47:52 | 103-ssh-client-config/1-install | running command: [kubectl -n argocd-image-updater-e2e apply -k .]
    logger.go:42: 19:47:53 | 103-ssh-client-config/1-install | serviceaccount/argocd-image-updater created
    logger.go:42: 19:47:53 | 103-ssh-client-config/1-install | role.rbac.authorization.k8s.io/argocd-image-updater created
    logger.go:42: 19:47:53 | 103-ssh-client-config/1-install | rolebinding.rbac.authorization.k8s.io/argocd-image-updater created
    logger.go:42: 19:47:53 | 103-ssh-client-config/1-install | configmap/argocd-image-updater-config created
    logger.go:42: 19:47:53 | 103-ssh-client-config/1-install | configmap/argocd-image-updater-ssh-config created
    logger.go:42: 19:47:53 | 103-ssh-client-config/1-install | secret/argocd-image-updater-secret created
    logger.go:42: 19:47:53 | 103-ssh-client-config/1-install | deployment.apps/argocd-image-updater created
    logger.go:42: 19:47:53 | 103-ssh-client-config/1-install | running command: [sleep 5]
    logger.go:42: 19:47:58 | 103-ssh-client-config/1-install | test step completed 1-install
    logger.go:42: 19:47:58 | 103-ssh-client-config/99-delete | starting test step 99-delete
    logger.go:42: 19:47:58 | 103-ssh-client-config/99-delete | running command: [kubectl -n argocd-image-updater-e2e delete -k .]
    logger.go:42: 19:47:59 | 103-ssh-client-config/99-delete | serviceaccount "argocd-image-updater" deleted
    logger.go:42: 19:47:59 | 103-ssh-client-config/99-delete | role.rbac.authorization.k8s.io "argocd-image-updater" deleted
    logger.go:42: 19:47:59 | 103-ssh-client-config/99-delete | rolebinding.rbac.authorization.k8s.io "argocd-image-updater" deleted
    logger.go:42: 19:47:59 | 103-ssh-client-config/99-delete | configmap "argocd-image-updater-config" deleted
    logger.go:42: 19:47:59 | 103-ssh-client-config/99-delete | configmap "argocd-image-updater-ssh-config" deleted
    logger.go:42: 19:47:59 | 103-ssh-client-config/99-delete | secret "argocd-image-updater-secret" deleted
    logger.go:42: 19:47:59 | 103-ssh-client-config/99-delete | deployment.apps "argocd-image-updater" deleted
    logger.go:42: 19:47:59 | 103-ssh-client-config/99-delete | running command: [sleep 5]
    logger.go:42: 19:48:04 | 103-ssh-client-config/99-delete | test step completed 99-delete
    logger.go:42: 19:48:04 | 103-ssh-client-config | skipping kubernetes event logging
=== NAME  kuttl
    harness.go:407: run tests finished
    harness.go:515: cleaning up
    harness.go:572: removing temp folder: ""
--- PASS: kuttl (12.08s)
    --- PASS: kuttl/harness (0.00s)
        --- PASS: kuttl/harness/103-ssh-client-config (12.07s)
```
