---
authors: Alok A (alok.cdac@gmail.com)
state: draft
---

# Job Worker Service
The Job Worker service manages Linux processes by offering capabilities to start, stop, query status, and stream output of jobs.
The service provides a gRPC API for clients to interact with it, ensuring secure communication using mutual TLS (mTLS) for
authentication and encryption.

## Required Approvers

* @tigrato && @tcsc && @rOmant && @russjones

## CLI UX

The CLI allows users to interact with the Job Worker server to manage jobs. The CLI tool is named runjob-cli, and it is built
using the Cobra library, which provides a flexible framework for parsing commands and arguments.

### Example CLI Commands
#### runjob-cli start --cmd="program" --cpu-limit=0.2 --mem-limit=256 --io-limit=100
The command returns a unique JobID that is used for further interactions with the job.
#### runjob-cli stop --job-id=`<JobID>`
#### runjob-cli status --job-id=`<JobID>`
Possible statuses are Running, Exited (Exit Code), or Terminated (Signal).
#### runjob-cli stream-output --job-id=`<JobID>`
Streams the real-time output (both stdout and stderr) of the running process to the terminal from the moment the job starts.
Use ctrl+C to stop streaming.
#### runjob-cli list-jobs
```
Job ID     Command                       Status
---------------------------------------------------------------
abc123     /bin/ls -al                    Running
xyz456     /usr/bin/top                   Exited
def789     /usr/bin/sleep 1000            Running
```

## Job Worker (Server)
The Job Worker Server manages the lifecycle of jobs, interacting with the underlying library and handling multiple clients. 
The server runs with root privileges as it needs to create namespaces for each job for isolation.
### gRPC API
```
service JobWorker {
    rpc StartJob(StartJobRequest) returns (StartJobResponse);
    rpc StopJob(StopJobRequest) returns (StopJobResponse);
    rpc GetJobStatus(GetJobStatusRequest) returns (GetJobStatusResponse);
    rpc StreamJobOutput(StreamJobOutputRequest) returns (stream JobOutputResponse);
    rpc ListJobs (ListJobsRequest) returns (ListJobsResponse);
}
message StartJobRequest {
    string command = 1;          
    repeated string args = 2;     
    optional float cpu_limit = 3; 
    optional int64 mem_limit = 4; 
    optional int64 io_limit = 5; 
}
message StartJobResponse {
    string job_id = 1;         
}
message StopJobRequest {
    string job_id = 1;        
}
message StopJobResponse {}

message GetJobStatusRequest {
    string job_id = 1;      
}
message GetJobStatusResponse {
    string status = 1;         // "Running", "Exited", or "Terminated"
}
message StreamJobOutputRequest {
    string job_id = 1;          
}
message JobOutputResponse {
    bytes output = 1;          // Raw byte output from the job log
}
message ListJobsRequest {}
message ListJobsResponse {
    repeated Job job_list = 1;
}
message Job {
    string job_id = 1;
    string command = 2;
    string status = 3;
}
```

Above proto file generates gRPC APIs which call the underlying library API to achieve the functionality desired. The gRPC 
StreamJobOutput() function needs special mention as it needs to pass the stream to the library API which library API uses 
to stream output. It also passes the context for client cancellation of streaming.

gRPC is configured with mTLS, requiring clients to provide valid certificates.

### mTLS Setup
The certs and keys for server, client and CA are pregenerated using openssl. Both the client and server are required to 
authenticate each other via certificates.
	• TLS Version: TLS 1.3
	• Cipher Suites: TLS_AES_256_GCM_SHA384
#### Authorization 
The server verifies the client's authorization based on the Common Name (CN) field of the certificate to keep things simple.

## Library
The library is responsible for handling the core functionality of the Job Worker service, including managing jobs, monitoring 
their lifecycle, and streaming output to the CLI. The library does not expose monitoring functions directly
### jobMap
The jobMap is a data structure that maintains the state of all jobs. It maps the JobID (the key) to a structure called 
JobInfo, which holds the following information:
* Pid: The Linux process ID of the job.
* Status: The current status of the job (Running, Exited, or Terminated).
* ExitCode: The exit code of the job if it exited normally.
* Signal Number: If the job was terminated by a signal, the signal number is stored.

