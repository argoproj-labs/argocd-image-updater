package argocd

import (
    "testing"
    "github.com/argoproj-labs/argocd-image-updater/pkg/common"
    v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    v1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
)

// Test that grouping does not mix different branches in one commit/push
func Test_groupIntentsByBranch(t *testing.T) {
    appA := &v1alpha1.Application{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{common.GitBranchAnnotation: "main:appA-branch"}}}
    appB := &v1alpha1.Application{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{common.GitBranchAnnotation: "main:appB-branch"}}}
    wbcA := &WriteBackConfig{GitRepo: "https://example/repo.git", GitWriteBranch: "appA-branch"}
    wbcB := &WriteBackConfig{GitRepo: "https://example/repo.git", GitWriteBranch: "appB-branch"}

    by := groupIntentsByBranch([]writeIntent{
        {app: appA, wbc: wbcA, changeList: []ChangeEntry{{}}, writeFn: writeOverrides},
        {app: appB, wbc: wbcB, changeList: []ChangeEntry{{}}, writeFn: writeOverrides},
        {app: appA, wbc: wbcA, changeList: []ChangeEntry{{}}, writeFn: writeOverrides},
    })

    if len(by["appA-branch"]) != 2 {
        t.Fatalf("expected 2 intents for appA-branch, got %d", len(by["appA-branch"]))
    }
    if len(by["appB-branch"]) != 1 {
        t.Fatalf("expected 1 intent for appB-branch, got %d", len(by["appB-branch"]))
    }
}
