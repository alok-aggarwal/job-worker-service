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
#### runjob-cli start `<program with args>`
The command returns a unique JobID that is used for further interactions with the job.
The command fails if no program is provided.
#### runjob-cli stop `<job-id>`
The command fails if no job-id is provided.
#### runjob-cli status `<job-id>`
The command fails if no job-id is provided.
Display format:
```
Job ID     Command                       Status           Exit Code     Signal Num
------------------------------------------------------------------------------------         
xyz456     /usr/bin/top                   Exited           0
```
Possible statuses are Running, Exited (Exit Code), Terminated (Signal), or Server Error
#### runjob-cli stream-output `<job-id>`
The command fails if no job-id is provided.
Streams the real-time output (both stdout and stderr) of the running process to the terminal from the moment the job starts.
Use ctrl+C to stop streaming.
#### runjob-cli list-jobs
Display format"
```
Job ID     Command                       Status           Exit Code     Signal Num
------------------------------------------------------------------------------------
abc123     /bin/ls -al                    Running           
xyz456     /usr/bin/top                   Exited           0
def789     /usr/bin/sleep 1000            Terminated                      9
```
#### runjob-cli clean-all-jobs
Stops all running jobs and deletes all job information.

## Job Worker (Server)
The Job Worker Server manages the lifecycle of jobs, interacting with the underlying library and handling multiple clients. 
The server runs with root privileges as it needs to create namespaces for each job for isolation.
### gRPC API
```
service JobWorker {
    rpc StartJob(StartJobRequest) returns (StartJobResponse);
    rpc StopJob(StopJobRequest) returns (StopJobResponse);
    rpc GetJobStatus(GetJobStatusRequest) returns (JobStatus);
    rpc StreamJobOutput(StreamJobOutputRequest) returns (stream JobOutputResponse);
    rpc ListJobs (ListJobsRequest) returns (ListJobsResponse);
    rpc CleanAllJobs (CleanAllJobsRequest) returns (CleanAllJobsResponse);
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
message JobStatus {      
    string job_id = 1;
    string command = 2;
    string status = 3;       // "Running", "Exited", "Terminated" or "Server Error"
    string exit_code = 4;    // non null if status = "Exited"
    string sig_num = 5;      // non null if status = "Terminated"
}
message StreamJobOutputRequest {
    string job_id = 1;          
}
message JobOutputResponse {
    bytes output = 1;          // Raw byte output from the job log
}
message ListJobsRequest {}
message ListJobsResponse {
    repeated JobStatus job_list = 1;
}

message CleanAllJobsRequest {}

message CleanAllJobsResponse {
    bool success = 1;
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

* PID: The Linux process ID of the job.
* Cmd: The job/process command with arguments
* Status: The current status of the job (Running, Exited, Terminated or "Server Error").
* ExitCode: The exit code of the job.
* Signal Number: If the job was terminated by a signal, the signal number is stored.
* Exit Channel: Used by the StartJob monitor goroutine to notify the process has exited or been terminated to streamOutput goroutine.

### Concurrency
 The jobMap is accessed by multiple clients and goroutines simultaneously, so it is protected with a read-write mutex 
 (sync.RWMutex). Its state is not persisted across server restarts.

### Process Execution Lifecycle
* Job Starting: Jobs are started using os/exec.Command(), and the process is placed into isolated namespaces 
  (PID, network, mount). A process group is assigned so that all children are in the same group.
* Resource Limits: When starting a job, cgroups limits are applied to control CPU, memory, and I/O usage. These are pre-
  configured static limits for each job.
* Monitoring Jobs: The lifecycle of each job is monitored internally using os.Exec.Wait(), capturing both the exit status
  and signal terminations.
* Job Termination: Stopping a job involves sending a KILL signal (SIGKILL) to the process group (parent process and its
  children) and updating the status in the jobMap accordingly.

### Job Worker Helper Process
The jw_helper process is a helper invoked by StartJob() library function. Its main purpose is to attach to cgroups before exec'ing
into the client requested command. It gets passed the JobId and the program command requested by the client as command line arguments. 
jw_helper allows the Job Worker server to keep its responsibilities focused on job management, while jw_helper handles low-level
system operations like setting up cgroups and namespaces. It performs the following tasks:
* Cgroup Association: It attaches itself to the cgroup created by the server
* Namespace Isolation: It sets up the job to run in isolated namespaces (PID, mount, and network) via flags CLONE_NEWPID, CLONE_NEWNET,
  CLONE_NEWNS.
* Log Redirection: It redirects the job’s stdout and stderr to a log file named <JobID>.log.
* Job Execution: After performing the setup tasks, jw_helper uses execve() to replace itself with the client-requested job, so the
  job runs in the same process.
#### Error reporting to the server process
Before jw_helper executes the requested job, it communicates any setup failures (like cgroup attachment or namespace setup errors)
through a pipe that was passed to it by the Job Worker (StartJob() library function). StartJob() creates the pipe before invoking 
jw_helper. During the setup phase, jw_helper writes either an "OK" (for success) or "FAIL" to the pipe. The Job Worker 
reads from the pipe. If an error message is received, the Job Worker terminates the job early, returns the error to the client and
updates the status "Server Error" in the JobMap status field.

### Library Functions
#### SetupCgroups() error
* Creates new memory, cpu and IO cgroup files (cpu.max, memory.max) and sets standard values in them. All jobs managed by the
Job Worker Service will be bounded by these limits.

#### StartJob(command string, args []string, cpu_limit float, mem_limit int, io_limit int) (jobId string, error)
* Creates a unique JobId using uuidv4, adds the job to the jobMap under a mutex with the initial status "Running".
* Creates a pipe before invoking jw_helper, used to collect any setup failes from jw_helper.
* invokes jw_helper, passing the client’s job command and JobID as arguments. It also passes the pipe file descriptor to jw_helper
  for setup status communication.
* It sets the jw_process group ID to the jw_process PID, so it forms a new group. Any children of this process will be in the same
  process group. This makes it easier to clean up the process and its children via StopJob().
* Starts a new goroutine to monitor the exit status of the job. It waits for the process to exit via os.Exec.Wait() call. If the
  process has errored it captures the error code or the terminating signal number. Once the process ends the goroutine
  acquires mutex and updates the status of the job in JobMap. It also notifies in the Job's exit channel that the process has ended.
* Returns the JobID for client interaction.

#### StopJob(JobID string) error
* Looks up the jobMap, if the job is "Running", gets the PID, which is also the PGID of the job, sends the signal SIGKILL to PGID
  to stop the job. This also cleans up all the children of the process.
* The goroutine monitoring the job, started by StartJob() function, updates the job status in JobMap when the job process exits.
* The log file associated with the job is cleaned up.

#### GetJobStatus(JobID string) (string, error)
* Returns the current status of the job ("Running", "Exited", "Terminated", or "Server Error" with "exit_code" or "signal_number").
* For simplicity just the above states are maintained. Nice to have could be fetching the process state from /proc/`<pid>`/status
* Status queries are made using a read lock (RLock()), allowing multiple clients to concurrently query the status of different 
  jobs without blocking
* Differentiating between normal exits and signal terminations ensures that clients are fully informed about why a job ended.

#### StreamJobOutput(ctx context.Context, jobId string, streamFunc func(string) error) error
* Constructs the logfile path using jobId
* Opens the logfile for reading. This is the same logfile which was created in StartJob() function by redirecting stderr and stdout.
* Streams the logfile byte by byte, using the streamFunc passed by the gRPC API
* To emulate "tail -f" behavior, it adds a watch on the log file using Inotify. Once EOF is reached, it watches for the IN_MODIFY event
  which is triggered when new data is written to the file. It again continues reading till EOF and repeats pends for more data. 
  The Inotify watch ends when the monitor goroutine started by StartJob() signals end of the process via the Job's Exit channel.
* Client should be able to end the stream using "ctrl+C". For this, pass the context (from the gRPC stream) into the library 
  function so it stops the Inotify watch when the client cancels the request. 
        
#### ListJobs() ([]JobInfo, error)
* Returns a list of jobs, each containing its Job ID, command, status, exit code and signal number.

## TradeOffs
* The Job Worker service runs with root priviledges so that it can isolate the processes into separate namespaces and can access 
  /proc. Rootless mode is possble but scoped out for this project
* UUIDv4 generates a 36 character jobID which may be too long for a good user experience
* Job log files rotation and cleanup is not considered
* No persistent connection between client and server. Every request is a new connection
* Client, Server and CA certs and keys are pre-generated
* Version management between the jw_helper and Server needs to be considered.

