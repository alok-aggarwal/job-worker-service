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

// cleanJobsCmd represents the 'clean-all-jobs' command, which stops all running jobs
// and cleans up their associated information.
var cleanJobsCmd = &cobra.Command{
	Use:   "clean-all-jobs",             // Command usage syntax
	Short: "Stop all jobs and clean up", // Short description of the command

	// Run executes the logic to stop all running jobs and clean them up.
	Run: func(cmd *cobra.Command, args []string) {
		// Establish a gRPC connection to the Job Worker service.
		conn, client := tlsconfig.GetClient()
		defer conn.Close()

		// Create a CleanAllJobsRequest to send to the gRPC server.
		req := &pb.CleanAllJobsRequest{}

		// Call the CleanAllJobs method on the gRPC client.
		res, err := client.CleanAllJobs(context.Background(), req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to clean jobs: %v\n", err)
			os.Exit(1)
		}

		// Print the result based on the response's success status.
		if res.Success {
			fmt.Println("All jobs cleaned successfully!")
		} else {
			fmt.Println("Failed to clean some jobs.")
		}
	},
}

// init registers the 'clean-all-jobs' command with the root command.
func init() {
	RootCmd.AddCommand(cleanJobsCmd)
}
