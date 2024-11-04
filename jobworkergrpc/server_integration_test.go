// Package main_test contains integration tests for the Job Worker gRPC service.
// It ensures that the server behaves correctly for various job operations such as
// starting, stopping, streaming output, and cleaning up jobs.
package main_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	pb "github.com/job-worker-service/jobworkergrpc/proto"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// setupGRPCClient initializes a new gRPC client with mutual TLS (mTLS) authentication.
// It loads the client certificate, CA certificate, and creates transport credentials.
// Returns the JobWorkerClient interface or an error if the setup fails.
func setupGRPCClient() (pb.JobWorkerClient, error) {
	cert, err := tls.LoadX509KeyPair("../certs/client.crt", "../certs/client.key")
	if err != nil {
		return nil, fmt.Errorf("failed to load client certificate: %w", err)
	}

	caCert, err := os.ReadFile("../certs/ca.crt")
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate: %w", err)
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	creds := credentials.NewTLS(&tls.Config{
		Certificates:       []tls.Certificate{cert},
		RootCAs:            caCertPool,
		InsecureSkipVerify: true, // Skip hostname verification
	})

	conn, err := grpc.Dial("localhost:60001", grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to gRPC server: %w", err)
	}

	return pb.NewJobWorkerClient(conn), nil
}

// TestGRPCServerConnection verifies if the gRPC server is reachable
// by listing the jobs through a client request.
func TestGRPCServerConnection(t *testing.T) {
	client, err := setupGRPCClient()
	assert.NoError(t, err, "Failed to setup gRPC client")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = client.ListJobs(ctx, &pb.ListJobsRequest{})
	assert.NoError(t, err, "Failed to connect to gRPC server and list jobs")
}

// setupGRPCClient initializes a new gRPC client with mutual TLS (mTLS) authentication.
// It loads the client certificate, CA certificate, and creates transport credentials.
// Returns the JobWorkerClient interface or an error if the setup fails.
func setupInvalidGRPCClient() (pb.JobWorkerClient, error) {
	cert, err := tls.LoadX509KeyPair("../certs/invalid_client.crt", "../certs/invalid_client.key")
	if err != nil {
		return nil, fmt.Errorf("failed to load client certificate: %w", err)
	}

	caCert, err := os.ReadFile("../certs/ca.crt")
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate: %w", err)
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	creds := credentials.NewTLS(&tls.Config{
		Certificates:       []tls.Certificate{cert},
		RootCAs:            caCertPool,
		InsecureSkipVerify: true, // Skip hostname verification
	})

	conn, err := grpc.Dial("localhost:60001", grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to gRPC server: %w", err)
	}

	return pb.NewJobWorkerClient(conn), nil
}

// TestInvalidClient verifies a client with an invalid certificate is not able to connect
func TestInvalidClient(t *testing.T) {
	client, err := setupInvalidGRPCClient()
	assert.NoError(t, err, "Failed to setup gRPC client")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = client.ListJobs(ctx, &pb.ListJobsRequest{})
	assert.Error(t, err, "Successfully connected to gRPC server and list jobs")
}

// TestGRPCStartJobShort tests starting and completing a short job successfully.
func TestGRPCStartJobShort(t *testing.T) {
	client, err := setupGRPCClient()
	defer client.CleanAllJobs(context.Background(), &pb.CleanAllJobsRequest{})
	assert.NoError(t, err, "Failed to set up gRPC client")

	req := &pb.StartJobRequest{Command: "/bin/cat", Args: []string{"/etc/passwd"}}
	res, err := client.StartJob(context.Background(), req)
	assert.NoError(t, err, "Failed to start job")

	jobID := res.JobId
	assert.NotEmpty(t, jobID, "JobID should not be empty")

	time.Sleep(3 * time.Second)

	statusRes, err := client.GetJobStatus(context.Background(), &pb.GetJobStatusRequest{JobId: jobID})
	assert.NoError(t, err, "Failed to get job status")
	assert.Equal(t, "Exited", statusRes.Status, "Job should be exited")
	assert.Equal(t, "0", statusRes.ExitCode, "Exit code should be 0")

}

