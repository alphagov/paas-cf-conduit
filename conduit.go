package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/alphagov/paas-cf-conduit/client"
	"github.com/alphagov/paas-cf-conduit/conduit"
	"github.com/alphagov/paas-cf-conduit/logging"
	"github.com/alphagov/paas-cf-conduit/service"
	"github.com/alphagov/paas-cf-conduit/util"

	"github.com/spf13/cobra"
)

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
  cf conduit mysql-instance -- mysql < backup.sql
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

		// create status writer
		status := util.NewStatus(os.Stderr, NonInteractive)
		defer status.Done()

		// create a client
		status.Text("Connecting client")
		cfClient, err := client.NewClient(ApiEndpoint, ApiToken, ApiInsecure)
		if err != nil {
			return err
		}

		app := conduit.NewApp(
			cfClient, status,
			ConduitLocalPort, ConduitOrg, ConduitSpace, ConduitAppName, !ConduitReuse,
			serviceInstanceNames, runargs,
		)

		app.RegisterServiceProvider("mysql", &service.MySQL{})
		app.RegisterServiceProvider("postgres", &service.Postgres{})
		app.RegisterServiceProvider("redis", &service.Redis{})

		defer func() {
			if err := app.Teardown(); err != nil {
				logging.Error(err)
			}
		}()

		if err := app.Init(); err != nil {
			return err
		}

		if err := app.DeployApp(); err != nil {
			return err
		}

		if err := app.SetupTunnels(); err != nil {
			return err
		}

		status.Done()

		if logging.Verbose || len(runargs) == 0 {
			app.PrintConnectionInfo()
		}

		finish := make(chan struct{})
		if len(runargs) > 0 {
			if err := app.RunCommand(finish); err != nil {
				return err
			}
		} else {
			fmt.Fprintln(os.Stderr, "\nPress Ctrl+C to shutdown.")
		}

		// wait
		select {
		case <-shutdown:
			return nil
		case <-finish:
			return nil
		}
	},
	SilenceUsage: true,
}
