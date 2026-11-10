package argocd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	argocdclient "github.com/argoproj/argo-cd/v3/pkg/apiclient"
	"github.com/argoproj/argo-cd/v3/pkg/apiclient/application"
	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/argoproj/argo-cd/v3/pkg/client/clientset/versioned/fake"
	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	"github.com/argoproj-labs/argocd-image-updater/pkg/kube"
	"github.com/argoproj-labs/argocd-image-updater/pkg/metrics"
)

// helmApp returns a Helm application with an image tag parameter.
func helmApp(tag string) *v1alpha1.Application {
	limit := int64(10)
	return &v1alpha1.Application{
		ObjectMeta: v1.ObjectMeta{Name: "test-app", Namespace: "testns"},
		Spec: v1alpha1.ApplicationSpec{
			Project:              "default",
			RevisionHistoryLimit: &limit,
			Destination:          v1alpha1.ApplicationDestination{Server: "https://kubernetes.default.svc", Namespace: "old-ns"},
			Source: &v1alpha1.ApplicationSource{
				RepoURL: "https://github.com/example/repo",
				Helm: &v1alpha1.ApplicationSourceHelm{
					Parameters: []v1alpha1.HelmParameter{
						{Name: "image.name", Value: "nginx"},
						{Name: "image.tag", Value: tag},
					},
				},
			},
		},
	}
}

// withNewTag returns a copy of the spec of app with the image tag set to tag,
// as Image Updater does when it finds a new version.
func withNewTag(app *v1alpha1.Application, tag string) *v1alpha1.ApplicationSpec {
	spec := app.Spec.DeepCopy()
	spec.Source.Helm.Parameters[1].Value = tag
	return spec
}

func updateSpecRequest(spec *v1alpha1.ApplicationSpec) *application.ApplicationUpdateSpecRequest {
	appName := "test-app"
	appNamespace := "testns"
	return &application.ApplicationUpdateSpecRequest{Name: &appName, AppNamespace: &appNamespace, Spec: spec}
}

func newTestK8sClient(t *testing.T, clientset *fake.Clientset) ArgoCD {
	t.Helper()
	client, err := NewK8SClient(&kube.ImageUpdaterKubernetesClient{ApplicationsClientset: clientset}, nil)
	require.NoError(t, err)
	return client
}

func getLiveApp(t *testing.T, clientset *fake.Clientset) *v1alpha1.Application {
	t.Helper()
	app, err := clientset.ArgoprojV1alpha1().Applications("testns").Get(context.Background(), "test-app", v1.GetOptions{})
	require.NoError(t, err)
	return app
}

// countUpdates counts update calls on the clientset.
func countUpdates(clientset *fake.Clientset) func() int {
	var mu sync.Mutex
	n := 0
	clientset.PrependReactor("update", "applications", func(action k8stesting.Action) (bool, runtime.Object, error) {
		mu.Lock()
		defer mu.Unlock()
		n++
		return false, nil, nil
	})
	return func() int {
		mu.Lock()
		defer mu.Unlock()
		return n
	}
}

// writeConcurrentlyOnFirstUpdate changes the stored Application right before
// the first update is processed, simulating another client writing in between,
// and reports a conflict for that first update.
func writeConcurrentlyOnFirstUpdate(t *testing.T, clientset *fake.Clientset, mutate func(app *v1alpha1.Application)) {
	first := true
	clientset.PrependReactor("update", "applications", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if !first {
			return false, nil, nil
		}
		first = false
		obj, err := clientset.Tracker().Get(v1alpha1.SchemeGroupVersion.WithResource("applications"), "testns", "test-app")
		require.NoError(t, err)
		live := obj.(*v1alpha1.Application).DeepCopy()
		mutate(live)
		require.NoError(t, clientset.Tracker().Update(v1alpha1.SchemeGroupVersion.WithResource("applications"), live, "testns"))
		return true, nil, errors.NewConflict(v1alpha1.Resource("applications"), "test-app", fmt.Errorf("conflict"))
	})
}

