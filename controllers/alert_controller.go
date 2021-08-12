/*
Copyright 2020 The Flux authors

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

package controllers

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	helper "github.com/fluxcd/pkg/runtime/controller"
	"github.com/fluxcd/pkg/runtime/patch"
	"github.com/fluxcd/pkg/runtime/predicates"
	"k8s.io/apimachinery/pkg/util/errors"

	"github.com/fluxcd/notification-controller/api/v1beta1"
)

// +kubebuilder:rbac:groups=notification.toolkit.fluxcd.io,resources=alerts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=notification.toolkit.fluxcd.io,resources=alerts/status,verbs=get;update;patch

// AlertReconciler reconciles a Alert object
type AlertReconciler struct {
	client.Client
	helper.Metrics

	Scheme *runtime.Scheme
}

var ProviderIndexKey = ".metadata.provider"

func (r *AlertReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.TODO(), &v1beta1.Alert{}, ProviderIndexKey,
		func(o client.Object) []string {
			alert := o.(*v1beta1.Alert)
			return []string{
				// we include namespace here as it clarifies object retrieval later
				fmt.Sprintf("%s/%s", alert.GetNamespace(), alert.Spec.ProviderRef.Name),
			}
		},
	); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1beta1.Alert{}).
		WithEventFilter(predicate.Or(predicate.GenerationChangedPredicate{}, predicates.ReconcileRequestedPredicate{})).
		Watches(
			&source.Kind{Type: &v1beta1.Provider{}},
			handler.EnqueueRequestsFromMapFunc(r.requestsForProviderChange),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}),
		).
		Complete(r)
}

func (r *AlertReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, retErr error) {
	start := time.Now()
	log := ctrl.LoggerFrom(ctx)

	obj := &v1beta1.Alert{}
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// record suspension metrics
	r.RecordSuspend(ctx, obj, obj.Spec.Suspend)

	// return early if the object is suspended
	if obj.Spec.Suspend {
		log.Info("Reconciliation is suspended for this object")
		return ctrl.Result{}, nil
	}

	// Initialize the patch helper
	patchHelper, err := patch.NewHelper(obj, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	defer func() {
		// Patch the object, ignoring conflicts on the conditions owned by this controller
		patchOpts := []patch.Option{
			patch.WithOwnedConditions{
				Conditions: []string{
					meta.ReadyCondition,
					meta.ReconcilingCondition,
					meta.StalledCondition,
				},
			},
		}

		// Determine if the resource is still being reconciled, or if it has stalled, and record this observation
		if retErr == nil && (result.IsZero() || !result.Requeue) {
			// We are no longer reconciling
			conditions.Delete(obj, meta.ReconcilingCondition)

			// We have now observed this generation
			patchOpts = append(patchOpts, patch.WithStatusObservedGeneration{})

			readyCondition := conditions.Get(obj, meta.ReadyCondition)
			switch readyCondition.Status {
			case metav1.ConditionFalse:
				// As we are no longer reconciling and the end-state is not ready, the reconciliation has stalled
				conditions.MarkStalled(obj, readyCondition.Reason, readyCondition.Message)
			case metav1.ConditionTrue:
				// As we are no longer reconciling and the end-state is ready, the reconciliation is no longer stalled
				conditions.Delete(obj, meta.StalledCondition)
			}
		}

		// Finally, patch the resource
		if err := patchHelper.Patch(ctx, obj, patchOpts...); err != nil {
			retErr = errors.NewAggregate([]error{retErr, err})
		}

		// Always record readiness and duration metrics
		r.Metrics.RecordReadiness(ctx, obj)
		r.Metrics.RecordDuration(ctx, obj, start)
	}()

	return r.reconcile(ctx, obj)
}

func (r *AlertReconciler) reconcile(ctx context.Context, obj *v1beta1.Alert) (ctrl.Result, error) {
	// Mark the resource as under reconciliation
	conditions.MarkReconciling(obj, meta.ProgressingReason, "")

	// validate alert spec and provider
	if err := r.validate(ctx, obj); err != nil {
		conditions.MarkFalse(obj, meta.ReadyCondition, meta.FailedReason, err.Error())
		return ctrl.Result{}, err
	}

	conditions.MarkTrue(obj, meta.ReadyCondition, meta.SucceededReason, v1beta1.InitializedReason)
	ctrl.LoggerFrom(ctx).Info("Alert initialised")

	return ctrl.Result{}, nil
}

func (r *AlertReconciler) validate(ctx context.Context, alert *v1beta1.Alert) error {
	var provider v1beta1.Provider
	providerName := types.NamespacedName{Namespace: alert.Namespace, Name: alert.Spec.ProviderRef.Name}
	if err := r.Get(ctx, providerName, &provider); err != nil {
		return fmt.Errorf("failed to get provider %s, error: %w", providerName.String(), err)
	}

	if !conditions.IsReady(&provider) {
		return fmt.Errorf("provider %s is not ready", providerName.String())
	}

	return nil
}

func (r *AlertReconciler) requestsForProviderChange(o client.Object) []reconcile.Request {
	provider, ok := o.(*v1beta1.Provider)
	if !ok {
		panic(fmt.Sprintf("Expected a provider, got %T", o))
	}

	ctx := context.Background()
	var list v1beta1.AlertList
	if err := r.List(ctx, &list, client.MatchingFields{
		ProviderIndexKey: client.ObjectKeyFromObject(provider).String(),
	}); err != nil {
		return nil
	}

	var reqs []reconcile.Request
	for _, i := range list.Items {
		reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&i)})
	}
	return reqs
}
