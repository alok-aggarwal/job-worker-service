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

// statusCmd represents the 'status' command, which retrieves the status of a job
// identified by its Job ID.
var statusCmd = &cobra.Command{
	Use:   "status <job-id>",         // Command usage syntax
	Short: "Get the status of a job", // Short description of the command

	// Args specifies that the 'status' command requires exactly one argument: the job ID.
	Args: cobra.ExactArgs(1),

	// Run executes the logic to retrieve and display the status of a job.
	Run: func(cmd *cobra.Command, args []string) {
		// Establish a gRPC connection to the Job Worker service.
		conn, client := tlsconfig.GetClient()
		defer conn.Close()

		// Extract the Job ID from the input arguments.
		jobId := args[0]

		// Create a GetJobStatusRequest to send to the gRPC server.
		req := &pb.GetJobStatusRequest{JobId: jobId}

		// Call the GetJobStatus method on the gRPC client.
		res, err := client.GetJobStatus(context.Background(), req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get job status: %v\n", err)
			os.Exit(1)
		}

		// Print the status details of the job.
		fmt.Printf("Job ID: %s | Command: %s | Status: %s | Exit Code: %s | Signal Num: %s\n",
			res.JobId, res.Command, res.Status, res.ExitCode, res.SigNum)
	},
}

// init registers the 'status' command with the root command.
func init() {
	RootCmd.AddCommand(statusCmd)
}