func TestKubernetesClientUpdateSpecWithBaseline(t *testing.T) {
	t.Run("Concurrent changes to other fields are preserved", func(t *testing.T) {
		// The spec as read at the start of the update cycle
		original := helmApp("1.0.0")

		// Meanwhile, another client changes unrelated fields
		live := helmApp("1.0.0")
		live.Spec.Destination.Namespace = "new-ns"
		newLimit := int64(42)
		live.Spec.RevisionHistoryLimit = &newLimit
		live.Spec.SyncPolicy = &v1alpha1.SyncPolicy{SyncOptions: v1alpha1.SyncOptions{"CreateNamespace=true"}}
		clientset := fake.NewSimpleClientset(live)

		_, err := newTestK8sClient(t, clientset).UpdateSpec(context.Background(), updateSpecRequest(withNewTag(original, "1.0.1")), &original.Spec)
		require.NoError(t, err)

		got := getLiveApp(t, clientset)
		assert.Equal(t, []v1alpha1.HelmParameter{{Name: "image.name", Value: "nginx"}, {Name: "image.tag", Value: "1.0.1"}}, got.Spec.Source.Helm.Parameters)
		assert.Equal(t, "new-ns", got.Spec.Destination.Namespace)
		assert.Equal(t, "https://kubernetes.default.svc", got.Spec.Destination.Server)
		require.NotNil(t, got.Spec.RevisionHistoryLimit)
		assert.Equal(t, int64(42), *got.Spec.RevisionHistoryLimit)
		require.NotNil(t, got.Spec.SyncPolicy)
		assert.Equal(t, v1alpha1.SyncOptions{"CreateNamespace=true"}, got.Spec.SyncPolicy.SyncOptions)
		assert.Equal(t, "default", got.Spec.Project)
		assert.Equal(t, "https://github.com/example/repo", got.Spec.Source.RepoURL)
	})

	t.Run("Concurrent change to the same field is not overwritten", func(t *testing.T) {
		original := helmApp("1.0.0")
		live := helmApp("1.0.0")
		live.Spec.Source.Helm.Parameters = append(live.Spec.Source.Helm.Parameters, v1alpha1.HelmParameter{Name: "replicas", Value: "3"})
		clientset := fake.NewSimpleClientset(live)
		updates := countUpdates(clientset)

		_, err := newTestK8sClient(t, clientset).UpdateSpec(context.Background(), updateSpecRequest(withNewTag(original, "1.0.1")), &original.Spec)
		require.ErrorIs(t, err, ErrConcurrentSpecChange)
		assert.Contains(t, err.Error(), "spec.source.helm.parameters")
		assert.Equal(t, 0, updates())
		assert.Equal(t, live.Spec.Source.Helm.Parameters, getLiveApp(t, clientset).Spec.Source.Helm.Parameters)
	})

	t.Run("Identical concurrent change to the same field is not a conflict", func(t *testing.T) {
		original := helmApp("1.0.0")
		live := helmApp("1.0.1")
		live.Spec.Destination.Namespace = "new-ns"
		clientset := fake.NewSimpleClientset(live)

		_, err := newTestK8sClient(t, clientset).UpdateSpec(context.Background(), updateSpecRequest(withNewTag(original, "1.0.1")), &original.Spec)
		require.NoError(t, err)

		got := getLiveApp(t, clientset)
		assert.Equal(t, "1.0.1", got.Spec.Source.Helm.Parameters[1].Value)
		assert.Equal(t, "new-ns", got.Spec.Destination.Namespace)
	})

	t.Run("Changes are rebased onto the re-fetched application after a conflict", func(t *testing.T) {
		original := helmApp("1.0.0")
		clientset := fake.NewSimpleClientset(helmApp("1.0.0"))
		writeConcurrentlyOnFirstUpdate(t, clientset, func(app *v1alpha1.Application) {
			app.Spec.Destination.Namespace = "new-ns"
		})

		_, err := newTestK8sClient(t, clientset).UpdateSpec(context.Background(), updateSpecRequest(withNewTag(original, "1.0.1")), &original.Spec)
		require.NoError(t, err)

		got := getLiveApp(t, clientset)
		assert.Equal(t, "1.0.1", got.Spec.Source.Helm.Parameters[1].Value)
		assert.Equal(t, "new-ns", got.Spec.Destination.Namespace)
	})

	t.Run("Conflicting change made between retries is detected", func(t *testing.T) {
		original := helmApp("1.0.0")
		clientset := fake.NewSimpleClientset(helmApp("1.0.0"))
		writeConcurrentlyOnFirstUpdate(t, clientset, func(app *v1alpha1.Application) {
			app.Spec.Source.Helm.Parameters[1].Value = "2.0.0"
		})

		_, err := newTestK8sClient(t, clientset).UpdateSpec(context.Background(), updateSpecRequest(withNewTag(original, "1.0.1")), &original.Spec)
		require.ErrorIs(t, err, ErrConcurrentSpecChange)
		assert.Equal(t, "2.0.0", getLiveApp(t, clientset).Spec.Source.Helm.Parameters[1].Value)
	})

	t.Run("Kustomize images and newly created fields are applied", func(t *testing.T) {
		original := &v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{Name: "test-app", Namespace: "testns"},
			Spec: v1alpha1.ApplicationSpec{
				Destination: v1alpha1.ApplicationDestination{Namespace: "old-ns"},
				Source:      &v1alpha1.ApplicationSource{RepoURL: "https://github.com/example/repo"},
			},
		}
		live := original.DeepCopy()
		live.Spec.Destination.Namespace = "new-ns"
		clientset := fake.NewSimpleClientset(live)

		updated := original.Spec.DeepCopy()
		updated.Source.Kustomize = &v1alpha1.ApplicationSourceKustomize{Images: v1alpha1.KustomizeImages{"nginx:1.0.1"}}

		_, err := newTestK8sClient(t, clientset).UpdateSpec(context.Background(), updateSpecRequest(updated), &original.Spec)
		require.NoError(t, err)

		got := getLiveApp(t, clientset)
		require.NotNil(t, got.Spec.Source.Kustomize)
		assert.Equal(t, v1alpha1.KustomizeImages{"nginx:1.0.1"}, got.Spec.Source.Kustomize.Images)
		assert.Equal(t, "new-ns", got.Spec.Destination.Namespace)
	})

	t.Run("Multi-source: concurrent change to another source conflicts", func(t *testing.T) {
		// A merge patch replaces arrays as a whole, so a change anywhere in
		// spec.sources must not be overwritten with the stale array.
		original := &v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{Name: "test-app", Namespace: "testns"},
			Spec: v1alpha1.ApplicationSpec{
				Sources: v1alpha1.ApplicationSources{
					{RepoURL: "https://github.com/example/chart", Helm: &v1alpha1.ApplicationSourceHelm{Parameters: []v1alpha1.HelmParameter{{Name: "image.tag", Value: "1.0.0"}}}},
					{RepoURL: "https://github.com/example/values", TargetRevision: "v1", Ref: "values"},
				},
			},
		}
		live := original.DeepCopy()
		live.Spec.Sources[1].TargetRevision = "v2"
		clientset := fake.NewSimpleClientset(live)

		updated := original.Spec.DeepCopy()
		updated.Sources[0].Helm.Parameters[0].Value = "1.0.1"

		_, err := newTestK8sClient(t, clientset).UpdateSpec(context.Background(), updateSpecRequest(updated), &original.Spec)
		require.ErrorIs(t, err, ErrConcurrentSpecChange)

		got := getLiveApp(t, clientset)
		assert.Equal(t, "v2", got.Spec.Sources[1].TargetRevision)
		assert.Equal(t, "1.0.0", got.Spec.Sources[0].Helm.Parameters[0].Value)
	})

	t.Run("Multi-source: update without concurrent changes", func(t *testing.T) {
		original := &v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{Name: "test-app", Namespace: "testns"},
			Spec: v1alpha1.ApplicationSpec{
				Sources: v1alpha1.ApplicationSources{
					{RepoURL: "https://github.com/example/chart", Helm: &v1alpha1.ApplicationSourceHelm{Parameters: []v1alpha1.HelmParameter{{Name: "image.tag", Value: "1.0.0"}}}},
					{RepoURL: "https://github.com/example/values", TargetRevision: "v1", Ref: "values"},
				},
			},
		}
		clientset := fake.NewSimpleClientset(original.DeepCopy())

		updated := original.Spec.DeepCopy()
		updated.Sources[0].Helm.Parameters[0].Value = "1.0.1"

		_, err := newTestK8sClient(t, clientset).UpdateSpec(context.Background(), updateSpecRequest(updated), &original.Spec)
		require.NoError(t, err)
		assert.Equal(t, *updated, getLiveApp(t, clientset).Spec)
	})

}

