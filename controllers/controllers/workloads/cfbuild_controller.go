/*
Copyright 2021.

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

package workloads

import (
	"context"
	"errors"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/config"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// CFBuildReconciler reconciles a CFBuild object
type CFBuildReconciler struct {
	Client           CFClient
	Scheme           *runtime.Scheme
	Log              logr.Logger
	ControllerConfig *config.ControllerConfig
	EnvBuilder       EnvBuilder
}

func NewCFBuildReconciler(client CFClient, scheme *runtime.Scheme, log logr.Logger, controllerConfig *config.ControllerConfig, envBuilder EnvBuilder) *CFBuildReconciler {
	return &CFBuildReconciler{Client: client, Scheme: scheme, Log: log, ControllerConfig: controllerConfig, EnvBuilder: envBuilder}
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfbuilds,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfbuilds/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfbuilds/finalizers,verbs=update

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=buildworkloads,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=buildworkloads/status,verbs=get

func (r *CFBuildReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	cfBuild := new(korifiv1alpha1.CFBuild)
	err := r.Client.Get(ctx, req.NamespacedName, cfBuild)
	if err != nil {
		r.Log.Error(err, "Error when fetching CFBuild")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	cfApp := new(korifiv1alpha1.CFApp)
	err = r.Client.Get(ctx, types.NamespacedName{Name: cfBuild.Spec.AppRef.Name, Namespace: cfBuild.Namespace}, cfApp)
	if err != nil {
		r.Log.Error(err, "Error when fetching CFApp")
		return ctrl.Result{}, err
	}

	originalCFBuild := cfBuild.DeepCopy()
	err = controllerutil.SetOwnerReference(cfApp, cfBuild, r.Scheme)
	if err != nil {
		r.Log.Error(err, "unable to set owner reference on CFBuild")
		return ctrl.Result{}, err
	}

	err = r.Client.Patch(ctx, cfBuild, client.MergeFrom(originalCFBuild))
	if err != nil {
		r.Log.Error(err, fmt.Sprintf("Error setting owner reference on the CFBuild %s/%s", req.Namespace, cfBuild.Name))
		return ctrl.Result{}, err
	}

	cfPackage := new(korifiv1alpha1.CFPackage)
	err = r.Client.Get(ctx, types.NamespacedName{Name: cfBuild.Spec.PackageRef.Name, Namespace: cfBuild.Namespace}, cfPackage)
	if err != nil {
		r.Log.Error(err, "Error when fetching CFPackage")
		return ctrl.Result{}, err
	}

	stagingStatus := getConditionOrSetAsUnknown(&cfBuild.Status.Conditions, korifiv1alpha1.StagingConditionType)
	succeededStatus := getConditionOrSetAsUnknown(&cfBuild.Status.Conditions, korifiv1alpha1.SucceededConditionType)

	if succeededStatus != metav1.ConditionUnknown {
		return ctrl.Result{}, nil
	}

	if stagingStatus == metav1.ConditionUnknown {
		err = r.createBuildWorkloadAndUpdateStatus(ctx, cfBuild, cfApp, cfPackage)
		return ctrl.Result{}, err
	}

	var buildWorkload korifiv1alpha1.BuildWorkload
	err = r.Client.Get(ctx, types.NamespacedName{Name: cfBuild.Name, Namespace: cfBuild.Namespace}, &buildWorkload)
	if err != nil {
		r.Log.Error(err, "Error when fetching BuildWorkload")
		// Ignore NotFound errors to account for eventual consistency
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	workloadSucceededStatus := meta.FindStatusCondition(buildWorkload.Status.Conditions, korifiv1alpha1.SucceededConditionType)
	if workloadSucceededStatus == nil {
		return ctrl.Result{}, nil
	}

	switch workloadSucceededStatus.Status {
	case metav1.ConditionFalse:
		failureStatusConditionMessage := fmt.Sprintf("%s:%s", workloadSucceededStatus.Reason, workloadSucceededStatus.Message)
		setStatusConditionOnLocalCopy(&cfBuild.Status.Conditions, korifiv1alpha1.StagingConditionType, metav1.ConditionFalse, "BuildWorkload", "BuildWorkload")
		setStatusConditionOnLocalCopy(&cfBuild.Status.Conditions, korifiv1alpha1.SucceededConditionType, metav1.ConditionFalse, "BuildWorkload", failureStatusConditionMessage)
	case metav1.ConditionTrue:
		setStatusConditionOnLocalCopy(&cfBuild.Status.Conditions, korifiv1alpha1.StagingConditionType, metav1.ConditionFalse, "BuildWorkload", "BuildWorkload")
		setStatusConditionOnLocalCopy(&cfBuild.Status.Conditions, korifiv1alpha1.SucceededConditionType, metav1.ConditionTrue, "BuildWorkload", "BuildWorkload")
		cfBuild.Status.Droplet = buildWorkload.Status.Droplet
	default:
		return ctrl.Result{}, nil
	}

	if err = r.Client.Status().Update(ctx, cfBuild); err != nil {
		r.Log.Error(err, "Error when updating CFBuild status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *CFBuildReconciler) createBuildWorkloadAndUpdateStatus(ctx context.Context, cfBuild *korifiv1alpha1.CFBuild, cfApp *korifiv1alpha1.CFApp, cfPackage *korifiv1alpha1.CFPackage) error {
	workloadName := cfBuild.Name
	namespace := cfBuild.Namespace
	desiredWorkload := korifiv1alpha1.BuildWorkload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workloadName,
			Namespace: namespace,
			Labels: map[string]string{
				korifiv1alpha1.CFBuildGUIDLabelKey: cfBuild.Name,
				korifiv1alpha1.CFAppGUIDLabelKey:   cfApp.Name,
			},
		},
		Spec: korifiv1alpha1.BuildWorkloadSpec{
			Source: korifiv1alpha1.PackageSource{
				Registry: korifiv1alpha1.Registry{
					Image:            cfPackage.Spec.Source.Registry.Image,
					ImagePullSecrets: cfPackage.Spec.Source.Registry.ImagePullSecrets,
				},
			},
		},
	}

	buildServices, err := r.prepareBuildServices(ctx, namespace, cfApp.Name)
	if err != nil {
		return fmt.Errorf("prepareBuildServices: %w", err)
	}
	desiredWorkload.Spec.Services = buildServices

	imageEnvironment, err := r.prepareEnvironment(ctx, cfApp)
	if err != nil {
		return fmt.Errorf("prepareEnvironment: %w", err)
	}
	desiredWorkload.Spec.Env = imageEnvironment

	err = controllerutil.SetOwnerReference(cfBuild, &desiredWorkload, r.Scheme)
	if err != nil {
		r.Log.Error(err, "failed to set OwnerRef on BuildWorkload")
		return fmt.Errorf("failed to set OwnerRef on BuildWorkload: %w", err)
	}

	err = r.createBuildWorkloadIfNotExists(ctx, desiredWorkload)
	if err != nil {
		return fmt.Errorf("createBuildWorkloadIfNotExists: %w", err)
	}

	setStatusConditionOnLocalCopy(&cfBuild.Status.Conditions, korifiv1alpha1.StagingConditionType, metav1.ConditionTrue, "BuildWorkload", "BuildWorkload")
	if err := r.Client.Status().Update(ctx, cfBuild); err != nil {
		r.Log.Error(err, "Error when updating CFBuild status")
		return fmt.Errorf("failed updating CFBuild status: %w", err)
	}

	return nil
}

func (r *CFBuildReconciler) prepareBuildServices(ctx context.Context, namespace, appGUID string) ([]corev1.ObjectReference, error) {
	serviceBindingsList := &korifiv1alpha1.CFServiceBindingList{}
	err := r.Client.List(ctx, serviceBindingsList,
		client.InNamespace(namespace),
		client.MatchingFields{shared.IndexServiceBindingAppGUID: appGUID},
	)
	if err != nil {
		return nil, fmt.Errorf("error listing CFServiceBindings: %w", err)
	}

	var buildServices []corev1.ObjectReference
	for _, serviceBinding := range serviceBindingsList.Items {
		if serviceBinding.Status.Binding.Name != "" {
			objRef := corev1.ObjectReference{
				Kind:       "Secret",
				Name:       serviceBinding.Status.Binding.Name,
				APIVersion: "v1",
			}
			buildServices = append(buildServices, objRef)
		} else {
			r.Log.Info("binding secret name is empty")
			return nil, errors.New("binding secret name is empty")
		}
	}
	return buildServices, nil
}

func (r *CFBuildReconciler) prepareEnvironment(ctx context.Context, cfApp *korifiv1alpha1.CFApp) ([]corev1.EnvVar, error) {
	env, err := r.EnvBuilder.BuildEnv(ctx, cfApp)
	if err != nil {
		r.Log.Error(err, "failed building environment")
		return nil, err
	}

	var imageEnvironment []corev1.EnvVar
	for k, v := range env {
		imageEnvironment = append(imageEnvironment, corev1.EnvVar{
			Name:  k,
			Value: v,
		})
	}

	if cfApp.Status.VCAPServicesSecret.Name == "" {
		err := fmt.Errorf("failed to get VCAP service secret name")
		r.Log.Error(err, "failed to get VCAP service secret name")
		return nil, err
	}

	vcapServicesEnv := corev1.EnvVar{
		Name: "VCAP_SERVICES",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: cfApp.Status.VCAPServicesSecret.Name,
				},
				Key: "VCAP_SERVICES",
			},
		},
	}
	imageEnvironment = append(imageEnvironment, vcapServicesEnv)

	return imageEnvironment, nil
}

func (r *CFBuildReconciler) createBuildWorkloadIfNotExists(ctx context.Context, desiredWorkload korifiv1alpha1.BuildWorkload) error {
	var foundWorkload korifiv1alpha1.BuildWorkload
	err := r.Client.Get(ctx, types.NamespacedName{Name: desiredWorkload.Name, Namespace: desiredWorkload.Namespace}, &foundWorkload)
	if err != nil {
		if apierrors.IsNotFound(err) {
			err = r.Client.Create(ctx, &desiredWorkload)
			if err != nil {
				r.Log.Error(err, "Error when creating BuildWorkload")
				return err
			}
		} else {
			r.Log.Error(err, "Error when checking if BuildWorkload exists")
			return err
		}
	}
	return nil
}

// getConditionOrSetAsUnknown is a helper function that retrieves the value of the provided conditionType, like
// "Succeeded" and returns the value: "True", "False", or "Unknown". If the value is not present, the pointer to the
// list of conditions provided to the function is used to add an entry to the list of Conditions with a value of
// "Unknown" and "Unknown" is returned
func getConditionOrSetAsUnknown(conditions *[]metav1.Condition, conditionType string) metav1.ConditionStatus {
	if conditionStatus := meta.FindStatusCondition(*conditions, conditionType); conditionStatus != nil {
		return conditionStatus.Status
	}

	meta.SetStatusCondition(conditions, metav1.Condition{
		Type:    conditionType,
		Status:  metav1.ConditionUnknown,
		Reason:  "Unknown",
		Message: "Unknown",
	})
	return metav1.ConditionUnknown
}

// SetupWithManager sets up the controller with the Manager.
func (r *CFBuildReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.CFBuild{}).
		Watches(
			&source.Kind{Type: &korifiv1alpha1.BuildWorkload{}},
			handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
				var requests []reconcile.Request
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      obj.GetLabels()[korifiv1alpha1.CFBuildGUIDLabelKey],
						Namespace: obj.GetNamespace(),
					},
				})
				return requests
			})).
		Complete(r)
}
