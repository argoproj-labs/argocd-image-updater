This test case verifies [filtering applications by name](https://argocd-image-updater.readthedocs.io/en/stable/install/reference/#flags) with `--match-application-name` command line options
* match against a non-existent application name
* match against an exact application name
* specify `--match-application-name` option multiple times to match against multiple application names
* use wild card `*` in application name pattern

This test case uses image from public container registry and application source from public GitHub repo.

To run this individual test case,

* make sure both docker daemon and k8s cluster is running
* `cd $HOME/go/src/image-updater/test/e2e`
* `SRC_DIR=$HOME/go/src/argocd-image-updater kubectl kuttl test --namespace argocd-image-updater-e2e --timeout 120 --test 102-kustomize-match-application-name`

The test output logs that during each test step, 0, 1, 2, or 3 images are updated, as specified by argocd-image-updater `--match-application-name` option:
```bash
102-kustomize-match-application-name/2-run-updater msg="Processing results: applications=0 images_considered=0 images_skipped=0 images_updated=0 errors=0"
102-kustomize-match-application-name/3-run-updater msg="Processing results: applications=1 images_considered=1 images_skipped=0 images_updated=1 errors=0"
102-kustomize-match-application-name/4-run-updater msg="Processing results: applications=2 images_considered=2 images_skipped=0 images_updated=2 errors=0"
102-kustomize-match-application-name/5-run-updater msg="Processing results: applications=3 images_considered=3 images_skipped=0 images_updated=3 errors=0"
```