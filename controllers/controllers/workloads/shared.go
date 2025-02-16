package workloads

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	rbacv1 "k8s.io/api/rbac/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

//counterfeiter:generate -o fake -fake-name CFClient . CFClient
type CFClient interface {
	Get(ctx context.Context, key client.ObjectKey, obj client.Object) error
	Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error
	List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error
	Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error
	Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error
	Status() client.StatusWriter
}

// This is a helper function for updating local copy of status conditions
func setStatusConditionOnLocalCopy(conditions *[]metav1.Condition, conditionType string, conditionStatus metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(conditions, metav1.Condition{
		Type:    conditionType,
		Status:  conditionStatus,
		Reason:  reason,
		Message: message,
	})
}

func createOrPatchNamespace(ctx context.Context, client client.Client, log logr.Logger, orgOrSpace client.Object, labels map[string]string) error {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: orgOrSpace.GetName(),
		},
	}

	result, err := controllerutil.CreateOrPatch(ctx, client, namespace, func() error {
		if namespace.Labels == nil {
			namespace.Labels = make(map[string]string)
		}

		for key, value := range labels {
			namespace.Labels[key] = value
		}
		return nil
	})
	if err != nil {
		return err
	}

	log.Info(fmt.Sprintf("Namespace/%s %s", orgOrSpace.GetName(), result))
	return nil
}

func propagateSecrets(ctx context.Context, client client.Client, log logr.Logger, orgOrSpace client.Object, secretName string) error {
	secret := new(corev1.Secret)
	err := client.Get(ctx, types.NamespacedName{Namespace: orgOrSpace.GetNamespace(), Name: secretName}, secret)
	if err != nil {
		log.Error(err, fmt.Sprintf("Error fetching secret  %s/%s", orgOrSpace.GetNamespace(), secretName))
		return err
	}

	newSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secret.Name,
			Namespace: orgOrSpace.GetName(),
		},
	}

	result, err := controllerutil.CreateOrPatch(ctx, client, newSecret, func() error {
		newSecret.Annotations = secret.Annotations
		newSecret.Labels = secret.Labels
		newSecret.Immutable = secret.Immutable
		newSecret.Data = secret.Data
		newSecret.StringData = secret.StringData
		newSecret.Type = secret.Type
		return nil
	})
	if err != nil {
		log.Error(err, fmt.Sprintf("Error creating/patching secret %s/%s", newSecret.Namespace, newSecret.Name))
		return err
	}

	log.Info(fmt.Sprintf("Secret %s/%s %s", newSecret.Namespace, newSecret.Name, result))
	return nil
}

func propagateRoles(ctx context.Context, kClient client.Client, log logr.Logger, orgOrSpace client.Object) error {
	roles := new(rbacv1.RoleList)
	err := kClient.List(ctx, roles, client.InNamespace(orgOrSpace.GetNamespace()))
	if err != nil {
		log.Error(err, fmt.Sprintf("Error listing roles from namespace %s", orgOrSpace.GetNamespace()))
		return err
	}

	for _, binding := range roles.Items {
		newRole := &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:      binding.Name,
				Namespace: orgOrSpace.GetName(),
			},
		}
		b := binding
		result, err := controllerutil.CreateOrPatch(ctx, kClient, newRole, func() error {
			newRole.Labels = b.Labels
			newRole.Annotations = b.Annotations
			newRole.Rules = b.Rules
			return nil
		})
		if err != nil {
			log.Error(err, fmt.Sprintf("Error creating/patching role  %s/%s", newRole.Namespace, newRole.Name))
			return err
		}

		log.Info(fmt.Sprintf("Role %s/%s %s", newRole.Namespace, newRole.Name, result))
	}

	return nil
}

