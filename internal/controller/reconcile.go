package controller

import (
	"context"
	"fmt"
	"sync"

	"golang.org/x/sync/semaphore"

	iuapi "github.com/argoproj-labs/argocd-image-updater/api/v1alpha1"
	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd"
	"github.com/argoproj-labs/argocd-image-updater/pkg/common"
	"github.com/argoproj-labs/argocd-image-updater/pkg/metrics"
	iutypes "github.com/argoproj-labs/argocd-image-updater/pkg/types"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/registry"
)

// RunImageUpdater is a main loop for argocd-image-controller
func (r *ImageUpdaterReconciler) RunImageUpdater(ctx context.Context, cr *iuapi.ImageUpdater, warmUp bool) (argocd.ImageUpdaterResult, error) {
	baseLogger := log.LoggerFromContext(ctx)

	result := argocd.ImageUpdaterResult{}
	var err error
	var argoClient argocd.ArgoCD
	switch r.Config.ApplicationsAPIKind {
	case common.ApplicationsAPIKindK8S:
		argoClient, err = argocd.NewK8SClient(r.Client)
	case common.ApplicationsAPIKindArgoCD:
		argoClient, err = argocd.NewAPIClient(&r.Config.ClientOpts)
	default:
		return argocd.ImageUpdaterResult{}, fmt.Errorf("application api '%s' is not supported", r.Config.ApplicationsAPIKind)
	}
	if err != nil {
		return result, err
	}
	r.Config.ArgoClient = argoClient

	// Get the list of applications that are allowed for updates.
	appList, err := r.Config.ArgoClient.FilterApplicationsForUpdate(ctx, cr)
	if err != nil {
		return result, err
	}

	metrics.Applications().SetNumberOfApplications(len(appList))

	if !warmUp {
		baseLogger.Infof("Starting image update cycle, considering %d annotated application(s) for update", len(appList))
	}

	syncState := argocd.NewSyncIterationState()

	// Allow a maximum of MaxConcurrency number of goroutines to exist at the
	// same time. If in warm-up mode, set to 1 explicitly.
	var concurrency int = r.Config.MaxConcurrency
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

		go func(app string, curApplication iutypes.ApplicationImages) {
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
			if !warmUp && !r.Config.DryRun {
				metrics.Applications().IncreaseImageUpdate(app, res.NumImagesUpdated)
			}
			metrics.Applications().IncreaseUpdateErrors(app, res.NumErrors)
			metrics.Applications().SetNumberOfImagesWatched(app, res.NumImagesConsidered)
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
