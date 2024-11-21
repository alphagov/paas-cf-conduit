package conduit

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/alphagov/paas-cf-conduit/client"
	"github.com/alphagov/paas-cf-conduit/logging"
	"github.com/alphagov/paas-cf-conduit/ssh"
	"github.com/alphagov/paas-cf-conduit/tls"
	"github.com/alphagov/paas-cf-conduit/util"
	"github.com/cloudfoundry/multierror"

	gocfclient "github.com/cloudfoundry-community/go-cfclient"
)

type AppExecution struct {
	ExitCode int
}

func (ae AppExecution) Error() string {
	return fmt.Sprintf("Exit with code %d", ae.ExitCode)
}

type App struct {
	cfClient             client.Client
	status               *util.Status
	nextPort             int64
	orgName              string
	spaceName            string
	appName              string
	bindParameters       map[string]interface{}
	deleteApp            bool
	serviceInstanceNames []string
	runArgs              []string
	program              string
	org                  *gocfclient.Org
	space                *gocfclient.Space
	appGUID              string
	appEnv               *client.Env
	serviceProviders     map[string]ServiceProvider
	runEnv               map[string]string
	forwardAddrs         []ssh.ForwardAddrs
	tunnel               *ssh.Tunnel
	tlsTunnels           []*tls.Tunnel
	tlsInsecure          bool
	tlsCipherSuites      []uint16
	tlsMinVersion        uint16
}

type ServiceProvider interface {
	IsTLSEnabled(creds client.Credentials) bool
	GetNonTLSClients() []string
	GetKnownClients() []string
	InitEnv(creds client.Credentials, env map[string]string) error
	Teardown() error
	AdditionalProgramArgs(serviceInstances []*client.VcapService) []string
}

func NewApp(
	cfClient client.Client,
	status *util.Status,
	localPort int64,
	orgName string,
	spaceName string,
	appName string,
	deleteApp bool,
	serviceInstanceNames []string,
	runArgs []string,
	bindParameters map[string]interface{},
	tlsInsecure bool,
	tlsCipherSuites []uint16,
	tlsMinVersion uint16,
) *App {
	var program string
	if len(runArgs) > 0 {
		program = runArgs[0]
	}
	return &App{
		cfClient:             cfClient,
		status:               status,
		nextPort:             localPort,
		orgName:              orgName,
		spaceName:            spaceName,
		appName:              appName,
		deleteApp:            deleteApp,
		serviceInstanceNames: serviceInstanceNames,
		runArgs:              runArgs,
		program:              program,
		serviceProviders:     make(map[string]ServiceProvider),
		runEnv:               make(map[string]string),
		forwardAddrs:         make([]ssh.ForwardAddrs, 0),
		bindParameters:       bindParameters,
		tlsInsecure:          tlsInsecure,
		tlsCipherSuites:      tlsCipherSuites,
		tlsMinVersion:        tlsMinVersion,
	}
}

func (a *App) RegisterServiceProvider(name string, serviceProvider ServiceProvider) {
	a.serviceProviders[name] = serviceProvider
}

func (a *App) Init() error {
	var err error
	// get org
	a.status.Text("Targeting org", a.orgName)
	a.org, err = a.cfClient.GetOrgByName(a.orgName)
	if err != nil {
		return err
	}
	// get space
	a.status.Text("Targeting space", a.spaceName)
	a.space, err = a.cfClient.GetSpaceByName(a.org.Guid, a.spaceName)
	if err != nil {
		return err
	}
	return nil
}

func (a *App) DeployApp() error {
	if err := a.deployApp(); err != nil {
		return err
	}

	if err := a.bindServices(); err != nil {
		return err
	}

	return nil
}

