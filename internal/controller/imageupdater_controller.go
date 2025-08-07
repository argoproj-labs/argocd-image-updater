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
	"text/template"
	"time"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/argoproj-labs/argocd-image-updater/ext/git"
	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd"
	"github.com/argoproj-labs/argocd-image-updater/pkg/kube"

	"github.com/sirupsen/logrus"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	api "github.com/argoproj-labs/argocd-image-updater/api/v1alpha1"
	"github.com/argoproj-labs/argocd-image-updater/pkg/common"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
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
	WebhookPort            int
	EnableWebhook          bool
}

// ImageUpdaterReconciler reconciles a ImageUpdater object
type ImageUpdaterReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	Config      *ImageUpdaterConfig
	CacheWarmed <-chan struct{}
}

// +kubebuilder:rbac:groups=argocd-image-updater.argoproj.io,resources=imageupdaters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=argocd-image-updater.argoproj.io,resources=imageupdaters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=argocd-image-updater.argoproj.io,resources=imageupdaters/finalizers,verbs=update

// Reconcile is the core operational loop of the ImageUpdater controller.
// It is invoked in response to events on ImageUpdater custom resources (CRs)
// (like create, update, delete) or due to periodic requeues. Its primary
// responsibility is to ensure that container images managed by ImageUpdater CRs
// are kept up-to-date according to the policies defined within each CR.
//
// The Reconcile function performs the following key steps:
// 1. Fetches the ImageUpdater CR instance identified by the request.
// 2. Inspects the CR's specification to determine:
//   - Which images to monitor.
//   - The image repositories and tags/versions policies (e.g., semver constraints).
//   - The target application(s) or resources to update (this might involve interacting
//     with other Kubernetes resources or systems like Argo CD Applications, which would
//     require additional RBAC permissions).
//     3. Queries the relevant container image registries to find the latest available and
//     permissible versions of the monitored images.
//     4. Compares these latest versions against the currently deployed versions or versions
//     recorded in the ImageUpdater CR's status.
//     5. If an update is warranted and permitted by the update strategy:
//   - It may trigger an update to the target application(s) by modifying their
//     declarative specifications (e.g., updating an image field in a Deployment or an
//     Argo CD Application CR).
//     6. Updates the status subresource of the ImageUpdater CR to reflect:
//   - The latest versions checked.
//   - The last update attempt time and result.
//   - Any errors encountered during the process.
//     7. Handles errors gracefully and determines if and when the reconciliation request
//     should be requeued using ctrl.Result. For instance, network issues during registry
//     queries might lead to a requeue with a backoff.
//
// Currently, this Reconcile function logs the reconciliation event and then unconditionally
// requeues the request after a configured interval (r.Interval). This serves as a
// basic periodic check mechanism, which will be expanded with the detailed logic described above.
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
	reqLogger.Infof("Successfully fetched ImageUpdater resource: %s, namespace: %s", imageUpdater.Name, imageUpdater.Namespace)

	// 2. Add finalizer logic if needed for cleanup before deletion.

	if r.Config.CheckInterval < 0 {
		reqLogger.Debugf("Requeue interval is not configured or below zero; will not requeue based on time unless an error occurs or explicitly requested.")
		return ctrl.Result{}, nil
	}

	if r.Config.CheckInterval == 0 {
		reqLogger.Debugf("Requeue interval is zero; will be requeued once.")
		_, err := r.RunImageUpdater(ctx, &imageUpdater, false)
		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	_, err := r.RunImageUpdater(ctx, &imageUpdater, false)
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
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(r)
}
