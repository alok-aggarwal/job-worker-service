// Package main implements a jobhelper that manages and monitors
// the execution of job processes, handling logging, signals,
// and communication with jobworker via a pipe.
package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

var (
	logger     *log.Logger // Logger instance for logging jobhelper events.
	jobCmd     *exec.Cmd   // Command representing the job process.
	reportOnce sync.Once   // Ensure the exit status is reported only once.
)

// initLogger initializes the logger and ensures that the log file is
// recreated at the start of the jobhelper.
func initLogger() {
	logFilePath := "jobhelper.log"
	if _, err := os.Stat(logFilePath); err == nil {
		if err := os.Remove(logFilePath); err != nil {
			log.Fatalf("[ERROR] Failed to delete previous log file: %v", err)
		}
	}

	logFile, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("[ERROR] Failed to open jobhelper log file: %v", err)
	}

	logger = log.New(logFile, "jobhelper: ", log.LstdFlags|log.Lshortfile)
	logger.Println("[INFO] Logger initialized.")
}

// main is the entry point of the jobhelper process, which sets up the environment,
// executes the job command, and communicates with jobworker.
func main() {
	initLogger()

	if len(os.Args) < 3 {
		logger.Println("[ERROR] Invalid arguments. Usage: jobhelper <jobID> <command> [args...]")
		os.Exit(1)
	}

	jobID := os.Args[1]
	command := os.Args[2]
	args := os.Args[3:]

	pipeW := os.NewFile(3, "pipe")

	// Setup the job environment and handle any setup errors.
	if err := setupJobEnvironment(pipeW); err != nil {
		logAndExit(pipeW, fmt.Sprintf("[ERROR] Setup failed: %v", err))
	}

	logFile, err := getLogFile(jobID)
	if err != nil {
		logAndExit(pipeW, fmt.Sprintf("[ERROR] Log file setup failed: %v", err))
	}
	defer logFile.Close()

	logger.Println("[INFO] Setup complete. Indicating success to parent process.")
	fmt.Fprint(pipeW, "OK")

	// Execute the job command and capture the exit status.
	logger.Printf("[INFO] Executing command: %s with args: %v", command, args)
	jobCmd = exec.Command(command, args...)
	jobCmd.Stdout = logFile
	jobCmd.Stderr = logFile
	jobCmd.SysProcAttr = &unix.SysProcAttr{Setpgid: true}

	err = jobCmd.Run()
	exitCode, signalNum := extractExitStatus(err)

	// Report the exit status to jobworker.
	reportOnce.Do(func() {
		reportExitStatus(pipeW, exitCode, signalNum)
	})
}

// setupJobEnvironment configures the job environment by setting up signal
// handlers, cgroups, namespaces, and process group IDs (PGID).
func setupJobEnvironment(pipeW *os.File) error {
	if err := setupSignalHandler(pipeW); err != nil {
		return fmt.Errorf("[ERROR] Failed to set up signal handler: %v", err)
	}
	if err := unix.Setpgid(0, 0); err != nil {
		return fmt.Errorf("[ERROR] Failed to set PGID: %v", err)
	}
	if err := attachToCgroup(os.Getpid()); err != nil {
		return fmt.Errorf("[ERROR] Cgroup attachment failed: %v", err)
	}
	if err := setupNamespaces(); err != nil {
		return fmt.Errorf("[ERROR] Namespace setup failed: %v", err)
	}
	return nil
}

// setupSignalHandler installs a signal handler for SIGTERM to ensure that
// the job process receives SIGKILL when jobhelper is terminated.
func setupSignalHandler(pipeW *os.File) error {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM)

	go func() {
		<-sigChan
		logger.Println("[INFO] Received SIGTERM. Sending SIGKILL to job process.")

		if jobCmd != nil && jobCmd.Process != nil {
			if err := jobCmd.Process.Kill(); err != nil {
				logger.Printf("[ERROR] Failed to send SIGKILL: %v", err)
			} else {
				logger.Println("[INFO] Job process killed.")
			}
		}

		reportOnce.Do(func() {
			reportExitStatus(pipeW, 137, int(syscall.SIGKILL)) // 137 indicates SIGKILL.
		})
	}()

	return nil
}

// reportExitStatus sends the exit status and signal number to the jobworker via the pipe.
func reportExitStatus(pipeW *os.File, exitCode, signalNum int) {
	logger.Printf("[INFO] Reporting exit status. ExitCode: %d, Signal: %d", exitCode, signalNum)
	fmt.Fprintf(pipeW, "EXIT %d %d\n", exitCode, signalNum)
	pipeW.Close()
}

// extractExitStatus extracts the exit code and signal number from the error returned by the job process.
func extractExitStatus(err error) (int, int) {
	if err == nil {
		return 0, 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		ws := exitErr.Sys().(syscall.WaitStatus)
		return ws.ExitStatus(), int(ws.Signal())
	}
	return 1, 0 // Default to 1 for general errors.
}

// logAndExit logs the error message and exits the jobhelper with status 1.
func logAndExit(pipeW *os.File, msg string) {
	logger.Println(msg)
	fmt.Fprintf(pipeW, "FAIL: %s\n", msg)
	pipeW.Close()
	os.Exit(1)
}

// attachToCgroup attaches the jobhelper process to a specific cgroup.
func attachToCgroup(pid int) error {
	cgroupDir := "/sys/fs/cgroup/job"
	procsFile := filepath.Join(cgroupDir, "cgroup.procs")
	pidStr := fmt.Sprintf("%d", pid)

	logger.Printf("[INFO] Attaching PID %s to cgroup at %s", pidStr, procsFile)
	if err := os.WriteFile(procsFile, []byte(pidStr), 0644); err != nil {
		return fmt.Errorf("[ERROR] Failed to attach to cgroup: %v", err)
	}
	logger.Println("[INFO] Successfully attached to cgroup.")
	return nil
}

// setupNamespaces sets up PID, network, and mount namespaces for the job process.
func setupNamespaces() error {
	logger.Println("[INFO] Setting up PID, network, and mount namespaces.")
	return unix.Unshare(unix.CLONE_NEWNS | unix.CLONE_NEWPID | unix.CLONE_NEWNET)
}

// getLogFile opens or creates the log file for the job process and returns its handle.
func getLogFile(jobID string) (*os.File, error) {
	logFilePath := filepath.Join("../jobworker/jobLogs", fmt.Sprintf("%s.log", jobID))
	logger.Printf("[INFO] Opening log file: %s", logFilePath)

	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("[ERROR] Failed to open log file: %v", err)
	}

	logger.Println("[INFO] Log file opened successfully.")
	return logFile, nil
}
