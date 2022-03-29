package repositories_test

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apierrors"
	. "code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories/fake"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks"
	"code.cloudfoundry.org/cf-k8s-controllers/tests/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

const (
	CFAppRevisionKey   = "workloads.cloudfoundry.org/app-rev"
	CFAppRevisionValue = "1"
	CFAppStoppedState  = "STOPPED"
)

//counterfeiter:generate -o fake -fake-name ClientWithWatch sigs.k8s.io/controller-runtime/pkg/client.WithWatch
var _ = Describe("AppRepository Units", func() {
	var (
		testCtx                context.Context
		appRepo                *AppRepo
		fakeUserClient         fake.ClientWithWatch
		fakeFactory            fake.UserK8sClientFactory
		fakeNamespaceRetriever fake.NamespaceRetriever
		fakeNsPerms            fake.NamespacePermissions
	)

	BeforeEach(func() {
		testCtx = context.Background()
		fakeUserClient = fake.ClientWithWatch{}
		fakeFactory = fake.UserK8sClientFactory{}
		fakeFactory.BuildClientReturns(&fakeUserClient, nil)
		fakeNsPerms = fake.NamespacePermissions{}
		fakeNamespaceRetriever = fake.NamespaceRetriever{}
		appRepo = NewAppRepo(&fakeNamespaceRetriever, &fakeFactory, &fakeNsPerms)
	})

	Describe("GetApp", func() {
		var retErr error
		JustBeforeEach(func() {
			_, retErr = appRepo.GetApp(testCtx, authInfo, "some-app-name")
		})

		When("the client factory reports an error", func() {
			BeforeEach(func() {
				fakeFactory.BuildClientReturns(nil, errors.New("oh no"))
			})

			It("returns the error", func() {
				Expect(retErr).To(MatchError(HavePrefix("failed to build user client")))
				Expect(retErr).To(MatchError(ContainSubstring("oh no")))
			})
		})
	})

	Describe("CreateApp", func() {
		var retErr error
		JustBeforeEach(func() {
			_, retErr = appRepo.CreateApp(testCtx, authInfo, CreateAppMessage{Name: prefixedGUID("app-")})
		})

		When("the client factory reports an error", func() {
			BeforeEach(func() {
				fakeFactory.BuildClientReturns(nil, errors.New("oh no"))
			})

			It("returns the error", func() {
				Expect(retErr).To(MatchError(HavePrefix("failed to build user client")))
				Expect(retErr).To(MatchError(ContainSubstring("oh no")))
			})
		})

		When("the app already exists", func() {
			BeforeEach(func() {
				fakeUserClient.CreateReturns(&k8serrors.StatusError{
					ErrStatus: metav1.Status{
						Reason: metav1.StatusReason(webhooks.DuplicateAppError.Marshal()),
					}})
			})

			It("reports a uniqueness error", func() {
				Expect(retErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.UniquenessError{}))
			})
		})

		When("a random error occurs", func() {
			BeforeEach(func() {
				fakeUserClient.CreateReturns(errors.New("something unexpected"))
			})
			It("returns the error", func() {
				Expect(retErr).To(MatchError(errors.New("something unexpected")))
			})
		})

		When("we get back a random error while patching the env vars", func() {
			BeforeEach(func() {
				fakeUserClient.GetReturns(errors.New("shucks"))
			})
			It("returns the error", func() {
				Expect(retErr).To(MatchError(errors.New("shucks")))
			})
		})
	})

	Describe("ListApps", func() {
		var (
			retErr     error
			appRecords []AppRecord
		)
		BeforeEach(func() {
			retErr = nil
			appRecords = []AppRecord{}
		})
		JustBeforeEach(func() {
			appRecords, retErr = appRepo.ListApps(testCtx, authInfo, ListAppsMessage{})
		})

		When("getting namespaces returns an error", func() {
			BeforeEach(func() {
				fakeNsPerms.GetAuthorizedSpaceNamespacesReturns(map[string]bool{}, errors.New("something happened"))
			})

			It("returns the error", func() {
				Expect(retErr).To(MatchError(HavePrefix("failed to list namespaces")))
				Expect(retErr).To(MatchError(ContainSubstring("something happened")))
			})
		})

		When("the client factory reports an error", func() {
			BeforeEach(func() {
				fakeFactory.BuildClientReturns(nil, errors.New("oh no"))
			})

			It("returns the error", func() {
				Expect(retErr).To(MatchError(HavePrefix("failed to build user client")))
				Expect(retErr).To(MatchError(ContainSubstring("oh no")))
			})
		})

		When("there is a namespace", func() {
			BeforeEach(func() {
				fakeNsPerms.GetAuthorizedSpaceNamespacesReturns(map[string]bool{"foo": true}, nil)
			})
			When("the user isn't authorized to list apps in the namespace", func() {
				BeforeEach(func() {
					fakeUserClient.ListReturns(&k8serrors.StatusError{
						ErrStatus: metav1.Status{
							Reason: metav1.StatusReasonForbidden,
						}})
				})
				It("returns an empty result set without erroring", func() {
					Expect(retErr).NotTo(HaveOccurred())
					Expect(appRecords).To(BeEmpty())
				})
			})

			When("listing apps fails for other reasons", func() {
				BeforeEach(func() {
					fakeUserClient.ListReturns(errors.New("random failure"))
				})
				It("returns the error", func() {
					Expect(retErr).To(MatchError(HavePrefix("failed to list apps in namespace foo")))
					Expect(retErr).To(MatchError(ContainSubstring("random failure")))
				})
			})
		})
	})

	Describe("PatchAppEnvVars", func() {
		var retErr error
		JustBeforeEach(func() {
			_, retErr = appRepo.PatchAppEnvVars(testCtx, authInfo, PatchAppEnvVarsMessage{})
		})

		When("the client factory reports an error", func() {
			BeforeEach(func() {
				fakeFactory.BuildClientReturns(nil, errors.New("oh no"))
			})

			It("returns the error", func() {
				Expect(retErr).To(MatchError(HavePrefix("failed to build user client")))
				Expect(retErr).To(MatchError(ContainSubstring("oh no")))
			})
		})

		When("we get back a random error while patching the env vars", func() {
			BeforeEach(func() {
				fakeUserClient.GetReturns(errors.New("shucks"))
			})
			It("returns the error", func() {
				Expect(retErr).To(MatchError(errors.New("shucks")))
			})
		})
	})

	Describe("SetCurrentDroplet", func() {
		var retErr error
		JustBeforeEach(func() {
			_, retErr = appRepo.SetCurrentDroplet(testCtx, authInfo, SetCurrentDropletMessage{})
		})

		When("the client factory reports an error", func() {
			BeforeEach(func() {
				fakeFactory.BuildClientReturns(nil, errors.New("oh no"))
			})

			It("returns the error", func() {
				Expect(retErr).To(MatchError(HavePrefix("failed to build user client")))
				Expect(retErr).To(MatchError(ContainSubstring("oh no")))
			})
		})
	})

	Describe("SetAppDesiredState", func() {
		var retErr error
		JustBeforeEach(func() {
			_, retErr = appRepo.SetAppDesiredState(testCtx, authInfo, SetAppDesiredStateMessage{})
		})

		When("the client factory reports an error", func() {
			BeforeEach(func() {
				fakeFactory.BuildClientReturns(nil, errors.New("oh no"))
			})

			It("returns the error", func() {
				Expect(retErr).To(MatchError(HavePrefix("failed to build user client")))
				Expect(retErr).To(MatchError(ContainSubstring("oh no")))
			})
		})
	})

	Describe("DeleteApp", func() {
		var retErr error
		JustBeforeEach(func() {
			retErr = appRepo.DeleteApp(testCtx, authInfo, DeleteAppMessage{})
		})

		When("the client factory reports an error", func() {
			BeforeEach(func() {
				fakeFactory.BuildClientReturns(nil, errors.New("oh no"))
			})

			It("returns the error", func() {
				Expect(retErr).To(MatchError(HavePrefix("failed to build user client")))
				Expect(retErr).To(MatchError(ContainSubstring("oh no")))
			})
		})
	})

	Describe("GetAppEnv", func() {
		var retErr error
		JustBeforeEach(func() {
			_, retErr = appRepo.GetAppEnv(testCtx, authInfo, "some-guid")
		})

		When("the client factory reports an error", func() {
			BeforeEach(func() {
				fakeUserClient.GetStub = func(ctx context.Context, name types.NamespacedName, obj client.Object) error {
					cfApp := obj.(*workloadsv1alpha1.CFApp)
					cfApp.Spec.EnvSecretName = "foo"
					return nil
				}
				fakeFactory.BuildClientReturnsOnCall(1, nil, errors.New("oh no"))
			})

			It("returns the error", func() {
				Expect(retErr).To(MatchError(HavePrefix("failed to build user client")))
				Expect(retErr).To(MatchError(ContainSubstring("oh no")))
			})
		})
	})
})

