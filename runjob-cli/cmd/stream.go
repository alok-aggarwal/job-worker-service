package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	pb "github.com/job-worker-service/jobworkergrpc/proto"
	"github.com/job-worker-service/runjob-cli/tlsconfig"
	"github.com/spf13/cobra"
)

var streamCmd = &cobra.Command{
	Use:   "stream-output <job-id>",
	Short: "Stream the output of a job",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		conn, client := tlsconfig.GetClient()
		defer conn.Close()

		jobId := args[0]
		req := &pb.StreamJobOutputRequest{JobId: jobId}

		stream, err := client.StreamJobOutput(context.Background(), req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to stream output: %v\n", err)
			os.Exit(1)
		}

		for {
			msg, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error receiving output: %v\n", err)
				os.Exit(1)
			}
			fmt.Print(string(msg.Output))
		}
	},
}

func init() {
	RootCmd.AddCommand(streamCmd)
}
