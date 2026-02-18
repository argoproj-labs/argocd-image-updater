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
	"context"
	"fmt"
	"time"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/argoproj-labs/argocd-image-updater/api/v1alpha1"
	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd"
)

// Condition type constants
const (
	ConditionTypeReady       = "Ready"
	ConditionTypeReconciling = "Reconciling"
	ConditionTypeError       = "Error"
)

// setReconcilingStatus sets the Reconciling condition to True at the start of a reconcile loop.
func (r *ImageUpdaterReconciler) setReconcilingStatus(ctx context.Context, imageUpdater *api.ImageUpdater) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Re-fetch the latest version to avoid conflicts
		if err := r.Get(ctx, client.ObjectKeyFromObject(imageUpdater), imageUpdater); err != nil {
			return err
		}

		apimeta.SetStatusCondition(&imageUpdater.Status.Conditions, metav1.Condition{
			Type:               ConditionTypeReconciling,
			Status:             metav1.ConditionTrue,
			Reason:             "Reconciling",
			Message:            "Image update check in progress.",
			ObservedGeneration: imageUpdater.Generation,
		})

		return r.Status().Update(ctx, imageUpdater)
	})
}

// updateStatusAfterReconcile updates the status subresource with all results from a reconciliation cycle.
func (r *ImageUpdaterReconciler) updateStatusAfterReconcile(
	ctx context.Context,
	imageUpdater *api.ImageUpdater,
	result argocd.ImageUpdaterResult,
	reconcileErr error,
) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Re-fetch the latest version to avoid conflicts
		if err := r.Get(ctx, client.ObjectKeyFromObject(imageUpdater), imageUpdater); err != nil {
			return err
		}

		now := metav1.NewTime(time.Now())

		imageUpdater.Status.ObservedGeneration = imageUpdater.Generation
		imageUpdater.Status.LastCheckedAt = &now
		imageUpdater.Status.ApplicationsMatched = int32(result.ApplicationsMatched)
		imageUpdater.Status.ImagesManaged = int32(result.NumImagesConsidered)

		if result.NumImagesUpdated > 0 {
			imageUpdater.Status.LastUpdatedAt = &now
			imageUpdater.Status.RecentUpdates = buildRecentUpdates(result.Changes, now)
		}

		setCompletionConditions(imageUpdater, result, reconcileErr)

		return r.Status().Update(ctx, imageUpdater)
	})
}

// buildRecentUpdates converts the ChangeEntry list from a reconciliation into
// the RecentUpdate status slice, aggregating by image alias.
func buildRecentUpdates(changes []argocd.ChangeEntry, now metav1.Time) []api.RecentUpdate {
	if len(changes) == 0 {
		return nil
	}

	type aggregateKey struct {
		alias      string
		newVersion string
	}
	aggregated := make(map[aggregateKey]*api.RecentUpdate)
	var order []aggregateKey

	for _, c := range changes {
		alias := c.Image.ImageAlias
		if alias == "" {
			alias = c.Image.ImageName
		}

		k := aggregateKey{
			alias:      alias,
			newVersion: c.NewTag.String(),
		}

		if existing, ok := aggregated[k]; ok {
			existing.ApplicationsUpdated++
		} else {
			ru := &api.RecentUpdate{
				Alias:               alias,
				Image:               c.Image.GetFullNameWithoutTag(),
				NewVersion:          c.NewTag.String(),
				ApplicationsUpdated: 1,
				UpdatedAt:           now,
				Message:             fmt.Sprintf("Updated from %s to %s.", c.OldTag.String(), c.NewTag.String()),
			}
			aggregated[k] = ru
			order = append(order, k)
		}
	}

	result := make([]api.RecentUpdate, 0, len(order))
	for _, k := range order {
		result = append(result, *aggregated[k])
	}
	return result
}

// setCompletionConditions sets Ready, Reconciling, and Error conditions
// based on reconciliation results.
func setCompletionConditions(
	imageUpdater *api.ImageUpdater,
	result argocd.ImageUpdaterResult,
	reconcileErr error,
) {
	gen := imageUpdater.Generation

	// Reconciling -> False (done)
	apimeta.SetStatusCondition(&imageUpdater.Status.Conditions, metav1.Condition{
		Type:               ConditionTypeReconciling,
		Status:             metav1.ConditionFalse,
		Reason:             "Idle",
		Message:            "Last check completed. Awaiting next cycle.",
		ObservedGeneration: gen,
	})

	if reconcileErr != nil {
		apimeta.SetStatusCondition(&imageUpdater.Status.Conditions, metav1.Condition{
			Type:               ConditionTypeError,
			Status:             metav1.ConditionTrue,
			Reason:             "ReconcileError",
			Message:            reconcileErr.Error(),
			ObservedGeneration: gen,
		})
		apimeta.SetStatusCondition(&imageUpdater.Status.Conditions, metav1.Condition{
			Type:               ConditionTypeReady,
			Status:             metav1.ConditionFalse,
			Reason:             "ReconcileFailed",
			Message:            fmt.Sprintf("Reconciliation failed: %v", reconcileErr),
			ObservedGeneration: gen,
		})
	} else if result.NumErrors > 0 {
		apimeta.SetStatusCondition(&imageUpdater.Status.Conditions, metav1.Condition{
			Type:               ConditionTypeError,
			Status:             metav1.ConditionTrue,
			Reason:             "PartialErrors",
			Message:            fmt.Sprintf("%d error(s) occurred during image update checks.", result.NumErrors),
			ObservedGeneration: gen,
		})
		apimeta.SetStatusCondition(&imageUpdater.Status.Conditions, metav1.Condition{
			Type:               ConditionTypeReady,
			Status:             metav1.ConditionTrue,
			Reason:             "ReconcileCompletedWithErrors",
			Message:            fmt.Sprintf("Reconciled %d applications with %d errors, %d images updated.", result.ApplicationsMatched, result.NumErrors, result.NumImagesUpdated),
			ObservedGeneration: gen,
		})
	} else {
		apimeta.SetStatusCondition(&imageUpdater.Status.Conditions, metav1.Condition{
			Type:               ConditionTypeError,
			Status:             metav1.ConditionFalse,
			Reason:             "NoErrors",
			Message:            "No errors during last reconciliation.",
			ObservedGeneration: gen,
		})
		apimeta.SetStatusCondition(&imageUpdater.Status.Conditions, metav1.Condition{
			Type:               ConditionTypeReady,
			Status:             metav1.ConditionTrue,
			Reason:             "ReconcileSucceeded",
			Message:            fmt.Sprintf("Reconciled %d applications, %d images updated.", result.ApplicationsMatched, result.NumImagesUpdated),
			ObservedGeneration: gen,
		})
	}
}
