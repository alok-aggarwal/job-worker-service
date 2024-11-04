package jobworker

import (
	"fmt"
	"os"
	"os/exec"
	"time"
)

// monitorJobHelper monitors the jobhelper process until it exits.
//
// Parameters:
//   - jobID: The unique identifier for the job being monitored.
//   - pipeR: The read end of the pipe used for communication.
//   - cmd: The exec.Cmd representing the jobhelper process.
//   - done: A channel to signal when monitoring is complete.
func (jm *JobManager) monitorJobHelper(jobID string, pipeR *os.File, cmd *exec.Cmd, done chan struct{}) {
	defer pipeR.Close()

	jm.logger.Printf("[INFO] Monitoring jobHelper %s", jobID)

	// Wait for the jobhelper process to exit and log the result.
	if err := cmd.Wait(); err != nil {
		jm.logger.Printf("[ERROR] jobhelper for job %s exited with error: %v", jobID, err)
	} else {
		jm.logger.Printf("[INFO] jobhelper for job %s exited cleanly", jobID)
	}

	// ToDo:
	// Replace time.Sleep with a sync.WaitGroup to handle synchronization more robustly.

	// Give the monitor some time to process messages from the pipe.
	time.Sleep(2 * time.Second)

	// Signal the monitor to exit if it's still running.
	close(done)

	// Give the monitor some time to update the job status.
	time.Sleep(1 * time.Second)
}

// monitorJob monitors the job status by reading messages from the pipe.
//
// Parameters:
//   - jobID: The unique identifier for the job being monitored.
//   - pipeR: The read end of the pipe used for communication.
//   - done: A channel to signal when monitoring is complete.
func (jm *JobManager) monitorJob(jobID string, pipeR *os.File, done chan struct{}) {
	jm.logger.Printf("[INFO] Monitoring job %s", jobID)

	buf := make([]byte, 128)
	for {
		select {
		case <-done:
			jm.logger.Printf("[ERROR] Monitor received 'done' signal for job %s. Marking as ServerError.", jobID)
			jm.updateJobStatus(jobID, StatusServerErr, 0, 0)
			return

		default:
			// Read status message from the pipe.
			n, err := pipeR.Read(buf)
			if err != nil {
				jm.logger.Printf("[ERROR] Failed to read from pipe for job %s: %v", jobID, err)
				jm.updateJobStatus(jobID, StatusServerErr, 0, 0)
				return
			}

			statusMsg := string(buf[:n])
			switch {
			case statusMsg == "OK":
				jm.logger.Printf("[INFO] Setup completed successfully for job %s", jobID)

			case statusMsg == "FAIL":
				jm.logger.Printf("[ERROR] Setup failed for job %s", jobID)
				jm.updateJobStatus(jobID, StatusServerErr, 0, 0)
				return

			default:
				// Parse exit code and signal number from the message.
				var exitCode, signalNum int
				if _, err := fmt.Sscanf(statusMsg, "EXIT %d %d", &exitCode, &signalNum); err != nil {
					jm.logger.Printf("[ERROR] Invalid exit status for job %s: %v", jobID, err)
					jm.updateJobStatus(jobID, StatusServerErr, 0, 0)
					return
				}
				if signalNum != 0 {
					jm.updateJobStatus(jobID, StatusTerminated, exitCode, signalNum)
				} else {
					jm.updateJobStatus(jobID, StatusExited, exitCode, 0)
				}
				return
			}
		}
	}
}

// updateJobStatus updates the status of a job in the jobMap.
//
// Parameters:
//   - jobID: The unique identifier for the job.
//   - status: The new status of the job.
//   - exitCode: The exit code of the job process, if available.
//   - signalNum: The signal number if the job was terminated by a signal.
func (jm *JobManager) updateJobStatus(jobID string, status JobStatusStr, exitCode, signalNum int) {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	job, exists := jm.jobMap[jobID]
	if !exists {
		jm.logger.Printf("[ERROR] Job %s not found in jobMap", jobID)
		return
	}

	job.Status = status
	job.ExitCode = exitCode
	job.SignalNum = signalNum

	// Close channels to signal job termination if the job is not running.
	if status != StatusRunning {
		close(job.ExitChannel)
		close(job.JobTerminationChannel)
	}
	jm.logger.Printf("[INFO] Job %s updated with status %s, exit code %d, signal %d", jobID, status, exitCode, signalNum)
}
