package main

import "github.com/spf13/cobra"

var Uninstall = &cobra.Command{
	Use:    "CLI-MESSAGE-UNINSTALL",
	Hidden: true,
	Short:  "Uninstall",
	Long:   "Runs any tasks required for a clean uninstall",
	RunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}
