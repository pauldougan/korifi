package integration_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
)

var _ = Describe("CFSpaceReconciler Integration Tests", func() {
	const (
		spaceName = "my-space"
	)

	var (
		ctx                 context.Context
		orgNamespace        *corev1.Namespace
		spaceGUID           string
		cfSpace             *korifiv1alpha1.CFSpace
		imageRegistrySecret *corev1.Secret
		role1               *rbacv1.Role
		role2               *rbacv1.Role
		rules1              []rbacv1.PolicyRule
		rules2              []rbacv1.PolicyRule
		username            string
		roleBinding         rbacv1.RoleBinding
		roleBinding2        rbacv1.RoleBinding
	)

	BeforeEach(func() {
		ctx = context.Background()
		orgNamespace = createNamespaceWithCleanup(ctx, k8sClient, PrefixedGUID("cf-org"))
		imageRegistrySecret = createSecret(ctx, k8sClient, packageRegistrySecretName, orgNamespace.Name)
		rules1 = []rbacv1.PolicyRule{
			{
				Verbs:         []string{"use"},
				APIGroups:     []string{"policy"},
				Resources:     []string{"podsecuritypolicies"},
				ResourceNames: []string{"eirini-workloads-app-psp"},
			},
		}
		role1 = createRole(ctx, k8sClient, PrefixedGUID("role"), orgNamespace.Name, rules1)
		rules2 = []rbacv1.PolicyRule{
			{
				Verbs:     []string{"patch"},
				APIGroups: []string{"eirini.cloudfoundry.org"},
				Resources: []string{"lrps/status"},
			},
		}
		role2 = createRole(ctx, k8sClient, PrefixedGUID("role"), orgNamespace.Name, rules2)

		username = PrefixedGUID("user")
		roleBinding = createRoleBinding(ctx, k8sClient, PrefixedGUID("role-binding"), username, role1.Name, orgNamespace.Name, map[string]string{})

		username2 := PrefixedGUID("user2")
		annotations := map[string]string{"cloudfoundry.org/propagate-cf-role": "false"}
		roleBinding2 = createRoleBinding(ctx, k8sClient, PrefixedGUID("role-binding2"), username2, role1.Name, orgNamespace.Name, annotations)

		spaceGUID = PrefixedGUID("cf-space")
		cfSpace = &korifiv1alpha1.CFSpace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      spaceGUID,
				Namespace: orgNamespace.Name,
			},
			Spec: korifiv1alpha1.CFSpaceSpec{
				DisplayName: spaceName,
			},
		}
	})

	When("the CFSpace is created", func() {
		JustBeforeEach(func() {
			Expect(k8sClient.Create(ctx, cfSpace)).To(Succeed())
		})

		It("creates a namespace and sets labels", func() {
			Eventually(func(g Gomega) {
				var createdSpace corev1.Namespace
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: spaceGUID}, &createdSpace)).To(Succeed())
				g.Expect(createdSpace.Labels).To(HaveKeyWithValue(korifiv1alpha1.SpaceNameLabel, spaceName))
			}).Should(Succeed())
		})

		It("sets the finalizer on cfSpace", func() {
			Eventually(func(g Gomega) []string {
				var createdCFSpace korifiv1alpha1.CFSpace
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: orgNamespace.Name, Name: spaceGUID}, &createdCFSpace)).To(Succeed())
				return createdCFSpace.ObjectMeta.Finalizers
			}).Should(ConsistOf([]string{
				"cfSpace.korifi.cloudfoundry.org",
			}))
		})

		It("propagates the image-registry-credentials to CFSpace", func() {
			Eventually(func(g Gomega) {
				var createdSecret corev1.Secret
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cfSpace.Name, Name: imageRegistrySecret.Name}, &createdSecret)).To(Succeed())
				g.Expect(createdSecret.Immutable).To(Equal(imageRegistrySecret.Immutable))
				g.Expect(createdSecret.Data).To(Equal(imageRegistrySecret.Data))
				g.Expect(createdSecret.StringData).To(Equal(imageRegistrySecret.StringData))
				g.Expect(createdSecret.Type).To(Equal(imageRegistrySecret.Type))
			}).Should(Succeed())
		})

		It("propagates the roles to CFSpace", func() {
			Eventually(func(g Gomega) {
				var createdRoles rbacv1.RoleList
				g.Expect(k8sClient.List(ctx, &createdRoles, client.InNamespace(cfSpace.Name))).To(Succeed())
				g.Expect(createdRoles.Items).To(ContainElements(
					MatchFields(IgnoreExtras, Fields{
						"ObjectMeta": MatchFields(IgnoreExtras, Fields{
							"Name": Equal(role1.Name),
						}),
						"Rules": Equal(rules1),
					}),
					MatchFields(IgnoreExtras, Fields{
						"ObjectMeta": MatchFields(IgnoreExtras, Fields{
							"Name": Equal(role2.Name),
						}),
						"Rules": Equal(rules2),
					}),
				))
			}).Should(Succeed())
		})

		It("propagates the role-bindings to CFSpace", func() {
			Eventually(func(g Gomega) {
				var createdRoleBindings rbacv1.RoleBindingList
				g.Expect(k8sClient.List(ctx, &createdRoleBindings, client.InNamespace(cfSpace.Name))).To(Succeed())
				g.Expect(createdRoleBindings.Items).To(ContainElements(
					MatchFields(IgnoreExtras, Fields{
						"ObjectMeta": MatchFields(IgnoreExtras, Fields{
							"Name": Equal(roleBinding.Name),
						}),
					}),
				))
			}).Should(Succeed())
		})

		It("does not propagate role-bindings with annotation \"cloudfoundry.org/propagate-cf-role\" set to false ", func() {
			Consistently(func(g Gomega) bool {
				var newRoleBinding rbacv1.RoleBinding
				return apierrors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Namespace: cfSpace.Name, Name: roleBinding2.Name}, &newRoleBinding))
			}, time.Second).Should(BeTrue())
		})

		It("creates the kpack service account", func() {
			var serviceAccount corev1.ServiceAccount
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Namespace: spaceGUID, Name: "kpack-service-account"}, &serviceAccount)
			}).Should(Succeed())

			Expect(serviceAccount.ImagePullSecrets).To(Equal([]corev1.LocalObjectReference{
				{Name: packageRegistrySecretName},
			}))

			Expect(serviceAccount.Secrets).To(Equal([]corev1.ObjectReference{
				{Name: packageRegistrySecretName},
			}))
		})

		It("creates the eirini service account", func() {
			Eventually(func() error {
				var serviceAccount corev1.ServiceAccount
				return k8sClient.Get(ctx, types.NamespacedName{Namespace: spaceGUID, Name: "eirini"}, &serviceAccount)
			}).Should(Succeed())
		})

		It("sets status on CFSpace", func() {
			Eventually(func(g Gomega) {
				var createdSpace korifiv1alpha1.CFSpace
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: cfSpace.Namespace, Name: cfSpace.Name}, &createdSpace)).To(Succeed())
				g.Expect(createdSpace.Status.GUID).To(Equal(cfSpace.Name))
				g.Expect(meta.IsStatusConditionTrue(createdSpace.Status.Conditions, "Ready")).To(BeTrue())
			}).Should(Succeed())
		})
	})

	When("roles are added/updated in CFOrg namespace after CFSpace creation", func() {
		var (
			rules3      []rbacv1.PolicyRule
			role3       *rbacv1.Role
			updatedRole *rbacv1.Role
		)
		BeforeEach(func() {
			Expect(k8sClient.Create(ctx, cfSpace)).To(Succeed())
			Eventually(func(g Gomega) {
				var createdSpace korifiv1alpha1.CFSpace
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: orgNamespace.Name, Name: spaceGUID}, &createdSpace)).To(Succeed())
				g.Expect(meta.IsStatusConditionTrue(createdSpace.Status.Conditions, "Ready")).To(BeTrue())
			}, 20*time.Second).Should(Succeed())

			rules3 = []rbacv1.PolicyRule{
				{
					Verbs:     []string{"update"},
					APIGroups: []string{"eirini.cloudfoundry.org"},
					Resources: []string{"lrps/status"},
				},
			}
			role3 = createRole(ctx, k8sClient, PrefixedGUID("role"), orgNamespace.Name, rules3)

			updatedRole = role2.DeepCopy()
			updatedRole.Rules = append(updatedRole.Rules, rbacv1.PolicyRule{
				Verbs:     []string{"create"},
				APIGroups: []string{"eirini.cloudfoundry.org"},
				Resources: []string{"lrps"},
			})
			Expect(k8sClient.Patch(ctx, updatedRole, client.MergeFrom(role2)))
		})

		It("propagates the role changes to CFSpace namespace", func() {
			Eventually(func(g Gomega) {
				var createdRoles rbacv1.RoleList
				g.Expect(k8sClient.List(ctx, &createdRoles, client.InNamespace(cfSpace.Name))).To(Succeed())
				g.Expect(createdRoles.Items).To(ContainElements(
					MatchFields(IgnoreExtras, Fields{
						"ObjectMeta": MatchFields(IgnoreExtras, Fields{
							"Name": Equal(role1.Name),
						}),
						"Rules": Equal(rules1),
					}),
					MatchFields(IgnoreExtras, Fields{
						"ObjectMeta": MatchFields(IgnoreExtras, Fields{
							"Name": Equal(role2.Name),
						}),
						"Rules": Equal(updatedRole.Rules),
					}),
					MatchFields(IgnoreExtras, Fields{
						"ObjectMeta": MatchFields(IgnoreExtras, Fields{
							"Name": Equal(role3.Name),
						}),
						"Rules": Equal(rules3),
					}),
				))
			}).Should(Succeed())
		})
	})

	When("role-bindings are added/updated in CFOrg namespace after CFSpace creation", func() {
		var roleBinding3 rbacv1.RoleBinding
		BeforeEach(func() {
			Expect(k8sClient.Create(ctx, cfSpace)).To(Succeed())
			Eventually(func(g Gomega) {
				var createdSpace korifiv1alpha1.CFSpace
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: orgNamespace.Name, Name: spaceGUID}, &createdSpace)).To(Succeed())
				g.Expect(meta.IsStatusConditionTrue(createdSpace.Status.Conditions, "Ready")).To(BeTrue())
			}, 20*time.Second).Should(Succeed())

			roleBinding3 = createRoleBinding(ctx, k8sClient, PrefixedGUID("role-binding"), username, role2.Name, orgNamespace.Name, map[string]string{})
		})

		It("propagates the new role-binding to CFSpace namespace", func() {
			Eventually(func(g Gomega) {
				var createdRoleBindings rbacv1.RoleBindingList
				g.Expect(k8sClient.List(ctx, &createdRoleBindings, client.InNamespace(cfSpace.Name))).To(Succeed())
				g.Expect(createdRoleBindings.Items).To(ContainElements(
					MatchFields(IgnoreExtras, Fields{
						"ObjectMeta": MatchFields(IgnoreExtras, Fields{
							"Name": Equal(roleBinding.Name),
						}),
					}),
					MatchFields(IgnoreExtras, Fields{
						"ObjectMeta": MatchFields(IgnoreExtras, Fields{
							"Name": Equal(roleBinding3.Name),
						}),
					}),
				))
			}).Should(Succeed())
		})
	})

	When("the CFSpace is deleted", func() {
		BeforeEach(func() {
			Expect(k8sClient.Create(ctx, cfSpace)).To(Succeed())
			Eventually(func(g Gomega) {
				var spaceNamespace corev1.Namespace
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: spaceGUID}, &spaceNamespace)).To(Succeed())
			}).Should(Succeed())

			Expect(k8sClient.Delete(ctx, cfSpace)).To(Succeed())
		})

		It("eventually deletes the CFSpace", func() {
			Eventually(func() bool {
				var createdCFSpace korifiv1alpha1.CFSpace
				return apierrors.IsNotFound(k8sClient.Get(context.Background(), types.NamespacedName{Name: spaceGUID, Namespace: orgNamespace.Name}, &createdCFSpace))
			}).Should(BeTrue(), "timed out waiting for CFSpace to be deleted")
		})

		It("eventually deletes the namespace", func() {
			// Envtests do not cleanup namespaces. For testing, we check for deletion timestamps on namespace.
			Eventually(func(g Gomega) bool {
				var spaceNamespace corev1.Namespace
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: spaceGUID}, &spaceNamespace)).To(Succeed())
				return spaceNamespace.GetDeletionTimestamp().IsZero()
			}).Should(BeFalse(), "timed out waiting for deletion timestamps to be set on namespace")
		})
	})
})
