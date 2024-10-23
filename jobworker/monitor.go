package jobworker

import (
	"fmt"
	"os"
	"os/exec"
	"time"
)

func (jm *JobManager) monitorJobHelper(jobID string, pipeR *os.File, cmd *exec.Cmd, done chan struct{}) {

	defer pipeR.Close()

	jm.logger.Printf("[INFO] Monitoring jobHelper %s", jobID)
	if err := cmd.Wait(); err != nil {
		jm.logger.Printf("[ERROR] jobhelper for job %s exited with error: %v", jobID, err)
	} else {
		jm.logger.Printf("[INFO] jobhelper for job %s exited cleanly", jobID)
	}

	// ToDo:
	// We need a better way to handle sync with monitorJob goroutine.
	// Perhaps replace time.Sleep with a sync.WaitGroup to avoid brittle timing assumptions.

	// Give the monitor some time to process the messages from the pipes.
	time.Sleep(2 * time.Second)

	// Signal the monitor to exit if it's still running.
	close(done)
	// Give the monitor some time to update the job status.
	time.Sleep(1 * time.Second)
}

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

func (jm *JobManager) updateJobStatus(jobID string, status JobStatus, exitCode, signalNum int) {
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

	if status != StatusRunning {
		close(job.ExitChannel)
		close(job.JobTerminationChannel)
	}
	jm.logger.Printf("[INFO] Job %s updated with status %s, exit code %d, signal %d", jobID, status, exitCode, signalNum)
}
