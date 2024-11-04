// Package jobworker provides functionality for managing jobs and streaming job logs.
package jobworker

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/fsnotify/fsnotify"
)

// StreamJobOutput streams the log file of the given job to the client via the provided stream function.
// It first streams any existing log data and then uses inotify to stream new log updates as they occur.
//
// Parameters:
//   - ctx: A context to manage the lifecycle of the streaming operation.
//   - jobID: The ID of the job whose log is to be streamed.
//   - streamFunc: A function to handle streaming of byte data.
//
// Returns an error if the job is not found, or if streaming encounters issues.
func (jm *JobManager) StreamJobOutput(ctx context.Context, jobID string, streamFunc func([]byte) error) error {
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

	jm.mu.RLock()
	jobStatus := job.Status
	jm.mu.RUnlock()

	buf := make([]byte, 1024)

	if err := jm.readAndStream(ctx, file, buf, streamFunc); err != nil {
		jm.logger.Printf("[ERROR] Error streaming log data: %v", err)
		return err
	}

	if jobStatus != StatusRunning {
		jm.logger.Printf("[INFO] Job %s has already exited. Done with streaming", jobID)
		return nil
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
				if err := jm.readAndStream(ctx, file, buf, streamFunc); err != nil {
					jm.logger.Printf("[ERROR] Error streaming log: %v", err)
					return err
				}
			}

		case <-job.JobTerminationChannel: // Job has exited.
			jm.logger.Printf("[INFO] Job %s exited. Streaming remaining log data.", jobID)
			if err := jm.readAndStream(ctx, file, buf, streamFunc); err != nil {
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

// readAndStream reads the log data from the given file and streams it to the client via the stream function.
//
// Parameters:
//   - ctx: A context to manage the lifecycle of the streaming operation.
//   - file: The file from which to read log data.
//   - buf: A buffer used for reading data.
//   - streamFunc: A function to handle streaming of byte data.
//
// Returns an error if reading from the file or streaming encounters issues.
func (jm *JobManager) readAndStream(ctx context.Context, file *os.File, buf []byte, streamFunc func([]byte) error) error {
	jm.logger.Println("[INFO] Reading and streaming log data...")
	for {
		select {
		case <-ctx.Done(): // Client canceled the request.
			jm.logger.Println("[INFO] Client canceled the log stream.")
			return nil

		default:
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
			if err := streamFunc(buf[:n]); err != nil {
				jm.logger.Printf("[ERROR] Error sending log data: %v", err)
				return err
			}
		}
	}
}
