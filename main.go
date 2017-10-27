package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"code.cloudfoundry.org/cli/plugin"
	"github.com/spf13/cobra"
)

var (
	Verbose  bool
	shutdown chan struct{}
	Stdin    = os.Stdin
	Stdout   = os.Stdout
	Stderr   = os.Stderr
)

func init() {
	shutdown = make(chan struct{})
	go func() {
		sig := make(chan os.Signal, 3)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)
		<-sig
		close(shutdown)
		for range sig {
			log.Println("...shutting down")
		}
	}()
	os.Stdin = nil
	os.Stdout = nil
	// os.Stderr = nil
}

func main() {
	cmd := &cobra.Command{
		Use: "cf",
	}
	cmd.PersistentFlags().BoolVarP(&Verbose, "verbose", "v", false, "verbose output")
	cmd.AddCommand(ConnectService)
	cmd.AddCommand(Uninstall)
	plugin.Start(&Plugin{cmd})
}
