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

type JobStatus string

const (
	StatusRunning    JobStatus = "Running"
	StatusExited     JobStatus = "Exited"
	StatusStopping   JobStatus = "Stopping"
	StatusTerminated JobStatus = "Terminated"
	StatusServerErr  JobStatus = "Server Error"
)

// Metadata of a job.
type JobInfo struct {
	PID                   int
	Cmd                   string
	Status                JobStatus
	ExitCode              int
	SignalNum             int
	ExitChannel           chan struct{}
	JobTerminationChannel chan struct{}
}

type JobManager struct {
	jobMap map[string]*JobInfo
	mu     sync.RWMutex
	logger *log.Logger
}

// Initialize
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

func getJWHelperPath() (string, error) {
	path := os.Getenv("JOB_HELPER_PATH")
	if path == "" {
		return "", fmt.Errorf("[ERROR] JOB_HELPER_PATH environment variable is not set")
	}
	return path, nil
}

func (jm *JobManager) AddJob(jobID string, job *JobInfo) {
	jm.mu.Lock()
	defer jm.mu.Unlock()
	jm.jobMap[jobID] = job
	jm.logger.Printf("[INFO] Job %s added to jobMap", jobID)
}

// Retrieve a job by its ID.
func (jm *JobManager) GetJob(jobID string) (*JobInfo, bool) {
	jm.mu.RLock()
	defer jm.mu.RUnlock()
	job, exists := jm.jobMap[jobID]
	return job, exists
}

func (jm *JobManager) GetJobStatus(jobID string) (JobStatus, bool) {
	job, exists := jm.GetJob(jobID)
	if !exists {
		return "", false
	}
	return job.Status, true
}

// Start a new job with the given command and arguments.
func (jm *JobManager) StartJob(cmd string, args []string) (string, error) {
	if args == nil {
		args = []string{}
	}
	jobID := uuid.New().String()
	jm.logger.Printf("[INFO] Starting job with ID %s: %s %v", jobID, cmd, args)

	jobhelperPath, err := getJWHelperPath()
	if err != nil {
		jm.logger.Printf("[ERROR] Failed to get jobhelper path for job %s: %v", jobID, err)
		return "", err
	}

	pipeR, pipeW, err := os.Pipe()
	if err != nil {
		jm.logger.Printf("[ERROR] Failed to create pipes for job %s: %v", jobID, err)
		return "", fmt.Errorf("[ERROR] Failed to create pipe: %v", err)
	}
	defer pipeR.Close()
	defer pipeW.Close()
	jm.logger.Printf("[INFO] Pipe created for job %s", jobID)

	jwCmd := exec.Command(jobhelperPath, append([]string{jobID, cmd}, args...)...)
	jwCmd.ExtraFiles = []*os.File{pipeW}

	if err := jwCmd.Start(); err != nil {
		jm.logger.Printf("[ERROR] Failed to start jobhelper for job %s: %v", jobID, err)
		return "", fmt.Errorf("[ERROR] Failed to start jobhelper: %v", err)
	}
	jm.logger.Printf("[INFO] jobhelper started for job %s with PID %d", jobID, jwCmd.Process.Pid)

	if err := jm.readSetupStatus(pipeR); err != nil {
		jm.logger.Printf("[ERROR] jwHelper setup failed for job %s: %v", jobID, err)
		return "", err
	}

	job := &JobInfo{
		PID:         jwCmd.Process.Pid,
		Cmd:         cmd,
		Status:      StatusRunning,
		ExitChannel: make(chan struct{}),
	}
	jm.AddJob(jobID, job)
	jm.logger.Printf("[INFO] Job %s added to jobMap", jobID)

	jm.logger.Printf("[INFO] Monitoring job %s", jobID)
	go jm.monitorJob(jobID, jwCmd)

	return jobID, nil
}

func (jm *JobManager) readSetupStatus(pipeR *os.File) error {
	buf := make([]byte, 128)
	n, err := pipeR.Read(buf)
	if err != nil {
		return fmt.Errorf("[ERROR] Failed to read from setup pipe: %v", err)
	}
	status := string(buf[:n])
	if status != "OK" {
		return fmt.Errorf("[ERROR] Setup failed: received status '%s'", status)
	}
	jm.logger.Printf("[INFO] Setup status 'OK' received from jobhelper")
	return nil
}

// Stop a running job by sending a SIGKILL.
func (jm *JobManager) StopJob(jobID string) error {
	jm.logger.Printf("[INFO] Stopping job with ID %s", jobID)

	jm.mu.Lock()
	defer jm.mu.Unlock()

	job, exists := jm.jobMap[jobID]
	if !exists {
		return fmt.Errorf("[ERROR] Job %s not found", jobID)
	}

	if job.Status == StatusStopping || job.Status == StatusExited || job.Status == StatusTerminated {
		return fmt.Errorf("[INFO] Job %s is already stopping or stopped", jobID)
	}

	job.Status = StatusStopping
	pgid := -job.PID
	jm.logger.Printf("[INFO] Sending SIGKILL to PGID %d for job %s", pgid, jobID)
	if err := syscall.Kill(pgid, syscall.SIGKILL); err != nil {
		return fmt.Errorf("[ERROR] Failed to kill job %s: %v", jobID, err)
	}

	jm.logger.Printf("[INFO] Waiting for job %s to exit", jobID)
	<-job.ExitChannel

	jm.logger.Printf("[INFO] Job %s has exited", jobID)
	return nil
}
