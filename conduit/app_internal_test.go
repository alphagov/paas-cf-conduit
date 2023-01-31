package conduit

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/alphagov/paas-cf-conduit/client/clientfakes"
	"github.com/alphagov/paas-cf-conduit/client"
	"github.com/alphagov/paas-cf-conduit/service"
	"github.com/alphagov/paas-cf-conduit/ssh"
	"github.com/alphagov/paas-cf-conduit/util"
)

var _ = Describe("Conduit App (internal behaviour)", func() {
	Describe("initServiceBindings()", func () {
		var (
			app *App
			fakeClient *clientfakes.FakeClient
			credentials *client.Credentials
			clientEnv *client.Env
		)

		BeforeEach(func () {
			status := util.NewStatus(GinkgoWriter, true)
			DeferCleanup(func() {
				status.Done()
			})
			fakeClient = &clientfakes.FakeClient{}
			app = &App{
				cfClient: fakeClient,
				status: status,
				serviceInstanceNames: []string{
					"my-service-foo",
				},
				program: "psql",
				appName: "my-app-bar",
				appGUID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
				serviceProviders: map[string]ServiceProvider{},
				runEnv: map[string]string{
					"USER": "human",
					"EDITOR": "ed",
				},
				nextPort: 9933,
			}

			app.RegisterServiceProvider("mysql", &service.MySQL{})
			app.RegisterServiceProvider("postgres", &service.Postgres{})
			app.RegisterServiceProvider("redis", &service.Redis{})
			app.RegisterServiceProvider("influxdb", &service.InfluxDB{})

			credentials = &client.Credentials{}
			clientEnv = &client.Env{
				SystemEnv: &client.SystemEnv{
					VcapServices: map[string][]*client.VcapService{
						"postgres": []*client.VcapService{
							&client.VcapService{
								Name: "some-binding",
								InstanceName: "my-service-foo",
								Credentials: *credentials,
							},
							&client.VcapService{
								Name: "another-binding",
								InstanceName: "other-service-bar",
							},
						},
						"unknown-service": []*client.VcapService{},
					},
				},
			}

			(*credentials)["host"] = "10.9.8.7"
			(*credentials)["port"] = "6543"
			(*credentials)["db"] = "some-database-789"
			(*credentials)["username"] = "some-user-xyz"
			(*credentials)["passwd"] = "cheese-abc"
			(*credentials)["url"] = "foo://10.9.8.7:6543/blah"
		})

		JustBeforeEach(func () {
			fakeClient.GetAppEnvReturns(clientEnv, nil)
		})

		Context("simple happy case", func () {
			It("processes credentials and variables correctly", func () {
				err := app.initServiceBindings()
				Expect(err).ToNot(HaveOccurred())

				var arg1 interface{}

				Expect(fakeClient.GetAppEnvCallCount()).To(Equal(1))
				arg1 = fakeClient.GetAppEnvArgsForCall(0)
				Expect(arg1).To(Equal("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"))

				Expect(len(fakeClient.Invocations())).To(Equal(1))

				Expect(app.runEnv).To(HaveKeyWithValue("USER", "human"))
				Expect(app.runEnv).To(HaveKeyWithValue("EDITOR", "ed"))

				Expect(app.runEnv).To(HaveKeyWithValue("PGUSER", "some-user-xyz"))
				Expect(app.runEnv).To(HaveKeyWithValue("PGPASSWORD", "cheese-abc"))
				Expect(app.runEnv).To(HaveKeyWithValue("PGDATABASE", "some-database-789"))
				Expect(app.runEnv).To(HaveKeyWithValue("PGHOST", "127.0.0.1"))
				Expect(app.runEnv).To(HaveKeyWithValue("PGPORT", "9933"))

				Expect(app.runEnv["VCAP_SERVICES"]).To(MatchJSON(`{
					"postgres": [
						{
							"name": "some-binding",
							"credentials": {
								"host": "127.0.0.1",
								"port": "9933",
								"db": "some-database-789",
								"username": "some-user-xyz",
								"passwd": "cheese-abc",
								"url": "foo://127.0.0.1:9933/blah"
							},
							"instance_name": "my-service-foo"
						}
					]
				}`))

				Expect(len(app.appEnv.SystemEnv.VcapServices)).To(Equal(1))
				Expect(len(app.appEnv.SystemEnv.VcapServices["postgres"])).To(Equal(1))
				Expect(app.appEnv.SystemEnv.VcapServices["postgres"][0].Credentials.Host()).To(Equal("127.0.0.1"))
				Expect(app.appEnv.SystemEnv.VcapServices["postgres"][0].Credentials.Port()).To(Equal(int64(9933)))
				Expect(app.appEnv.SystemEnv.VcapServices["postgres"][0].Credentials.Database()).To(Equal("some-database-789"))
				Expect(app.appEnv.SystemEnv.VcapServices["postgres"][0].Credentials.Username()).To(Equal("some-user-xyz"))
				Expect(app.appEnv.SystemEnv.VcapServices["postgres"][0].Credentials.Password()).To(Equal("cheese-abc"))
				Expect(app.appEnv.SystemEnv.VcapServices["postgres"][0].Credentials.URI()).To(Equal("foo://127.0.0.1:9933/blah"))

				Expect(len(app.forwardAddrs)).To(Equal(1))
				Expect(app.forwardAddrs[0]).To(Equal(ssh.ForwardAddrs{
					LocalPort: int64(9933),
					RemoteAddr: "10.9.8.7:6543",
				}))
			})
		})

		When("app is not bound to any services", func () {
			BeforeEach(func () {
				clientEnv.SystemEnv.VcapServices = map[string][]*client.VcapService{}
			})

			It("returns the correct error", func () {
				err := app.initServiceBindings()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("can't find binding information"))
			})
		})

		When("service is a different type to that expected by program", func () {
			BeforeEach(func () {
				clientEnv.SystemEnv.VcapServices = map[string][]*client.VcapService{
					"mysql": []*client.VcapService{
						&client.VcapService{
							Name: "some-binding",
							InstanceName: "my-service-foo",
							Credentials: *credentials,
						},
					},
				}
			})

			It("returns the correct error", func () {
				err := app.initServiceBindings()
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("psql program expects one of the following service types: postgres"))
			})
		})

		When("service is of a completely unrecognized type", func () {
			BeforeEach(func () {
				clientEnv.SystemEnv.VcapServices = map[string][]*client.VcapService{
					"waffle": []*client.VcapService{
						&client.VcapService{
							Name: "some-binding",
							InstanceName: "my-service-foo",
							Credentials: *credentials,
						},
					},
				}
			})

			It("returns the correct error", func () {
				err := app.initServiceBindings()
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("App my-app-bar: service instance my-service-foo is of unknown type waffle, don't know how to handle its credentials"))
			})
		})

		When("multiple services of different types are requested", func () {
			var (
				credentialsMysql *client.Credentials
			)

			BeforeEach(func () {
				app.serviceInstanceNames = append(app.serviceInstanceNames, "my-other-service-baz")
				credentialsMysql = &client.Credentials{}
				clientEnv.SystemEnv.VcapServices["mysql"] = []*client.VcapService{
					&client.VcapService{
						Name: "another-binding",
						InstanceName: "my-other-service-baz",
						Credentials: *credentialsMysql,
					},
				}

				(*credentialsMysql)["host"] = "6.5.4.3"
				(*credentialsMysql)["hostname"] = "6.5.4.3"  // deliberate duplication
				(*credentialsMysql)["port"] = "7788"
				(*credentialsMysql)["database"] = "other-database-123"
				(*credentialsMysql)["user"] = "other-user-qux"
				(*credentialsMysql)["password"] = "fondue-999"
				(*credentialsMysql)["uri"] = "bla://6.5.4.3:7788/moo"
				(*credentialsMysql)["unexpected"] = "string-with-6.5.4.3:7788"
			})

			It("processes credentials and variables correctly", func () {
				err := app.initServiceBindings()
				Expect(err).ToNot(HaveOccurred())

				var arg1 interface{}

				Expect(fakeClient.GetAppEnvCallCount()).To(Equal(1))
				arg1 = fakeClient.GetAppEnvArgsForCall(0)
				Expect(arg1).To(Equal("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"))

				Expect(len(fakeClient.Invocations())).To(Equal(1))

				Expect(app.runEnv).To(HaveKeyWithValue("USER", "human"))
				Expect(app.runEnv).To(HaveKeyWithValue("EDITOR", "ed"))

				Expect(app.runEnv).To(HaveKeyWithValue("PGUSER", "some-user-xyz"))
				Expect(app.runEnv).To(HaveKeyWithValue("PGPASSWORD", "cheese-abc"))
				Expect(app.runEnv).To(HaveKeyWithValue("PGDATABASE", "some-database-789"))
				Expect(app.runEnv).To(HaveKeyWithValue("PGHOST", "127.0.0.1"))
				Expect(app.runEnv).To(HaveKeyWithValue("PGPORT", "9934"))

				// only env vars for correct program should be present
				Expect(app.runEnv).ToNot(HaveKey("MYSQL_HOME"))

				Expect(app.runEnv["VCAP_SERVICES"]).To(MatchJSON(`{
					"mysql": [
						{
							"name": "another-binding",
							"credentials": {
								"database": "other-database-123",
								"host": "127.0.0.1",
								"hostname": "127.0.0.1",
								"password": "fondue-999",
								"port": "9933",
								"unexpected": "string-with-127.0.0.1:9933",
								"uri": "bla://127.0.0.1:9933/moo",
								"user": "other-user-qux"
							},
							"instance_name": "my-other-service-baz"
						}
					],
					"postgres": [
						{
							"name": "some-binding",
							"credentials": {
								"db": "some-database-789",
								"host": "127.0.0.1",
								"passwd": "cheese-abc",
								"port": "9934",
								"url": "foo://127.0.0.1:9934/blah",
								"username": "some-user-xyz"
							},
							"instance_name": "my-service-foo"
						}
					]
				}`))

				Expect(len(app.appEnv.SystemEnv.VcapServices)).To(Equal(2))
				Expect(len(app.appEnv.SystemEnv.VcapServices["mysql"])).To(Equal(1))
				Expect(app.appEnv.SystemEnv.VcapServices["mysql"][0].Credentials.Host()).To(Equal("127.0.0.1"))
				Expect(app.appEnv.SystemEnv.VcapServices["mysql"][0].Credentials.Port()).To(Equal(int64(9933)))
				Expect(app.appEnv.SystemEnv.VcapServices["mysql"][0].Credentials.Database()).To(Equal("other-database-123"))
				Expect(app.appEnv.SystemEnv.VcapServices["mysql"][0].Credentials.Username()).To(Equal("other-user-qux"))
				Expect(app.appEnv.SystemEnv.VcapServices["mysql"][0].Credentials.Password()).To(Equal("fondue-999"))
				Expect(app.appEnv.SystemEnv.VcapServices["mysql"][0].Credentials.URI()).To(Equal("bla://127.0.0.1:9933/moo"))
				Expect(len(app.appEnv.SystemEnv.VcapServices["postgres"])).To(Equal(1))
				Expect(app.appEnv.SystemEnv.VcapServices["postgres"][0].Credentials.Host()).To(Equal("127.0.0.1"))
				Expect(app.appEnv.SystemEnv.VcapServices["postgres"][0].Credentials.Port()).To(Equal(int64(9934)))
				Expect(app.appEnv.SystemEnv.VcapServices["postgres"][0].Credentials.Database()).To(Equal("some-database-789"))
				Expect(app.appEnv.SystemEnv.VcapServices["postgres"][0].Credentials.Username()).To(Equal("some-user-xyz"))
				Expect(app.appEnv.SystemEnv.VcapServices["postgres"][0].Credentials.Password()).To(Equal("cheese-abc"))
				Expect(app.appEnv.SystemEnv.VcapServices["postgres"][0].Credentials.URI()).To(Equal("foo://127.0.0.1:9934/blah"))

				Expect(len(app.forwardAddrs)).To(Equal(2))
				Expect(app.forwardAddrs[0]).To(Equal(ssh.ForwardAddrs{
					LocalPort: int64(9933),
					RemoteAddr: "6.5.4.3:7788",
				}))
				Expect(app.forwardAddrs[1]).To(Equal(ssh.ForwardAddrs{
					LocalPort: int64(9934),
					RemoteAddr: "10.9.8.7:6543",
				}))
			})

			When("the supplied program is mysql", func () {
				BeforeEach(func () {
					app.program = "mysql"
				})

				It("processes credentials and variables correctly", func () {
					err := app.initServiceBindings()
					Expect(err).ToNot(HaveOccurred())

					var arg1 interface{}

					Expect(fakeClient.GetAppEnvCallCount()).To(Equal(1))
					arg1 = fakeClient.GetAppEnvArgsForCall(0)
					Expect(arg1).To(Equal("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"))

					Expect(len(fakeClient.Invocations())).To(Equal(1))

					Expect(app.runEnv).To(HaveKeyWithValue("USER", "human"))
					Expect(app.runEnv).To(HaveKeyWithValue("EDITOR", "ed"))

					Expect(app.runEnv).To(HaveKey("MYSQL_HOME"))
					Expect(app.runEnv["MYSQL_HOME"]).To(BeADirectory())
					Expect(app.runEnv["MYSQL_HOME"] + "/my.cnf").To(BeARegularFile())

					// only env vars for correct program should be present
					Expect(app.runEnv).ToNot(HaveKey("PGUSER"))
					Expect(app.runEnv).ToNot(HaveKey("PGPASSWORD"))
					Expect(app.runEnv).ToNot(HaveKey("PGDATABASE"))
					Expect(app.runEnv).ToNot(HaveKey("PGHOST"))
					Expect(app.runEnv).ToNot(HaveKey("PGPORT"))

					Expect(app.runEnv["VCAP_SERVICES"]).To(MatchJSON(`{
						"mysql": [
							{
								"name": "another-binding",
								"credentials": {
									"database": "other-database-123",
									"host": "127.0.0.1",
									"hostname": "127.0.0.1",
									"password": "fondue-999",
									"port": "9933",
									"unexpected": "string-with-127.0.0.1:9933",
									"uri": "bla://127.0.0.1:9933/moo",
									"user": "other-user-qux"
								},
								"instance_name": "my-other-service-baz"
							}
						],
						"postgres": [
							{
								"name": "some-binding",
								"credentials": {
									"db": "some-database-789",
									"host": "127.0.0.1",
									"passwd": "cheese-abc",
									"port": "9934",
									"url": "foo://127.0.0.1:9934/blah",
									"username": "some-user-xyz"
								},
								"instance_name": "my-service-foo"
							}
						]
					}`))

					Expect(len(app.appEnv.SystemEnv.VcapServices)).To(Equal(2))
					Expect(len(app.appEnv.SystemEnv.VcapServices["mysql"])).To(Equal(1))
					Expect(app.appEnv.SystemEnv.VcapServices["mysql"][0].Credentials.Host()).To(Equal("127.0.0.1"))
					Expect(app.appEnv.SystemEnv.VcapServices["mysql"][0].Credentials.Port()).To(Equal(int64(9933)))
					Expect(app.appEnv.SystemEnv.VcapServices["mysql"][0].Credentials.Database()).To(Equal("other-database-123"))
					Expect(app.appEnv.SystemEnv.VcapServices["mysql"][0].Credentials.Username()).To(Equal("other-user-qux"))
					Expect(app.appEnv.SystemEnv.VcapServices["mysql"][0].Credentials.Password()).To(Equal("fondue-999"))
					Expect(app.appEnv.SystemEnv.VcapServices["mysql"][0].Credentials.URI()).To(Equal("bla://127.0.0.1:9933/moo"))
					Expect(len(app.appEnv.SystemEnv.VcapServices["postgres"])).To(Equal(1))
					Expect(app.appEnv.SystemEnv.VcapServices["postgres"][0].Credentials.Host()).To(Equal("127.0.0.1"))
					Expect(app.appEnv.SystemEnv.VcapServices["postgres"][0].Credentials.Port()).To(Equal(int64(9934)))
					Expect(app.appEnv.SystemEnv.VcapServices["postgres"][0].Credentials.Database()).To(Equal("some-database-789"))
					Expect(app.appEnv.SystemEnv.VcapServices["postgres"][0].Credentials.Username()).To(Equal("some-user-xyz"))
					Expect(app.appEnv.SystemEnv.VcapServices["postgres"][0].Credentials.Password()).To(Equal("cheese-abc"))
					Expect(app.appEnv.SystemEnv.VcapServices["postgres"][0].Credentials.URI()).To(Equal("foo://127.0.0.1:9934/blah"))

					Expect(len(app.forwardAddrs)).To(Equal(2))
					Expect(app.forwardAddrs[0]).To(Equal(ssh.ForwardAddrs{
						LocalPort: int64(9933),
						RemoteAddr: "6.5.4.3:7788",
					}))
					Expect(app.forwardAddrs[1]).To(Equal(ssh.ForwardAddrs{
						LocalPort: int64(9934),
						RemoteAddr: "10.9.8.7:6543",
					}))
				})
			})

			When("there is no supplied program", func () {
				BeforeEach(func () {
					app.program = ""
				})

				It("processes credentials and variables correctly", func () {
					err := app.initServiceBindings()
					Expect(err).ToNot(HaveOccurred())

					var arg1 interface{}

					Expect(fakeClient.GetAppEnvCallCount()).To(Equal(1))
					arg1 = fakeClient.GetAppEnvArgsForCall(0)
					Expect(arg1).To(Equal("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"))

					Expect(len(fakeClient.Invocations())).To(Equal(1))

					Expect(app.runEnv).To(HaveKeyWithValue("USER", "human"))
					Expect(app.runEnv).To(HaveKeyWithValue("EDITOR", "ed"))

					// env vars for neither service should be present
					Expect(app.runEnv).ToNot(HaveKey("MYSQL_HOME"))

					Expect(app.runEnv).ToNot(HaveKey("PGUSER"))
					Expect(app.runEnv).ToNot(HaveKey("PGPASSWORD"))
					Expect(app.runEnv).ToNot(HaveKey("PGDATABASE"))
					Expect(app.runEnv).ToNot(HaveKey("PGHOST"))
					Expect(app.runEnv).ToNot(HaveKey("PGPORT"))

					Expect(app.runEnv["VCAP_SERVICES"]).To(MatchJSON(`{
						"mysql": [
							{
								"name": "another-binding",
								"credentials": {
									"database": "other-database-123",
									"host": "127.0.0.1",
									"hostname": "127.0.0.1",
									"password": "fondue-999",
									"port": "9933",
									"unexpected": "string-with-127.0.0.1:9933",
									"uri": "bla://127.0.0.1:9933/moo",
									"user": "other-user-qux"
								},
								"instance_name": "my-other-service-baz"
							}
						],
						"postgres": [
							{
								"name": "some-binding",
								"credentials": {
									"db": "some-database-789",
									"host": "127.0.0.1",
									"passwd": "cheese-abc",
									"port": "9934",
									"url": "foo://127.0.0.1:9934/blah",
									"username": "some-user-xyz"
								},
								"instance_name": "my-service-foo"
							}
						]
					}`))

					Expect(len(app.appEnv.SystemEnv.VcapServices)).To(Equal(2))
					Expect(len(app.appEnv.SystemEnv.VcapServices["mysql"])).To(Equal(1))
					Expect(app.appEnv.SystemEnv.VcapServices["mysql"][0].Credentials.Host()).To(Equal("127.0.0.1"))
					Expect(app.appEnv.SystemEnv.VcapServices["mysql"][0].Credentials.Port()).To(Equal(int64(9933)))
					Expect(app.appEnv.SystemEnv.VcapServices["mysql"][0].Credentials.Database()).To(Equal("other-database-123"))
					Expect(app.appEnv.SystemEnv.VcapServices["mysql"][0].Credentials.Username()).To(Equal("other-user-qux"))
					Expect(app.appEnv.SystemEnv.VcapServices["mysql"][0].Credentials.Password()).To(Equal("fondue-999"))
					Expect(app.appEnv.SystemEnv.VcapServices["mysql"][0].Credentials.URI()).To(Equal("bla://127.0.0.1:9933/moo"))
					Expect(len(app.appEnv.SystemEnv.VcapServices["postgres"])).To(Equal(1))
					Expect(app.appEnv.SystemEnv.VcapServices["postgres"][0].Credentials.Host()).To(Equal("127.0.0.1"))
					Expect(app.appEnv.SystemEnv.VcapServices["postgres"][0].Credentials.Port()).To(Equal(int64(9934)))
					Expect(app.appEnv.SystemEnv.VcapServices["postgres"][0].Credentials.Database()).To(Equal("some-database-789"))
					Expect(app.appEnv.SystemEnv.VcapServices["postgres"][0].Credentials.Username()).To(Equal("some-user-xyz"))
					Expect(app.appEnv.SystemEnv.VcapServices["postgres"][0].Credentials.Password()).To(Equal("cheese-abc"))
					Expect(app.appEnv.SystemEnv.VcapServices["postgres"][0].Credentials.URI()).To(Equal("foo://127.0.0.1:9934/blah"))

					Expect(len(app.forwardAddrs)).To(Equal(2))
					Expect(app.forwardAddrs[0]).To(Equal(ssh.ForwardAddrs{
						LocalPort: int64(9933),
						RemoteAddr: "6.5.4.3:7788",
					}))
					Expect(app.forwardAddrs[1]).To(Equal(ssh.ForwardAddrs{
						LocalPort: int64(9934),
						RemoteAddr: "10.9.8.7:6543",
					}))
				})
			})

			When("just one of the requested services isn't found", func () {
				BeforeEach(func () {
					app.serviceInstanceNames = append(app.serviceInstanceNames, "not-a-real-service")
				})

				It("returns the correct error", func () {
					err := app.initServiceBindings()
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("can't find binding information"))
				})
			})
		})

		When("multiple services of the same type are requested", func () {
			var (
				credentials2 *client.Credentials
			)

			BeforeEach(func () {
				app.serviceInstanceNames = append(app.serviceInstanceNames, "my-other-service-buz")
				credentials2 = &client.Credentials{}
				clientEnv.SystemEnv.VcapServices["postgres"] = append(
					clientEnv.SystemEnv.VcapServices["postgres"], 
					&client.VcapService{
						Name: "binding-abc",
						InstanceName: "my-other-service-buz",
						Credentials: *credentials2,
					},
				)

				(*credentials2)["host"] = "1.2.1.2"
				(*credentials2)["port"] = "4466"
				(*credentials2)["db"] = "this-database-888"
				(*credentials2)["username"] = "that-user-bla"
				(*credentials2)["passwd"] = "mildew-2"
				(*credentials2)["url"] = "postgresql://that-user-bla:mildew-2@1.2.1.2:4466/this-database-888"
			})

			It("processes credentials and variables correctly", func () {
				err := app.initServiceBindings()
				Expect(err).ToNot(HaveOccurred())

				var arg1 interface{}

				Expect(fakeClient.GetAppEnvCallCount()).To(Equal(1))
				arg1 = fakeClient.GetAppEnvArgsForCall(0)
				Expect(arg1).To(Equal("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"))

				Expect(len(fakeClient.Invocations())).To(Equal(1))

				Expect(app.runEnv).To(HaveKeyWithValue("USER", "human"))
				Expect(app.runEnv).To(HaveKeyWithValue("EDITOR", "ed"))

				// credentials in env correspond to the *first* service found
				// (matching the `program`)
				Expect(app.runEnv).To(HaveKeyWithValue("PGUSER", "some-user-xyz"))
				Expect(app.runEnv).To(HaveKeyWithValue("PGPASSWORD", "cheese-abc"))
				Expect(app.runEnv).To(HaveKeyWithValue("PGDATABASE", "some-database-789"))
				Expect(app.runEnv).To(HaveKeyWithValue("PGHOST", "127.0.0.1"))
				Expect(app.runEnv).To(HaveKeyWithValue("PGPORT", "9933"))

				Expect(app.runEnv["VCAP_SERVICES"]).To(MatchJSON(`{
					"postgres": [
						{
							"name": "some-binding",
							"credentials": {
								"db": "some-database-789",
								"host": "127.0.0.1",
								"passwd": "cheese-abc",
								"port": "9933",
								"url": "foo://127.0.0.1:9933/blah",
								"username": "some-user-xyz"
							},
							"instance_name": "my-service-foo"
						},
						{
							"name": "binding-abc",
							"credentials": {
								"db": "this-database-888",
								"host": "127.0.0.1",
								"passwd": "mildew-2",
								"port": "9934",
								"url": "postgresql://that-user-bla:mildew-2@127.0.0.1:9934/this-database-888",
								"username": "that-user-bla"
							},
							"instance_name": "my-other-service-buz"
						}
					]
				}`))

				Expect(len(app.appEnv.SystemEnv.VcapServices)).To(Equal(1))
				Expect(len(app.appEnv.SystemEnv.VcapServices["postgres"])).To(Equal(2))
				Expect(app.appEnv.SystemEnv.VcapServices["postgres"][0].Credentials.Host()).To(Equal("127.0.0.1"))
				Expect(app.appEnv.SystemEnv.VcapServices["postgres"][0].Credentials.Port()).To(Equal(int64(9933)))
				Expect(app.appEnv.SystemEnv.VcapServices["postgres"][0].Credentials.Database()).To(Equal("some-database-789"))
				Expect(app.appEnv.SystemEnv.VcapServices["postgres"][0].Credentials.Username()).To(Equal("some-user-xyz"))
				Expect(app.appEnv.SystemEnv.VcapServices["postgres"][0].Credentials.Password()).To(Equal("cheese-abc"))
				Expect(app.appEnv.SystemEnv.VcapServices["postgres"][0].Credentials.URI()).To(Equal("foo://127.0.0.1:9933/blah"))
				Expect(app.appEnv.SystemEnv.VcapServices["postgres"][1].Credentials.Host()).To(Equal("127.0.0.1"))
				Expect(app.appEnv.SystemEnv.VcapServices["postgres"][1].Credentials.Port()).To(Equal(int64(9934)))
				Expect(app.appEnv.SystemEnv.VcapServices["postgres"][1].Credentials.Database()).To(Equal("this-database-888"))
				Expect(app.appEnv.SystemEnv.VcapServices["postgres"][1].Credentials.Username()).To(Equal("that-user-bla"))
				Expect(app.appEnv.SystemEnv.VcapServices["postgres"][1].Credentials.Password()).To(Equal("mildew-2"))
				Expect(app.appEnv.SystemEnv.VcapServices["postgres"][1].Credentials.URI()).To(Equal("postgresql://that-user-bla:mildew-2@127.0.0.1:9934/this-database-888"))

				Expect(len(app.forwardAddrs)).To(Equal(2))
				Expect(app.forwardAddrs[0]).To(Equal(ssh.ForwardAddrs{
					LocalPort: int64(9933),
					RemoteAddr: "10.9.8.7:6543",
				}))
				Expect(app.forwardAddrs[1]).To(Equal(ssh.ForwardAddrs{
					LocalPort: int64(9934),
					RemoteAddr: "1.2.1.2:4466",
				}))
			})
		})

		When("the program is a known non-TLS client", func () {
			BeforeEach(func () {
				app.program = "redis-cli"

				clientEnv.SystemEnv.VcapServices = map[string][]*client.VcapService{
					"redis": []*client.VcapService{
						&client.VcapService{
							Name: "some-binding",
							InstanceName: "my-service-foo",
							Credentials: *credentials,
						},
					},
				}
			})

			When("the endpoint is already TLS-protected", func () {
				BeforeEach(func () {
					(*credentials)["uri"] = fmt.Sprintf(
						"rediss://%s:%d",
						credentials.Host(),
						credentials.Port(),
					)
				})

				It("prepares to set up a TLS tunnel", func () {
					err := app.initServiceBindings()
					Expect(err).ToNot(HaveOccurred())

					var arg1 interface{}

					Expect(fakeClient.GetAppEnvCallCount()).To(Equal(1))
					arg1 = fakeClient.GetAppEnvArgsForCall(0)
					Expect(arg1).To(Equal("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"))

					Expect(len(fakeClient.Invocations())).To(Equal(1))

					Expect(len(app.appEnv.SystemEnv.VcapServices)).To(Equal(1))
					Expect(len(app.appEnv.SystemEnv.VcapServices["redis"])).To(Equal(1))
					Expect(app.appEnv.SystemEnv.VcapServices["redis"][0].Credentials.Host()).To(Equal("127.0.0.1"))
					Expect(app.appEnv.SystemEnv.VcapServices["redis"][0].Credentials.Port()).To(Equal(int64(9934)))

					Expect(len(app.forwardAddrs)).To(Equal(1))
					Expect(app.forwardAddrs[0]).To(Equal(ssh.ForwardAddrs{
						LocalPort: int64(9933),
						TLSTunnelPort: int64(9934),
						RemoteAddr: "10.9.8.7:6543",
					}))
				})
			})

			When("the endpoint is not TLS-protected", func () {
				BeforeEach(func () {
					(*credentials)["uri"] = fmt.Sprintf(
						"redis://%s:%d",
						credentials.Host(),
						credentials.Port(),
					)
				})

				It("doesn't prepare for a TLS tunnel", func () {
					err := app.initServiceBindings()
					Expect(err).ToNot(HaveOccurred())

					var arg1 interface{}

					Expect(fakeClient.GetAppEnvCallCount()).To(Equal(1))
					arg1 = fakeClient.GetAppEnvArgsForCall(0)
					Expect(arg1).To(Equal("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"))

					Expect(len(fakeClient.Invocations())).To(Equal(1))

					Expect(len(app.appEnv.SystemEnv.VcapServices)).To(Equal(1))
					Expect(len(app.appEnv.SystemEnv.VcapServices["redis"])).To(Equal(1))
					Expect(app.appEnv.SystemEnv.VcapServices["redis"][0].Credentials.Host()).To(Equal("127.0.0.1"))
					Expect(app.appEnv.SystemEnv.VcapServices["redis"][0].Credentials.Port()).To(Equal(int64(9933)))

					Expect(len(app.forwardAddrs)).To(Equal(1))
					Expect(app.forwardAddrs[0]).To(Equal(ssh.ForwardAddrs{
						LocalPort: int64(9933),
						TLSTunnelPort: int64(0),
						RemoteAddr: "10.9.8.7:6543",
					}))
				})
			})
		})
	})
})
