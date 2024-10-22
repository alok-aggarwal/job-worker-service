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
	var output string
	streamFunc := func(line string) error {
		output += line
		//fmt.Print(line)
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
