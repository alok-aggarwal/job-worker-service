package jobworker

import (
	"os/exec"
	"syscall"
)

// Monitors the job's status and updates its state in the job map.
func (jm *JobManager) monitorJob(jobID string, cmd *exec.Cmd) {
	jm.logger.Printf("[INFO] Monitoring jobID: %s, PID: %d", jobID, cmd.Process.Pid)

	err := cmd.Wait()
	if err != nil {
		jm.logger.Printf("[ERROR] JobID: %s, PID: %d encountered error: %v", jobID, cmd.Process.Pid, err)
	} else {
		jm.logger.Printf("[INFO] JobID: %s, PID: %d completed successfully", jobID, cmd.Process.Pid)
	}

	jm.mu.Lock()
	defer jm.mu.Unlock()

	job, exists := jm.jobMap[jobID]
	if !exists {
		jm.logger.Printf("[ERROR] JobID: %s not found in jobMap. Skipping update.", jobID)
		return
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		job.ExitCode = exitErr.ExitCode()
		jm.logger.Printf("[INFO] JobID: %s exited with code: %d", jobID, job.ExitCode)

		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			job.SignalNum = int(status.Signal())
			job.Status = StatusTerminated
			jm.logger.Printf("[INFO] JobID: %s was terminated by signal: %d", jobID, job.SignalNum)
		} else {
			job.Status = StatusExited
			jm.logger.Printf("[INFO] JobID: %s exited normally with exit code: %d", jobID, job.ExitCode)
		}
	} else {
		job.Status = StatusExited
		jm.logger.Printf("[INFO] JobID: %s exited without errors.", jobID)
	}

	// Close the ExitChannel to notify other routines.
	jm.logger.Printf("[INFO] Closing ExitChannel for JobID: %s", jobID)
	close(job.ExitChannel)

	jm.logger.Printf("[INFO] Finished monitoring jobID: %s", jobID)
}