// fakeArgoCDAPIClient is an Argo CD API client whose application service is
// backed by fakeApplicationService.
type fakeArgoCDAPIClient struct {
	argocdclient.Client
	appService *fakeApplicationService
}

func (f *fakeArgoCDAPIClient) NewApplicationClient() (io.Closer, application.ApplicationServiceClient, error) {
	return io.NopCloser(strings.NewReader("")), f.appService, nil
}

func (f *fakeArgoCDAPIClient) ClientOptions() argocdclient.ClientOptions {
	return argocdclient.ClientOptions{ServerAddr: "argocd.example.com"}
}

// fakeApplicationService serves a single Application. Patch applies the patch
// to the stored Application like the Argo CD API server does.
type fakeApplicationService struct {
	application.ApplicationServiceClient
	app           *v1alpha1.Application
	beforePatch   func(app *v1alpha1.Application)
	patchRequests []*application.ApplicationPatchRequest
	updateSpecs   int
}

func (f *fakeApplicationService) Get(ctx context.Context, in *application.ApplicationQuery, opts ...grpc.CallOption) (*v1alpha1.Application, error) {
	return f.app.DeepCopy(), nil
}

func (f *fakeApplicationService) UpdateSpec(ctx context.Context, in *application.ApplicationUpdateSpecRequest, opts ...grpc.CallOption) (*v1alpha1.ApplicationSpec, error) {
	f.updateSpecs++
	f.app.Spec = *in.Spec
	return &f.app.Spec, nil
}

