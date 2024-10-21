package jobworker

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/fsnotify/fsnotify"
)

// Stream the job's log file to the client.
func (jm *JobManager) StreamJobOutput(ctx context.Context, jobID string, streamFunc func(string) error) error {
	jm.logger.Printf("[INFO] Starting log stream for jobID: %s", jobID)

	jm.mu.RLock()
	job, exists := jm.jobMap[jobID]
	jm.mu.RUnlock()
	if !exists {
		jm.logger.Printf("[ERROR] Job not found: %s", jobID)
		return fmt.Errorf("job not found")
	}

	logFilePath := fmt.Sprintf("../jobworker/jobLogs/%s.log", jobID)
	jm.logger.Printf("[INFO] Opening log file: %s", logFilePath)

	file, err := os.Open(logFilePath)
	if err != nil {
		jm.logger.Printf("[ERROR] Failed to open log file: %v", err)
		return fmt.Errorf("failed to open log file: %v", err)
	}
	defer file.Close()
	jm.logger.Printf("[INFO] Successfully opened log file: %s", logFilePath)

	// Check if the job is already finished.
	jm.mu.RLock()
	jobStatus := job.Status
	jm.mu.RUnlock()
	if jobStatus != StatusRunning {
		jm.logger.Printf("[INFO] Job %s has already exited. Streaming entire log.", jobID)
		return jm.streamEntireLog(ctx, file, streamFunc)
	}

	jm.logger.Println("[INFO] Setting up Inotify watcher...")
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		jm.logger.Printf("[ERROR] Failed to create Inotify watcher: %v", err)
		return fmt.Errorf("failed to create inotify watcher: %v", err)
	}
	defer watcher.Close()

	if err := watcher.Add(logFilePath); err != nil {
		jm.logger.Printf("[ERROR] Failed to add Inotify watch: %v", err)
		return fmt.Errorf("failed to add inotify watch: %v", err)
	}
	jm.logger.Println("[INFO] Inotify watcher added successfully.")

	buf := make([]byte, 1024)

	for {
		select {
		case <-ctx.Done(): // Client canceled the stream.
			jm.logger.Printf("[INFO] Log stream canceled by client for jobID: %s", jobID)
			if err := watcher.Close(); err != nil {
				jm.logger.Printf("[ERROR] Failed to close watcher: %v", err)
				return fmt.Errorf("failed to close watcher: %v", err)
			}
			return nil

		case event, ok := <-watcher.Events:
			if !ok {
				jm.logger.Printf("[INFO] Watcher closed for jobID: %s", jobID)
				return nil
			}
			if event.Op&fsnotify.Write == fsnotify.Write {
				jm.logger.Printf("[INFO] Detected log file update for jobID: %s", jobID)
				if err := jm.readAndStream(file, buf, streamFunc); err != nil {
					jm.logger.Printf("[ERROR] Error streaming log: %v", err)
					return err
				}
			}

		case <-job.JobTerminationChannel: // Job has exited.
			jm.logger.Printf("[INFO] Job %s exited. Streaming remaining log data.", jobID)
			if err := jm.readAndStream(file, buf, streamFunc); err != nil {
				jm.logger.Printf("[ERROR] Error streaming remaining log data: %v", err)
				return err
			}
			return nil

		case err, ok := <-watcher.Errors:
			if !ok {
				jm.logger.Printf("[INFO] Watcher closed for jobID: %s", jobID)
				return nil
			}
			jm.logger.Printf("[ERROR] Inotify error: %v", err)
			return fmt.Errorf("inotify error: %v", err)
		}
	}
}

func (jm *JobManager) readAndStream(file *os.File, buf []byte, streamFunc func(string) error) error {
	jm.logger.Println("[INFO] Reading and streaming log data...")
	for {
		n, err := file.Read(buf)
		if err == io.EOF {
			jm.logger.Println("[INFO] Reached end of log file.")
			return nil
		}
		if err != nil {
			jm.logger.Printf("[ERROR] Error reading log file: %v", err)
			return fmt.Errorf("error reading log file: %v", err)
		}
		jm.logger.Printf("[INFO] Streaming %d bytes of log data.", n)
		if err := streamFunc(string(buf[:n])); err != nil {
			jm.logger.Printf("[ERROR] Error sending log data: %v", err)
			return err
		}
	}
}

func (jm *JobManager) streamEntireLog(ctx context.Context, file *os.File, streamFunc func(string) error) error {
	jm.logger.Println("[INFO] Streaming entire log file from the beginning.")
	buf := make([]byte, 1024)

	for {
		select {
		case <-ctx.Done(): // Client canceled the request.
			jm.logger.Println("[INFO] Client canceled the log stream.")
			return nil

		default:
			n, err := file.Read(buf)
			if err == io.EOF {
				jm.logger.Println("[INFO] Finished streaming the entire log file.")
				return nil
			}
			if err != nil {
				jm.logger.Printf("[ERROR] Error reading log file: %v", err)
				return fmt.Errorf("error reading log file: %v", err)
			}
			jm.logger.Printf("[INFO] Streaming %d bytes of log data.", n)
			if err := streamFunc(string(buf[:n])); err != nil {
				jm.logger.Printf("[ERROR] Error sending log data: %v", err)
				return err
			}
		}
	}
}
