# job-worker-service
Run Arbitrary Process Server and Client



# Directory Structure
```
/job-worker-service
│
├── jobworker/                  # Core library package
│   ├── jobworker.go            # Job lifecycle management (StartJob, StopJob, etc.)
│   ├── monitor.go              # Job Monitoring goroutine (monitorJob)
│   ├── stream.go               # Log streaming logic with fsnotify
│   ├── cgroups.go              # Set up Cgroups
|   ├── Makefile
│   ├── jobLogs/                # Stores logs for each job (e.g., <jobId>.log)
│ 
├── certs/                      # Directory for TLS certificates  
│
├── jobhelper/                  # jobhelper package: Helper process for job setup and execution
│   └── jobhelper.go            # Implementation of jobhelper
|
├── jobworkergrpc/              # gRPC server package
│   ├── server.go               # gRPC server implementation using jobworker APIs
|   ├── Makefile
│   ├──proto                    # Protocol Buffers definition for gRPC APIs
│       ├──  api.pb.go          # Generated Go code from api.proto
|       |──  api_grpc.pb.go     # Generated Go code from api.proto
|       ├──  api.proto          # Protocol Buffers definition for gRPC APIs
│
├── runjob-cli/
│   ├── cmd/                    # Package containing CLI commands
│   │   ├── root.go             # Root command definition
│   │   ├── start.go            # Command to start a job
│   │   ├── stop.go             # Command to stop a job
│   │   ├── status.go           # Command to get the status of a job
│   │   ├── list_jobs.go        # Command to list all jobs
│   │   ├── clean_jobs.go       # Command to clean all jobs
│   │   ├── Makefile            # Makefile to build and run the CLI
│   │
│   ├── tlsconfig/              # Internal utilities and configurations
│   │    └── tlsconfig.go       # TLS configuration and gRPC client logic
│   │── client.go               # Main entry point for the CLI
│
├── go.mod                      # Go module file for dependency management
├── go.sum                      # Go dependency checksums


```