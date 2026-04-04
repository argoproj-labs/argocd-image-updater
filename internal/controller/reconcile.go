package controller

import (
	"context"
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/semaphore"

	iuapi "github.com/argoproj-labs/argocd-image-updater/api/v1alpha1"
	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd"
	"github.com/argoproj-labs/argocd-image-updater/pkg/metrics"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/image"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/registry"
)

// RunImageUpdater is a main loop for argocd-image-controller.
// When EnableBatchCommit is true, it uses a two-phase approach: first polling
// all registries in parallel, then batching git write-back operations per
// repository to minimize clone/fetch/push overhead.
// When EnableBatchCommit is false (the default), it uses the original per-app
// flow where each application polls its registry and commits individually.
func (r *ImageUpdaterReconciler) RunImageUpdater(ctx context.Context, cr *iuapi.ImageUpdater, warmUp bool, webhookEvent *argocd.WebhookEvent) (argocd.ImageUpdaterResult, error) {
	baseLogger := log.LoggerFromContext(ctx)

	result := argocd.ImageUpdaterResult{}

	argoClient, err := argocd.NewArgoCDK8sClient(r.Client)
	if err != nil {
		return result, err
	}
	r.Config.ArgoClient = argoClient

	// Get the list of applications that are allowed for updates.
	appList, err := argocd.FilterApplicationsForUpdate(ctx, argoClient, r.Config.KubeClient, cr, webhookEvent)
	if err != nil {
		return result, err
	}

	result.ApplicationsMatched = len(appList)

	if !warmUp {
		if r.Config != nil && r.Config.EnableCRMetrics && metrics.ImageUpdaterCR() != nil {
			metrics.ImageUpdaterCR().SetNumberOfApplications(cr.Name, cr.Namespace, result.ApplicationsMatched)
		}
		baseLogger.Infof("Starting image update cycle, considering %d application(s) for update", result.ApplicationsMatched)
	}

	syncState := argocd.NewSyncIterationState()

	// Allow a maximum of MaxConcurrentApps number of goroutines to exist at the
	// same time. If in warm-up mode, set to 1 explicitly.
	var concurrency int = r.Config.MaxConcurrentApps
	if warmUp {
		concurrency = 1
	}
	var dryRun bool = r.Config.DryRun
	if warmUp {
		dryRun = true
	}
	sem := semaphore.NewWeighted(int64(concurrency))

	var wg sync.WaitGroup
	var pendingMu sync.Mutex
	var pendingWrites []*argocd.PendingWrite
	var allChanges []argocd.ChangeEntry
	wg.Add(len(appList))

	for app, curApplication := range appList {
		appLogger := baseLogger.WithField("application", app)
		appCtx := log.ContextWithLogger(ctx, appLogger)

		lockErr := sem.Acquire(ctx, 1)
		if lockErr != nil {
			appLogger.Errorf("Could not acquire semaphore: %v", lockErr)
			wg.Done()
			continue
		}

		go func(appCtx context.Context, app string, curApplication argocd.ApplicationImages) {
			defer sem.Release(1)
			defer wg.Done()

			appLogger := log.LoggerFromContext(appCtx)
			appLogger.Debugf("Processing application")

			upconf := &argocd.UpdateConfiguration{
				NewRegFN:               registry.NewClient,
				ArgoClient:             r.Config.ArgoClient,
				KubeClient:             r.Config.KubeClient,
				UpdateApp:              &curApplication,
				DryRun:                 dryRun,
				GitCommitUser:          r.Config.GitCommitUser,
				GitCommitEmail:         r.Config.GitCommitMail,
				GitCommitMessage:       r.Config.GitCommitMessage,
				GitCommitSigningKey:    r.Config.GitCommitSigningKey,
				GitCommitSigningMethod: r.Config.GitCommitSigningMethod,
				GitCommitSignOff:       r.Config.GitCommitSignOff,
				DisableKubeEvents:      r.Config.DisableKubeEvents,
				GitCreds:               r.Config.GitCreds,
			}

			if r.Config.EnableBatchCommit {
				// Two-phase approach: poll only, collect pending git writes for Phase 2.
				// CheckApplicationImages returns pw==nil in two cases:
				//   (a) no updates needed
				//   (b) already committed immediately (non-git, PR mode, write-branch)
				// In case (b) we must emit success metrics and collect changes here.
				res, pw := argocd.CheckApplicationImages(appCtx, upconf, syncState)

				pendingMu.Lock()
				result.NumApplicationsProcessed += 1
				result.NumErrors += res.NumErrors
				result.NumImagesConsidered += res.NumImagesConsidered
				result.NumImagesUpdated += res.NumImagesUpdated
				result.NumSkipped += res.NumSkipped
				allChanges = append(allChanges, res.Changes...)
				if pw != nil {
					pendingWrites = append(pendingWrites, pw)
				}
				pendingMu.Unlock()

				if !warmUp && r.Config != nil && r.Config.EnableCRMetrics && metrics.ImageUpdaterCR() != nil {
					metrics.ImageUpdaterCR().IncreaseUpdateErrors(cr.Name, cr.Namespace, res.NumErrors)
					// For immediate commits (pw == nil), emit success metrics now.
					// For batched writes (pw != nil), defer to phase 2.
					if pw == nil && !r.Config.DryRun {
						metrics.ImageUpdaterCR().IncreaseImageUpdate(cr.Name, cr.Namespace, res.NumImagesUpdated)
					}
				}
			} else {
				// Original path: poll + commit per app individually.
				res := argocd.UpdateApplication(appCtx, upconf, syncState)

				pendingMu.Lock()
				result.NumApplicationsProcessed += 1
				result.NumErrors += res.NumErrors
				result.NumImagesConsidered += res.NumImagesConsidered
				result.NumImagesUpdated += res.NumImagesUpdated
				result.NumSkipped += res.NumSkipped
				allChanges = append(allChanges, res.Changes...)
				pendingMu.Unlock()

				if !warmUp && r.Config != nil && r.Config.EnableCRMetrics && metrics.ImageUpdaterCR() != nil {
					if !r.Config.DryRun {
						metrics.ImageUpdaterCR().IncreaseImageUpdate(cr.Name, cr.Namespace, res.NumImagesUpdated)
					}
					metrics.ImageUpdaterCR().IncreaseUpdateErrors(cr.Name, cr.Namespace, res.NumErrors)
				}
			}
		}(appCtx, app, curApplication)
	}

	// Wait for all goroutines to finish
	wg.Wait()

	result.Changes = allChanges

	// Set images-watched gauge once here with the CR-wide aggregate.
	if !warmUp && r.Config != nil && r.Config.EnableCRMetrics && metrics.ImageUpdaterCR() != nil {
		metrics.ImageUpdaterCR().SetNumberOfImagesWatched(cr.Name, cr.Namespace, result.NumImagesConsidered)
	}

	// Phase 2 (batch mode only): batch git write-back operations by repo+branch.
	if r.Config.EnableBatchCommit && len(pendingWrites) > 0 {
		baseLogger.Infof("Phase 2: batching %d pending git write(s)", len(pendingWrites))

		// Group pending writes by repo+branch
		batches := make(map[string][]*argocd.PendingWrite)
		for _, pw := range pendingWrites {
			key := pw.BatchKey()
			batches[key] = append(batches[key], pw)
		}

		for _, batch := range batches {
			baseLogger.Infof("Executing batch of %d app(s)", len(batch))
			batchErrors := argocd.BatchCommitChangesGit(ctx, batch, syncState)

			// Process results: emit kube events and metrics for successful writes,
			// count errors for failed ones.
			for _, pw := range batch {
				if batchErr, hasBatchErr := batchErrors[pw.AppName]; hasBatchErr {
					baseLogger.Errorf("Batch commit failed for app %s: %v", pw.AppName, batchErr)
					result.NumErrors += 1
					// Undo the image update count since the commit failed
					result.NumImagesUpdated -= pw.Result.NumImagesUpdated
				} else {
					baseLogger.Infof("Successfully updated application %s via batch commit", pw.AppName)
					argocd.EmitKubeEvents(ctx, pw.UpdateConf, pw.ChangeList, pw.AppName)
					// Emit image-update success metric now that the push succeeded.
					if !warmUp && r.Config != nil && r.Config.EnableCRMetrics && metrics.ImageUpdaterCR() != nil && !r.Config.DryRun {
						metrics.ImageUpdaterCR().IncreaseImageUpdate(cr.Name, cr.Namespace, pw.Result.NumImagesUpdated)
					}
				}
			}
		}
	}

	baseLogger.Infof("Processing results: applications=%d images_considered=%d images_skipped=%d images_updated=%d errors=%d",
		result.NumApplicationsProcessed,
		result.NumImagesConsidered,
		result.NumSkipped,
		result.NumImagesUpdated,
		result.NumErrors)

	return result, nil
}