var _ = Describe("AppRepository", func() {
	var (
		testCtx                context.Context
		appRepo                *AppRepo
		org                    *v1alpha2.SubnamespaceAnchor
		space1, space2, space3 *v1alpha2.SubnamespaceAnchor
		cfApp1, cfApp2, cfApp3 *workloadsv1alpha1.CFApp
	)

	BeforeEach(func() {
		testCtx = context.Background()

		appRepo = NewAppRepo(namespaceRetriever, userClientFactory, nsPerms)

		org = createOrgWithCleanup(testCtx, prefixedGUID("org"))
		space1 = createSpaceWithCleanup(testCtx, org.Name, prefixedGUID("space1"))
		space2 = createSpaceWithCleanup(testCtx, org.Name, prefixedGUID("space2"))
		space3 = createSpaceWithCleanup(testCtx, org.Name, prefixedGUID("space3"))
	})

	Describe("GetApp", func() {
		var (
			appGUID string
			app     AppRecord
			getErr  error
		)

		BeforeEach(func() {
			cfApp1 = createApp(space1.Name)
			appGUID = cfApp1.Name
		})

		JustBeforeEach(func() {
			app, getErr = appRepo.GetApp(testCtx, authInfo, appGUID)
		})

		When("authorized in the space", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space1.Name)
			})

			AfterEach(func() {
				Expect(k8sClient.Delete(context.Background(), cfApp1)).To(Succeed())
			})

			It("can fetch the AppRecord CR we're looking for", func() {
				Expect(getErr).NotTo(HaveOccurred())

				Expect(app.GUID).To(Equal(cfApp1.Name))
				Expect(app.EtcdUID).To(Equal(cfApp1.GetUID()))
				Expect(app.Revision).To(Equal(CFAppRevisionValue))
				Expect(app.Name).To(Equal(cfApp1.Spec.Name))
				Expect(app.SpaceGUID).To(Equal(space1.Name))
				Expect(app.State).To(Equal(DesiredState("STOPPED")))
				Expect(app.DropletGUID).To(Equal(cfApp1.Spec.CurrentDropletRef.Name))
				Expect(app.Lifecycle).To(Equal(Lifecycle{
					Type: string(cfApp1.Spec.Lifecycle.Type),
					Data: LifecycleData{
						Buildpacks: cfApp1.Spec.Lifecycle.Data.Buildpacks,
						Stack:      cfApp1.Spec.Lifecycle.Data.Stack,
					},
				}))
			})
		})

		When("the user is not authorized in the space", func() {
			It("returns a forbidden error", func() {
				Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})

		When("duplicate Apps exist across namespaces with the same GUIDs", func() {
			BeforeEach(func() {
				cfApp2 = createAppWithGUID(space2.Name, appGUID)
			})

			AfterEach(func() {
				Expect(k8sClient.Delete(context.Background(), cfApp1)).To(Succeed())
				Expect(k8sClient.Delete(context.Background(), cfApp2)).To(Succeed())
			})

			It("returns an error", func() {
				Expect(getErr).To(HaveOccurred())
				Expect(getErr).To(MatchError("get-app duplicate records exist"))
			})
		})

		When("the app guid is not found", func() {
			BeforeEach(func() {
				appGUID = "does-not-exist"
			})

			It("returns an error", func() {
				Expect(getErr).To(HaveOccurred())
				Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})
	})

	Describe("GetAppByNameAndSpace", func() {
		var (
			appRecord      AppRecord
			getErr         error
			querySpaceName string
		)

		BeforeEach(func() {
			cfApp1 = createApp(space1.Name)
			querySpaceName = space1.Name
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(context.Background(), cfApp1)).To(Succeed())
		})

		JustBeforeEach(func() {
			appRecord, getErr = appRepo.GetAppByNameAndSpace(context.Background(), authInfo, cfApp1.Spec.Name, querySpaceName)
		})

		When("the user is authorized in the space", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, querySpaceName)
			})

			It("returns the record", func() {
				Expect(getErr).NotTo(HaveOccurred())

				Expect(appRecord.Name).To(Equal(cfApp1.Spec.Name))
				Expect(appRecord.GUID).To(Equal(cfApp1.Name))
				Expect(appRecord.EtcdUID).To(Equal(cfApp1.UID))
				Expect(appRecord.SpaceGUID).To(Equal(space1.Name))
				Expect(appRecord.State).To(BeEquivalentTo(cfApp1.Spec.DesiredState))
				Expect(appRecord.Lifecycle.Type).To(BeEquivalentTo(cfApp1.Spec.Lifecycle.Type))
			})
		})

		When("the user is not authorized in the space", func() {
			It("returns a forbidden error", func() {
				Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})

		When("the App doesn't exist in the Space (but is in another Space)", func() {
			BeforeEach(func() {
				querySpaceName = space2.Name
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, querySpaceName)
			})
			It("returns a NotFoundError", func() {
				Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})

	})

	Describe("ListApps", Serial, func() {
		var (
			message        ListAppsMessage
			nonCFNamespace string
		)

		BeforeEach(func() {
			message = ListAppsMessage{}

			var cfAppList workloadsv1alpha1.CFAppList
			Expect(
				k8sClient.List(context.Background(), &cfAppList),
			).To(Succeed())

			for _, app := range cfAppList.Items {
				Expect(
					k8sClient.Delete(context.Background(), &app),
				).To(Succeed())
			}

			nonCFNamespace = prefixedGUID("non-cf")
			Expect(k8sClient.Create(
				testCtx,
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nonCFNamespace}},
			)).To(Succeed())

			createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space1.Name)
			createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space2.Name)
			createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, nonCFNamespace)
		})

		Describe("when filters are NOT provided", func() {
			When("no Apps exist", func() {
				It("returns an empty list of apps", func() {
					appList, err := appRepo.ListApps(testCtx, authInfo, message)
					Expect(err).NotTo(HaveOccurred())
					Expect(appList).To(BeEmpty())
				})
			})

			When("multiple Apps exist", func() {
				BeforeEach(func() {
					cfApp1 = createApp(space1.Name)
					cfApp2 = createApp(space2.Name)
					createApp(space3.Name)
					createApp(nonCFNamespace)
				})

				It("returns all the AppRecord CRs where client has permission", func() {
					appList, err := appRepo.ListApps(testCtx, authInfo, message)
					Expect(err).NotTo(HaveOccurred())
					Expect(appList).To(ConsistOf(
						MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfApp1.Name)}),
						MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfApp2.Name)}),
					))

					sortedByName := sort.SliceIsSorted(appList, func(i, j int) bool {
						return appList[i].Name < appList[j].Name
					})

					Expect(sortedByName).To(BeTrue(), fmt.Sprintf("AppList was not sorted by Name : App1 : %s , App2: %s", appList[0].Name, appList[1].Name))
				})
			})
		})

		Describe("when filters are provided", func() {
			When("a name filter is provided", func() {
				When("no Apps exist that match the filter", func() {
					BeforeEach(func() {
						createApp(space1.Name)
						createApp(space2.Name)
					})

					It("returns an empty list of apps", func() {
						message = ListAppsMessage{Names: []string{"some-other-app"}}
						appList, err := appRepo.ListApps(testCtx, authInfo, message)
						Expect(err).NotTo(HaveOccurred())
						Expect(appList).To(BeEmpty())
					})
				})

				When("some Apps match the filter", func() {
					BeforeEach(func() {
						createApp(space1.Name)
						cfApp2 = createApp(space2.Name)
						cfApp3 = createApp(space1.Name)
					})

					It("returns the matching apps", func() {
						message = ListAppsMessage{Names: []string{cfApp2.Spec.Name, cfApp3.Spec.Name}}
						appList, err := appRepo.ListApps(testCtx, authInfo, message)
						Expect(err).NotTo(HaveOccurred())
						Expect(appList).To(ConsistOf(
							MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfApp2.Name)}),
							MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfApp3.Name)}),
						))
					})
				})
			})

			When("a guid filter is provided", func() {
				When("no Apps exist that match the filter", func() {
					BeforeEach(func() {
						createApp(space1.Name)
						createApp(space2.Name)
					})

					It("returns an empty list of apps", func() {
						message = ListAppsMessage{Guids: []string{"some-other-app-guid"}}
						appList, err := appRepo.ListApps(testCtx, authInfo, message)
						Expect(err).NotTo(HaveOccurred())
						Expect(appList).To(BeEmpty())
					})
				})

				When("some Apps match the filter", func() {
					BeforeEach(func() {
						createApp(space1.Name)
						cfApp2 = createAppWithGUID(space2.Name, "app-guid-2")
						cfApp3 = createAppWithGUID(space1.Name, "app-guid-3")
					})

					It("returns the matching apps", func() {
						message = ListAppsMessage{Guids: []string{"app-guid-2", "app-guid-3"}}
						appList, err := appRepo.ListApps(testCtx, authInfo, message)
						Expect(err).NotTo(HaveOccurred())
						Expect(appList).To(ConsistOf(
							MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfApp2.Name)}),
							MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfApp3.Name)}),
						))
					})
				})
			})

			When("a space filter is provided", func() {
				When("no Apps exist that match the filter", func() {
					BeforeEach(func() {
						createApp(space1.Name)
						createApp(space1.Name)
					})

					It("returns an empty list of apps", func() {
						message = ListAppsMessage{SpaceGuids: []string{"some-other-space-guid"}}
						appList, err := appRepo.ListApps(testCtx, authInfo, message)
						Expect(err).NotTo(HaveOccurred())
						Expect(appList).To(BeEmpty())
					})
				})

				When("some Apps match the filter", func() {
					BeforeEach(func() {
						createApp(space1.Name)
						cfApp2 = createApp(space2.Name)
						cfApp3 = createApp(space2.Name)
					})

					It("returns the matching apps", func() {
						message = ListAppsMessage{SpaceGuids: []string{space2.Name}}
						appList, err := appRepo.ListApps(testCtx, authInfo, message)
						Expect(err).NotTo(HaveOccurred())
						Expect(appList).To(ConsistOf(
							MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfApp2.Name)}),
							MatchFields(IgnoreExtras, Fields{"GUID": Equal(cfApp3.Name)}),
						))
					})
				})
			})

			When("both name and space filters are provided", func() {
				When("no Apps exist that match the union of the filters", func() {
					BeforeEach(func() {
						cfApp1 = createApp(space1.Name)
						cfApp2 = createApp(space1.Name)
					})

					When("an App matches by Name but not by Space", func() {
						It("returns an empty list of apps", func() {
							message = ListAppsMessage{Names: []string{cfApp1.Spec.Name}, SpaceGuids: []string{"some-other-space-guid"}}
							appList, err := appRepo.ListApps(testCtx, authInfo, message)
							Expect(err).NotTo(HaveOccurred())
							Expect(appList).To(BeEmpty())
						})
					})

					When("an App matches by Space but not by Name", func() {
						It("returns an empty list of apps", func() {
							message = ListAppsMessage{Names: []string{"fake-app-name"}, SpaceGuids: []string{space1.Name}}
							appList, err := appRepo.ListApps(testCtx, authInfo, message)
							Expect(err).NotTo(HaveOccurred())
							Expect(appList).To(BeEmpty())
						})
					})
				})

				When("some Apps match the union of the filters", func() {
					BeforeEach(func() {
						cfApp1 = createApp(space1.Name)
						cfApp2 = createApp(space2.Name)
						cfApp3 = createApp(space2.Name)
					})

					It("returns the matching apps", func() {
						message = ListAppsMessage{Names: []string{cfApp2.Spec.Name}, SpaceGuids: []string{space2.Name}}
						appList, err := appRepo.ListApps(testCtx, authInfo, message)
						Expect(err).NotTo(HaveOccurred())
						Expect(appList).To(HaveLen(1))

						Expect(appList[0].GUID).To(Equal(cfApp2.Name))
					})
				})
			})
		})
	})

	Describe("CreateApp", func() {
		const (
			testAppName = "test-app-name"
		)
		var (
			appCreateMessage CreateAppMessage
			spaceGUID        string
		)

		BeforeEach(func() {
			spaceGUID = generateGUID()

			Expect(k8sClient.Create(testCtx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: spaceGUID},
			})).To(Succeed())

			createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, spaceGUID)

			appCreateMessage = initializeAppCreateMessage(testAppName, spaceGUID)
		})

		AfterEach(func() {
			err := k8sClient.Delete(testCtx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: spaceGUID},
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("creates a new app CR", func() {
			createdAppRecord, err := appRepo.CreateApp(testCtx, authInfo, appCreateMessage)
			Expect(err).NotTo(HaveOccurred())
			Expect(createdAppRecord).NotTo(BeNil())

			cfAppLookupKey := types.NamespacedName{Name: createdAppRecord.GUID, Namespace: spaceGUID}
			createdCFApp := new(workloadsv1alpha1.CFApp)
			Eventually(func() error {
				return k8sClient.Get(context.Background(), cfAppLookupKey, createdCFApp)
			}, 10*time.Second, 250*time.Millisecond).Should(Succeed())
		})

		It("returns an AppRecord with correct fields", func() {
			createdAppRecord, err := appRepo.CreateApp(context.Background(), authInfo, appCreateMessage)
			Expect(err).NotTo(HaveOccurred())
			Expect(createdAppRecord).NotTo(Equal(AppRecord{}))
			Expect(createdAppRecord.GUID).To(MatchRegexp("^[-0-9a-f]{36}$"), "record GUID was not a 36 character guid")
			Expect(createdAppRecord.SpaceGUID).To(Equal(spaceGUID), "App SpaceGUID in record did not match input")
			Expect(createdAppRecord.Name).To(Equal(testAppName), "App Name in record did not match input")

			recordCreatedTime, err := time.Parse(TimestampFormat, createdAppRecord.CreatedAt)
			Expect(err).NotTo(HaveOccurred())
			Expect(recordCreatedTime).To(BeTemporally("~", time.Now(), 2*time.Second))

			recordUpdatedTime, err := time.Parse(TimestampFormat, createdAppRecord.UpdatedAt)
			Expect(err).NotTo(HaveOccurred())
			Expect(recordUpdatedTime).To(BeTemporally("~", time.Now(), 2*time.Second))
		})

		When("no environment variables are given", func() {
			BeforeEach(func() {
				appCreateMessage.EnvironmentVariables = nil
			})

			It("creates an empty secret and sets the environment variable secret ref on the App", func() {
				createdAppRecord, err := appRepo.CreateApp(testCtx, authInfo, appCreateMessage)
				Expect(err).NotTo(HaveOccurred())
				Expect(createdAppRecord).NotTo(BeNil())

				cfAppLookupKey := types.NamespacedName{Name: createdAppRecord.GUID, Namespace: spaceGUID}
				createdCFApp := new(workloadsv1alpha1.CFApp)
				Eventually(func() error {
					return k8sClient.Get(context.Background(), cfAppLookupKey, createdCFApp)
				}, 10*time.Second, 250*time.Millisecond).Should(Succeed())

				Expect(createdCFApp.Spec.EnvSecretName).NotTo(BeEmpty())

				secretLookupKey := types.NamespacedName{Name: createdCFApp.Spec.EnvSecretName, Namespace: spaceGUID}
				createdSecret := new(corev1.Secret)
				Eventually(func() error {
					return k8sClient.Get(context.Background(), secretLookupKey, createdSecret)
				}, 10*time.Second, 250*time.Millisecond).Should(Succeed())

				Expect(createdSecret.Data).To(BeEmpty())
			})
		})

		When("environment variables are given", func() {
			BeforeEach(func() {
				appCreateMessage.EnvironmentVariables = map[string]string{
					"FOO": "foo",
					"BAR": "bar",
				}
			})

			It("creates an secret for the environment variables and sets the ref on the App", func() {
				createdAppRecord, err := appRepo.CreateApp(testCtx, authInfo, appCreateMessage)
				Expect(err).NotTo(HaveOccurred())
				Expect(createdAppRecord).NotTo(BeNil())

				cfAppLookupKey := types.NamespacedName{Name: createdAppRecord.GUID, Namespace: spaceGUID}
				createdCFApp := new(workloadsv1alpha1.CFApp)
				Eventually(func() error {
					return k8sClient.Get(context.Background(), cfAppLookupKey, createdCFApp)
				}, 10*time.Second, 250*time.Millisecond).Should(Succeed())

				Expect(createdCFApp.Spec.EnvSecretName).NotTo(BeEmpty())

				secretLookupKey := types.NamespacedName{Name: createdCFApp.Spec.EnvSecretName, Namespace: spaceGUID}
				createdSecret := new(corev1.Secret)
				Eventually(func() error {
					return k8sClient.Get(context.Background(), secretLookupKey, createdSecret)
				}, 10*time.Second, 250*time.Millisecond).Should(Succeed())

				Expect(createdSecret.Data).To(MatchAllKeys(Keys{
					"FOO": BeEquivalentTo("foo"),
					"BAR": BeEquivalentTo("bar"),
				}))
			})
		})
	})

	Describe("PatchAppEnvVars", func() {
		const (
			key0 = "KEY0"
			key1 = "KEY1"
			key2 = "KEY2"
		)

		var (
			testAppGUID              string
			testAppEnvSecretName     string
			originalEnvVars          map[string]string
			requestEnvVars           map[string]*string
			expectedEnvVars          map[string]string
			testAppEnvSecretPatchMsg PatchAppEnvVarsMessage
			secretRecord             AppEnvVarsRecord
			patchErr                 error
		)

		BeforeEach(func() {
			testAppGUID = generateGUID()
			testAppEnvSecretName = generateAppEnvSecretName(testAppGUID)

			originalEnvVars = map[string]string{
				key0: "VAL0",
				key1: "original-value",
			}

			secret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      GenerateEnvSecretName(testAppGUID),
					Namespace: space1.Name,
				},
				StringData: originalEnvVars,
			}
			Expect(k8sClient.Create(testCtx, &secret)).To(Succeed())

			var value1 *string
			value2 := "VAL2"

			requestEnvVars = map[string]*string{
				key1: value1,
				key2: &value2,
			}
			testAppEnvSecretPatchMsg = PatchAppEnvVarsMessage{
				AppGUID:              testAppGUID,
				SpaceGUID:            space1.Name,
				EnvironmentVariables: requestEnvVars,
			}

			expectedEnvVars = map[string]string{
				key0: originalEnvVars[key0],
				key2: *requestEnvVars[key2],
			}
		})

		JustBeforeEach(func() {
			secretRecord, patchErr = appRepo.PatchAppEnvVars(context.Background(), authInfo, testAppEnvSecretPatchMsg)
		})

		When("the user is authorized and an app exists with a secret", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space1.Name)
			})

			It("returns the updated secret record", func() {
				Expect(patchErr).NotTo(HaveOccurred())
				Expect(secretRecord.EnvironmentVariables).To(Equal(expectedEnvVars))
			})

			It("eventually patches the underlying secret", func() {
				cfAppSecretLookupKey := types.NamespacedName{Name: testAppEnvSecretName, Namespace: space1.Name}

				var updatedSecret corev1.Secret
				Eventually(func() map[string][]byte {
					err := k8sClient.Get(context.Background(), cfAppSecretLookupKey, &updatedSecret)
					if err != nil {
						return map[string][]byte{}
					}
					return updatedSecret.Data
				}, timeCheckThreshold*time.Second).Should(HaveKey(key2))

				Expect(updatedSecret.Data).To(HaveLen(len(expectedEnvVars)))
				Expect(updatedSecret.Data).To(HaveKey(key0))
				Expect(string(updatedSecret.Data[key0])).To(Equal(expectedEnvVars[key0]))
				Expect(updatedSecret.Data).NotTo(HaveKey(key1))
				Expect(updatedSecret.Data).To(HaveKey(key2))
				Expect(string(updatedSecret.Data[key2])).To(Equal(expectedEnvVars[key2]))
			})
		})

		When("the user is not authorized", func() {
			It("return a forbidden error", func() {
				Expect(patchErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})
	})

	Describe("CreateOrPatchAppEnvVars", func() {
		const (
			testAppName = "some-app-name"
			key1        = "KEY1"
			key2        = "KEY2"
		)

		var (
			testAppGUID              string
			cfAppCR                  *workloadsv1alpha1.CFApp
			testAppEnvSecretName     string
			requestEnvVars           map[string]string
			testAppEnvSecret         CreateOrPatchAppEnvVarsMessage
			returnedAppEnvVarsRecord AppEnvVarsRecord
			returnedErr              error
		)

		BeforeEach(func() {
			testAppGUID = generateGUID()
			cfAppCR = createAppCR(testCtx, k8sClient, testAppName, testAppGUID, space1.Name, CFAppStoppedState)

			testAppEnvSecretName = generateAppEnvSecretName(testAppGUID)
			requestEnvVars = map[string]string{
				key1: "VAL1",
				key2: "VAL2",
			}
			testAppEnvSecret = CreateOrPatchAppEnvVarsMessage{
				AppGUID:              testAppGUID,
				AppEtcdUID:           cfAppCR.GetUID(),
				SpaceGUID:            space1.Name,
				EnvironmentVariables: requestEnvVars,
			}
		})

		JustBeforeEach(func() {
			returnedAppEnvVarsRecord, returnedErr = appRepo.CreateOrPatchAppEnvVars(context.Background(), authInfo, testAppEnvSecret)
		})

		When("the user is authorized", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space1.Name)
			})

			When("the secret doesn't already exist", func() {
				It("returns a record matching the input and no error", func() {
					Expect(returnedAppEnvVarsRecord.AppGUID).To(Equal(testAppEnvSecret.AppGUID))
					Expect(returnedAppEnvVarsRecord.SpaceGUID).To(Equal(testAppEnvSecret.SpaceGUID))
					Expect(returnedAppEnvVarsRecord.EnvironmentVariables).To(HaveLen(len(testAppEnvSecret.EnvironmentVariables)))
					Expect(returnedErr).To(BeNil())
				})

				It("returns a record with the created Secret's name", func() {
					Expect(returnedAppEnvVarsRecord.Name).ToNot(BeEmpty())
				})

				It("the App record GUID returned should equal the App GUID provided", func() {
					// Used a strings.Trim to remove characters, which cause the behavior in Issue #103
					testAppEnvSecret.AppGUID = "estringtrimmedguid"

					returnedUpdatedAppEnvVarsRecord, returnedUpdatedErr := appRepo.CreateOrPatchAppEnvVars(testCtx, authInfo, testAppEnvSecret)
					Expect(returnedUpdatedErr).ToNot(HaveOccurred())
					Expect(returnedUpdatedAppEnvVarsRecord.AppGUID).To(Equal(testAppEnvSecret.AppGUID), "Expected App GUID to match after transform")
				})

				It("eventually creates a secret that matches the request record", func() {
					cfAppSecretLookupKey := types.NamespacedName{Name: testAppEnvSecretName, Namespace: space1.Name}
					createdCFAppSecret := &corev1.Secret{}
					Eventually(func() bool {
						err := k8sClient.Get(context.Background(), cfAppSecretLookupKey, createdCFAppSecret)
						return err == nil
					}, timeCheckThreshold*time.Second, 250*time.Millisecond).Should(BeTrue(), "could not find the secret created by the repo")

					// Secret has an owner reference that points to the App CR
					Expect(createdCFAppSecret.OwnerReferences)
					Expect(createdCFAppSecret.ObjectMeta.OwnerReferences).To(ConsistOf([]metav1.OwnerReference{
						{
							APIVersion: "workloads.cloudfoundry.org/v1alpha1",
							Kind:       "CFApp",
							Name:       cfAppCR.Name,
							UID:        cfAppCR.GetUID(),
						},
					}))

					Expect(createdCFAppSecret).ToNot(BeZero())
					Expect(createdCFAppSecret.Name).To(Equal(testAppEnvSecretName))
					Expect(createdCFAppSecret.Labels).To(HaveKeyWithValue(CFAppGUIDLabel, testAppGUID))
					Expect(createdCFAppSecret.Data).To(HaveLen(len(testAppEnvSecret.EnvironmentVariables)))
				})
			})

			When("the secret does exist", func() {
				const (
					key0              = "KEY0"
					expectedMapLength = 3
				)
				var originalEnvVars map[string]string
				BeforeEach(func() {
					originalEnvVars = map[string]string{
						key0: "VAL0",
						key1: "original-value", // This variable will change after the manifest is applied
					}
					originalSecret := corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      testAppEnvSecretName,
							Namespace: space1.Name,
							Labels: map[string]string{
								CFAppGUIDLabel: testAppGUID,
							},
						},
						StringData: originalEnvVars,
					}
					Expect(
						k8sClient.Create(context.Background(), &originalSecret),
					).To(Succeed())
				})

				It("returns a record matching the input and no error", func() {
					Expect(returnedErr).NotTo(HaveOccurred())
					Expect(returnedAppEnvVarsRecord.AppGUID).To(Equal(testAppEnvSecret.AppGUID))
					Expect(returnedAppEnvVarsRecord.Name).ToNot(BeEmpty())
					Expect(returnedAppEnvVarsRecord.SpaceGUID).To(Equal(testAppEnvSecret.SpaceGUID))
					Expect(returnedAppEnvVarsRecord.EnvironmentVariables).To(Equal(map[string]string{
						key0: originalEnvVars[key0],
						key1: requestEnvVars[key1],
						key2: requestEnvVars[key2],
					}))
				})

				It("eventually creates a secret that matches the request record", func() {
					cfAppSecretLookupKey := types.NamespacedName{Name: testAppEnvSecretName, Namespace: space1.Name}

					var updatedSecret corev1.Secret
					Eventually(func() error {
						err := k8sClient.Get(context.Background(), cfAppSecretLookupKey, &updatedSecret)
						if err == nil && len(updatedSecret.Data) != expectedMapLength {
							return errors.New("the data entries in the secret were not updated")
						}
						return err
					}, timeCheckThreshold*time.Second).Should(Succeed())

					Expect(updatedSecret).ToNot(BeZero())
					Expect(updatedSecret.Name).To(Equal(testAppEnvSecretName))
					Expect(updatedSecret.Labels).To(HaveKeyWithValue(CFAppGUIDLabel, testAppGUID))
					Expect(updatedSecret.Data).To(HaveLen(expectedMapLength))
					Expect(updatedSecret.Data).To(HaveKey(key1))
					Expect(string(updatedSecret.Data[key1])).To(Equal(requestEnvVars[key1]))
					Expect(updatedSecret.Data).To(HaveKey(key2))
					Expect(string(updatedSecret.Data[key2])).To(Equal(requestEnvVars[key2]))
					Expect(updatedSecret.Data).To(HaveKey(key0))
					Expect(string(updatedSecret.Data[key0])).To(Equal(originalEnvVars[key0]))
				})
			})
		})

		When("the user is not authorized in the space", func() {
			It("returns a forbidden error", func() {
				Expect(returnedErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})
	})

	Describe("SetCurrentDroplet", func() {
		const (
			appGUID     = "the-app-guid"
			dropletGUID = "the-droplet-guid"
			spaceGUID   = "default"
		)

		var (
			appCR     *workloadsv1alpha1.CFApp
			dropletCR *workloadsv1alpha1.CFBuild
		)

		BeforeEach(func() {
			beforeCtx := context.Background()
			appCR = createAppCR(beforeCtx, k8sClient, "some-app", appGUID, spaceGUID, CFAppStoppedState)
			dropletCR = createDropletCR(beforeCtx, k8sClient, dropletGUID, appGUID, spaceGUID)
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(context.Background(), appCR)).To(Succeed())
			Expect(k8sClient.Delete(context.Background(), dropletCR)).To(Succeed())
		})

		When("user has the space developer role", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, spaceGUID)
			})

			It("returns a CurrentDroplet record", func() {
				record, err := appRepo.SetCurrentDroplet(testCtx, authInfo, SetCurrentDropletMessage{
					AppGUID:     appGUID,
					DropletGUID: dropletGUID,
					SpaceGUID:   spaceGUID,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(record).To(Equal(CurrentDropletRecord{
					AppGUID:     appGUID,
					DropletGUID: dropletGUID,
				}))
			})

			It("sets the spec.current_droplet_ref.name to the Droplet GUID", func() {
				_, err := appRepo.SetCurrentDroplet(testCtx, authInfo, SetCurrentDropletMessage{
					AppGUID:     appGUID,
					DropletGUID: dropletGUID,
					SpaceGUID:   spaceGUID,
				})
				Expect(err).NotTo(HaveOccurred())

				lookupKey := types.NamespacedName{Name: appGUID, Namespace: spaceGUID}
				updatedApp := new(workloadsv1alpha1.CFApp)
				Eventually(func() error {
					return k8sClient.Get(context.Background(), lookupKey, updatedApp)
				}, 10*time.Second, 250*time.Millisecond).ShouldNot(HaveOccurred())
				Expect(updatedApp.Spec.CurrentDropletRef.Name).To(Equal(dropletGUID))
			})

			When("the app doesn't exist", func() {
				It("errors", func() {
					_, err := appRepo.SetCurrentDroplet(testCtx, authInfo, SetCurrentDropletMessage{
						AppGUID:     "no-such-app",
						DropletGUID: dropletGUID,
						SpaceGUID:   spaceGUID,
					})
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(ContainSubstring("not found")))
				})
			})
		})

		When("the user is not authorized", func() {
			It("errors", func() {
				_, err := appRepo.SetCurrentDroplet(testCtx, authInfo, SetCurrentDropletMessage{
					AppGUID:     appGUID,
					DropletGUID: dropletGUID,
					SpaceGUID:   spaceGUID,
				})
				Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})
	})

	Describe("SetDesiredState", func() {
		const (
			appName         = "some-app"
			spaceGUID       = "default"
			appStartedValue = "STARTED"
			appStoppedValue = "STOPPED"
		)

		var (
			appGUID           string
			returnedAppRecord *AppRecord
			returnedErr       error
			initialAppState   string
			desiredAppState   string
		)

		BeforeEach(func() {
			initialAppState = appStartedValue
			desiredAppState = appStartedValue
		})

		JustBeforeEach(func() {
			beforeCtx := context.Background()
			appGUID = generateGUID()
			_ = createAppCR(beforeCtx, k8sClient, appName, appGUID, spaceGUID, initialAppState)
			appRecord, err := appRepo.SetAppDesiredState(beforeCtx, authInfo, SetAppDesiredStateMessage{
				AppGUID:      appGUID,
				SpaceGUID:    spaceGUID,
				DesiredState: desiredAppState,
			})
			returnedAppRecord = &appRecord
			returnedErr = err
		})

		When("the user has permission to set the app state", func() {
			BeforeEach(func() {
				createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, spaceGUID)
			})

			When("starting an app", func() {
				BeforeEach(func() {
					initialAppState = appStoppedValue
				})

				It("doesn't return an error", func() {
					Expect(returnedErr).ToNot(HaveOccurred())
				})

				It("returns the updated app record", func() {
					Expect(returnedAppRecord.GUID).To(Equal(appGUID))
					Expect(returnedAppRecord.Name).To(Equal(appName))
					Expect(returnedAppRecord.SpaceGUID).To(Equal(spaceGUID))
					Expect(returnedAppRecord.State).To(Equal(DesiredState("STARTED")))
				})

				It("eventually changes the desired state of the App", func() {
					cfAppLookupKey := types.NamespacedName{Name: appGUID, Namespace: spaceGUID}
					updatedCFApp := new(workloadsv1alpha1.CFApp)
					Eventually(func() string {
						err := k8sClient.Get(context.Background(), cfAppLookupKey, updatedCFApp)
						if err != nil {
							return ""
						}
						return string(updatedCFApp.Spec.DesiredState)
					}, 10*time.Second, 250*time.Millisecond).Should(Equal(appStartedValue))
				})
			})

			When("stopping an app", func() {
				BeforeEach(func() {
					desiredAppState = appStoppedValue
				})

				It("doesn't return an error", func() {
					Expect(returnedErr).ToNot(HaveOccurred())
				})

				It("returns the updated app record", func() {
					Expect(returnedAppRecord.GUID).To(Equal(appGUID))
					Expect(returnedAppRecord.Name).To(Equal(appName))
					Expect(returnedAppRecord.SpaceGUID).To(Equal(spaceGUID))
					Expect(returnedAppRecord.State).To(Equal(DesiredState("STOPPED")))
				})

				It("eventually changes the desired state of the App", func() {
					cfAppLookupKey := types.NamespacedName{Name: appGUID, Namespace: spaceGUID}
					updatedCFApp := new(workloadsv1alpha1.CFApp)
					Eventually(func() string {
						err := k8sClient.Get(context.Background(), cfAppLookupKey, updatedCFApp)
						if err != nil {
							return ""
						}
						return string(updatedCFApp.Spec.DesiredState)
					}, 10*time.Second, 250*time.Millisecond).Should(Equal(appStoppedValue))
				})
			})

			When("the app doesn't exist", func() {
				It("returns an error", func() {
					_, err := appRepo.SetAppDesiredState(context.Background(), authInfo, SetAppDesiredStateMessage{
						AppGUID:      "fake-app-guid",
						SpaceGUID:    spaceGUID,
						DesiredState: appStartedValue,
					})

					Expect(err).To(MatchError(ContainSubstring("\"fake-app-guid\" not found")))
				})
			})
		})

		When("not allowed to set the application state", func() {
			It("returns a forbidden error", func() {
				Expect(returnedErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})
	})

	Describe("DeleteApp", func() {
		var appGUID string

		BeforeEach(func() {
			appGUID = generateGUID()
			_ = createAppCR(context.Background(), k8sClient, "some-app", appGUID, space1.Name, CFAppStoppedState)
			createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space1.Name)
		})

		When("on the happy path", func() {
			It("deletes the CFApp resource", func() {
				err := appRepo.DeleteApp(testCtx, authInfo, DeleteAppMessage{
					AppGUID:   appGUID,
					SpaceGUID: space1.Name,
				})
				Expect(err).NotTo(HaveOccurred())
			})
		})

		When("the app doesn't exist", func() {
			It("errors", func() {
				err := appRepo.DeleteApp(testCtx, authInfo, DeleteAppMessage{
					AppGUID:   "no-such-app",
					SpaceGUID: space1.Name,
				})
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ContainSubstring("not found")))
			})
		})
	})

	Describe("GetAppEnv", func() {
		When("the app exists", func() {
			var (
				envVars map[string]string
				secret  *corev1.Secret
			)

			BeforeEach(func() {
				cfApp1 = createApp(space1.Name)
				cfApp2 = createApp(space2.Name)

				envVars = map[string]string{
					"RAILS_ENV": "production",
					"LUNCHTIME": "12:00",
				}

				secret = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "the-env-secret",
						Namespace: space2.Name,
					},
					StringData: envVars,
				}

				Expect(
					k8sClient.Create(context.Background(), secret),
				).To(Succeed())

				appRepo = NewAppRepo(namespaceRetriever, userClientFactory, nsPerms)
			})

			AfterEach(func() {
				Expect(k8sClient.Delete(context.Background(), cfApp1)).To(Succeed())
				Expect(k8sClient.Delete(context.Background(), cfApp2)).To(Succeed())
			})

			When("the user can read secrets in the space", func() {
				BeforeEach(func() {
					createRoleBinding(testCtx, userName, spaceDeveloperRole.Name, space2.Name)
				})

				When("the EnvSecret exists", func() {
					BeforeEach(func() {
						ogCFApp2 := cfApp2.DeepCopy()
						cfApp2.Spec.EnvSecretName = secret.Name
						Expect(
							k8sClient.Patch(context.Background(), cfApp2, client.MergeFrom(ogCFApp2)),
						).To(Succeed())
					})

					It("returns the env vars stored on the secret", func() {
						res, err := appRepo.GetAppEnv(testCtx, authInfo, cfApp2.Name)
						Expect(err).NotTo(HaveOccurred())
						Expect(res).To(Equal(envVars))
					})
				})

				When("the EnvSecret doesn't exist", func() {
					BeforeEach(func() {
						ogCFApp2 := cfApp2.DeepCopy()
						cfApp2.Spec.EnvSecretName = "no-such-secret"
						Expect(
							k8sClient.Patch(context.Background(), cfApp2, client.MergeFrom(ogCFApp2)),
						).To(Succeed())
					})

					It("errors", func() {
						_, err := appRepo.GetAppEnv(testCtx, authInfo, cfApp2.Name)
						Expect(err).To(MatchError(ContainSubstring("Secret")))
					})
				})

				When("EnvSecretName is blank", func() {
					BeforeEach(func() {
						ogCFApp2 := cfApp2.DeepCopy()
						cfApp2.Spec.EnvSecretName = ""
						Expect(
							k8sClient.Patch(context.Background(), cfApp2, client.MergeFrom(ogCFApp2)),
						).To(Succeed())
					})

					It("returns an empty map", func() {
						res, err := appRepo.GetAppEnv(testCtx, authInfo, cfApp2.Name)
						Expect(err).NotTo(HaveOccurred())
						Expect(res).To(BeEmpty())
					})
				})
			})

			When("the user doesn't have permission to get secrets in the space", func() {
				BeforeEach(func() {
					createRoleBinding(testCtx, userName, spaceAuditorRole.Name, space2.Name)
				})

				When("the EnvSecret exists", func() {
					BeforeEach(func() {
						ogCFApp2 := cfApp2.DeepCopy()
						cfApp2.Spec.EnvSecretName = secret.Name
						Expect(
							k8sClient.Patch(context.Background(), cfApp2, client.MergeFrom(ogCFApp2)),
						).To(Succeed())
					})

					It("errors", func() {
						_, err := appRepo.GetAppEnv(testCtx, authInfo, cfApp2.Name)
						Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
					})
				})
			})
		})

		When("no Apps exist", func() {
			It("returns an error", func() {
				_, err := appRepo.GetAppEnv(testCtx, authInfo, "i don't exist")
				Expect(err).To(HaveOccurred())
				Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})
	})
})

func createApp(space string) *workloadsv1alpha1.CFApp {
	return createAppWithGUID(space, generateGUID())
}

func createAppWithGUID(space, guid string) *workloadsv1alpha1.CFApp {
	cfApp := &workloadsv1alpha1.CFApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      guid,
			Namespace: space,
			Annotations: map[string]string{
				CFAppRevisionKey: CFAppRevisionValue,
			},
		},
		Spec: workloadsv1alpha1.CFAppSpec{
			Name:         generateGUID(),
			DesiredState: "STOPPED",
			Lifecycle: workloadsv1alpha1.Lifecycle{
				Type: "buildpack",
				Data: workloadsv1alpha1.LifecycleData{
					Buildpacks: []string{"java"},
				},
			},
			CurrentDropletRef: corev1.LocalObjectReference{
				Name: generateGUID(),
			},
		},
	}
	Expect(k8sClient.Create(context.Background(), cfApp)).To(Succeed())

	return cfApp
}