func (f *fakeApplicationService) Patch(ctx context.Context, in *application.ApplicationPatchRequest, opts ...grpc.CallOption) (*v1alpha1.Application, error) {
	f.patchRequests = append(f.patchRequests, in)
	if f.beforePatch != nil {
		f.beforePatch(f.app)
	}
	if in.GetPatchType() != "merge" {
		return nil, fmt.Errorf("unexpected patch type %q", in.GetPatchType())
	}
	appJSON, err := json.Marshal(f.app)
	if err != nil {
		return nil, err
	}
	patched, err := jsonpatch.MergePatch(appJSON, []byte(in.GetPatch()))
	if err != nil {
		return nil, err
	}
	newApp := &v1alpha1.Application{}
	if err := json.Unmarshal(patched, newApp); err != nil {
		return nil, err
	}
	f.app = newApp
	return newApp.DeepCopy(), nil
}

var initMetricsOnce sync.Once

func newTestAPIClient(appService *fakeApplicationService) ArgoCD {
	initMetricsOnce.Do(func() {
		if metrics.Clients() == nil {
			metrics.InitMetrics()
		}
	})
	return &argoCD{Client: &fakeArgoCDAPIClient{appService: appService}}
}

func TestArgoCDAPIClientUpdateSpecWithBaseline(t *testing.T) {
	t.Run("Only the changed fields are sent as a merge patch", func(t *testing.T) {
		original := helmApp("1.0.0")
		live := helmApp("1.0.0")
		live.Spec.Destination.Namespace = "new-ns"
		appService := &fakeApplicationService{app: live}
		// Another client changes a field after we checked for conflicts,
		// right before the patch is applied on the server.
		appService.beforePatch = func(app *v1alpha1.Application) {
			app.Spec.SyncPolicy = &v1alpha1.SyncPolicy{SyncOptions: v1alpha1.SyncOptions{"CreateNamespace=true"}}
		}

		spec, err := newTestAPIClient(appService).UpdateSpec(context.Background(), updateSpecRequest(withNewTag(original, "1.0.1")), &original.Spec)
		require.NoError(t, err)
		assert.Equal(t, "1.0.1", spec.Source.Helm.Parameters[1].Value)

		require.Len(t, appService.patchRequests, 1)
		req := appService.patchRequests[0]
		assert.Equal(t, "test-app", req.GetName())
		assert.Equal(t, "testns", req.GetAppNamespace())
		assert.Equal(t, "merge", req.GetPatchType())
		assert.JSONEq(t, `{"spec":{"source":{"helm":{"parameters":[{"name":"image.name","value":"nginx"},{"name":"image.tag","value":"1.0.1"}]}}}}`, req.GetPatch())
		assert.Equal(t, 0, appService.updateSpecs)

		got := appService.app
		assert.Equal(t, "1.0.1", got.Spec.Source.Helm.Parameters[1].Value)
		assert.Equal(t, "new-ns", got.Spec.Destination.Namespace)
		require.NotNil(t, got.Spec.SyncPolicy)
		assert.Equal(t, v1alpha1.SyncOptions{"CreateNamespace=true"}, got.Spec.SyncPolicy.SyncOptions)
	})

	t.Run("Concurrent change to the same field is not overwritten", func(t *testing.T) {
		original := helmApp("1.0.0")
		appService := &fakeApplicationService{app: helmApp("2.0.0")}

		_, err := newTestAPIClient(appService).UpdateSpec(context.Background(), updateSpecRequest(withNewTag(original, "1.0.1")), &original.Spec)
		require.ErrorIs(t, err, ErrConcurrentSpecChange)
		assert.Empty(t, appService.patchRequests)
		assert.Equal(t, 0, appService.updateSpecs)
		assert.Equal(t, "2.0.0", appService.app.Spec.Source.Helm.Parameters[1].Value)
	})

	t.Run("Without a baseline the whole spec is replaced", func(t *testing.T) {
		original := helmApp("1.0.0")
		appService := &fakeApplicationService{app: helmApp("1.0.0")}

		_, err := newTestAPIClient(appService).UpdateSpec(context.Background(), updateSpecRequest(withNewTag(original, "1.0.1")), nil)
		require.NoError(t, err)
		assert.Empty(t, appService.patchRequests)
		assert.Equal(t, 1, appService.updateSpecs)
		assert.Equal(t, "1.0.1", appService.app.Spec.Source.Helm.Parameters[1].Value)
	})
}