// ProcessImageUpdaterCRs processes a list of ImageUpdater CRs with optional webhook event
func (r *ImageUpdaterReconciler) ProcessImageUpdaterCRs(ctx context.Context, crs []iuapi.ImageUpdater, warmUp bool, webhookEvent *argocd.WebhookEvent) error {
	baseLogger := log.LoggerFromContext(ctx)

	if len(crs) == 0 {
		baseLogger.Infof("No ImageUpdater CRs to process")
		return nil
	}

	baseLogger.Infof("Processing %d ImageUpdater CRs (warmUp: %v, webhook: %v)",
		len(crs), warmUp, webhookEvent != nil)

	// Use semaphore to limit concurrency
	sem := semaphore.NewWeighted(int64(r.MaxConcurrentReconciles))
	var wg sync.WaitGroup
	wg.Add(len(crs))

	var errors []error
	var mu sync.Mutex

	for _, cr := range crs {
		// Acquire semaphore
		if err := sem.Acquire(ctx, 1); err != nil {
			baseLogger.Errorf("Could not acquire semaphore: %v", err)
			wg.Done()
			continue
		}

		go func(ctx context.Context, imageUpdater iuapi.ImageUpdater) {
			defer sem.Release(1)
			defer wg.Done()

			// Create logger for this CR - extract logger name from existing context
			crLogger := baseLogger.WithFields(logrus.Fields{
				"imageUpdater_namespace": imageUpdater.Namespace,
				"imageUpdater_name":      imageUpdater.Name,
			})
			crCtx := log.ContextWithLogger(ctx, crLogger)

			crLogger.Debugf("Processing CR")

			if !warmUp {
				if statusErr := r.setReconcilingStatus(crCtx, &imageUpdater); statusErr != nil {
					crLogger.Warnf("Failed to set Reconciling status condition for %s/%s, status may be stale: %v", imageUpdater.Namespace, imageUpdater.Name, statusErr)
				}
			}

			result, err := r.RunImageUpdater(crCtx, &imageUpdater, warmUp, webhookEvent)

			if !warmUp {
				if statusErr := r.updateStatusAfterReconcile(crCtx, &imageUpdater, result, err); statusErr != nil {
					crLogger.Warnf("Failed to update status after reconcile for %s/%s, status may be stale: %v", imageUpdater.Namespace, imageUpdater.Name, statusErr)
				}
			}

			if err != nil {
				crLogger.Errorf("Failed to process ImageUpdater CR: %v", err)

				mu.Lock()
				errors = append(errors, fmt.Errorf("CR %s/%s: %w", imageUpdater.Namespace, imageUpdater.Name, err))
				mu.Unlock()
			} else {
				if warmUp {
					entries := 0
					eps := registry.ConfiguredEndpoints()
					for _, ep := range eps {
						r, err := registry.GetRegistryEndpoint(crCtx, &image.ContainerImage{RegistryURL: ep})
						if err == nil {
							entries += r.Cache.NumEntries()
						}
					}
					crLogger.Infof("Finished cache warm-up for CR. Pre-loaded %d meta data entries from %d registries", entries, len(eps))
				}
				crLogger.Infof("Successfully processed ImageUpdater CR")
			}
		}(ctx, cr)
	}

	// Wait for all goroutines to finish
	wg.Wait()

	// Return combined errors if any occurred
	if len(errors) > 0 {
		return fmt.Errorf("failed to process %d ImageUpdater CRs: %v", len(errors), errors)
	}

	baseLogger.Infof("Successfully processed all %d ImageUpdater CRs", len(crs))
	return nil
}
