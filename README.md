# job-worker-service
Run Arbitrary Process Server and Client



# Directory Structure

/job-worker-service
│
├── jobworker/              # Core library package
│   ├── jobworker.go        # Job lifecycle management (StartJob, StopJob, etc.)
│   ├── monitor.go          # Job Monitoring goroutine (monitorJob)
│   ├── stream.go           # Log streaming logic with fsnotify
│   ├── cgroups.go          # Set up Cgroups
│   ├── jobLogs/            # Stores logs for each job (e.g., <jobId>.log)
│   
│
├── jobhelper/              # jobhelper package: Helper process for job setup and execution
│   └── jobhelper.go        # Implementation of jobhelper