func (a *App) PrepareForExistingApp() error {
	a.status.Text("Fetching information about app", a.appName)
	app, err := a.cfClient.GetAppByName(a.org.Guid, a.space.Guid, a.appName)
	if err != nil {
		return err
	}

	a.appGUID = app.Guid

	// check it's actually bound to the requested services
	a.status.Text("Fetching service infomation")
	serviceInstances, err := a.cfClient.GetServiceInstances(
		fmt.Sprintf("space_guid:%s", a.space.Guid),
	)
	if err != nil {
		return err
	}

	a.status.Text("Fetching binding infomation")
	serviceBindings, err := a.cfClient.GetServiceBindings(
		fmt.Sprintf("app_guid:%s", a.appGUID),
	)
	if err != nil {
		return err
	}

	for _, serviceInstanceName := range a.serviceInstanceNames {
		serviceInstanceGUID := ""
		for siGUID, serviceInstance := range serviceInstances {
			if serviceInstance.Name == serviceInstanceName {
				serviceInstanceGUID = siGUID
				break
			}
		}
		if serviceInstanceGUID == "" {
			return fmt.Errorf(
				"Service '%s' was not found in space '%s'\n",
				serviceInstanceName,
				a.space.Name,
			)
		}

		if _, ok := serviceBindings[serviceInstanceGUID]; !ok {
			return fmt.Errorf(
				"App '%s' doesn't appear to be bound to service '%s'. Using an existing app requires the app to have an existing binding to the desired service(s).\n",
				a.appName,
				serviceInstanceName,
			)
		}
	}

	return nil
}

func (a *App) deployApp() error {
	var err error
	a.status.Text("Deploying", a.appName)
	a.appGUID, err = a.cfClient.CreateApp(a.appName, a.space.Guid)
	if err != nil {
		return err
	}

	// upload bits if not staged
	a.status.Text("Uploading", a.appName, "bits")
	if err := a.cfClient.UploadStaticAppBits(a.appGUID); err != nil {
		return err
	}
	// start app
	a.status.Text("Starting", a.appName)
	if err := a.cfClient.StartApp(a.appGUID); err != nil {
		return err
	}

	// poll for started state
	a.status.Text("Waiting for conduit app to become available")
	if err := a.cfClient.PollForAppState(a.appGUID, "STARTED", 15); err != nil {
		return err
	}

	return nil
}

func (a *App) destroyApp() error {
	logging.Debug("destroying", a.appName, a.appGUID)
	if err := a.cfClient.DestroyApp(a.appGUID); err != nil {
		logging.Debug("failed to delete app", a.appName, "err:", err)

		logging.Debug("refreshing auth token")
		if err := a.cfClient.RefreshAccessToken(); err != nil {
			logging.Debug("failed to refresh access token, err:", err)
			return fmt.Errorf("failed to delete %s app, please delete it manually\n", a.appName)
		}

		if err := a.cfClient.DestroyApp(a.appGUID); err != nil {
			logging.Debug("failed to delete app", a.appName, "err:", err)
			return fmt.Errorf("failed to delete %s app, please delete it manually\n", a.appName)
		}
	}
	return nil
}

func (a *App) bindServices() error {
	var err error
	// get service instances
	a.status.Text("Fetching service information")
	serviceInstances, err := a.cfClient.GetServiceInstances(
		fmt.Sprintf("space_guid:%s", a.space.Guid),
	)
	if err != nil {
		return err
	}

	for _, name := range a.serviceInstanceNames {
		bound := false
		for serviceInstanceGUID, serviceInstance := range serviceInstances {
			if name != serviceInstance.Name {
				continue
			}
			// bind conduit app to service instance
			a.status.Text("Binding", serviceInstance.Name)
			logging.Debug("binding", serviceInstanceGUID, "to", a.appGUID)
			creds, err := a.cfClient.BindService(a.appGUID, serviceInstanceGUID, a.bindParameters)
			if err != nil {
				return err
			}
			if creds.Host() == "" || creds.Port() == 0 {
				return fmt.Errorf("%s service is missing host, hostname or port", name)
			}
			bound = true
		}
		if !bound {
			return fmt.Errorf("failed to bind service: '%s' was not found in space '%s'", name, a.space.Name)
		}
	}
	return nil
}

func (a *App) getAllValidServiceTypesForProgram(program string) []string {
	validServiceTypes := []string{}
	for serviceType, serviceProvider := range a.serviceProviders {
		for _, knownClient := range serviceProvider.GetKnownClients() {
			if knownClient == program {
				validServiceTypes = append(validServiceTypes, serviceType)
			}
		}
	}
	return validServiceTypes
}

