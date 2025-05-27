package controller

import (
	"context"
	"fmt"
	"github.com/argoproj-labs/argocd-image-updater/ext/git"
	"github.com/argoproj-labs/argocd-image-updater/pkg/kube"
	"sync"
	"text/template"
	"time"

	api "github.com/argoproj-labs/argocd-image-updater/api/v1alpha1"
	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd"
	"github.com/argoproj-labs/argocd-image-updater/pkg/common"
	"github.com/argoproj-labs/argocd-image-updater/pkg/health"
	"github.com/argoproj-labs/argocd-image-updater/pkg/metrics"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/registry"
	"github.com/argoproj/argo-cd/v2/reposerver/askpass"

	"golang.org/x/sync/semaphore"
)

// ImageUpdaterConfig contains global configuration and required runtime data
type ImageUpdaterConfig struct {
	ApplicationsAPIKind    string
	ClientOpts             argocd.ClientOptions
	ArgocdNamespace        string
	AppNamespace           string
	DryRun                 bool
	CheckInterval          time.Duration
	ArgoClient             argocd.ArgoCD
	LogLevel               string
	KubeClient             *kube.ImageUpdaterKubernetesClient
	MaxConcurrency         int
	HealthPort             int
	MetricsPort            int
	RegistriesConf         string
	AppNamePatterns        []string
	AppLabel               string
	GitCommitUser          string
	GitCommitMail          string
	GitCommitMessage       *template.Template
	GitCommitSigningKey    string
	GitCommitSigningMethod string
	GitCommitSignOff       bool
	DisableKubeEvents      bool
	GitCreds               git.CredsStore
	WarmUpCache            bool
}

var lastRun time.Time

