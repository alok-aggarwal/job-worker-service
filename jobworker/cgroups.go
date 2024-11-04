package jobworker

import (
	"fmt"
	"io/ioutil"
	"os"
)

// Set up CPU, memory, and I/O limits for jobs.
func SetupCgroups() error {
	cpuPath := "/sys/fs/cgroup/job/cpu.max"
	memPath := "/sys/fs/cgroup/job/memory.max"
	ioPath := "/sys/fs/cgroup/job/io.max" // For I/O limits

	if err := os.MkdirAll("/sys/fs/cgroup/job", 0755); err != nil {
		return fmt.Errorf("failed to create job cgroup directory: %v", err)
	}

	if err := ioutil.WriteFile(cpuPath, []byte("100000 100000"), 0644); err != nil {
		return fmt.Errorf("failed to set CPU limit: %v", err)
	}

	if err := ioutil.WriteFile(memPath, []byte("50000000"), 0644); err != nil {
		return fmt.Errorf("failed to set memory limit: %v", err)
	}

	ioLimit := "8:0 wbps=50000000"
	if err := ioutil.WriteFile(ioPath, []byte(ioLimit), 0644); err != nil {
		return fmt.Errorf("failed to set I/O limit: %v", err)
	}

	return nil
}
