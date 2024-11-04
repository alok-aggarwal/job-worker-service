package jobworker

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/google/uuid"
)

// JobStatus represents the possible states a job can have.
type JobStatus string

const (
	StatusRunning    JobStatus = "Running"      // Job is currently running.
	StatusExited     JobStatus = "Exited"       // Job exited normally.
	StatusStopping   JobStatus = "Stopping"     // Job is in the process of stopping.
	StatusTerminated JobStatus = "Terminated"   // Job was terminated by a signal.
	StatusServerErr  JobStatus = "Server Error" // A server error occurred.
)

// JobInfo holds metadata for a job, including its status, PID, and exit information.
type JobInfo struct {
	PID                   int           // Process ID of the jobhelper.
	Cmd                   string        // Command executed for the job.
	Status                JobStatus     // Current status of the job.
	ExitCode              int           // Exit code of the job.
	SignalNum             int           // Signal number if the job was terminated by a signal.
	ExitChannel           chan struct{} // Channel to signal job exit.
	JobTerminationChannel chan struct{} // Channel to signal job termination to streaming functions.
}

// JobManager manages jobs, including starting, stopping, and monitoring their status.
type JobManager struct {
	jobMap map[string]*JobInfo // Map of job IDs to JobInfo.
	mu     sync.RWMutex        // Mutex to protect access to jobMap.
	logger *log.Logger         // Logger for logging job-related events.
}

// NewJobManager initializes a new JobManager with logging and cgroup setup.
//
// It removes any existing log file and creates a new one.
// Returns the initialized JobManager instance.
func NewJobManager() *JobManager {
	logFilePath := "jobworker.log"

	if _, err := os.Stat(logFilePath); err == nil {
		if err := os.Remove(logFilePath); err != nil {
			log.Fatalf("[ERROR] Failed to delete previous log file: %v", err)
		}
	}

	logFile, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("[ERROR] Failed to open log file: %v", err)
	}

	logger := log.New(logFile, "jobworker: ", log.LstdFlags|log.Lshortfile)

	if err := SetupCgroups(); err != nil {
		logger.Printf("[ERROR] Failed to set up cgroups: %v", err)
	}

	return &JobManager{
		jobMap: make(map[string]*JobInfo),
		logger: logger,
	}
}

// getJobHelperPath retrieves the path to the jobhelper binary from the environment variable.
//
// Returns the path or an error if the variable is not set.
func getJobHelperPath() (string, error) {
	path := os.Getenv("JOB_HELPER_PATH")
	if path == "" {
		return "", fmt.Errorf("[ERROR] JOB_HELPER_PATH environment variable is not set")
	}
	return path, nil
}

// AddJob adds a new job to the jobMap.
//
// Parameters:
//   - jobID: The unique identifier for the job.
//   - job: The JobInfo containing job metadata.
func (jm *JobManager) AddJob(jobID string, job *JobInfo) {
	jm.mu.Lock()
	defer jm.mu.Unlock()
	jm.jobMap[jobID] = job
	jm.logger.Printf("[INFO] Job %s added to jobMap", jobID)
}

// GetJob retrieves a job by its ID.
//
// Parameters:
//   - jobID: The unique identifier for the job.
//
// Returns:
//   - *JobInfo: The job metadata, if found.
//   - bool: True if the job exists, false otherwise.
func (jm *JobManager) GetJob(jobID string) (*JobInfo, bool) {
	jm.mu.RLock()
	defer jm.mu.RUnlock()
	job, exists := jm.jobMap[jobID]
	return job, exists
}

// GetJobStatus retrieves the status of a job by its ID.
//
// Parameters:
//   - jobID: The unique identifier for the job.
//
// Returns:
//   - JobStatus: The status of the job.
//   - bool: True if the job exists, false otherwise.
func (jm *JobManager) GetJobStatus(jobID string) (JobStatus, bool) {
	jm.mu.RLock()
	defer jm.mu.RUnlock()

	job, exists := jm.jobMap[jobID]
	if !exists {
		return "", false
	}
	return job.Status, true
}

// StartJob starts a new job with the specified command and arguments.
//
// Parameters:
//   - cmd: The command to execute.
//   - args: Optional arguments for the command.
//
// Returns:
//   - string: The unique job ID.
//   - error: An error if job startup fails.
func (jm *JobManager) StartJob(cmd string, args []string) (string, error) {
	if args == nil {
		args = []string{}
	}
	jobID := uuid.New().String()
	jm.logger.Printf("[INFO] Starting job with ID %s: %s %v", jobID, cmd, args)

	jobhelperPath, err := getJobHelperPath()
	if err != nil {
		jm.logger.Printf("[ERROR] Failed to get jobhelper path for job %s: %v", jobID, err)
		return "", err
	}

	pipeR, pipeW, err := os.Pipe()
	if err != nil {
		jm.logger.Printf("[ERROR] Failed to create pipes for job %s: %v", jobID, err)
		return "", fmt.Errorf("[ERROR] Failed to create pipe: %v", err)
	}

	jwCmd := exec.Command(jobhelperPath, append([]string{jobID, cmd}, args...)...)
	jwCmd.ExtraFiles = []*os.File{pipeW}

	if err := jwCmd.Start(); err != nil {
		jm.logger.Printf("[ERROR] Failed to start jobhelper for job %s: %v", jobID, err)
		return "", fmt.Errorf("[ERROR] Failed to start jobhelper: %v", err)
	}
	jm.logger.Printf("[INFO] jobhelper started for job %s with PID %d", jobID, jwCmd.Process.Pid)

	job := &JobInfo{
		PID:                   jwCmd.Process.Pid,
		Cmd:                   cmd,
		Status:                StatusRunning,
		ExitChannel:           make(chan struct{}),
		JobTerminationChannel: make(chan struct{}),
	}
	jm.AddJob(jobID, job)

	done := make(chan struct{})

	go jm.monitorJob(jobID, pipeR, done)

	go jm.monitorJobHelper(jobID, pipeR, jwCmd, done)

	return jobID, nil
}

// StopJob stops a running job by sending a SIGTERM to the jobhelper.
//
// Parameters:
//   - jobID: The unique identifier for the job to stop.
//
// Returns:
//   - error: An error if the job could not be stopped or was not found.
func (jm *JobManager) StopJob(jobID string) error {
	jm.logger.Printf("[INFO] Stopping job with ID %s", jobID)

	jm.mu.Lock()
	job, exists := jm.jobMap[jobID]

	if !exists {
		return fmt.Errorf("[ERROR] Job %s not found", jobID)
	}

	if job.Status == StatusStopping || job.Status == StatusExited || job.Status == StatusTerminated {
		return fmt.Errorf("[INFO] Job %s is already stopping or stopped", jobID)
	}

	job.Status = StatusStopping
	jm.mu.Unlock()
	jm.logger.Printf("[INFO] Sending SIGTERM to PID %d for job %s", job.PID, jobID)
	if err := syscall.Kill(job.PID, syscall.SIGTERM); err != nil {
		return fmt.Errorf("[ERROR] Failed to send SIGTERM to job %s: %v", jobID, err)
	}

	jm.logger.Printf("[INFO] Waiting for jobhelper of job %s to exit", jobID)
	<-job.ExitChannel

	jm.logger.Printf("[INFO] Jobhelper for job %s has exited", jobID)
	return nil
}
