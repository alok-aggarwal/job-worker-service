package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/sys/unix"
)

var logger *log.Logger

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

	logger.Printf("[INFO] Starting jobhelper for jobID: %s, command: %s, args: %v", jobID, command, args)

	// Set PGID to self PID to form a new process group.
	if err := unix.Setpgid(0, 0); err != nil {
		logAndExit(pipeW, fmt.Sprintf("[ERROR] Failed to set PGID: %v", err))
	}
	logger.Println("[INFO] Set process group ID to self PID.")

	if err := attachToCgroup(os.Getpid()); err != nil {
		logAndExit(pipeW, fmt.Sprintf("[ERROR] Cgroup attachment failed: %v", err))
	}

	if err := setupNamespaces(); err != nil {
		logAndExit(pipeW, fmt.Sprintf("[ERROR] Namespace setup failed: %v", err))
	}

	logFile, err := getLogFile(jobID)
	if err != nil {
		logAndExit(pipeW, fmt.Sprintf("[ERROR] Log file setup failed: %v", err))
	}
	defer logFile.Close()

	logger.Println("[INFO] Setup complete. Indicating success to parent process.")
	fmt.Fprint(pipeW, "OK")
	pipeW.Close()

	logger.Printf("[INFO] Executing command: %s with args: %v", command, args)
	if err := runCommand(command, args, logFile); err != nil {
		logger.Fatalf("[ERROR] Command execution failed: %v", err)
	}

	logger.Println("[INFO] Command executed successfully.")
}

func logAndExit(pipeW *os.File, msg string) {
	logger.Println(msg)
	fmt.Fprintf(pipeW, "FAIL: %s\n", msg)
	pipeW.Close()
	os.Exit(1)
}

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

func setupNamespaces() error {
	logger.Println("[INFO] Setting up PID, network, and mount namespaces.")
	return unix.Unshare(unix.CLONE_NEWNS | unix.CLONE_NEWPID | unix.CLONE_NEWNET)
}

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

func runCommand(command string, args []string, logFile *os.File) error {
	cmd := exec.Command(command, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	cmd.SysProcAttr = &unix.SysProcAttr{Setpgid: true}

	logger.Printf("[INFO] Running command: %s with args: %v", command, args)
	return cmd.Run()
}
