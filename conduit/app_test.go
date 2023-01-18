package conduit_test

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/alphagov/paas-cf-conduit/client"
	"github.com/alphagov/paas-cf-conduit/client/clientfakes"
	"github.com/alphagov/paas-cf-conduit/conduit"
	"github.com/alphagov/paas-cf-conduit/util"

	cfclient "github.com/cloudfoundry-community/go-cfclient"
)

var _ = Describe("Conduit App", func() {
	var (
		fakeClient *clientfakes.FakeClient
		org *cfclient.Org
		space *cfclient.Space
		serviceInstances map[string]*cfclient.ServiceInstance
		status *util.Status
		conduitApp *conduit.App

		newAppArgCfClient *clientfakes.FakeClient
		newAppArgStatus *util.Status
		newAppArgLocalPort int64
		newAppArgOrgName string
		newAppArgSpaceName string
		newAppArgAppName string
		newAppArgDeleteApp bool
		newAppArgServiceInstanceNames []string
		newAppArgRunArgs []string
		newAppArgBindParameters map[string]interface{}
		newAppArgTlsInsecure bool
		newAppArgTlsCipherSuites []uint16
		newAppArgTlsMinVersion uint16

		appInitErr error
		appInitAllowErr bool
	)

	BeforeEach(func() {
		org = &cfclient.Org{
			Name: "foo-org",
			Guid: "11111111-1111-1111-1111-111111111111",
		}
		space = &cfclient.Space{
			Name: "bar-space",
			OrganizationGuid: org.Guid,
			Guid: "22222222-2222-2222-2222-222222222222",
		}
		serviceInstances = map[string]*cfclient.ServiceInstance{
			"dddddddd-dddd-dddd-dddd-dddddddddddd": &cfclient.ServiceInstance{
				Name: "my-service-d",
				Guid: "dddddddd-dddd-dddd-dddd-dddddddddddd",
				SpaceGuid: space.Guid,
			},
			"eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee": &cfclient.ServiceInstance{
				Name: "my-service-e",
				Guid: "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee",
				SpaceGuid: space.Guid,
			},
			"ffffffff-ffff-ffff-ffff-ffffffffffff": &cfclient.ServiceInstance{
				Name: "my-service-f",
				Guid: "ffffffff-ffff-ffff-ffff-ffffffffffff",
				SpaceGuid: space.Guid,
			},
		}

		fakeClient = &clientfakes.FakeClient{}
		fakeClient.GetOrgByNameCalls(func(name string) (*cfclient.Org, error) {
			if name == org.Name {
				return org, nil
			}
			return nil, errors.New("Org not found")
		})
		fakeClient.GetSpaceByNameCalls(func(orgGuid, name string) (*cfclient.Space, error) {
			if orgGuid == space.OrganizationGuid && name == space.Name {
				return space, nil
			}
			return nil, errors.New("Space not found")
		})
		fakeClient.GetServiceInstancesCalls(func(filters ...string) (map[string]*cfclient.ServiceInstance, error) {
			if len(filters) == 1 && filters[0] == "space_guid:" + space.Guid {
				return serviceInstances, nil
			}
			return map[string]*cfclient.ServiceInstance{}, nil
		})

		status = util.NewStatus(GinkgoWriter, true)
		DeferCleanup(func() {
			status.Done()
		})

		newAppArgCfClient = fakeClient
		newAppArgStatus = status
		newAppArgLocalPort = 31415
		newAppArgOrgName = org.Name
		newAppArgSpaceName = space.Name
        newAppArgAppName = "baz-app"
		newAppArgDeleteApp = true
		newAppArgServiceInstanceNames = []string{"my-service-e"}
		newAppArgRunArgs = []string{}
		newAppArgBindParameters = map[string]interface{}{
			"seven": "eight",
		}
		newAppArgTlsInsecure = false
		newAppArgTlsCipherSuites = []uint16{123,456}
		newAppArgTlsMinVersion = 555
	})

	JustBeforeEach(func() {
		By("initializing the conduit App", func () {
			conduitApp = conduit.NewApp(
				newAppArgCfClient,
				newAppArgStatus,
				newAppArgLocalPort,
				newAppArgOrgName,
				newAppArgSpaceName,
				newAppArgAppName,
				newAppArgDeleteApp,
				newAppArgServiceInstanceNames,
				newAppArgRunArgs,
				newAppArgBindParameters,
				newAppArgTlsInsecure,
				newAppArgTlsCipherSuites,
				newAppArgTlsMinVersion,
			)
			Expect(conduitApp).ToNot(BeNil())

			appInitErr = conduitApp.Init()
			if !appInitAllowErr {
				Expect(appInitErr).ToNot(HaveOccurred())
			}
		})
	})

	When("initializing an App", func() {
		It("retrieved the specified org and space", func() {
			Expect(fakeClient.GetOrgByNameCallCount()).To(Equal(1))
			Expect(fakeClient.GetOrgByNameArgsForCall(0)).To(Equal(org.Name))
			Expect(fakeClient.GetSpaceByNameCallCount()).To(Equal(1))
			arg1, arg2 := fakeClient.GetSpaceByNameArgsForCall(0)
			Expect(arg1).To(Equal(org.Guid))
			Expect(arg2).To(Equal(space.Name))
		})
	})

	When("deploying a new app", func () {
		var (
			deployAppErr error
			deployAppAllowErr bool
		)

		BeforeEach(func () {
			fakeClient.CreateAppReturns("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", nil)
			fakeClient.UploadStaticAppBitsReturns(nil)
			fakeClient.StartAppReturns(nil)
			fakeClient.PollForAppStateReturns(nil)

			bindingCredentials := &client.Credentials{
				"hostname": "123.123.210.210",
				"port": "6543",
			}
			fakeClient.BindServiceReturns(bindingCredentials, nil)
		})

		JustBeforeEach(func () {
			By("calling DeployApp", func () {
				deployAppErr = conduitApp.DeployApp()
				if !deployAppAllowErr {
					Expect(deployAppErr).ToNot(HaveOccurred())
				}
			})
		})

		Context("happy path", func () {
			It("made the expected client calls", func () {
				var arg1, arg2, arg3 interface{}

				Expect(fakeClient.CreateAppCallCount()).To(Equal(1))
				arg1, arg2 = fakeClient.CreateAppArgsForCall(0)
				Expect(arg1).To(Equal("baz-app"))
				Expect(arg2).To(Equal(space.Guid))

				Expect(fakeClient.UploadStaticAppBitsCallCount()).To(Equal(1))
				arg1 = fakeClient.UploadStaticAppBitsArgsForCall(0)
				Expect(arg1).To(Equal("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"))

				Expect(fakeClient.StartAppCallCount()).To(Equal(1))
				arg1 = fakeClient.StartAppArgsForCall(0)
				Expect(arg1).To(Equal("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"))

				Expect(fakeClient.PollForAppStateCallCount()).To(Equal(1))
				arg1, arg2, arg3 = fakeClient.PollForAppStateArgsForCall(0)
				Expect(arg1).To(Equal("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"))
				Expect(arg2).To(Equal("STARTED"))
				Expect(arg3).To(Equal(15))

				Expect(fakeClient.GetServiceInstancesCallCount()).To(Equal(1))
				arg1 = fakeClient.GetServiceInstancesArgsForCall(0)
				Expect(arg1).To(Equal([]string{"space_guid:22222222-2222-2222-2222-222222222222"}))

				Expect(fakeClient.BindServiceCallCount()).To(Equal(1))
				arg1, arg2, arg3 = fakeClient.BindServiceArgsForCall(0)
				Expect(arg1).To(Equal("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"))
				Expect(arg2).To(Equal("eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"))
				Expect(arg3).To(Equal(map[string]interface{}{
					"seven": "eight",
				}))

				Expect(len(fakeClient.Invocations())).To(Equal(8))
			})
		})

		When("app with requested name already exists", func () {
			BeforeEach(func () {
				fakeClient.CreateAppReturns("", errors.New("foo"))

				deployAppAllowErr = true
			})

			It("returned the correct error and made expected client calls", func () {
				Expect(deployAppErr).To(HaveOccurred())
				Expect(deployAppErr.Error()).To(ContainSubstring("foo"))

				var arg1, arg2 interface{}

				Expect(fakeClient.CreateAppCallCount()).To(Equal(1))
				arg1, arg2 = fakeClient.CreateAppArgsForCall(0)
				Expect(arg1).To(Equal("baz-app"))
				Expect(arg2).To(Equal(space.Guid))

				Expect(len(fakeClient.Invocations())).To(Equal(3))
			})
		})

		When("the requested service doesn't exist in the space", func () {
			BeforeEach(func () {
				serviceInstances = map[string]*cfclient.ServiceInstance{
					"dddddddd-dddd-dddd-dddd-dddddddddddd": &cfclient.ServiceInstance{
						Name: "my-service-d",
						Guid: "dddddddd-dddd-dddd-dddd-dddddddddddd",
						SpaceGuid: space.Guid,
					},
					"ffffffff-ffff-ffff-ffff-ffffffffffff": &cfclient.ServiceInstance{
						Name: "my-service-f",
						Guid: "ffffffff-ffff-ffff-ffff-ffffffffffff",
						SpaceGuid: space.Guid,
					},
				}

				deployAppAllowErr = true
			})

			It("returned the correct error and made expected client calls", func () {
				Expect(deployAppErr).To(HaveOccurred())
				Expect(deployAppErr.Error()).To(ContainSubstring("was not found in space"))

				var arg1, arg2, arg3 interface{}

				Expect(fakeClient.CreateAppCallCount()).To(Equal(1))
				arg1, arg2 = fakeClient.CreateAppArgsForCall(0)
				Expect(arg1).To(Equal("baz-app"))
				Expect(arg2).To(Equal(space.Guid))

				Expect(fakeClient.UploadStaticAppBitsCallCount()).To(Equal(1))
				arg1 = fakeClient.UploadStaticAppBitsArgsForCall(0)
				Expect(arg1).To(Equal("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"))

				Expect(fakeClient.StartAppCallCount()).To(Equal(1))
				arg1 = fakeClient.StartAppArgsForCall(0)
				Expect(arg1).To(Equal("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"))

				Expect(fakeClient.PollForAppStateCallCount()).To(Equal(1))
				arg1, arg2, arg3 = fakeClient.PollForAppStateArgsForCall(0)
				Expect(arg1).To(Equal("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"))
				Expect(arg2).To(Equal("STARTED"))
				Expect(arg3).To(Equal(15))

				Expect(fakeClient.GetServiceInstancesCallCount()).To(Equal(1))
				arg1 = fakeClient.GetServiceInstancesArgsForCall(0)
				Expect(arg1).To(Equal([]string{"space_guid:22222222-2222-2222-2222-222222222222"}))

				Expect(len(fakeClient.Invocations())).To(Equal(7))
			})
		})
	})

	When("using an existing app", func () {
		var (
			prepareForExistingAppErr error
			prepareForExistingAppAllowErr bool
		)

		BeforeEach(func () {
			fakeClient.GetAppByNameReturns(&cfclient.App{
				Name: "baz-app",
				Guid: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
			}, nil)
			fakeClient.GetServiceBindingsReturns(map[string]*cfclient.ServiceBinding{
				"ffffffff-ffff-ffff-ffff-ffffffffffff": &cfclient.ServiceBinding{
					Guid: "11111111-ffff-ffff-ffff-ffffffffffff",
					ServiceInstanceGuid: "ffffffff-ffff-ffff-ffff-ffffffffffff",
				},
				"eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee": &cfclient.ServiceBinding{
					Guid: "11111111-eeee-eeee-eeee-eeeeeeeeeeee",
					ServiceInstanceGuid: "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee",
				},
			}, nil)
		})

		JustBeforeEach(func () {
			By("calling PrepareForExistingApp", func () {
				prepareForExistingAppErr = conduitApp.PrepareForExistingApp()
				if !prepareForExistingAppAllowErr {
					Expect(prepareForExistingAppErr).ToNot(HaveOccurred())
				}
			})
		})

		Context("happy path", func () {
			It("made the expected client calls", func () {
				var arg1, arg2, arg3 interface{}

				Expect(fakeClient.GetAppByNameCallCount()).To(Equal(1))
				arg1, arg2, arg3 = fakeClient.GetAppByNameArgsForCall(0)
				Expect(arg1).To(Equal(org.Guid))
				Expect(arg2).To(Equal(space.Guid))
				Expect(arg3).To(Equal(newAppArgAppName))

				Expect(fakeClient.GetServiceInstancesCallCount()).To(Equal(1))
				arg1 = fakeClient.GetServiceInstancesArgsForCall(0)
				Expect(arg1).To(Equal([]string{"space_guid:22222222-2222-2222-2222-222222222222"}))

				Expect(fakeClient.GetServiceBindingsCallCount()).To(Equal(1))
				arg1 = fakeClient.GetServiceBindingsArgsForCall(0)
				Expect(arg1).To(Equal([]string{"app_guid:bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"}))

				Expect(len(fakeClient.Invocations())).To(Equal(5))
			})
		})

		When("app is not bound to the requested service", func () {
			BeforeEach(func () {
				fakeClient.GetServiceBindingsReturns(map[string]*cfclient.ServiceBinding{
					"ffffffff-ffff-ffff-ffff-ffffffffffff": &cfclient.ServiceBinding{
						Guid: "11111111-ffff-ffff-ffff-ffffffffffff",
						ServiceInstanceGuid: "ffffffff-ffff-ffff-ffff-ffffffffffff",
					},
					"dddddddd-dddd-dddd-dddd-dddddddddddd": &cfclient.ServiceBinding{
						Guid: "11111111-dddd-dddd-dddd-dddddddddddd",
						ServiceInstanceGuid: "dddddddd-dddd-dddd-dddd-dddddddddddd",
					},
				}, nil)

				prepareForExistingAppAllowErr = true
			})

			It("returned the correct error and made expected client calls", func () {
				Expect(prepareForExistingAppErr).To(HaveOccurred())
				Expect(prepareForExistingAppErr.Error()).To(ContainSubstring("doesn't appear to be bound to service"))

				var arg1, arg2, arg3 interface{}

				Expect(fakeClient.GetAppByNameCallCount()).To(Equal(1))
				arg1, arg2, arg3 = fakeClient.GetAppByNameArgsForCall(0)
				Expect(arg1).To(Equal(org.Guid))
				Expect(arg2).To(Equal(space.Guid))
				Expect(arg3).To(Equal(newAppArgAppName))

				Expect(fakeClient.GetServiceInstancesCallCount()).To(Equal(1))
				arg1 = fakeClient.GetServiceInstancesArgsForCall(0)
				Expect(arg1).To(Equal([]string{"space_guid:22222222-2222-2222-2222-222222222222"}))

				Expect(fakeClient.GetServiceBindingsCallCount()).To(Equal(1))
				arg1 = fakeClient.GetServiceBindingsArgsForCall(0)
				Expect(arg1).To(Equal([]string{"app_guid:bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"}))

				Expect(len(fakeClient.Invocations())).To(Equal(5))
			})

			When("the requested service doesn't exist in the space", func () {
				BeforeEach(func () {
					serviceInstances = map[string]*cfclient.ServiceInstance{
						"dddddddd-dddd-dddd-dddd-dddddddddddd": &cfclient.ServiceInstance{
							Name: "my-service-d",
							Guid: "dddddddd-dddd-dddd-dddd-dddddddddddd",
							SpaceGuid: space.Guid,
						},
						"ffffffff-ffff-ffff-ffff-ffffffffffff": &cfclient.ServiceInstance{
							Name: "my-service-f",
							Guid: "ffffffff-ffff-ffff-ffff-ffffffffffff",
							SpaceGuid: space.Guid,
						},
					}

					prepareForExistingAppAllowErr = true
				})

				It("returned the correct error and made expected client calls", func () {
					Expect(prepareForExistingAppErr).To(HaveOccurred())
					Expect(prepareForExistingAppErr.Error()).To(ContainSubstring("was not found in space"))

					var arg1, arg2, arg3 interface{}

					Expect(fakeClient.GetAppByNameCallCount()).To(Equal(1))
					arg1, arg2, arg3 = fakeClient.GetAppByNameArgsForCall(0)
					Expect(arg1).To(Equal(org.Guid))
					Expect(arg2).To(Equal(space.Guid))
					Expect(arg3).To(Equal(newAppArgAppName))

					Expect(fakeClient.GetServiceInstancesCallCount()).To(Equal(1))
					arg1 = fakeClient.GetServiceInstancesArgsForCall(0)
					Expect(arg1).To(Equal([]string{"space_guid:22222222-2222-2222-2222-222222222222"}))

					Expect(fakeClient.GetServiceBindingsCallCount()).To(Equal(1))
					arg1 = fakeClient.GetServiceBindingsArgsForCall(0)
					Expect(arg1).To(Equal([]string{"app_guid:bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"}))

					Expect(len(fakeClient.Invocations())).To(Equal(5))
				})
			})
		})
	})
})
