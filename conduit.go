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

var (
	LocalPort int64
)

const tunnelInfo = `
The following services are ready for you to connect to:

{{range $serviceName, $services := .VcapServices}}
	{{range $serviceIndex, $service := $services}}
		service: {{$service.name}} ({{$serviceName}})
		host: {{$service.credentials.host}}
		port: {{$service.credentials.port}}
		username: {{$service.credentials.username}}
		password: {{$service.credentials.password}}
		db: {{$service.credentials.name}}
	{{end}}
{{end}}

Press Ctrl+C to shutdown.
`

func init() {
	ConnectService.Flags().Int64VarP(&LocalPort, "local-port", "p", 7080, "start selecting local ports from")
}

func waitForConnection(port int64) chan error {
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
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), timeout)
			if err != nil {
				if tries < 15 {
					continue
				}
				connection <- fmt.Errorf("timeout waiting for tunnel to start: %s", err)
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
		lg := func(args ...interface{}) {
			if Verbose {
				fmt.Fprintln(Stderr, args...)
			}
		}
		var serviceInstanceNames []string
		var runargs []string
		if cmd.ArgsLenAtDash() > -1 {
			serviceInstanceNames = args[:cmd.ArgsLenAtDash()]
			runargs = args[cmd.ArgsLenAtDash():]
		} else {
			serviceInstanceNames = args
			runargs = []string{}
		}
		cf := Cf{Cli: conn, Verbose: Verbose}

		tempAppDir, err := cf.generateEmptyApp()
		defer os.RemoveAll(tempAppDir)
		if err != nil {
			return err
		}

		appName := filepath.Base(tempAppDir)
		lg("Deploying tunnel", appName)
		err = cf.pushAppWithoutRoute(appName, tempAppDir)
		defer func() {
			lg("Destroying tunnel", appName)
			err = cf.forceDeleteAppWithRetries(appName, 3)
			if err != nil {
				lg("Run: `cf delete -f", appName, "` to remove artifact manually.")
				lg("Failed to shutdown cleanly:", err)
				os.Exit(2)
			}
		}()
		if err != nil {
			return err
		}

		lg("Getting ID of the temporary app", appName)
		appGUID, err := cf.getAppGUID(appName)
		if err != nil {
			return err
		}

		for _, serviceInstanceName := range serviceInstanceNames {
			lg("Binding service", serviceInstanceName)
			err = cf.bindService(appName, serviceInstanceName)
			if err != nil {
				return err
			}
		}

		lg("Fetching environment")
		appEnvJSON, err := cf.getAppEnv(appGUID)
		if err != nil {
			return err
		}

		lg("Parsing environment")
		appEnv := Env{}
		err = json.Unmarshal([]byte(appEnvJSON), &appEnv)
		if err != nil {
			return err
		}

		runenv := map[string]string{}
		sshErrs := make(chan error, 1)
		localPort := LocalPort
		for serviceName, services := range appEnv.SystemEnvJSON.VcapServices {
			for i, service := range services {
				lg("Configuring port forwarding for", serviceName, i)
				localPort++
				credentialsIface, ok := service["credentials"]
				if !ok {
					return fmt.Errorf("failed to get 'credentials' from VCAP_SERVICES environment")
				}
				credentials, ok := credentialsIface.(map[string]interface{})
				if !ok {
					return fmt.Errorf(`VCAP_SERVICES[%s][%d]["credentials"] was malformed`, serviceName, i)
				}
				portIface, ok := credentials["port"]
				if !ok {
					return fmt.Errorf(`VCAP_SERVICES[%s][%d]["credentials"]["port"] was missing`, serviceName, i)
				}
				portFloat, ok := portIface.(float64)
				if !ok {
					return fmt.Errorf(`VCAP_SERVICES[%s][%d]["credentials"]["port"] was not of type int got %v`, serviceName, i, portIface)
				}
				port := int64(portFloat)
				credentials["port"] = localPort
				hostIface, ok := credentials["host"]
				if !ok {
					return fmt.Errorf(`VCAP_SERVICES[%s][%d]["credentials"]["host"] was missing`, serviceName, i)
				}
				host, ok := hostIface.(string)
				if !ok {
					return fmt.Errorf(`VCAP_SERVICES[%s][%d]["credentials"]["host"] was not of type string`, serviceName, i)
				}
				credentials["host"] = "127.0.0.1"
				dbnameIface, ok := credentials["name"]
				if !ok {
					return fmt.Errorf(`VCAP_SERVICES[%s][%d]["credentials"]["name"] was missing`, serviceName, i)
				}
				dbname, ok := dbnameIface.(string)
				if !ok {
					return fmt.Errorf(`VCAP_SERVICES[%s][%d]["credentials"]["name"] was not of type string`, serviceName, i)
				}
				usernameIface, ok := credentials["username"]
				if !ok {
					return fmt.Errorf(`VCAP_SERVICES[%s][%d]["credentials"]["username"] was missing`, serviceName, i)
				}
				username, ok := usernameIface.(string)
				if !ok {
					return fmt.Errorf(`VCAP_SERVICES[%s][%d]["credentials"]["username"] was not of type string`, serviceName, i)
				}
				passwordIface, ok := credentials["password"]
				if !ok {
					return fmt.Errorf(`VCAP_SERVICES[%s][%d]["credentials"]["password"] was missing`, serviceName, i)
				}
				password, ok := passwordIface.(string)
				if !ok {
					return fmt.Errorf(`VCAP_SERVICES[%s][%d]["credentials"]["password"] was not of type string`, serviceName, i)
				}
				switch serviceName {
				case "postgres":
					runenv["PGDATABASE"] = dbname
					runenv["PGHOST"] = "127.0.0.1"
					runenv["PGPORT"] = fmt.Sprintf("%d", localPort)
					runenv["PGUSER"] = username
					runenv["PGPASSWORD"] = password
				case "mysql":
					mycnfPath := filepath.Join(tempAppDir, "my.cnf")
					mycnf := "[mysql]\n"
					mycnf += fmt.Sprintf("user = %s\n", username)
					mycnf += fmt.Sprintf("password = %s\n", password)
					mycnf += fmt.Sprintf("host = 127.0.0.1\n")
					mycnf += fmt.Sprintf("port = %d\n", localPort)
					mycnf += fmt.Sprintf("database = %s\n", dbname)
					mycnf += "[mysqldump]\n"
					mycnf += fmt.Sprintf("user = %s\n", username)
					mycnf += fmt.Sprintf("password = %s\n", password)
					mycnf += fmt.Sprintf("host = 127.0.0.1\n")
					mycnf += fmt.Sprintf("port = %d\n", localPort)
					if err := ioutil.WriteFile(mycnfPath, []byte(mycnf), 0644); err != nil {
						return fmt.Errorf("failed to create temporary mysql config: %s", err)
					}
					runenv["MYSQL_HOME"] = tempAppDir
				}

				ssh, err := cf.sshPortForward(appName, localPort, host, port)
				if err != nil {
					return fmt.Errorf("failed to setup port forwarding: %s", err)
				}
				err = ssh.Start()
				if err != nil {
					return fmt.Errorf("failed to setup port forwarding: %s", err)
				}
				go func() {
					sshErrs <- ssh.Wait()
				}()
				select {
				case err := <-waitForConnection(localPort):
					if err != nil {
						return err
					}
				case err := <-sshErrs:
					if err != nil {
						return err
					}
				}
			}
		}
		if b, err := json.Marshal(appEnv.SystemEnvJSON.VcapServices); err != nil {
			return fmt.Errorf("failed to marshal VCAP_SERVICES: %s", err)
		} else {
			runenv["VCAP_SERVICES"] = string(b)
			fmt.Println(string(b))
		}

		if Verbose || len(runargs) == 0 {
			t := template.Must(template.New("tunnelInfo").Parse(tunnelInfo))
			var out bytes.Buffer
			t.Execute(&out, appEnv.SystemEnvJSON)
			fmt.Fprintln(Stderr, out.String())
		}

		runargChan := make(chan struct{})
		if len(runargs) > 0 {
			exe, err := exec.LookPath(runargs[0])
			if err != nil {
				return fmt.Errorf("cannot find '%s' in PATH")
			}
			proc := exec.Command(exe, runargs[1:]...)
			proc.Env = os.Environ()
			for k, v := range runenv {
				proc.Env = append(proc.Env, fmt.Sprintf("%s=%s", k, v))
			}
			proc.Stdout = Stdout
			proc.Stdin = Stdin
			proc.Stderr = Stderr
			lg("starting", exe)
			if err := proc.Start(); err != nil {
				return fmt.Errorf("%s: %s", exe, err)
			}
			go func() {
				defer close(runargChan)
				proc.Wait()
			}()
		}

		select {
		case err := <-sshErrs:
			return err
		case <-shutdown:
			return nil
		case <-runargChan:
			return nil
		}
	},
}
