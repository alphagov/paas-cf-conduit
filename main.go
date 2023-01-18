package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh/terminal"

	"code.cloudfoundry.org/cli/plugin"
	"github.com/alphagov/paas-cf-conduit/logging"
	"github.com/spf13/cobra"
)

var (
	NonInteractive    bool
	ConduitReuse      bool
	ConduitAppName    string
	ConduitOrg        string
	ConduitSpace      string
	ConduitLocalPort  int64
	ApiEndpoint       string
	ApiToken          string
	ApiInsecure       bool
	RawBindParameters string
	CipherSuites      []string
	MinTLSVersion     string
	shutdown          chan struct{}
)

func init() {
	shutdown = make(chan struct{})
	go func() {
		sig := make(chan os.Signal, 3)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGHUP)
		<-sig
		close(shutdown)
		for range sig {
			log.Println("...shutting down")
		}
	}()
}

func GenerateRandomString(length int) string {
	seed := rand.NewSource(time.Now().UnixNano())
	generator := rand.New(seed)
	bytes := make([]byte, length)
	for i := 0; i < length; i++ {
		r := generator.Intn(36)
		if r <= 25 {
			bytes[i] = byte(97 + r) // a = 97 and z = 97+25,
		} else {
			bytes[i] = byte(22 + r) // 0 = 22+26 and 9 = 22+36
		}
	}
	return string(bytes)
}

func main() {
	if terminal.IsTerminal(int(os.Stdout.Fd())) && terminal.IsTerminal(int(os.Stderr.Fd())) {
		NonInteractive = false
	} else {
		NonInteractive = true
	}
	cmd := &cobra.Command{Use: "cf"}
	cmd.PersistentFlags().BoolVarP(&logging.Verbose, "verbose", "", false, "verbose output")
	cmd.PersistentFlags().BoolVarP(&NonInteractive, "no-interactive", "", NonInteractive, "disable progress indicator and status output")
	cmd.PersistentFlags().StringVarP(&ConduitOrg, "org", "o", "", "target org (defaults to currently targeted org)")
	cmd.PersistentFlags().StringVarP(&ConduitSpace, "space", "s", "", "target space (defaults to currently targeted space)")
	cmd.PersistentFlags().BoolVarP(&ConduitReuse, "reuse", "r", false, "speed up multiple invocations of conduit by not destroying the tunnelling app")
	cmd.PersistentFlags().MarkHidden("reuse")
	cmd.PersistentFlags().StringVarP(&ConduitAppName, "app-name", "n", fmt.Sprintf("__conduit_%s__", GenerateRandomString(8)), "app name to use for tunnelling app (must not exist)")
	cmd.PersistentFlags().MarkHidden("app-name")
	cmd.PersistentFlags().Int64VarP(&ConduitLocalPort, "local-port", "p", 7080, "start selecting local ports from")
	cmd.PersistentFlags().StringVar(&ApiEndpoint, "endpoint", "", "set API endpoint")
	cmd.PersistentFlags().MarkHidden("endpoint")
	cmd.PersistentFlags().StringVar(&ApiToken, "token", "", "set API token")
	cmd.PersistentFlags().MarkHidden("token")
	cmd.PersistentFlags().BoolVar(&ApiInsecure, "insecure", false, "allow insecure API endpoint")
	cmd.PersistentFlags().MarkHidden("insecure")
	cmd.PersistentFlags().StringVarP(&RawBindParameters, "bind-parameters", "c", "{}", "bind parameters in JSON format")
	cmd.PersistentFlags().StringSliceVar(&CipherSuites, "cipher-suites", []string{}, "list of cipher suites to use")
	cmd.PersistentFlags().StringVar(&MinTLSVersion, "minimum-tls-version", "", "set minimum TLS version (e.g. TLS13)")
	cmd.AddCommand(ConnectService)
	cmd.AddCommand(Uninstall)

	if len(CipherSuites) == 0 && os.Getenv("CF_CONDUIT_CIPHERSUITES") != "" {
		CipherSuites = strings.Split(os.Getenv("CF_CONDUIT_CIPHERSUITES"), ",")
	}

	if MinTLSVersion == "" {
		MinTLSVersion = "TLS12"
		if os.Getenv("CF_CONDUIT_MIN_TLS_VERSION") != "" {
			MinTLSVersion = os.Getenv("CF_CONDUIT_MIN_TLS_VERSION")
		}
	}

	plugin.Start(&Plugin{cmd})
}