func propagateRoleBindings(ctx context.Context, kClient client.Client, log logr.Logger, orgOrSpace client.Object) error {
	roleBindings := new(rbacv1.RoleBindingList)
	err := kClient.List(ctx, roleBindings, client.InNamespace(orgOrSpace.GetNamespace()))
	if err != nil {
		log.Error(err, fmt.Sprintf("Error listing role-bindings from namespace %s", orgOrSpace.GetName()))
		return err
	}

	for _, binding := range roleBindings.Items {
		if binding.Annotations[korifiv1alpha1.PropagateRoleBindingAnnotation] == "false" {
			continue
		}
		newRoleBinding := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      binding.Name,
				Namespace: orgOrSpace.GetName(),
			},
		}
		b := binding
		result, err := controllerutil.CreateOrPatch(ctx, kClient, newRoleBinding, func() error {
			newRoleBinding.Labels = b.Labels
			newRoleBinding.Annotations = b.Annotations
			newRoleBinding.Subjects = b.Subjects
			newRoleBinding.RoleRef = b.RoleRef
			return nil
		})
		if err != nil {
			log.Error(err, fmt.Sprintf("Error creating/patching role-bindings %s/%s", newRoleBinding.Namespace, newRoleBinding.Name))
			return err
		}

		log.Info(fmt.Sprintf("Role Binding %s/%s %s", newRoleBinding.Namespace, newRoleBinding.Name, result))
	}

	return nil
}

func isFinalizing(orgOrSpace client.Object) bool {
	if orgOrSpace.GetDeletionTimestamp() != nil && !orgOrSpace.GetDeletionTimestamp().IsZero() {
		return true
	}
	return false
}

func finalize(ctx context.Context, kClient client.Client, log logr.Logger, orgOrSpace client.Object, finalizerName string) (ctrl.Result, error) {
	log.Info(fmt.Sprintf("Reconciling deletion of %s", orgOrSpace.GetName()))

	if !controllerutil.ContainsFinalizer(orgOrSpace, finalizerName) {
		return ctrl.Result{}, nil
	}

	err := kClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: orgOrSpace.GetName()}})
	if err != nil {
		log.Error(err, fmt.Sprintf("Failed to delete namespace %s/%s", orgOrSpace.GetNamespace(), orgOrSpace.GetName()))
		return ctrl.Result{}, err
	}

	originalCFObject := orgOrSpace.DeepCopyObject().(client.Object)
	controllerutil.RemoveFinalizer(orgOrSpace, finalizerName)

	if err = kClient.Patch(ctx, orgOrSpace, client.MergeFrom(originalCFObject)); err != nil {
		log.Error(err, fmt.Sprintf("Failed to remove finalizer on CFSpace %s/%s", orgOrSpace.GetNamespace(), orgOrSpace.GetName()))
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func updateStatus(ctx context.Context, client client.Client, orgOrSpace client.Object, conditionStatus metav1.ConditionStatus) error {
	switch obj := orgOrSpace.(type) {
	case *korifiv1alpha1.CFOrg:
		cfOrg := new(korifiv1alpha1.CFOrg)
		obj.DeepCopyInto(cfOrg)
		setStatusConditionOnLocalCopy(&cfOrg.Status.Conditions, StatusConditionReady, conditionStatus, StatusConditionReady, "")
		err := client.Status().Update(ctx, cfOrg)
		return err
	case *korifiv1alpha1.CFSpace:
		cfSpace := new(korifiv1alpha1.CFSpace)
		obj.DeepCopyInto(cfSpace)
		setStatusConditionOnLocalCopy(&cfSpace.Status.Conditions, StatusConditionReady, conditionStatus, StatusConditionReady, "")
		err := client.Status().Update(ctx, cfSpace)
		return err
	default:
		return fmt.Errorf("unknown object passed to updateStatus function")
	}
}

func getNamespace(ctx context.Context, client client.Client, namespaceName string) (*corev1.Namespace, bool) {
	namespace := new(corev1.Namespace)
	err := client.Get(ctx, types.NamespacedName{Name: namespaceName}, namespace)
	if err != nil {
		return nil, false
	}
	return namespace, true
}
