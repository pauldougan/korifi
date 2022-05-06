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
	"fmt"
	"time"

	workloadsv1alpha1 "code.cloudfoundry.org/korifi/controllers/apis/workloads/v1alpha1"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	SpaceNameLabel = "cloudfoundry.org/space-name"
)

// CFSpaceReconciler reconciles a CFSpace object
type CFSpaceReconciler struct {
	client client.Client
	scheme *runtime.Scheme
	log    logr.Logger
}

func NewCFSpaceReconciler(client client.Client, scheme *runtime.Scheme, log logr.Logger) *CFSpaceReconciler {
	return &CFSpaceReconciler{
		client: client,
		scheme: scheme,
		log:    log,
	}
}

//+kubebuilder:rbac:groups=workloads.cloudfoundry.org,resources=cfspaces,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=workloads.cloudfoundry.org,resources=cfspaces/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=workloads.cloudfoundry.org,resources=cfspaces/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the CFSpace object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *CFSpaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	cfSpace := new(workloadsv1alpha1.CFSpace)
	err := r.client.Get(ctx, req.NamespacedName, cfSpace)
	if err != nil {
		r.log.Error(err, fmt.Sprintf("Error when trying to fetch CFSpace %s/%s", req.Namespace, req.Name))
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if cfSpace.ObjectMeta.DeletionTimestamp != nil && !cfSpace.ObjectMeta.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	err = r.createOrPatchNamespace(ctx, cfSpace)
	if err != nil {
		r.log.Error(err, fmt.Sprintf("Error when trying to create namespace %s", req.Name))
		return ctrl.Result{}, err
	}

	namespace, ok := getNamespace(ctx, r.client, cfSpace.Name)
	if !ok {
		return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
	}

	err = r.duplicateRoles(ctx, cfSpace)
	if err != nil {
		r.log.Error(err, fmt.Sprintf("Error when trying to duplicate roles into namespace %s", req.Name))
		return ctrl.Result{}, err
	}

	err = r.duplicateRoleBindings(ctx, cfSpace)
	if err != nil {
		r.log.Error(err, fmt.Sprintf("Error when trying to duplicate rolebindings into namespace %s", req.Name))
		return ctrl.Result{}, err
	}

	err = r.duplicateSecrets(ctx, cfSpace)
	if err != nil {
		r.log.Error(err, fmt.Sprintf("Error when trying to duplicate secrets into namespace %s", req.Name))
		return ctrl.Result{}, err
	}

	cfSpace.Status.GUID = namespace.Name
	err = updateStatus(ctx, r.client, cfSpace, metav1.ConditionTrue)
	if err != nil {
		r.log.Error(err, "unable to update CFSpace status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *CFSpaceReconciler) createOrPatchNamespace(ctx context.Context, cfSpace *workloadsv1alpha1.CFSpace) error {
	ns := new(v1.Namespace)
	err := r.client.Get(ctx, types.NamespacedName{Name: cfSpace.Name}, ns)

	if err != nil {
		if k8serrors.IsNotFound(err) {
			ns = &v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: cfSpace.Name,
				},
			}
		} else {
			return err
		}
	}

	result, err := controllerutil.CreateOrPatch(ctx, r.client, ns, func() error {
		if ns.ObjectMeta.Labels == nil {
			ns.ObjectMeta.Labels = make(map[string]string)
		}

		ns.ObjectMeta.Labels["cloudfoundry.org/space-name"] = cfSpace.Spec.DisplayName

		// TODO: Need to use a finalizer to handle deletion of the namespace
		// err = controllerutil.SetOwnerReference(cfSpace, ns, r.scheme)
		// if err != nil {
		// 	r.log.Error(err, "failed to set OwnerRef on Namespace")
		// 	return err
		// }

		return nil
	})
	if err != nil {
		r.log.Error(err, "failed to create/patch ns")
		return err
	}

	r.log.Info(fmt.Sprintf("Namespace/%s %s", cfSpace.Name, result))
	return nil
}

func (r *CFSpaceReconciler) duplicateRoles(ctx context.Context, cfSpace *workloadsv1alpha1.CFSpace) error {
	var roles rbacv1.RoleList
	listOptions := client.ListOptions{Namespace: cfSpace.Namespace}
	err := r.client.List(ctx, &roles, &listOptions)
	if err != nil {
		r.log.Error(err, "failed to list roles")
		return err
	}

	for _, role := range roles.Items {
		err = r.createOrPatchRole(ctx, cfSpace, role)
		if err != nil {
			r.log.Error(err, "failed to duplicate role")
			return err
		}
	}

	return nil
}

