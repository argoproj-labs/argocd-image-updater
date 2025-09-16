package controller

import (
	"context"
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/semaphore"

	iuapi "github.com/argoproj-labs/argocd-image-updater/api/v1alpha1"
	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/image"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/registry"
)

// RunImageUpdater is a main loop for argocd-image-controller
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

	// TODO: metrics will be implemented in GITOPS-7113
	//metrics.Applications().SetNumberOfApplications(len(appList))

	if !warmUp {
		baseLogger.Infof("Starting image update cycle, considering %d application(s) for update", len(appList))
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
	wg.Add(len(appList))

	for app, curApplication := range appList {
		appLogger := baseLogger.WithField("application", app)

		lockErr := sem.Acquire(ctx, 1)
		if lockErr != nil {
			appLogger.Errorf("Could not acquire semaphore: %v", lockErr)
			// Release entry in wait group on error, too - we're never going to execute
			wg.Done()
			continue
		}

		go func(app string, curApplication argocd.ApplicationImages) {
			defer sem.Release(1)

			ctx = log.ContextWithLogger(ctx, appLogger)
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
			res := argocd.UpdateApplication(ctx, upconf, syncState)
			result.NumApplicationsProcessed += 1
			result.NumErrors += res.NumErrors
			result.NumImagesConsidered += res.NumImagesConsidered
			result.NumImagesUpdated += res.NumImagesUpdated
			result.NumSkipped += res.NumSkipped
			// TODO: metrics will be implemnted in GITOPS-7113
			//if !warmUp && !r.Config.DryRun {
			//	metrics.Applications().IncreaseImageUpdate(app, res.NumImagesUpdated)
			//}
			//metrics.Applications().IncreaseUpdateErrors(app, res.NumErrors)
			//metrics.Applications().SetNumberOfImagesWatched(app, res.NumImagesConsidered)
			wg.Done()
		}(app, curApplication)
	}

	// Wait for all goroutines to finish
	wg.Wait()

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

		go func(imageUpdater iuapi.ImageUpdater) {
			defer sem.Release(1)

			// Create logger for this CR - extract logger name from existing context
			crLogger := baseLogger.WithFields(logrus.Fields{
				"imageUpdater_namespace": imageUpdater.Namespace,
				"imageUpdater_name":      imageUpdater.Name,
			})
			crCtx := log.ContextWithLogger(ctx, crLogger)

			crLogger.Debugf("Processing CR")

			_, err := r.RunImageUpdater(crCtx, &imageUpdater, warmUp, webhookEvent)
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
			wg.Done()
		}(cr)
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
