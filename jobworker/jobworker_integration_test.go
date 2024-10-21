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
	JW_HELPER_PATH_ENV = "JW_HELPER_PATH"
	LOGS_DIR           = "./jobLogs"
)

func TestStartJobIntegration(t *testing.T) {
	// Set the environment variable for jobhelper path.
	os.Setenv(JW_HELPER_PATH_ENV, "../jobhelper/jobhelper")

	jm := jobworker.NewJobManager()

	//jobID, err := jm.StartJob("/bin/ps", []string{"-ef"})
	jobID, err := jm.StartJob("/bin/cat", []string{"/etc/passwd"})
	//jobID, err := jm.StartJob("/bin/top", []string{"-b"})
	assert.NoError(t, err, "Failed to start job")

	job, exists := jm.GetJob(jobID)
	assert.True(t, exists, "Job not found in jobMap")

	<-job.ExitChannel // Wait for the job to complete.

	status, exists := jm.GetJobStatus(jobID)
	assert.True(t, exists, "Job status not found")
	assert.Equal(t, jobworker.StatusExited, status, "Job should be exited")
	assert.Equal(t, 0, job.ExitCode, "Exit code should be 0")

	logFilePath := filepath.Join(LOGS_DIR, fmt.Sprintf("%s.log", jobID))
	_, err = os.Stat(logFilePath)
	info, err := os.Stat(logFilePath)
	assert.NoError(t, err, "Logfile not found")
	assert.Greater(t, info.Size(), int64(0), "Logfile is empty")

	streamFunc := func(line string) error {
		fmt.Print(line) // Print streamed output.
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = jm.StreamJobOutput(ctx, jobID, streamFunc)
	assert.NoError(t, err, "Failed to stream job output")
}