func (r *CFSpaceReconciler) createOrPatchRole(ctx context.Context, cfSpace *workloadsv1alpha1.CFSpace, role rbacv1.Role) error {
	newRole := new(rbacv1.Role)
	err := r.client.Get(ctx, types.NamespacedName{Name: role.Name, Namespace: cfSpace.Name}, newRole)

	if err != nil {
		if k8serrors.IsNotFound(err) {
			newRole = &rbacv1.Role{
				ObjectMeta: metav1.ObjectMeta{
					Name:      role.Name,
					Namespace: cfSpace.Name,
				},
			}
		} else {
			return err
		}
	}

	result, err := controllerutil.CreateOrPatch(ctx, r.client, newRole, func() error {
		newRole.Rules = role.Rules

		return nil
	})
	if err != nil {
		r.log.Error(err, "failed to create/patch role")
		return err
	}

	r.log.Info(fmt.Sprintf("Role/%s %s", role.Name, result))
	return nil
}

func (r *CFSpaceReconciler) duplicateRoleBindings(ctx context.Context, cfSpace *workloadsv1alpha1.CFSpace) error {
	var rolebindings rbacv1.RoleBindingList
	labelSelector, err := labels.Parse("cloudfoundry.org/propagate-cf-role notin (false)")
	if err != nil {
		r.log.Error(err, "failed to generate label selector to exclude cf roles")
		return err
	}

	listOptions := client.ListOptions{
		LabelSelector: labelSelector,
		Namespace:     cfSpace.Namespace,
	}
	err = r.client.List(ctx, &rolebindings, &listOptions)
	if err != nil {
		r.log.Error(err, "failed to list roles")
		return err
	}

	for _, rolebinding := range rolebindings.Items {
		err = r.createOrPatchRoleBinding(ctx, cfSpace, rolebinding)
		if err != nil {
			r.log.Error(err, "failed to duplicate rolebinding")
			return err
		}
	}

	return nil
}

func (r *CFSpaceReconciler) createOrPatchRoleBinding(ctx context.Context, cfSpace *workloadsv1alpha1.CFSpace, rolebinding rbacv1.RoleBinding) error {
	newRoleBinding := new(rbacv1.RoleBinding)
	err := r.client.Get(ctx, types.NamespacedName{Name: rolebinding.Name, Namespace: cfSpace.Name}, newRoleBinding)

	if err != nil {
		if k8serrors.IsNotFound(err) {
			newRoleBinding = &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      rolebinding.Name,
					Namespace: cfSpace.Name,
				},
			}
		} else {
			return err
		}
	}

	result, err := controllerutil.CreateOrPatch(ctx, r.client, newRoleBinding, func() error {
		newRoleBinding.Subjects = rolebinding.Subjects
		newRoleBinding.RoleRef = rolebinding.RoleRef

		return nil
	})
	if err != nil {
		r.log.Error(err, "failed to create/patch role binding")
		return err
	}

	r.log.Info(fmt.Sprintf("RoleBinding/%s %s", rolebinding.Name, result))
	return nil
}

func (r *CFSpaceReconciler) duplicateSecrets(ctx context.Context, cfSpace *workloadsv1alpha1.CFSpace) error {
	var secret v1.Secret

	err := r.client.Get(ctx, types.NamespacedName{Name: "image-registry-credentials", Namespace: cfSpace.Namespace}, &secret)
	if err != nil {
		r.log.Error(err, "failed to get secret in parent namespace")
		return err
	}

	err = r.createOrPatchSecret(ctx, cfSpace, secret)
	if err != nil {
		r.log.Error(err, "failed to duplicate secret")
		return err
	}

	return nil
}

func (r *CFSpaceReconciler) createOrPatchSecret(ctx context.Context, cfSpace *workloadsv1alpha1.CFSpace, secret v1.Secret) error {
	newSecret := new(v1.Secret)
	err := r.client.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: cfSpace.Name}, newSecret)

	if err != nil {
		if k8serrors.IsNotFound(err) {
			newSecret = &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secret.Name,
					Namespace: cfSpace.Name,
				},
			}
		} else {
			return err
		}
	}

	result, err := controllerutil.CreateOrPatch(ctx, r.client, newSecret, func() error {
		newSecret.Immutable = secret.Immutable
		newSecret.Data = secret.Data
		newSecret.StringData = secret.StringData
		newSecret.Type = secret.Type

		return nil
	})
	if err != nil {
		r.log.Error(err, "failed to create/patch secret")
		return err
	}

	r.log.Info(fmt.Sprintf("Secret/%s %s", secret.Name, result))
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CFSpaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&workloadsv1alpha1.CFSpace{}).
		Complete(r)
}
