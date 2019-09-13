package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"code.cloudfoundry.org/cli/plugin"
)

type Plugin struct {
	cmd *cobra.Command
}

func (p *Plugin) Run(conn plugin.CliConnection, args []string) {
	// set defaults from plugin info
	org, err := conn.GetCurrentOrg()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	p.cmd.PersistentFlags().Lookup("org").Value.Set(org.Name)
	space, err := conn.GetCurrentSpace()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	p.cmd.PersistentFlags().Lookup("space").Value.Set(space.Name)
	api, err := conn.ApiEndpoint()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	p.cmd.PersistentFlags().Lookup("endpoint").Value.Set(api)
	token, err := conn.AccessToken()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	p.cmd.PersistentFlags().Lookup("token").Value.Set(token)
	insecure, err := conn.IsSSLDisabled()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if insecure {
		p.cmd.PersistentFlags().Lookup("insecure").Value.Set("true")
	}
	// parse
	p.cmd.SetArgs(args)
	if err := p.cmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	os.Exit(0)
}

func (p *Plugin) GetMetadata() plugin.PluginMetadata {
	meta := plugin.PluginMetadata{
		Name: "conduit",
		Version: plugin.VersionType{
			Major: 0,
			Minor: 0,
			Build: 8,
		},
		MinCliVersion: plugin.VersionType{
			Major: 6,
			Minor: 26,
			Build: 0,
		},
		Commands: []plugin.Command{},
	}
	for _, cmd := range p.cmd.Commands() {
		if cmd.Hidden {
			continue
		}
		opts := map[string]string{}
		meta.Commands = append(meta.Commands, plugin.Command{
			Name:     cmd.Name(),
			HelpText: cmd.Long,
			UsageDetails: plugin.Usage{
				Usage:   cmd.UsageString(),
				Options: opts,
			},
		})
	}
	return meta
}