// TestGRPCStartJobLong tests starting, running, and stopping a long-running job.
func TestGRPCStartJobLong(t *testing.T) {
	client, err := setupGRPCClient()
	defer client.CleanAllJobs(context.Background(), &pb.CleanAllJobsRequest{})
	assert.NoError(t, err, "Failed to set up gRPC client")

	req := &pb.StartJobRequest{Command: "/bin/top", Args: []string{"-b"}}
	res, err := client.StartJob(context.Background(), req)
	assert.NoError(t, err, "Failed to start job")

	assert.NotEmpty(t, res.JobId, "JobID should not be empty")

	time.Sleep(3 * time.Second)

	statusRes, err := client.GetJobStatus(context.Background(), &pb.GetJobStatusRequest{JobId: res.JobId})
	assert.NoError(t, err, "Failed to get job status")
	assert.Equal(t, "Running", statusRes.Status, "Job should be running")

	_, err = client.StopJob(context.Background(), &pb.StopJobRequest{JobId: res.JobId})
	assert.NoError(t, err, "Failed to stop job")

	time.Sleep(2 * time.Second)

	statusRes, err = client.GetJobStatus(context.Background(), &pb.GetJobStatusRequest{JobId: res.JobId})
	assert.NoError(t, err, "Failed to get job status after stopping")
	assert.Equal(t, "Terminated", statusRes.Status, "Job status should be 'Terminated'")
}

