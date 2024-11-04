// Package cmd provides the CLI commands to interact with the Job Worker service.
package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	pb "github.com/job-worker-service/jobworkergrpc/proto"
	"github.com/job-worker-service/runjob-cli/tlsconfig"
	"github.com/spf13/cobra"
)

// listJobsCmd represents the 'list-jobs' command, which retrieves and displays
// all active jobs managed by the Job Worker service.
var listJobsCmd = &cobra.Command{
	Use:   "list-jobs",            // Command usage syntax
	Short: "List all active jobs", // Short description of the command

	// Run executes the logic to list all active jobs.
	Run: func(cmd *cobra.Command, args []string) {
		// Establish a gRPC connection to the Job Worker service.
		conn, client := tlsconfig.GetClient()
		defer conn.Close()

		// Create a ListJobsRequest to send to the gRPC server.
		req := &pb.ListJobsRequest{}

		// Call the ListJobs method on the gRPC client.
		res, err := client.ListJobs(context.Background(), req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to list jobs: %v\n", err)
			os.Exit(1)
		}

		// Check if there are no active jobs.
		if len(res.JobList) == 0 {
			fmt.Println("No active jobs found.")
			return
		}

		// Print the job list in a tabular format.
		writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.AlignRight|tabwriter.Debug)
		fmt.Fprintln(writer, "Job ID\tCommand\tStatus\tExit Code\tSignal Num")

		// Iterate over each job and print its details.
		for _, job := range res.JobList {
			fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n",
				job.JobId, job.Command, job.Status, job.ExitCode, job.SigNum)
		}

		// Flush the writer to output the formatted table.
		writer.Flush()
	},
}

// init registers the 'list-jobs' command with the root command.
func init() {
	RootCmd.AddCommand(listJobsCmd)
}
