# Argo CD Image Updater E2E Tests

!!!note
    The E2E test framework in this directory is based on the test framework from the [Argo CD Operator](https://github.com/argoproj-labs/argocd-operator/tree/master/tests/ginkgo).

Argo CD Image Updater E2E tests are defined within the `test/ginkgo` directory.

These tests are written with the Ginkgo/Gomega test framework.

## Running tests

### Running E2E tests locally

This workflow is designed for developers to test their local changes in a realistic, automated environment. All commands should be run from the project root directory.

#### Prerequisites

You must have the following tools installed locally:
- [Docker](https://docs.docker.com/get-docker/)
- [k3d](https://k3d.io/#installation)

#### Architecture

The `test-e2e-local` target in `test/ginkgo/Makefile` automates the entire testing lifecycle:

1.  **Cluster Creation**: A new `k3d` cluster is created for a clean test run.
2.  **Build**: The target calls the `docker-build` command in the root `Makefile` to build a Docker image from your current source code.
3.  **Image Import**: The newly built local image is imported directly into the `k3d` cluster's nodes, making it available to Kubernetes without a registry.
4.  **Operator Deployment**: The `argocd-operator` is deployed into the cluster. Its deployment is then patched to use your local image by setting the `ARGOCD_IMAGE_UPDATER_IMAGE` environment variable.
5.  **Test Execution**: The Ginkgo E2E test suite is run against the deployed operator and image updater.
6.  **Cluster Deletion**: After the tests complete, the `k3d` cluster is automatically deleted to clean up all resources.

#### Instructions

**To run the entire E2E test suite:**

This single command will perform all the steps described above.
```bash
make -C test/ginkgo test-e2e-local
```

**To clean up the cluster manually:**

This is only necessary if the `test-e2e-local` target is interrupted before it can clean up after itself.
```bash
make -C test/ginkgo k3d-cluster-delete
```

### Run a specific test:

```bash
# 'make ginkgo' to download ginkgo, if needed
# Examples:
./bin/ginkgo -vv -focus "1-001_validate_image_updater_test" -r ./test/ginkgo/parallel
```

## Test Code

Argo CD Image Updater E2E tests are defined within `test/ginkgo`.

These tests are written with the [Ginkgo/Gomega test frameworks](https://github.com/onsi/ginkgo), and were ported from previous Kuttl tests.

### Tests are currently grouped as follows:
- `sequential`: Tests that are not safe to run in parallel with other tests.
    - A test is NOT safe to run in parallel with other tests if:
        - It modifies resources in operator namespaces
        - It modifies cluster-scoped resources, such as `ClusterRoles`/`ClusterRoleBindings`, or `Namespaces` that are shared between tests
        - More generally, if it writes to a K8s resource that is used by another test.
- `parallel`: Tests that are safe to run in parallel with other tests
    - A test is safe to run in parallel if it does not have any of the above problematic behaviours. 
    - It is fine for a parallel test to READ shared or cluster-scoped resources (such as resources in operator namespaces)
    - But a parallel test should NEVER write to resources that may be shared with other tests (some cluster-scoped resources, etc.)

*Guidance*: Look at the list of restrictions for sequential. If your test is doing any of those things, it needs to run sequential. Otherwise, parallel is fine.

### Test fixture:
- Utility functions for writing tests can be found within the `fixture/` folder.
- `fixture/fixture.go` contains utility functions that are generally useful to writing tests.
    - Most important are:
    - `EnsureParallelCleanSlate`: Should be called at the beginning of every parallel test.
    - `EnsureSequentialCleanSlate`: Should be called at the beginning of every sequential test.
- `fixture/(name of resource)` contains functions that are specific to working with a particular resource.
    - For example, if you wanted to wait for an `Application` CR to be Synced/Healthy, you would use the functions defined in `fixture/application`.
    - Likewise, if you want to check a `Deployment`, see `fixture/deployment`.
- The goal of this test fixture is to make it easy to write tests, and to ensure it is easy to understand and maintain existing tests.
- See existing k8s tests for usage examples.

## Writing new tests

Ginkgo tests are read from left to right. For example:
- `Expect(k8sClient.Create(ctx, argoCD)).To(Succeed())`
    - Can be read as: Expect create of argo cd CR to succeed.
- `Eventually(appControllerPod, "3m", "5s").Should(k8sFixture.ExistByName())`
    - Can be read as: Eventually the `(argo cd application controller pod)` should exist (within 3 minute, checking every 5 seconds.)
- `fixture.Update(argoCD, func(){ (...)})`
    - Can be read as: Update Argo CD CR using the given function

The E2E tests we use within this repo uses the standard controller-runtime k8s go API to interact with kubernetes (controller-runtime). This API is very familiar to anyone already writing go operator/controller code (such as developers of this project).

The best way to learn how to write a new test (or matcher/fixture), is just to copy an existing one!

### Standard patterns you can use

#### To verify a K8s resource has an expected status/spec:
- `fixture` packages
    - Fixture packages contain utility functions which exists for (nearly) all resources (described in detail elsewhere)
    - Most often, a function in a `fixture` will already exist for what you are looking for. 
        - For example, use `argocdFixture` to check if Argo CD is available:
            - `Eventually(argoCDbeta1, "5m", "5s").Should(argocdFixture.BeAvailable())`
    - Consider adding new functions to fixtures, so that tests can use them as well.
- If no fixture package function exists, just use a function that returns bool

#### To create an object:
- `Expect(k8sClient.Create(ctx, (object))).Should(Succeed())`

#### To update an object, use `fixture.Update`
- `fixture.Update(object, func(){})` function
	- Test will automatically retry the update if update fails.
		- This avoids a common issue in k8s tests, where update fails which causes the test to fail.

#### To delete a k8s object
- `Expect(k8sClient.Delete(ctx, (object))).Should(Succeed())`
    - Where `(object)` is any k8s resource

#### When writing sequential tests, ensure you:

A) Call EnsureSequentialCleanSlate before each test:
```go
	BeforeEach(func() {
		fixture.EnsureSequentialCleanSlate()
	}
```

Unlike with parallel tests, you don't need to clean up namespace after each test. Sequential will automatically clean up namespaces created via the `fixture.Create(...)Namespace` API. (But if you want to delete it using `defer`, it doesn't hurt).

#### When writing parallel tests, ensure you:

A) Call EnsureParallelCleanSlate before each test
```go
	BeforeEach(func() {
		fixture.EnsureParallelCleanSlate()
	})
```

B) Clean up any namespaces (or any cluster-scoped resources you created) using `defer`:
```go
// Create a namespace to use for the duration of the test, and then automatically clean it up after.
ns, cleanupFunc := fixture.CreateRandomE2ETestNamespaceWithCleanupFunc()
defer cleanupFunc()
```

