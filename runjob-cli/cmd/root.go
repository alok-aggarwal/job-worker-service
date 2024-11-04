// Package cmd provides the CLI commands to interact with the Job Worker service.
package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// RootCmd represents the base command for the runjob-cli tool.
// It serves as the entry point for all subcommands and provides
// a description of the CLI's purpose.
var RootCmd = &cobra.Command{
	Use:   "runjob-cli",                                 // Command usage syntax
	Short: "CLI to interact with the Job Worker server", // Short description of the CLI
}

// Execute runs the root command, which parses and executes
// any provided subcommands. If an error occurs, it exits the
// program with a non-zero status code.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
