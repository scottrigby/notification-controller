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
	"crypto/sha256"
	"fmt"

	"k8s.io/client-go/tools/reference"

	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/metrics"

	"github.com/fluxcd/notification-controller/api/v1beta1"
)

// ReceiverReconciler reconciles a Receiver object
type ReceiverReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	MetricsRecorder *metrics.Recorder
}

// +kubebuilder:rbac:groups=notification.toolkit.fluxcd.io,resources=receivers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=notification.toolkit.fluxcd.io,resources=receivers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=source.fluxcd.io,resources=buckets,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=source.fluxcd.io,resources=buckets/status,verbs=get
// +kubebuilder:rbac:groups=source.fluxcd.io,resources=gitrepositories,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=source.fluxcd.io,resources=gitrepositories/status,verbs=get
// +kubebuilder:rbac:groups=source.fluxcd.io,resources=helmrepositories,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=source.fluxcd.io,resources=helmrepositories/status,verbs=get
// +kubebuilder:rbac:groups=image.fluxcd.io,resources=imagerepositories,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=image.fluxcd.io,resources=imagerepositories/status,verbs=get
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *ReceiverReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logr.FromContext(ctx)

	var obj v1beta1.Receiver
	if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// record suspension metrics
	defer r.recordSuspension(ctx, obj)

	token, err := r.token(ctx, obj)
	if err != nil {
		conditions.MarkFalse(&obj, meta.ReadyCondition, v1beta1.TokenNotFoundReason, err.Error())
		if err := r.patchStatus(ctx, req, obj.Status); err != nil {
			return ctrl.Result{Requeue: true}, err
		}
		return ctrl.Result{}, err
	}

	isReady := conditions.IsReady(&obj)
	receiverURL := fmt.Sprintf("/hook/%s", sha256sum(token+obj.Name+obj.Namespace))
	if obj.Status.URL == receiverURL && isReady && obj.Status.ObservedGeneration == obj.Generation {
		return ctrl.Result{}, nil
	}

	conditions.MarkTrue(&obj, meta.ReadyCondition, v1beta1.InitializedReason, "Receiver initialised with URL: "+receiverURL,
		receiverURL)
	obj.Status.ObservedGeneration = obj.Generation
	if err := r.patchStatus(ctx, req, obj.Status); err != nil {
		return ctrl.Result{Requeue: true}, err
	}

	log.Info("Receiver initialised")

	return ctrl.Result{}, nil
}

func (r *ReceiverReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1beta1.Receiver{}).
		Complete(r)
}

// token extract the token value from the secret object
func (r *ReceiverReconciler) token(ctx context.Context, receiver v1beta1.Receiver) (string, error) {
	token := ""
	secretName := types.NamespacedName{
		Namespace: receiver.GetNamespace(),
		Name:      receiver.Spec.SecretRef.Name,
	}

	var secret corev1.Secret
	err := r.Client.Get(ctx, secretName, &secret)
	if err != nil {
		return "", fmt.Errorf("unable to read token from secret '%s' error: %w", secretName, err)
	}

	if val, ok := secret.Data["token"]; ok {
		token = string(val)
	} else {
		return "", fmt.Errorf("invalid '%s' secret data: required fields 'token'", secretName)
	}

	return token, nil
}

func (r *ReceiverReconciler) recordSuspension(ctx context.Context, rcvr v1beta1.Receiver) {
	if r.MetricsRecorder == nil {
		return
	}
	log := logr.FromContext(ctx)

	objRef, err := reference.GetReference(r.Scheme, &rcvr)
	if err != nil {
		log.Error(err, "unable to record suspended metric")
		return
	}

	if !rcvr.DeletionTimestamp.IsZero() {
		r.MetricsRecorder.RecordSuspend(*objRef, false)
	} else {
		r.MetricsRecorder.RecordSuspend(*objRef, rcvr.Spec.Suspend)
	}
}

func sha256sum(val string) string {
	digest := sha256.Sum256([]byte(val))
	return fmt.Sprintf("%x", digest)
}

func (r *ReceiverReconciler) patchStatus(ctx context.Context, req ctrl.Request, newStatus v1beta1.ReceiverStatus) error {
	var receiver v1beta1.Receiver
	if err := r.Get(ctx, req.NamespacedName, &receiver); err != nil {
		return err
	}

	patch := client.MergeFrom(receiver.DeepCopy())
	receiver.Status = newStatus

	return r.Status().Patch(ctx, &receiver, patch)
}