### General Tips
- DON'T ADD SLEEP STATEMENTS TO TESTS (unless it's absolutely necessary, but it rarely is!)
	- Use `Eventually`/`Consistently` with a condition, instead.
- Use `By("")` to document each step for what the test is doing.
	- This is very helpful for other team members that need to maintain your test after you wrote it.
	- Also, all `By("")`s are included in test output as `Step: (...)`, which makes it easy to tell what the test is doing when the test is running.

## Tips for debugging tests

### If you are debugging tests in CI
- If you are debugging a test failure, considering adding a call to the `fixture.OutputDebugOnFail()` function at the end of the test.
- `OutputDebugOnFail` will output helpful information when a test fails (such as namespace contents and operator pod logs)
- See existing test code for examples.

### If you are debugging tests locally
- Consider setting the `E2E_DEBUG_SKIP_CLEANUP` variable when debugging tests locally.
- The `E2E_DEBUG_SKIP_CLEANUP` environment variable will skip cleanup at the end of the test. 
    - The default E2E test behaviour is to clean up test resources at the end of the test. 
    - This is good when tests are succeeding, but when they are failing it can be helpful to look at the state of those K8s resources at the time of failure.
    - Those old tests resources WILL still be cleaned up when you next start the test again.
- This will allow you to `kubectl get` the test resource to see why the test failed. 

Example:
```bash
E2E_DEBUG_SKIP_CLEANUP=true ./bin/ginkgo -vv -focus "1-001_validate_image_updater_test"  -r ./test/ginkgo/parallel
```

## External Documentation

[**Ginkgo/Gomega docs**](https://onsi.github.io/gomega/): The Ginkgo/Gomega docs are great! they are very detailed, with lots of good examples. There are also plenty of other examples of Ginkgo/Gomega you can find via searching.

**Ask an LLM (Gemini/Cursor/etc)**: Ginkgo/gomega are popular enough that LLMs are able to answer questions and write code for them.
- For example, I performed the following Gemini Pro query, and got an excellent answer:
    - `With Ginkgo/Gomega (https://onsi.github.io/gomega) and Go lang, how do I create a matcher which checks whether a Kubernetes Deployment (via Deployment go object) has ready replicas of 1`
