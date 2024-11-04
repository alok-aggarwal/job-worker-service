# Job Worker Service
The Job Worker Service is a robust and efficient server designed to manage and monitor the execution of jobs on a host system. 
This project provides a complete solution for job management, with functionality to start, stop, monitor, and stream output 
for various processes. It leverages gRPC for client-server communication, mTLS for secure interactions, and cgroups for resource
management, making it ideal for scenarios that require controlled and secure job execution.

## Key Components
### JobWorker Server

The main component that provides gRPC APIs to manage jobs.
Each job is executed through a helper process (JobHelper) which manages the job's lifecycle, logging, and cleanup.
Uses a structured job map to track job status, exit codes, and resource limits in real time.

### JobHelper:

A lightweight helper process responsible for setting up and executing the job command.
Attaches jobs to specified cgroups to manage resource consumption and isolates jobs using Linux namespaces.
Reports setup status and final job status (including exit codes and termination signals) back to the JobWorker through inter-process
communication.

### gRPC API:

* Offers endpoints for:
StartJob: Launches a job and returns a unique JobID.
StopJob: Terminates a running job.
GetJobStatus: Retrieves the current status of a job.
StreamJobOutput: Streams live job output to the client.
ListJobs: Lists all current jobs and their statuses.
CleanAllJobs: Stops all running jobs and clears the job map.
* Designed for parallel requests and secure access, with mTLS authentication and authorization based on client certificate Common Names.


### runjob-cli
runjob-cli is a command-line interface (CLI) tool designed to interact with the Job Worker server, which manages job execution, monitoring,
and resource control. This CLI connects to the server using gRPC with mutual TLS (mTLS) authentication to ensure secure communication.


## Directory Structure
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

## Building and Running the Project
### Server
```
alok@alok:~/job-worker-service/jobworkergrpc$ make all

alok@alok:~/job-worker-service/jobworkergrpc$ sudo ./server 
[sudo] password for alok: 

```

### Client
```
alok@alok:~/job-worker-service/runjob-cli$ make all

alok@alok:~/job-worker-service/runjob-cli$ ./runjob-cli help
CLI to interact with the Job Worker server

Usage:
  runjob-cli [command]

Available Commands:
  clean-all-jobs Stop all jobs and clean up
  completion     Generate the autocompletion script for the specified shell
  help           Help about any command
  list-jobs      List all active jobs
  start          Start a new job
  status         Get the status of a job
  stop           Stop a running job
  stream-output  Stream the output of a job

alok@alok:~/job-worker-service/runjob-cli$ ./runjob-cli start /bin/cat /etc/passwd
Job started successfully! Job ID: 850d7d6b-b25c-41db-a8d3-7bf0076d170a

alok@alok:~/job-worker-service/runjob-cli$ ./runjob-cli start /bin/sleep 1
Job started successfully! Job ID: 43c78509-626e-4f02-ab85-c23c934a9400

alok@alok:~/job-worker-service/runjob-cli$ ./runjob-cli start /bin/top -b
Job started successfully! Job ID: 33a6474c-acd6-49fb-9823-9b876a5f6eab

alok@alok:~/job-worker-service/runjob-cli$ ./runjob-cli list-jobs
                                Job ID|               Command|   Status|  Exit Code|Signal Num
  850d7d6b-b25c-41db-a8d3-7bf0076d170a|  /bin/cat /etc/passwd|   Exited|          0|0
  43c78509-626e-4f02-ab85-c23c934a9400|          /bin/sleep 1|   Exited|          0|0
  33a6474c-acd6-49fb-9823-9b876a5f6eab|           /bin/top -b|  Running|          0|0

alok@alok:~/job-worker-service/runjob-cli$ ./runjob-cli stream-output 33a6474c-acd6-49fb-9823-9b876a5f6eab
top - 16:39:06 up 2 days, 19:11,  2 users,  load average: 0.01, 0.04, 0.11
Tasks: 380 total,   1 running, 379 sleeping,   0 stopped,   0 zombie
%Cpu(s):  1.9 us,  1.9 sy,  0.0 ni, 96.1 id,  0.0 wa,  0.0 hi,  0.0 si,  0.0 st
MiB Mem :   4870.6 total,    267.8 free,   2778.8 used,   1824.0 buff/cache
MiB Swap:   2048.0 total,   1032.9 free,   1015.1 used.   1730.1 avail Mem 

    PID USER      PR  NI    VIRT    RES    SHR S  %CPU  %MEM     TIME+ COMMAND
  16669 alok      20   0  579316  73572  45316 S   6.2   1.5   2:48.96 gnome-t+
 184277 root      20   0   13352   4224   3328 R   6.2   0.1   0:00.01 top
      1 root      20   0  168220  12476   7868 S   0.0   0.3   0:09.76 systemd
      2 root      20   0       0      0      0 S   0.0   0.0   0:00.25 kthreadd
      3 root      20   0       0      0      0 S   0.0   0.0   0:00.00 pool_wo+
      4 root       0 -20       0      0      0 I   0.0   0.0   0:00.00 kworker+
      5 root       0 -20       0      0      0 I   0.0   0.0   0:00.00 kworker+
      6 root       0 -20       0      0      0 I   0.0   0.0   0:00.00 kworker+
      7 root       0 -20       0      0      0 I   0.0   0.0   0:00.00 kworker+
      9 root       0 -20       0      0      0 I   0.0   0.0   0:03.56 kworker+
     11 root      20   0       0      0      0 I   0.0   0.0   0:00.00 kworker+
     12 root       0 -20       0      0      0 I   0.0   0.0   0:00.00 kworker+
     13 root      20   0       0      0      0 I   0.0   0.0   0:00.00 rcu_tas+
     14 root      20   0       0      0      0 I   0.0   0.0   0:00.00 rcu_tas+
     15 root      20   0       0      0      0 I   0.0   0.0   0:00.00 rcu_tas+
  ...... truncated by ctrl+c

alok@alok:~/job-worker-service/runjob-cli$ ./runjob-cli stop 33a6474c-acd6-49fb-9823-9b876a5f6eab
Job stopped successfully!

alok@alok:~/job-worker-service/runjob-cli$ ./runjob-cli list-jobs
                                Job ID|               Command|      Status|  Exit Code|Signal Num
  850d7d6b-b25c-41db-a8d3-7bf0076d170a|  /bin/cat /etc/passwd|      Exited|          0|0
  43c78509-626e-4f02-ab85-c23c934a9400|          /bin/sleep 1|      Exited|          0|0
  33a6474c-acd6-49fb-9823-9b876a5f6eab|           /bin/top -b|  Terminated|        137|9

alok@alok:~/job-worker-service/runjob-cli$ ./runjob-cli clean-all-jobs
All jobs cleaned successfully!

alok@alok:~/job-worker-service/runjob-cli$ ./runjob-cli list-jobs
No active jobs found.
```

## Further Improvements
- Add more tests to cover more scenarios and edge cases. 
- Convert structures into interfaces to make it easier to mock and test.
- Add more logging and metrics to the server and client.
- Add tests to cover cgroup limits enforcement.
- Add tests to cover namespace isolation.
- provision to run the service from systemd
- Top level Makefile to build and run the service
- Run thw whole code through a race detector
- Reap the job processes properly if the jobhelper dies
