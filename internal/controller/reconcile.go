package controller

import (
	"context"
	"fmt"
	"sync"

	"golang.org/x/sync/semaphore"

	api "github.com/argoproj-labs/argocd-image-updater/api/v1alpha1"
	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd"
	"github.com/argoproj-labs/argocd-image-updater/pkg/common"
	"github.com/argoproj-labs/argocd-image-updater/pkg/metrics"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/registry"
)

// RunImageUpdater is a main loop for argocd-image-controller
func (r *ImageUpdaterReconciler) RunImageUpdater(ctx context.Context, cr *api.ImageUpdater, warmUp bool) (argocd.ImageUpdaterResult, error) {
	log := log.LoggerFromContext(ctx)

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

	apps, err := r.Config.ArgoClient.ListApplications(ctx, cr, r.Config.AppLabel)
	if err != nil {
		log.Errorf("error while communicating with ArgoCD: %v", err)
		return result, err
	}

	// Get the list of applications that are allowed for updates, that is, those
	// applications which have correct annotation.
	appList, err := argocd.FilterApplicationsForUpdate(apps, r.Config.AppNamePatterns)
	if err != nil {
		return result, err
	}

	metrics.Applications().SetNumberOfApplications(len(appList))

	if !warmUp {
		log.Infof("Starting image update cycle, considering %d annotated application(s) for update", len(appList))
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
		lockErr := sem.Acquire(context.TODO(), 1)
		if lockErr != nil {
			log.Errorf("Could not acquire semaphore for application %s: %v", app, lockErr)
			// Release entry in wait group on error, too - we're never going to execute
			wg.Done()
			continue
		}

		go func(app string, curApplication argocd.ApplicationImages) {
			defer sem.Release(1)
			log.Debugf("Processing application %s", app)
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
			res := argocd.UpdateApplication(upconf, syncState)
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

	log.Infof("Processing results: applications=%d images_considered=%d images_skipped=%d images_updated=%d errors=%d",
		result.NumApplicationsProcessed,
		result.NumImagesConsidered,
		result.NumSkipped,
		result.NumImagesUpdated,
		result.NumErrors)

	return result, nil
}