// responsible for populating:
// - a.appEnv with a `.SystemEnv.VcapServices` that is pruned of irrelevant
//   binding credentials and has relevant binding credentials modified to
//   target the local tunnel
// - a.runEnv["VCAP_SERVICES"] with a jsonified version of the above
// - a.forwardAddrs with the details of the planned tunnels
func (a *App) initServiceBindings() error {
	// fetch the full app env
	a.status.Text("Fetching environment")
	if appEnv, err := a.cfClient.GetAppEnv(a.appGUID); err == nil {
		a.appEnv = appEnv
	} else {
		return err
	}

	// if only for the sake of easier testing, choose a deterministic
	// order to iterate through VcapServices as it affects e.g. port
	// number selection
	orderedVcapServicesKeys := []string{}
	for k, _ := range a.appEnv.SystemEnv.VcapServices {
		orderedVcapServicesKeys = append(orderedVcapServicesKeys, k)
	}
	sort.Strings(orderedVcapServicesKeys)

	unsatisfiedServiceInstanceNames := map[string]bool{}
	for _, name := range a.serviceInstanceNames {
		unsatisfiedServiceInstanceNames[name] = true
	}
	programServiceTypeSatisfied := false
	for _, serviceName := range orderedVcapServicesKeys {
		keptServiceInstances := []*client.VcapService{}
		for _, si := range a.appEnv.SystemEnv.VcapServices[serviceName] {
			if _, ok := unsatisfiedServiceInstanceNames[si.InstanceName]; !ok {
				continue
			}
			// now satisfied
			delete(unsatisfiedServiceInstanceNames, si.InstanceName)

			if serviceProvider, ok := a.serviceProviders[serviceName]; !ok {
				return fmt.Errorf(
					"App %s: service instance %s is of unknown type %s, don't know how to handle its credentials",
					a.appName,
					si.InstanceName,
					serviceName,
				)
			} else {
				// this one is relevant to our interests
				keptServiceInstances = append(keptServiceInstances, si)

				forwardAddr := ssh.ForwardAddrs{
					RemoteAddr: fmt.Sprintf("%s:%d", si.Credentials.Host(), si.Credentials.Port()),
					LocalPort:  a.nextPort,
				}
				logging.Debug("remote address for tunnel will be", forwardAddr.RemoteAddr)
				a.nextPort++

				createTLSTunnel := false
				for _, nonTLSClient := range serviceProvider.GetNonTLSClients() {
					if nonTLSClient == a.program {
						createTLSTunnel = true
						break
					}
				}

				if serviceName == "redis" && serviceProvider.IsTLSEnabled(si.Credentials) && createTLSTunnel {
					forwardAddr.TLSTunnelPort = a.nextPort
					a.nextPort++
				}

				a.forwardAddrs = append(a.forwardAddrs, forwardAddr)

				si.Credentials.SetAddress("127.0.0.1", forwardAddr.ConnectPort())

				if !programServiceTypeSatisfied {
					for _, knownClient := range serviceProvider.GetKnownClients() {
						if knownClient == a.program {
							serviceProvider.InitEnv(si.Credentials, a.runEnv)
							programServiceTypeSatisfied = true
							break
						}
					}
				}
			}
		}

		// discard service instances we didn't match
		if len(keptServiceInstances) == 0 {
			// drop map entry entirely if none kept
			delete(a.appEnv.SystemEnv.VcapServices, serviceName)
		} else {
			a.appEnv.SystemEnv.VcapServices[serviceName] = keptServiceInstances
		}
	}
	if len(unsatisfiedServiceInstanceNames) != 0 {
		names := []string{}
		for k, _ := range unsatisfiedServiceInstanceNames {
			names = append(names, k)
		}

		return fmt.Errorf(
			"App %s: can't find binding information for services: %s",
			a.appName,
			strings.Join(names, ", "),
		)
	}
	if a.program != "" && !programServiceTypeSatisfied {
		validServiceTypes := a.getAllValidServiceTypesForProgram(a.program)
		if len(validServiceTypes) == 0 {
			return fmt.Errorf(
				"Unknown program %s: can't determine what service types it expects",
				a.program,
 			) 
		}

		return fmt.Errorf(
			"%s program expects one of the following service types: %s",
			a.program,
			strings.Join(validServiceTypes, ", "),
		)
	}

	logging.Debug("runenv", a.runEnv)

	// add modified VCAP_SERVICES to environment
	if b, err := json.Marshal(a.appEnv.SystemEnv.VcapServices); err != nil {
		return fmt.Errorf("failed to marshal VCAP_SERVICES: %s", err)
	} else {
		a.runEnv["VCAP_SERVICES"] = string(b)
		logging.Debug("VCAP_SERVICES", string(b))
	}

	return nil
}

func (a *App) PrintConnectionInfo() {
	fmt.Fprintf(os.Stderr, "\nThe following services are ready for you to connect to:\n\n")
	for serviceType, serviceInstances := range a.appEnv.SystemEnv.VcapServices {
		for _, si := range serviceInstances {
			fmt.Fprintf(os.Stderr, "* service: %s (%s)\n", si.Name, serviceType)
			si.Credentials.Fprint(os.Stderr, "  ")
			fmt.Fprintln(os.Stderr)
		}
	}
}