### Concurrency
 The jobMap is accessed by multiple clients and goroutines simultaneously, so it is protected with a read-write mutex 
 (sync.RWMutex). Its state is not persisted across server restarts.

### Process Execution Lifecycle
* Job Starting: Jobs are started using os/exec.Command(), and the process is placed into isolated namespaces 
  (PID, network, mount). A process group is assigned so that all children are in the same group.
* Resource Limits: When starting a job, cgroups are optionally applied to control CPU, memory, and I/O usage 
* Monitoring Jobs: The lifecycle of each job is monitored internally using os.Exec.Wait(), capturing both the exit status
  and signal terminations.
* Job Termination: Stopping a job involves sending a KILL signal (SIGKILL) to the process group (parent process and its
  children) and updating the status in the jobMap accordingly.

### Library Functions
#### StartJob(command string, args []string, cpu_limit float, mem_limit int, io_limit int) (jobId string, error)
* Starts a new job, creates namespace isolation for PID, network, and mount namespaces via flags CLONE_NEWPID, CLONE_NEWNET,
  CLONE_NEWNS. It also creates new memory, cpu and IO cgroup files (cpu.max, memory.max), and adds this pid to them. It
  sets the process group ID to the process itself, so it forms a new group. Any children of this process will be in the same
  process group. This makes it easier to clean up the process and its children via StopJob().
* Creates a unique JobId using uuidv4, adds the job to the jobMap under a mutex with the initial status "Running".
* Redirects the process's stderr and stdout to a logfile called jobLogs/`<jobId>`.log
* Starts a new goroutine to monitor the exit status of the job. It waits for the process to exit via os.Exec.Wait() call. If the
  process has errored it captures the error code or the terminating signal number. Once the process ends the goroutine
  acquires mutex and updates the status of the job in JobMap.
* Returns the JobID for client interaction.

#### StopJob(JobID string) error
* Looks up the jobMap, if the job is "Running", gets the PID, which is also the PGID of the job, sends the signal SIGKILL to PGID
  to stop the job. This also cleans up all the children of the process.
* The goroutine monitoring the job, started by StartJob() function, updates the job status in JobMap when the job process exits.
* The log file associated with the job is cleaned up.

#### GetJobStatus(JobID string) (string, error)
* Returns the current status of the job ("Running", "Exited (`<exit code>`)", or "Terminated (Signal: `<signal number>`)")
* For simplicity just the above states are maintained. Nice to have could be fetching the process state from /proc/`<pid>`/status
* Status queries are made using a read lock (RLock()), allowing multiple clients to concurrently query the status of different 
  jobs without blocking
* Differentiating between normal exits and signal terminations ensures that clients are fully informed about why a job ended.

#### StreamJobOutput(ctx context.Context, jobId string, streamFunc func(string) error) error
* Constructs the logfile path using jobId
* Opens the logfile for reading. This is the same logfile which was created in StartJob() function by redirecting stderr and stdout.
* Streams the logfile byte by byte, using the streamFunc passed by the gRPC API
* To emulate "tail -f" behavior, once EOF is reached, it polls for new bytes being written in the file as long as the jobStatus
  is "Running"
* Client should be able to end the stream using "ctrl+C". For this, pass the context (from the gRPC stream) into the library 
  function so it stops processing when the client cancels the request.
        
#### ListJobs() ([]JobInfo, error)
* Returns a list of jobs, each containing its Job ID, command, and status

## TradeOffs
* The Job Worker service runs with root priviledges so that it can isolate the processes into separate namespaces and can access 
  /proc. Rootless mode is possble but scoped out for this project
* UUIDv4 generates a 36 character jobID which may be too long for a good user experience
* Job log files rotation and cleanup is not considered
* No persistent connection between client and server. Every request is a new connection
* Client, Server and CA certs and keys are pre-generated

