/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/argoproj-labs/argocd-image-updater/api/v1alpha1"
	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/image"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/tag"
)

func TestBuildRecentUpdates(t *testing.T) {
	now := metav1.NewTime(time.Now())

	t.Run("nil changes returns nil", func(t *testing.T) {
		result := buildRecentUpdates(nil, now)
		assert.Nil(t, result)
	})

	t.Run("empty changes returns nil", func(t *testing.T) {
		result := buildRecentUpdates([]argocd.ChangeEntry{}, now)
		assert.Nil(t, result)
	})

	t.Run("single change", func(t *testing.T) {
		changes := []argocd.ChangeEntry{
			{
				Image: &image.ContainerImage{
					ImageName:  "nginx",
					ImageAlias: "web",
				},
				OldTag:  tag.NewImageTag("1.20", time.Now(), ""),
				NewTag:  tag.NewImageTag("1.21", time.Now(), ""),
				AppName: "my-app",
			},
		}

		result := buildRecentUpdates(changes, now)
		assert.Len(t, result, 1)
		assert.Equal(t, "web", result[0].Alias)
		assert.Equal(t, "1.21", result[0].NewVersion)
		assert.Equal(t, 1, result[0].ApplicationsUpdated)
		assert.Equal(t, now, result[0].UpdatedAt)
		assert.Contains(t, result[0].Message, "1.20")
		assert.Contains(t, result[0].Message, "1.21")
	})

	t.Run("same image updated in multiple apps aggregates", func(t *testing.T) {
		changes := []argocd.ChangeEntry{
			{
				Image: &image.ContainerImage{
					ImageName:  "nginx",
					ImageAlias: "web",
				},
				OldTag:  tag.NewImageTag("1.20", time.Now(), ""),
				NewTag:  tag.NewImageTag("1.21", time.Now(), ""),
				AppName: "app-1",
			},
			{
				Image: &image.ContainerImage{
					ImageName:  "nginx",
					ImageAlias: "web",
				},
				OldTag:  tag.NewImageTag("1.20", time.Now(), ""),
				NewTag:  tag.NewImageTag("1.21", time.Now(), ""),
				AppName: "app-2",
			},
		}

		result := buildRecentUpdates(changes, now)
		assert.Len(t, result, 1)
		assert.Equal(t, "web", result[0].Alias)
		assert.Equal(t, 2, result[0].ApplicationsUpdated)
	})

	t.Run("different images produce separate entries", func(t *testing.T) {
		changes := []argocd.ChangeEntry{
			{
				Image: &image.ContainerImage{
					ImageName:  "nginx",
					ImageAlias: "web",
				},
				OldTag:  tag.NewImageTag("1.20", time.Now(), ""),
				NewTag:  tag.NewImageTag("1.21", time.Now(), ""),
				AppName: "app-1",
			},
			{
				Image: &image.ContainerImage{
					ImageName:  "redis",
					ImageAlias: "cache",
				},
				OldTag:  tag.NewImageTag("6.0", time.Now(), ""),
				NewTag:  tag.NewImageTag("7.0", time.Now(), ""),
				AppName: "app-1",
			},
		}

		result := buildRecentUpdates(changes, now)
		assert.Len(t, result, 2)
		assert.Equal(t, "web", result[0].Alias)
		assert.Equal(t, "cache", result[1].Alias)
	})

	t.Run("empty alias falls back to image name", func(t *testing.T) {
		changes := []argocd.ChangeEntry{
			{
				Image: &image.ContainerImage{
					ImageName:  "nginx",
					ImageAlias: "",
				},
				OldTag:  tag.NewImageTag("1.20", time.Now(), ""),
				NewTag:  tag.NewImageTag("1.21", time.Now(), ""),
				AppName: "app-1",
			},
		}

		result := buildRecentUpdates(changes, now)
		assert.Len(t, result, 1)
		assert.Equal(t, "nginx", result[0].Alias)
	})
}

func TestSetCompletionConditions(t *testing.T) {
	t.Run("successful reconciliation with no errors", func(t *testing.T) {
		iu := &api.ImageUpdater{
			ObjectMeta: metav1.ObjectMeta{Generation: 3},
		}
		result := argocd.ImageUpdaterResult{
			ApplicationsMatched: 5,
			NumImagesUpdated:    2,
			NumErrors:           0,
		}

		setCompletionConditions(iu, result, nil)

		assert.Len(t, iu.Status.Conditions, 3)

		readyCond := findCondition(iu.Status.Conditions, ConditionTypeReady)
		assert.NotNil(t, readyCond)
		assert.Equal(t, metav1.ConditionTrue, readyCond.Status)
		assert.Equal(t, "ReconcileSucceeded", readyCond.Reason)
		assert.Equal(t, int64(3), readyCond.ObservedGeneration)

		reconcilingCond := findCondition(iu.Status.Conditions, ConditionTypeReconciling)
		assert.NotNil(t, reconcilingCond)
		assert.Equal(t, metav1.ConditionFalse, reconcilingCond.Status)
		assert.Equal(t, "Idle", reconcilingCond.Reason)

		errorCond := findCondition(iu.Status.Conditions, ConditionTypeError)
		assert.NotNil(t, errorCond)
		assert.Equal(t, metav1.ConditionFalse, errorCond.Status)
		assert.Equal(t, "NoErrors", errorCond.Reason)
	})

	t.Run("reconciliation with partial errors", func(t *testing.T) {
		iu := &api.ImageUpdater{
			ObjectMeta: metav1.ObjectMeta{Generation: 5},
		}
		result := argocd.ImageUpdaterResult{
			ApplicationsMatched: 5,
			NumImagesUpdated:    1,
			NumErrors:           2,
		}

		setCompletionConditions(iu, result, nil)

		readyCond := findCondition(iu.Status.Conditions, ConditionTypeReady)
		assert.NotNil(t, readyCond)
		assert.Equal(t, metav1.ConditionTrue, readyCond.Status)
		assert.Equal(t, "ReconcileCompletedWithErrors", readyCond.Reason)

		errorCond := findCondition(iu.Status.Conditions, ConditionTypeError)
		assert.NotNil(t, errorCond)
		assert.Equal(t, metav1.ConditionTrue, errorCond.Status)
		assert.Equal(t, "PartialErrors", errorCond.Reason)
		assert.Contains(t, errorCond.Message, "2 error(s)")
	})

	t.Run("reconciliation with fatal error", func(t *testing.T) {
		iu := &api.ImageUpdater{
			ObjectMeta: metav1.ObjectMeta{Generation: 7},
		}
		result := argocd.ImageUpdaterResult{}
		reconcileErr := fmt.Errorf("connection refused")

		setCompletionConditions(iu, result, reconcileErr)

		readyCond := findCondition(iu.Status.Conditions, ConditionTypeReady)
		assert.NotNil(t, readyCond)
		assert.Equal(t, metav1.ConditionFalse, readyCond.Status)
		assert.Equal(t, "ReconcileFailed", readyCond.Reason)
		assert.Contains(t, readyCond.Message, "connection refused")

		errorCond := findCondition(iu.Status.Conditions, ConditionTypeError)
		assert.NotNil(t, errorCond)
		assert.Equal(t, metav1.ConditionTrue, errorCond.Status)
		assert.Equal(t, "ReconcileError", errorCond.Reason)
	})
}

// findCondition is a test helper that finds a condition by type.
func findCondition(conditions []metav1.Condition, condType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == condType {
			return &conditions[i]
		}
	}
	return nil
}
