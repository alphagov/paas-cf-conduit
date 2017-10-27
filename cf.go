package main

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"code.cloudfoundry.org/cli/plugin"
)

type Cf struct {
	Cli     plugin.CliConnection
	Verbose bool
}

func (c *Cf) generateEmptyApp() (string, error) {
	// Create a temporary directory containing a minimal staticfile-buildpack app.
	tempAppDir, err := ioutil.TempDir("", "tmp-tunnel-")
	if err != nil {
		return "", err
	}
	staticfilePath := filepath.Join(tempAppDir, "Staticfile")
	err = ioutil.WriteFile(staticfilePath, []byte(""), 0644)
	if err != nil {
		os.RemoveAll(tempAppDir)
		return "", err
	}
	return tempAppDir, nil
}

func (c *Cf) pushAppWithoutRoute(appName string, appDir string) error {
	return c.run("push", appName,
		"-p", appDir,
		"-b", "staticfile_buildpack",
		"-m", "64M",
		"-k", "64M",
		"-i", "1",
		"--health-check-type", "none",
		"--no-route",
		"--no-manifest",
	)
}

func (c *Cf) getAppGUID(appName string) (string, error) {
	appGUIDLines, err := c.output("app", "--guid", appName)
	if err != nil {
		return "", err
	}
	appGUID := strings.TrimSpace(appGUIDLines)
	if appGUID == "" {
		return "", fmt.Errorf("Expected app ID for '%s' was empty.", appName)
	}
	return appGUID, nil
}

func (c *Cf) getAppEnv(appGUID string) (string, error) {
	return c.output("curl", "/v2/apps/"+appGUID+"/env")
}

func (c *Cf) bindService(appName string, serviceInstanceName string) error {
	return c.run("bind-service", appName, serviceInstanceName)
}

func (c *Cf) forceDeleteApp(appName string) error {
	return c.run("delete", "-f", appName)
}

func (c *Cf) forceDeleteAppWithRetries(appName string, tries int) (err error) {
	for try := 0; try < tries; try++ {
		err = c.forceDeleteApp(appName)
		if err == nil {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return err
}

func (c *Cf) sshPortForward(appName string, localPort int64, remoteHost string, remotePort int64) (*exec.Cmd, error) {
	portSpec := fmt.Sprintf("%d:%s:%d",
		localPort,
		remoteHost,
		remotePort,
	)
	return c.newCommand("ssh", appName, "-L", portSpec, "-N")
}

func (c *Cf) output(args ...string) (string, error) {
	cmd, err := c.newCommand(args...)
	if err != nil {
		return "", err
	}
	b, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (c *Cf) run(args ...string) error {
	cmd, err := c.newCommand(args...)
	if err != nil {
		return err
	}
	return cmd.Run()
}

func (c *Cf) newCommand(args ...string) (*exec.Cmd, error) {
	exe, err := exec.LookPath("cf")
	if err != nil {
		return nil, errors.New("cannot find 'cf' command in PATH")
	}
	cxt := context.Background()
	cmd := exec.CommandContext(cxt, exe, args...)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Env = os.Environ()
	return cmd, nil
}
