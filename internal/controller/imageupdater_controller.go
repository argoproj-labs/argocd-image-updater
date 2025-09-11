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
	"sync"
	"text/template"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	api "github.com/argoproj-labs/argocd-image-updater/api/v1alpha1"
	"github.com/argoproj-labs/argocd-image-updater/ext/git"
	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd"
	"github.com/argoproj-labs/argocd-image-updater/pkg/common"
	"github.com/argoproj-labs/argocd-image-updater/pkg/kube"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
)

// ImageUpdaterConfig contains global configuration and required runtime data
type ImageUpdaterConfig struct {
	ArgocdNamespace        string
	DryRun                 bool
	CheckInterval          time.Duration
	ArgoClient             argocd.ArgoCD
	LogLevel               string
	KubeClient             *kube.ImageUpdaterKubernetesClient
	MaxConcurrentApps      int
	HealthPort             int
	MetricsPort            int
	RegistriesConf         string
	GitCommitUser          string
	GitCommitMail          string
	GitCommitMessage       *template.Template
	GitCommitSigningKey    string
	GitCommitSigningMethod string
	GitCommitSignOff       bool
	DisableKubeEvents      bool
	GitCreds               git.CredsStore
	EnableWebhook          bool
}

// ImageUpdaterReconciler reconciles a ImageUpdater object
type ImageUpdaterReconciler struct {
	client.Client
	Scheme                  *runtime.Scheme
	Config                  *ImageUpdaterConfig
	MaxConcurrentReconciles int
	CacheWarmed             <-chan struct{}
	// Channel to signal manager to stop
	StopChan chan struct{}
	// For run-once mode: wait for all CRs to complete
	Once bool
	Wg   sync.WaitGroup
}

const (
	// ResourcesFinalizerName is the name of the finalizer used by the ImageUpdater controller.
	ResourcesFinalizerName = "resources-finalizer.argocd-image-updater.argoproj.io"
)

// +kubebuilder:rbac:groups=argocd-image-updater.argoproj.io,resources=imageupdaters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=argocd-image-updater.argoproj.io,resources=imageupdaters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=argocd-image-updater.argoproj.io,resources=imageupdaters/finalizers,verbs=update

// Reconcile is the core operational loop of the ImageUpdater controller.
// It is invoked in response to events on ImageUpdater custom resources (CRs)
// (like create, update, delete) or due to periodic requeues. Its primary
// responsibility is to ensure that container images managed by ImageUpdater CRs
// are kept up-to-date according to the policies defined within each CR.
func (r *ImageUpdaterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := common.LogFields(logrus.Fields{
		"logger":                 "reconcile",
		"imageUpdater_namespace": req.NamespacedName.Namespace,
		"imageUpdater_name":      req.NamespacedName.Name,
	})
	ctx = log.ContextWithLogger(ctx, reqLogger)

	select {
	case <-r.CacheWarmed:
		// The warm-up is complete, proceed with reconciliation.
	default:
		// The warm-up is not yet complete.
		reqLogger.Debugf("Reconciliation for %s is waiting for cache warm-up to complete...", req.NamespacedName)
		// Requeue the request to try again after a short delay.
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	reqLogger.Debugf("Starting reconciliation for ImageUpdater resource.")

	// Check if client is available
	if r.Client == nil {
		reqLogger.Errorf("client is nil, cannot proceed with reconciliation")
		return ctrl.Result{}, fmt.Errorf("client is nil")
	}

	// 1. Fetch the ImageUpdater resource:
	var imageUpdater api.ImageUpdater
	if err := r.Get(ctx, req.NamespacedName, &imageUpdater); err != nil {
		if client.IgnoreNotFound(err) != nil {
			reqLogger.Errorf("unable to fetch ImageUpdater %v", err)
			return ctrl.Result{}, err
		}
		reqLogger.Infof("ImageUpdater resource not found. Ignoring since object must be deleted.")
		return ctrl.Result{}, nil
	}
	reqLogger.Infof("Successfully fetched ImageUpdater resource.")

	// 2. Handle finalizer logic for graceful deletion.
	isBeingDeleted := !imageUpdater.ObjectMeta.DeletionTimestamp.IsZero()
	hasFinalizer := controllerutil.ContainsFinalizer(&imageUpdater, ResourcesFinalizerName)

	if isBeingDeleted {
		if hasFinalizer {
			reqLogger.Debugf("ImageUpdater resource is being deleted, running finalizer.")
			// --- FINALIZER LOGIC ---
			// Currently, there is nothing to clean up.

			// Remove the finalizer from the list and update the object.
			reqLogger.Debugf("Finalizer logic complete, removing finalizer from the resource.")
			controllerutil.RemoveFinalizer(&imageUpdater, ResourcesFinalizerName)
			if err := r.Update(ctx, &imageUpdater); err != nil {
				reqLogger.Errorf("Failed to remove finalizer: %v", err)
				return ctrl.Result{}, err
			}
		}
		reqLogger.Debugf("Stop reconciliation as the ImageUpdater is being deleted.")
		return ctrl.Result{}, nil
	}

	// If the object is not being deleted, add the finalizer if it's not present.
	if !hasFinalizer {
		reqLogger.Debugf("Finalizer not found, adding it to the ImageUpdater resource.")
		controllerutil.AddFinalizer(&imageUpdater, ResourcesFinalizerName)
		if err := r.Update(ctx, &imageUpdater); err != nil {
			reqLogger.Errorf("Failed to add finalizer: %v", err)
			return ctrl.Result{}, err
		}
	}

	if r.Config.CheckInterval < 0 {
		reqLogger.Debugf("Requeue interval is not configured or below zero; will not requeue based on time unless an error occurs or explicitly requested.")
		return ctrl.Result{}, nil
	}

	if r.Config.CheckInterval == 0 {
		reqLogger.Debugf("Requeue interval is zero; will run once and stop.")
		_, err := r.RunImageUpdater(ctx, &imageUpdater, false, nil)
		if err != nil {
			reqLogger.Errorf("Error processing CR %s/%s: %v", imageUpdater.Namespace, imageUpdater.Name, err)
		} else {
			reqLogger.Infof("Finish Reconciliation for CR %s/%s", imageUpdater.Namespace, imageUpdater.Name)
		}

		// Signal that this CR is complete (regardless of success/failure)
		if r.Once {
			r.Wg.Done()
		}

		return ctrl.Result{}, nil
	}

	_, err := r.RunImageUpdater(ctx, &imageUpdater, false, nil)
	if err != nil {
		return ctrl.Result{}, err
	}

	reqLogger.Debugf("Reconciliation will requeue after interval %s", r.Config.CheckInterval.String())
	return ctrl.Result{RequeueAfter: r.Config.CheckInterval}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ImageUpdaterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&api.ImageUpdater{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: r.MaxConcurrentReconciles}).
		Complete(r)
}