// newControllerCommand implements "run" command
func newControllerCommand(cr api.ImageUpdater) error {
	var cfg *ImageUpdaterConfig = &ImageUpdaterConfig{}

	// Health server will start in a go routine and run asynchronously
	var hsErrCh chan error
	var msErrCh chan error
	if cfg.HealthPort > 0 {
		log.Infof("Starting health probe server TCP port=%d", cfg.HealthPort)
		hsErrCh = health.StartHealthServer(cfg.HealthPort)
	}

	if cfg.MetricsPort > 0 {
		log.Infof("Starting metrics server on TCP port=%d", cfg.MetricsPort)
		msErrCh = metrics.StartMetricsServer(cfg.MetricsPort)
	}

	if cfg.WarmUpCache {
		err := warmupImageCache(cfg, cr)
		if err != nil {
			log.Errorf("Error warming up cache: %v", err)
			return err
		}
	}

	// Start up the credentials store server
	cs := askpass.NewServer(askpass.SocketPath)
	csErrCh := make(chan error)
	go func() {
		log.Debugf("Starting askpass server")
		csErrCh <- cs.Run()
	}()

	var err error
	// Wait for cred server to be started, just in case
	err = <-csErrCh
	if err != nil {
		log.Errorf("Error running askpass server: %v", err)
		return err
	}

	cfg.GitCreds = cs

	// This is our main loop. We leave it only when our health probe server
	// returns an error.
	for {
		select {
		case err := <-hsErrCh:
			if err != nil {
				log.Errorf("Health probe server exited with error: %v", err)
			} else {
				log.Infof("Health probe server exited gracefully")
			}
			return nil
		case err := <-msErrCh:
			if err != nil {
				log.Errorf("Metrics server exited with error: %v", err)
			} else {
				log.Infof("Metrics server exited gracefully")
			}
			return nil
		default:
			if lastRun.IsZero() || time.Since(lastRun) > cfg.CheckInterval {
				result, err := RunImageUpdater(cfg, cr, false)
				if err != nil {
					log.Errorf("Error: %v", err)
				} else {
					log.Infof("Processing results: applications=%d images_considered=%d images_skipped=%d images_updated=%d errors=%d",
						result.NumApplicationsProcessed,
						result.NumImagesConsidered,
						result.NumSkipped,
						result.NumImagesUpdated,
						result.NumErrors)
				}
				lastRun = time.Now()
			}
		}
		if cfg.CheckInterval == 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	log.Infof("Finished.")
	return nil

}

// Main loop for argocd-image-controller
func RunImageUpdater(cfg *ImageUpdaterConfig, cr api.ImageUpdater, warmUp bool) (argocd.ImageUpdaterResult, error) {
	result := argocd.ImageUpdaterResult{}
	var err error
	var argoClient argocd.ArgoCD
	switch cfg.ApplicationsAPIKind {
	case common.ApplicationsAPIKindK8S:
		argoClient, err = argocd.NewK8SClient(cfg.KubeClient, &argocd.K8SClientOptions{AppNamespace: cr.Spec.Namespace})
	case common.ApplicationsAPIKindArgoCD:
		argoClient, err = argocd.NewAPIClient(&cfg.ClientOpts)
	default:
		return argocd.ImageUpdaterResult{}, fmt.Errorf("application api '%s' is not supported", cfg.ApplicationsAPIKind)
	}
	if err != nil {
		return result, err
	}
	cfg.ArgoClient = argoClient

	apps, err := cfg.ArgoClient.ListApplications(cfg.AppLabel)
	if err != nil {
		log.WithContext().
			AddField("argocd_server", cfg.ClientOpts.ServerAddr).
			AddField("grpc_web", cfg.ClientOpts.GRPCWeb).
			AddField("grpc_webroot", cfg.ClientOpts.GRPCWebRootPath).
			AddField("plaintext", cfg.ClientOpts.Plaintext).
			AddField("insecure", cfg.ClientOpts.Insecure).
			Errorf("error while communicating with ArgoCD")
		return result, err
	}

	// Get the list of applications that are allowed for updates, that is, those
	// applications which have correct annotation.
	appList, err := argocd.FilterApplicationsForUpdate(apps, cfg.AppNamePatterns)
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
	var concurrency int = cfg.MaxConcurrency
	if warmUp {
		concurrency = 1
	}
	var dryRun bool = cfg.DryRun
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
			// Release entry in wait group on error, too - we're never gonna execute
			wg.Done()
			continue
		}

		go func(app string, curApplication argocd.ApplicationImages) {
			defer sem.Release(1)
			log.Debugf("Processing application %s", app)
			upconf := &argocd.UpdateConfiguration{
				NewRegFN:               registry.NewClient,
				ArgoClient:             cfg.ArgoClient,
				KubeClient:             cfg.KubeClient,
				UpdateApp:              &curApplication,
				DryRun:                 dryRun,
				GitCommitUser:          cfg.GitCommitUser,
				GitCommitEmail:         cfg.GitCommitMail,
				GitCommitMessage:       cfg.GitCommitMessage,
				GitCommitSigningKey:    cfg.GitCommitSigningKey,
				GitCommitSigningMethod: cfg.GitCommitSigningMethod,
				GitCommitSignOff:       cfg.GitCommitSignOff,
				DisableKubeEvents:      cfg.DisableKubeEvents,
				GitCreds:               cfg.GitCreds,
			}
			res := argocd.UpdateApplication(upconf, syncState)
			result.NumApplicationsProcessed += 1
			result.NumErrors += res.NumErrors
			result.NumImagesConsidered += res.NumImagesConsidered
			result.NumImagesUpdated += res.NumImagesUpdated
			result.NumSkipped += res.NumSkipped
			if !warmUp && !cfg.DryRun {
				metrics.Applications().IncreaseImageUpdate(app, res.NumImagesUpdated)
			}
			metrics.Applications().IncreaseUpdateErrors(app, res.NumErrors)
			metrics.Applications().SetNumberOfImagesWatched(app, res.NumImagesConsidered)
			wg.Done()
		}(app, curApplication)
	}

	// Wait for all goroutines to finish
	wg.Wait()

	return result, nil
}

// warmupImageCache performs a cache warm-up, which is basically one cycle of
// the image update process with dryRun set to true and a maximum concurrency
// of 1, i.e. sequential processing.
func warmupImageCache(cfg *ImageUpdaterConfig, cr api.ImageUpdater) error {
	log.Infof("Warming up image cache")
	_, err := RunImageUpdater(cfg, cr, true)
	if err != nil {
		return nil
	}
	entries := 0
	eps := registry.ConfiguredEndpoints()
	for _, ep := range eps {
		r, err := registry.GetRegistryEndpoint(ep)
		if err == nil {
			entries += r.Cache.NumEntries()
		}
	}
	log.Infof("Finished cache warm-up, pre-loaded %d meta data entries from %d registries", entries, len(eps))
	return nil
}