// TestGRPCStopJobWhileStreaming tests stopping a job during streaming output.
func TestGRPCStopJobWhileStreaming(t *testing.T) {
	client, err := setupGRPCClient()
	defer client.CleanAllJobs(context.Background(), &pb.CleanAllJobsRequest{})
	assert.NoError(t, err, "Failed to set up gRPC client")

	req := &pb.StartJobRequest{Command: "/bin/top", Args: []string{"-b"}}
	res, err := client.StartJob(context.Background(), req)
	assert.NoError(t, err, "Failed to start job")

	jobID := res.JobId
	assert.NotEmpty(t, jobID, "JobID should not be empty")

	time.Sleep(500 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stream, err := client.StreamJobOutput(ctx, &pb.StreamJobOutputRequest{JobId: jobID})
	assert.NoError(t, err, "Failed to stream job output")

	streamFinished := make(chan struct{})
	go func() {
		defer close(streamFinished)
		for {
			_, err := stream.Recv()
			if err != nil {
				return
			}
		}
	}()

	time.Sleep(2 * time.Second)

	_, err = client.StopJob(context.Background(), &pb.StopJobRequest{JobId: jobID})
	assert.NoError(t, err, "Failed to stop job")

	select {
	case <-streamFinished:
	case <-time.After(5 * time.Second):
		t.Fatal("Streaming did not finish after stopping the job")
	}

	statusRes, err := client.GetJobStatus(context.Background(), &pb.GetJobStatusRequest{JobId: jobID})
	assert.NoError(t, err, "Failed to get job status")
	assert.Equal(t, "Terminated", statusRes.Status, "Job status should be 'Terminated'")
}

// TestGRPCCleanAllJobs verifies that all jobs are cleaned up successfully.
func TestGRPCCleanAllJobs(t *testing.T) {
	client, err := setupGRPCClient()
	assert.NoError(t, err, "Failed to set up gRPC client")

	sleepJob, err := client.StartJob(context.Background(), &pb.StartJobRequest{
		Command: "/bin/sleep", Args: []string{"10"},
	})
	assert.NoError(t, err, "Failed to start sleep job")

	echoJob, err := client.StartJob(context.Background(), &pb.StartJobRequest{
		Command: "/bin/echo", Args: []string{"Hello"},
	})
	assert.NoError(t, err, "Failed to start echo job")

	time.Sleep(1 * time.Second)

	listRes, err := client.ListJobs(context.Background(), &pb.ListJobsRequest{})
	assert.NoError(t, err, "Failed to list jobs")

	var sleepJobFound, echoJobFound bool
	for _, job := range listRes.JobList {
		switch job.JobId {
		case sleepJob.JobId:
			assert.Equal(t, "Running", job.Status, "Sleep job should be running")
			sleepJobFound = true
		case echoJob.JobId:
			assert.Equal(t, "Exited", job.Status, "Echo job should be exited")
			assert.Equal(t, "0", job.ExitCode, "Echo job should have exit code 0")
			echoJobFound = true
		}
	}
	assert.True(t, sleepJobFound, "Sleep job not found")
	assert.True(t, echoJobFound, "Echo job not found")

	cleanRes, err := client.CleanAllJobs(context.Background(), &pb.CleanAllJobsRequest{})
	assert.NoError(t, err, "Failed to clean all jobs")
	assert.True(t, cleanRes.Success, "CleanAllJobs operation should be successful")

	listRes, err = client.ListJobs(context.Background(), &pb.ListJobsRequest{})
	assert.NoError(t, err, "Failed to list jobs after cleaning")
	assert.Empty(t, listRes.JobList, "Job list should be empty")
}

// TestGRPCStreamJobOutput verifies that the gRPC server correctly streams job output.
func TestGRPCStreamJobOutput(t *testing.T) {

	client, err := setupGRPCClient()
	defer client.CleanAllJobs(context.Background(), &pb.CleanAllJobsRequest{})

	assert.NoError(t, err, "Failed to set up gRPC client")

	// Start a long-running job with continuous output.
	req := &pb.StartJobRequest{Command: "/bin/top", Args: []string{"-b"}}
	res, err := client.StartJob(context.Background(), req)
	assert.NoError(t, err, "Failed to start job")

	// Give time for job to start
	time.Sleep(2 * time.Second)

	jobID := res.JobId
	assert.NotEmpty(t, jobID, "JobID should not be empty")

	// Stream job output.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stream, err := client.StreamJobOutput(ctx, &pb.StreamJobOutputRequest{JobId: jobID})
	assert.NoError(t, err, "Failed to initiate streaming")

	outputReceived := make(chan bool)
	go func() {
		for {
			resp, err := stream.Recv()
			if err == io.EOF {
				// Stream completed.
				return
			}
			if err != nil {
				// Log the error and exit the goroutine.
				t.Logf("Error receiving stream data: %v", err)
				return
			}
			if resp == nil {
				t.Log("Received nil response")
				return
			}
			assert.NotEmpty(t, resp.Output, "Received empty output")
			fmt.Printf("%x", resp.Output) // Optional: Print streamed output for visibility.
			outputReceived <- true
		}
	}()

	// Wait briefly to ensure streaming starts.
	time.Sleep(2 * time.Second)

	// Verify that some output was received.
	select {
	case <-outputReceived:
		// Output received as expected.
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out waiting for stream output")
	}

	// Stop the job.
	_, err = client.StopJob(context.Background(), &pb.StopJobRequest{JobId: jobID})
	assert.NoError(t, err, "Failed to stop job")

	// Verify final job status is "Terminated".
	statusRes, err := client.GetJobStatus(context.Background(), &pb.GetJobStatusRequest{JobId: jobID})
	assert.NoError(t, err, "Failed to get job status")
	assert.Equal(t, "Terminated", statusRes.Status, "Job status should be 'Terminated'")

}

// TestGRPCMultipleClients verifies that multiple clients can start jobs,
// and ensures that the job statuses are correctly reflected.
func TestGRPCMultipleClients(t *testing.T) {
	client, _ := setupGRPCClient()
	client.CleanAllJobs(context.Background(), &pb.CleanAllJobsRequest{})
	defer client.CleanAllJobs(context.Background(), &pb.CleanAllJobsRequest{})

	var wg sync.WaitGroup
	clientCount := 4
	totalJobs := 2 * clientCount

	// To store any errors from goroutines.
	errCh := make(chan error, totalJobs)

	// Create 4 clients, each starting 2 jobs.
	for i := 0; i < clientCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			client, err := setupGRPCClient()
			if err != nil {
				errCh <- err
				return
			}

			// Start a long-running job.
			_, err = client.StartJob(context.Background(), &pb.StartJobRequest{
				Command: "/bin/sleep", Args: []string{"10"},
			})
			if err != nil {
				errCh <- err
				return
			}

			// Start a short-running job.
			_, err = client.StartJob(context.Background(), &pb.StartJobRequest{
				Command: "/bin/echo", Args: []string{"Hello"},
			})
			if err != nil {
				errCh <- err
				return
			}
		}()
	}

	// Wait for all clients to complete their job submissions.
	wg.Wait()
	close(errCh)

	// Ensure there were no errors during job submissions.
	for err := range errCh {
		assert.NoError(t, err, "Job submission failed")
	}

	// Allow some time for jobs to start and run.
	time.Sleep(2 * time.Second)

	// List all jobs and verify their states.
	listRes, err := client.ListJobs(context.Background(), &pb.ListJobsRequest{})
	assert.NoError(t, err, "Failed to list jobs")
	assert.Len(t, listRes.JobList, totalJobs, "Unexpected number of jobs listed")

	runningCount, exitedCount := 0, 0
	for _, job := range listRes.JobList {
		if job.Status == "Running" {
			runningCount++
		} else if job.Status == "Exited" {
			assert.Equal(t, "0", job.ExitCode, "Exited job should have exit code 0")
			exitedCount++
		}
	}

	// Verify the counts.
	assert.Equal(t, clientCount, runningCount, "Expected 4 jobs to be in Running state")
	assert.Equal(t, clientCount, exitedCount, "Expected 4 jobs to be in Exited state")
}
