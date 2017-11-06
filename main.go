package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh/terminal"

	"code.cloudfoundry.org/cli/plugin"
	"github.com/spf13/cobra"
)

var (
	Verbose          bool
	NonInteractive   bool
	ConduitKeepApp   bool
	ConduitAppName   string
	ConduitOrg       string
	ConduitSpace     string
	ConduitLocalPort int64
	shutdown         chan struct{}
	fatalshutdown    chan struct{}
)

func init() {
	fatalshutdown = make(chan struct{})
	shutdown = make(chan struct{})
	go func() {
		sig := make(chan os.Signal, 3)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)
		select {
		case <-sig:
		case <-fatalshutdown:
		}
		close(shutdown)
		for range sig {
			log.Println("...shutting down")
		}
	}()
}

func fatal(args ...interface{}) {
	fmt.Fprintln(os.Stderr, args...)
	close(fatalshutdown)
	time.Sleep(10 * time.Second)
	os.Exit(1)
}

func debug(args ...interface{}) {
	if Verbose {
		fmt.Fprintln(os.Stderr, args...)
	}
}

func retry(fn func() error) error {
	delayBetweenRetries := 500 * time.Millisecond
	maxRetries := 10
	try := 0
	for {
		try++
		err := fn()
		if err == nil {
			return nil
		}
		if try > maxRetries {
			return err
		}
		time.Sleep(delayBetweenRetries)
	}
}

func main() {
	if terminal.IsTerminal(int(os.Stdout.Fd())) && terminal.IsTerminal(int(os.Stderr.Fd())) {
		NonInteractive = false
	} else {
		NonInteractive = true
	}
	cmd := &cobra.Command{Use: "cf"}
	cmd.PersistentFlags().BoolVarP(&Verbose, "verbose", "", false, "verbose output")
	cmd.PersistentFlags().BoolVarP(&NonInteractive, "no-interactive", "", NonInteractive, "disable progress indicator and status output")
	cmd.PersistentFlags().StringVarP(&ConduitOrg, "org", "o", "", "target org (defaults to currently targeted org)")
	cmd.PersistentFlags().StringVarP(&ConduitSpace, "space", "s", "", "target space (defaults to currently targeted space)")
	cmd.PersistentFlags().BoolVarP(&ConduitKeepApp, "keep-app", "", false, "speed up multiple invocations of conduit by not destroying the tunnelling app")
	cmd.PersistentFlags().StringVarP(&ConduitAppName, "app-name", "", fmt.Sprintf("__conduit_%d__", os.Getpid()), "app name to use for tunnelling app (must not exist)")
	cmd.PersistentFlags().Int64VarP(&ConduitLocalPort, "local-port", "p", 7080, "start selecting local ports from")
	cmd.AddCommand(ConnectService)
	cmd.AddCommand(Uninstall)
	plugin.Start(&Plugin{cmd})
}
