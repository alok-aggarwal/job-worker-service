package jobworker_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/job-worker-service/jobworker"
	"github.com/stretchr/testify/assert"
)

const (
	JOB_HELPER_PATH_ENV = "JOB_HELPER_PATH"
	LOGS_DIR            = "./jobLogs"
)

func setupJobManager() *jobworker.JobManager {
	os.Setenv(JOB_HELPER_PATH_ENV, "../jobhelper/jobhelper")
	return jobworker.NewJobManager()
}

func assertJobExists(t *testing.T, jm *jobworker.JobManager, jobID string) *jobworker.JobInfo {
	t.Helper()
	job, exists := jm.GetJob(jobID)
	assert.True(t, exists, "Job not found in jobMap")
	return job
}

func streamToString(ctx context.Context, jm *jobworker.JobManager, jobID string) (string, error) {
	var output []byte
	streamFunc := func(data []byte) error {
		output = append(output, data...)
		fmt.Printf("%x", data)
		return nil
	}

	err := jm.StreamJobOutput(ctx, jobID, streamFunc)
	return string(output), err
}

func TestStartJobShort(t *testing.T) {
	jm := setupJobManager()

	jobID, err := jm.StartJob("/bin/cat", []string{"/etc/passwd"})
	assert.NoError(t, err, "Failed to start job")

	job := assertJobExists(t, jm, jobID)
	<-job.ExitChannel // Wait for the job to complete.

	status, exists := jm.GetJobStatus(jobID)
	assert.True(t, exists, "Job status not found")
	assert.Equal(t, jobworker.StatusExited, status, "Job should be exited")
	assert.Equal(t, 0, job.ExitCode, "Exit code should be 0")

	logFilePath := filepath.Join(LOGS_DIR, fmt.Sprintf("%s.log", jobID))
	//defer os.Remove(logFilePath) // Cleanup

	info, err := os.Stat(logFilePath)
	assert.NoError(t, err, "Logfile not found")
	assert.Greater(t, info.Size(), int64(0), "Logfile is empty")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	output, err := streamToString(ctx, jm, jobID)
	assert.NoError(t, err, "Failed to stream job output")
	assert.NotEmpty(t, output, "Job output is empty")
}

func TestStartJobLong(t *testing.T) {
	jm := setupJobManager()

	jobID, err := jm.StartJob("/bin/top", []string{"-b"})
	assert.NoError(t, err, "Failed to start job")

	assertJobExists(t, jm, jobID)
	time.Sleep(500 * time.Millisecond) // Give some time for the job to start

	status, exists := jm.GetJobStatus(jobID)
	assert.True(t, exists, "Job status not found")
	assert.Equal(t, jobworker.StatusRunning, status, "Job should be running")

	logFilePath := filepath.Join(LOGS_DIR, fmt.Sprintf("%s.log", jobID))
	//defer os.Remove(logFilePath) // Cleanup

	info, err := os.Stat(logFilePath)
	assert.NoError(t, err, "Logfile not found")
	assert.Greater(t, info.Size(), int64(0), "Logfile is empty")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	output, err := streamToString(ctx, jm, jobID)
	assert.NoError(t, err, "Failed to stream job output")
	assert.NotEmpty(t, output, "Job output is empty")
}

func TestStopJobWhileStreaming(t *testing.T) {
	jm := setupJobManager()

	// Start a long-running job.
	jobID, err := jm.StartJob("/bin/top", []string{"-b"})
	assert.NoError(t, err, "Failed to start job")

	assertJobExists(t, jm, jobID)
	time.Sleep(500 * time.Millisecond) // Allow the job to start.

	// Verify the job is running.
	status, exists := jm.GetJobStatus(jobID)
	assert.True(t, exists, "Job status not found")
	assert.Equal(t, jobworker.StatusRunning, status, "Job should be running")

	// Start streaming the job output.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	streamFinished := make(chan struct{})
	go func() {
		defer close(streamFinished)

		output, err := streamToString(ctx, jm, jobID)
		assert.NoError(t, err, "Failed to stream job output")
		assert.NotEmpty(t, output, "Job output is empty")
	}()

	// Wait  to ensure streaming has started.
	time.Sleep(4 * time.Second)

	// Issue StopJob to terminate the job while streaming is in progress.
	err = jm.StopJob(jobID)
	assert.NoError(t, err, "Failed to stop job")

	// Ensure streaming ends after StopJob.
	select {
	case <-streamFinished:
		// Successfully stopped streaming
	case <-time.After(5 * time.Second):
		t.Fatal("Streaming did not finish after stopping the job")
	}

	status, exists = jm.GetJobStatus(jobID)
	assert.True(t, exists, "Job status not found")
	assert.Equal(t, jobworker.StatusTerminated, status, "Job status should be 'Terminated'")
}
