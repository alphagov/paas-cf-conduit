package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

const tunnelInfo = `
The following services are ready for you to connect to:

{{range $serviceName, $services := .VcapServices}}
	{{range $serviceIndex, $service := $services}}
		service: {{$service.Name}} ({{$serviceName}})
		host: {{$service.Credentials.Host}}
		port: {{$service.Credentials.Port}}
		username: {{$service.Credentials.Username}}
		password: {{$service.Credentials.Password}}
		db: {{$service.Credentials.Name}}
	{{end}}
{{end}}
`

func waitForConnection(addr string) chan error {
	timeout := 3 * time.Second
	connection := make(chan error)
	go func() {
		defer close(connection)
		tries := 0
		for {
			if tries > 5 {
				time.Sleep(2 * time.Second)
			} else {
				time.Sleep(1 * time.Second)
			}
			tries++
			debug("waiting for", addr, "attempt", tries)
			conn, err := net.DialTimeout("tcp", addr, timeout)
			if err != nil {
				if tries < 15 {
					continue
				}
				connection <- fmt.Errorf("connection fail after %d attempts: %s", tries, err)
				break
			}
			defer conn.Close()
			connection <- nil
			break
		}
	}()
	return connection
}

var ConnectService = &cobra.Command{
	Use: "conduit [flags] SERVICE_INSTANCE [-- COMMAND]",
	Example: `  Create a tunnel between your machine and a remote running service:
  cf conduit my-service
  
  Run a local application that uses VCAP_SERVICES environment to connect to remote services:
  cf conduit my-service-1 my-service-2 -- /path/to/app
    
  Export a postgres database:
  cf conduit postgres-instance -- pg_dump -f backup.sql

  Import a postgres script:
  cf conduit postgres-instance -- psql < backup.sql

  Dump a mysql database:
  cf conduit mysql-instance -- mysqldump --all-databases > backup.sql

  Import a mysql script:
  cf conduit mysql-instance -- psql < backup.psql
  `,
	Short: "enables temporarily binding services to local running processes",
	Long:  "spawns a temporary application, binds your desired service and creates an ssh tunnel from the application to your local machine enabling communication directly with the remote service.",
	Args: func(cmd *cobra.Command, args []string) error {
		if cmd.ArgsLenAtDash() > -1 {
			if cmd.ArgsLenAtDash() < 1 {
				return errors.New("requires at least one SERVICE_INSTANCE argument to be specified")
			}
		} else if len(args) < 1 {
			return errors.New("requires at least one SERVICE_INSTANCE argument to be specified")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// create status writer
		status := NewStatus(os.Stderr)
		defer status.Done()
		// parse args
		var serviceInstanceNames []string
		var runargs []string
		if cmd.ArgsLenAtDash() > -1 {
			serviceInstanceNames = args[:cmd.ArgsLenAtDash()]
			runargs = args[cmd.ArgsLenAtDash():]
		} else {
			serviceInstanceNames = args
			runargs = []string{}
		}
		// create a client
		status.Text("Connecting client")
		client, err := NewClient(ApiEndpoint, ApiToken, ApiInsecure)
		if err != nil {
			return err
		}
		// get org
		status.Text("Targeting org", ConduitOrg)
		org, err := client.GetOrgByName(ConduitOrg)
		if err != nil {
			return err
		}
		// get space
		status.Text("Targeting space", ConduitSpace)
		space, err := client.GetSpaceByName(org.Guid, ConduitSpace)
		if err != nil {
			return err
		}
		// create tunnel app
		status.Text("Deploying", ConduitAppName)
		appGuid, err := client.CreateApp(ConduitAppName, space.Guid)
		if err != nil {
			return err
		}
		defer func() {
			if ConduitReuse {
				debug("destroying", ConduitAppName, appGuid)
				if err := client.DestroyApp(appGuid); err != nil {
					debug("failed to cleanup", ConduitAppName, "app:", err)
				}
			}
		}()
		// upload bits if not staged
		status.Text("Uploading", ConduitAppName, "bits")
		err = client.UploadStaticAppBits(appGuid)
		if err != nil {
			return err
		}
		// start app
		status.Text("Starting", ConduitAppName)
		err = client.UpdateApp(appGuid, map[string]interface{}{
			"state": "STARTED",
		})
		if err != nil {
			return err
		}
		// get service instances
		status.Text("Fetching service infomation")
		serviceInstances, err := client.GetServiceInstances(fmt.Sprintf("space_guid:%s", space.Guid))
		if err != nil {
			return err
		}
		// configure tunnel
		t := &Tunnel{
			AppGuid:       appGuid,
			TunnelAddr:    client.Info.AppSshEndpoint,
			TunnelHostKey: client.Info.AppSshHostKey,
			ForwardAddrs:  []ForwardAddrs{},
			PasswordFunc:  client.SSHCode,
		}
		// for each service instance
		localPort := ConduitLocalPort
		for _, name := range serviceInstanceNames {
			bound := false
			for serviceInstanceGuid, serviceInstance := range serviceInstances {
				if name != serviceInstance.Name {
					continue
				}
				// bind conduit app to service instance
				status.Text("Binding", serviceInstance.Name)
				debug("binding", serviceInstanceGuid, "to", appGuid)
				creds, err := client.BindService(appGuid, serviceInstanceGuid)
				if err != nil {
					return err
				}
				// configure the port forwarding
				debug("creds", creds)
				localPort++
				t.ForwardAddrs = append(t.ForwardAddrs, ForwardAddrs{
					LocalAddr:   fmt.Sprintf("127.0.0.1:%d", localPort),
					RemoteAddr:  fmt.Sprintf("%s:%d", creds.Host, creds.Port),
					Credentials: creds,
				})
				bound = true
			}
			if !bound {
				return fmt.Errorf("failed to bind service: '%s' was not found in space '%s'", name, space.Name)
			}
		}
		// fetch the full app env
		status.Text("Fetching environment")
		appEnv, err := client.GetAppEnv(appGuid)
		if err != nil {
			return err
		}
		// configure the environment
		runenv := map[string]string{}
		for serviceName, serviceInstances := range appEnv.SystemEnv.VcapServices {
			for _, si := range serviceInstances {
				// modify
				si.Credentials.Host = "127.0.0.1"
				si.Credentials.Port = localPort
				switch serviceName {
				case "postgres":
					runenv["PGDATABASE"] = si.Credentials.Name
					runenv["PGHOST"] = si.Credentials.Host
					runenv["PGPORT"] = fmt.Sprintf("%d", si.Credentials.Port)
					runenv["PGUSER"] = si.Credentials.Username
					runenv["PGPASSWORD"] = si.Credentials.Password
				case "mysql":
					tmpdir, err := ioutil.TempDir("", "conduit")
					if err != nil {
						return err
					}
					defer os.RemoveAll(tmpdir)
					mycnfPath := filepath.Join(tmpdir, "my.cnf")
					mycnf := "[mysql]\n"
					mycnf += fmt.Sprintf("user = %s\n", si.Credentials.Username)
					mycnf += fmt.Sprintf("password = %s\n", si.Credentials.Password)
					mycnf += fmt.Sprintf("host = %s\n", si.Credentials.Host)
					mycnf += fmt.Sprintf("port = %d\n", si.Credentials.Port)
					mycnf += fmt.Sprintf("database = %s\n", si.Credentials.Name)
					mycnf += "[mysqldump]\n"
					mycnf += fmt.Sprintf("user = %s\n", si.Credentials.Username)
					mycnf += fmt.Sprintf("password = %s\n", si.Credentials.Password)
					mycnf += fmt.Sprintf("host = 127.0.0.1\n")
					mycnf += fmt.Sprintf("port = %d\n", localPort)
					if err := ioutil.WriteFile(mycnfPath, []byte(mycnf), 0644); err != nil {
						return fmt.Errorf("failed to create temporary mysql config: %s", err)
					}
					runenv["MYSQL_HOME"] = tmpdir
				}
			}
		}
		debug("runenv", runenv)
		// poll for started state
		status.Text("Waiting for conduit app to become available")
		err = client.PollForAppState(appGuid, "STARTED", 15)
		if err != nil {
			return err
		}
		// start the tunnel
		status.Text("Starting port forwarding")
		err = t.Start()
		if err != nil {
			return err
		}
		defer t.Stop()
		// wait for port forwarding to become active
		status.Text("Waiting for port forwarding")
		for _, fwd := range t.ForwardAddrs {
			select {
			case err := <-waitForConnection(fwd.LocalAddr):
				if err != nil {
					return err
				}
			case err := <-t.WaitChan():
				if err != nil {
					return err
				}
			}
		}
		status.Done()
		// add modified VCAP_SERVICES to environment
		if b, err := json.Marshal(appEnv.SystemEnv.VcapServices); err != nil {
			return fmt.Errorf("failed to marshal VCAP_SERVICES: %s", err)
		} else {
			runenv["VCAP_SERVICES"] = string(b)
			debug("VCAP_SERVICES", string(b))
		}
		// render message about ports
		if Verbose || len(runargs) == 0 {
			t := template.Must(template.New("tunnelInfo").Parse(tunnelInfo))
			var out bytes.Buffer
			t.Execute(&out, appEnv.SystemEnv)
			fmt.Fprintln(os.Stderr, out.String())
		}
		// execute CMD with enviornment
		runargChan := make(chan struct{})
		if len(runargs) > 0 {
			status.Text("Preparing command:", runargs)
			exe, err := exec.LookPath(runargs[0])
			if err != nil {
				return fmt.Errorf("cannot find '%s' in PATH")
			}
			proc := exec.Command(exe, runargs[1:]...)
			proc.Env = os.Environ()
			for k, v := range runenv {
				proc.Env = append(proc.Env, fmt.Sprintf("%s=%s", k, v))
			}
			proc.Stdout = os.Stdout
			proc.Stdin = os.Stdin
			proc.Stderr = os.Stderr
			status.Done()
			debug("running", runargs)
			if err := proc.Start(); err != nil {
				return fmt.Errorf("%s: %s", exe, err)
			}
			go func() {
				defer close(runargChan)
				proc.Wait()
			}()
		} else {
			fmt.Fprintln(os.Stderr, "\n\nPress Ctrl+C to shutdown.")
		}
		// wait
		select {
		case <-shutdown:
			return nil
		case <-runargChan:
			return nil
		}
	},
}
