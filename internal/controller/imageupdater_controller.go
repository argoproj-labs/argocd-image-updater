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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"time"

	argocdimageupdaterv1alpha1 "github.com/argoproj-labs/argocd-image-updater/api/v1alpha1"
)

// ImageUpdaterReconciler reconciles a ImageUpdater object
type ImageUpdaterReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Interval time.Duration
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
//    - Which images to monitor.
//    - The image repositories and tags/versions policies (e.g., semver constraints).
//    - The target application(s) or resources to update (this might involve interacting
//      with other Kubernetes resources or systems like Argo CD Applications, which would
//      require additional RBAC permissions).
// 3. Queries the relevant container image registries to find the latest available and
//    permissible versions of the monitored images.
// 4. Compares these latest versions against the currently deployed versions or versions
//    recorded in the ImageUpdater CR's status.
// 5. If an update is warranted and permitted by the update strategy:
//    - It may trigger an update to the target application(s) by modifying their
//      declarative specifications (e.g., updating an image field in a Deployment or an
//      Argo CD Application CR).
// 6. Updates the status subresource of the ImageUpdater CR to reflect:
//    - The latest versions checked.
//    - The last update attempt time and result.
//    - Any errors encountered during the process.
// 7. Handles errors gracefully and determines if and when the reconciliation request
//    should be requeued using ctrl.Result. For instance, network issues during registry
//    queries might lead to a requeue with a backoff.
//
// Currently, this Reconcile function logs the reconciliation event and then unconditionally
// requeues the request after a configured interval (r.Interval). This serves as a
// basic periodic check mechanism, which will be expanded with the detailed logic described above.

// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *ImageUpdaterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log = log.WithValues("imageupdater", req.NamespacedName) // Add context to logs
	log.Info("Reconciling ImageUpdater")

	// TODO: Implement the full reconciliation logic as described in the docstring:
	// 1. Fetch the ImageUpdater resource:
	var imageUpdater argocdimageupdaterv1alpha1.ImageUpdater
	if err := r.Get(ctx, req.NamespacedName, &imageUpdater); err != nil {
		if client.IgnoreNotFound(err) != nil {
			log.Error(err, "unable to fetch ImageUpdater")
			return ctrl.Result{}, err
		}
		log.Info("ImageUpdater resource not found. Ignoring since object must be deleted.")
		return ctrl.Result{}, nil
	}
	log.Info("Successfully fetched ImageUpdater resource", "resourceVersion", imageUpdater.ResourceVersion)

	// 2. Add finalizer logic if needed for cleanup before deletion.

	// 3. Implement image checking and application update logic based on imageUpdater.Spec.

	// 4. Update imageUpdater.Status with the results.
	//    if err := r.Status().Update(ctx, &imageUpdater); err != nil {
	//        log.Error(err, "unable to update ImageUpdater status")
	//        return ctrl.Result{}, err
	//    }

	// For now, just requeue periodically.
	// This interval might be a default, or could be overridden by logic
	// that inspects the imageUpdater CR itself for a custom interval.
	if r.Interval <= 0 {
		log.Info("Requeue interval is not configured or is zero; will not requeue based on time unless an error occurs or explicitly requested.")
		return ctrl.Result{}, nil
	}

	log.Info("Reconciliation logic placeholder: will requeue after interval", "interval", r.Interval.String())
	return ctrl.Result{RequeueAfter: r.Interval}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ImageUpdaterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&argocdimageupdaterv1alpha1.ImageUpdater{}).
		Complete(r)
}