func (a *App) SetupTunnels() error {
	if err := a.initServiceBindings(); err != nil {
		return err
	}

	if err := a.startSSHTunnels(); err != nil {
		return err
	}

	if err := a.startTLSTunnels(); err != nil {
		return err
	}

	return nil
}

func (a *App) startSSHTunnels() error {
	a.tunnel = &ssh.Tunnel{
		AppGuid:       a.appGUID,
		TunnelAddr:    a.cfClient.AppSSHEndpoint(),
		TunnelHostKey: a.cfClient.AppSSHHostKeyFingerprint(),
		ForwardAddrs:  a.forwardAddrs,
		PasswordFunc:  a.cfClient.SSHCode,
	}

	// start the tunnel
	a.status.Text("Starting port forwarding")
	if err := a.tunnel.Start(); err != nil {
		return err
	}

	// wait for port forwarding to become active
	a.status.Text("Waiting for port forwarding")
	for _, fwd := range a.tunnel.ForwardAddrs {
		select {
		case err := <-util.WaitForConnection(fwd.LocalAddress()):
			if err != nil {
				return err
			}
		case err := <-a.tunnel.WaitChan():
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (a *App) startTLSTunnels() error {
	// Start TLS proxies
	for _, addr := range a.forwardAddrs {
		if addr.TLSTunnelPort == 0 {
			continue
		}

		tlsTunnel := tls.NewTunnel(addr.TLSTunnelAddress(), addr.LocalAddress(), addr.RemoteAddr, a.tlsInsecure, a.tlsCipherSuites, a.tlsMinVersion)
		_, err := tlsTunnel.Start()
		if err != nil {
			return err
		}
		a.tlsTunnels = append(a.tlsTunnels, tlsTunnel)

		err = <-util.WaitForConnection(addr.TLSTunnelAddress())
		if err != nil {
			return err
		}
	}

	return nil
}

func (a *App) RunCommand() error {
	// execute CMD with environment
	a.status.Text("Preparing command:", strings.Join(a.runArgs, " "))

	extraArgs := a.getProgramSpecificArgs(a.program)
	runArgs := append([]string{a.program}, extraArgs...)
	runArgs = append(runArgs, a.runArgs[1:]...)

	exe, err := exec.LookPath(a.program)
	if err != nil {
		return fmt.Errorf("cannot find '%s' in PATH", a.program)
	}

	logging.Debug("running command", exe, strings.Join(runArgs[1:], " "))

	proc := exec.Command(exe, runArgs[1:]...)
	proc.Env = os.Environ()
	for k, v := range a.runEnv {
		proc.Env = append(proc.Env, fmt.Sprintf("%s=%s", k, v))
	}
	proc.Stdout = os.Stdout
	proc.Stdin = os.Stdin
	proc.Stderr = os.Stderr

	a.status.Done()

	if err := proc.Start(); err != nil {
		return fmt.Errorf("%s: %s", exe, err)
	}

	proc.Wait()

	if proc.ProcessState != nil {
		exitCode := proc.ProcessState.ExitCode()

		if exitCode != 0 {
			return AppExecution{ExitCode: exitCode}
		}
	}

	return nil
}

func (a *App) getProgramSpecificArgs(program string) []string {
	for serviceName, provider := range a.serviceProviders {
		for _, knownProgram := range provider.GetKnownClients() {
			if knownProgram == program {
				serviceInstances := a.appEnv.SystemEnv.VcapServices[serviceName]
				return provider.AdditionalProgramArgs(serviceInstances)
			}
		}
	}
	return nil
}

func (a *App) Teardown() error {
	errs := &multierror.MultiError{}

	for _, tlsTunnel := range a.tlsTunnels {
		if err := tlsTunnel.Stop(); err != nil {
			errs.Add(err)
		}
	}

	if a.tunnel != nil {
		if err := a.tunnel.Stop(); err != nil {
			errs.Add(err)
		}
	}

	for _, sp := range a.serviceProviders {
		if err := sp.Teardown(); err != nil {
			errs.Add(err)
		}
	}

	if a.deleteApp && a.appGUID != "" {
		if err := a.destroyApp(); err != nil {
			errs.Add(err)
		}
	}
	if len(errs.Errors) > 0 {
		return errs
	}
	return nil
}