func Test_mergePatchesConflict(t *testing.T) {
	tests := []struct {
		name     string
		ours     map[string]any
		theirs   map[string]any
		conflict bool
		path     string
	}{
		{name: "disjoint fields", ours: map[string]any{"a": 1}, theirs: map[string]any{"b": 2}},
		{name: "disjoint nested fields", ours: map[string]any{"a": map[string]any{"x": 1}}, theirs: map[string]any{"a": map[string]any{"y": 2}}},
		{name: "same field different value", ours: map[string]any{"a": map[string]any{"x": 1}}, theirs: map[string]any{"a": map[string]any{"x": 2}}, conflict: true, path: "spec.a.x"},
		{name: "same field same value", ours: map[string]any{"a": []any{"v"}}, theirs: map[string]any{"a": []any{"v"}}},
		{name: "they removed the parent object", ours: map[string]any{"a": map[string]any{"x": 1}}, theirs: map[string]any{"a": nil}, conflict: true, path: "spec.a"},
		{name: "they replaced the array we change", ours: map[string]any{"a": []any{"1"}}, theirs: map[string]any{"a": []any{"2"}}, conflict: true, path: "spec.a"},
		{name: "no concurrent changes", ours: map[string]any{"a": 1}, theirs: map[string]any{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, conflict := mergePatchesConflict(tt.ours, tt.theirs, "spec")
			assert.Equal(t, tt.conflict, conflict)
			assert.Equal(t, tt.path, path)
		})
	}
}
