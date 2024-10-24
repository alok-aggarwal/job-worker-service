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

// stopCmd represents the 'stop' command, which stops a running job
// identified by its Job ID.
var stopCmd = &cobra.Command{
	Use:   "stop <job-id>",      // Command usage syntax
	Short: "Stop a running job", // Short description of the command

	// Args specifies that the 'stop' command requires exactly one argument: the job ID.
	Args: cobra.ExactArgs(1),

	// Run executes the logic to stop a running job.
	Run: func(cmd *cobra.Command, args []string) {
		// Establish a gRPC connection to the Job Worker service.
		conn, client := tlsconfig.GetClient()
		defer conn.Close()

		// Extract the Job ID from the input arguments.
		jobId := args[0]

		// Create a StopJobRequest to send to the gRPC server.
		req := &pb.StopJobRequest{JobId: jobId}

		// Call the StopJob method on the gRPC client.
		_, err := client.StopJob(context.Background(), req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to stop job: %v\n", err)
			os.Exit(1)
		}

		// Print confirmation that the job was stopped successfully.
		fmt.Println("Job stopped successfully!")
	},
}

// init registers the 'stop' command with the root command.
func init() {
	RootCmd.AddCommand(stopCmd)
}
