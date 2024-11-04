// Package cmd provides the CLI commands to interact with the Job Worker service.
package cmd

import (
	"context"
	"fmt"
	"os"

	pb "github.com/job-worker-service/jobworkergrpc/proto"
	"github.com/job-worker-service/runjob-cli/tlsconfig"

	"github.com/spf13/cobra"
)

// startCmd represents the 'start' command, which starts a new job
// by executing the provided program along with its arguments.
var startCmd = &cobra.Command{
	Use:                "start <program with args>", // Command usage syntax
	Short:              "Start a new job",           // Short description of the command
	DisableFlagParsing: true,                        // Allow arbitrary program flags to pass through

	// Run executes the logic to start a job.
	Run: func(cmd *cobra.Command, args []string) {
		// Check if at least one argument (the program) is provided.
		if len(args) < 1 {
			fmt.Fprintln(os.Stderr, "Error: No program specified.")
			cmd.Usage()
			os.Exit(1)
		}

		// Establish a gRPC connection to the Job Worker service.
		conn, client := tlsconfig.GetClient()
		defer conn.Close()

		// Extract the command and its arguments from the input.
		command := args[0]  // Program to run
		jobArgs := args[1:] // Arguments to the program

		// Create a StartJobRequest to send to the gRPC server.
		req := &pb.StartJobRequest{
			Command: command,
			Args:    jobArgs,
		}

		// Call the StartJob method on the gRPC client.
		res, err := client.StartJob(context.Background(), req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to start job: %v\n", err)
			os.Exit(1)
		}

		// Print the Job ID if the job starts successfully.
		fmt.Printf("Job started successfully! Job ID: %s\n", res.JobId)
	},
}

// init registers the 'start' command with the root command.
func init() {
	RootCmd.AddCommand(startCmd)
}
