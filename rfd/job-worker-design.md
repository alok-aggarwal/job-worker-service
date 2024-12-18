---
authors: Alok A (alok.cdac@gmail.com)
state: draft
---

# jobworker Service
The jobworker service manages Linux processes by offering capabilities to start, stop, query status, and stream output of jobs.
The service provides a gRPC API for clients to interact with it, ensuring secure communication using mutual TLS (mTLS) for
authentication and encryption.

## Required Approvers

* @tigrato && @tcsc && @rOmant && @russjones

## CLI UX

The CLI allows users to interact with the jobworker server to manage jobs. The CLI tool is named runjob-cli, and it is built
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

## jobworker (Server)
The jobworker Server manages the lifecycle of jobs, interacting with the underlying library and handling multiple clients. 
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
The library is responsible for handling the core functionality of the jobworker service, including managing jobs, monitoring 
their lifecycle, and streaming output to the CLI. The library does not expose monitoring functions directly
### jobMap
The jobMap is a data structure that maintains the state of all jobs. It maps the JobID (the key) to a structure called 
JobInfo, which holds the following information:

* PID: The Linux process ID of the job.
* Cmd: The job/process command with arguments
* Status: The current status of the job (Running, Exited, Terminated or "Server Error").
* ExitCode: The exit code of the job.
* Signal Number: If the job was terminated by a signal, the signal number is stored.
* Exit Channel: Used by the StartJob monitor goroutine to notify the process has exited or been terminated to streamOutput
goroutine.

### Concurrency
 The jobMap is accessed by multiple clients and goroutines simultaneously, so it is protected with a read-write mutex 
 (sync.RWMutex). Its state is not persisted across server restarts.

### Process Execution Lifecycle
* Job Starting: The jobworker offloads job process lifecycle management to jobhelper process which is spawned during
  StartJob() For every new job there is a jobhelper process. The jobhelper are suses os/exec, and the process is placed
  into isolated namespaces (PID, network, mount). A process group is assigned so that all children are in the same group.
* Resource Limits: When starting a job, cgroups limits are applied to control CPU, memory, and I/O usage. These are pre-
  configured static limits for each job.
* Monitoring Jobs: The lifecycle of each job is monitored by jobhelper using os.Exec.Wait(), capturing both the exit status
  and signal terminations.
* Job Termination: Stopping a job involves sending a KILL signal (SIGKILL) to the job process and updating the status to
  jobworker via pipe.

### Job Helper Process
The jobhelper process is a helper invoked by StartJob() library function. Its main purpose is to attach to cgroups before 
exec'ing the client requested command. It gets passed the JobId, the program command requested by the client and the 
write end  of a pipe command line arguments. 
jobhelper allows the jobworker to keep its responsibilities focused on monitoring the jobhelper and the status reported by
jobhelper via pipe, while jobhelper handles low-level system operations like setting up cgroups and namespaces and managing 
the job process itself. It performs the following tasks:
* Cgroup Association: It attaches itself to the cgroup created by the server
* Namespace Isolation: It sets up the job to run in isolated namespaces (PID, mount, and network) via flags CLONE_NEWPID, 
  CLONE_NEWNET, CLONE_NEWNS.
* Log Redirection: It redirects the job’s stdout and stderr to a log file named <JobID>.log.
* Job Execution: After performing the setup tasks, jobhelper uses os/Exec to execute the requested job as a child process.
* Monitoring: jobhelper monitors the child process and updates the exit status and signal number to the jobworker server.
* Termination: If it received SIGTERM from jobworker, jobhelper sends a SIGKILL signal to the job process to terminagte it.

#### Error reporting to the server process (jobworker)
Before jobhelper executes the requested job, it communicates any setup failures (like cgroup attachment or namespace setup errors)
through a pipe that was passed to it by the jobworker (StartJob() library function). StartJob() creates the pipe before invoking 
jobhelper. During the setup phase, jobhelper writes either an "OK" (for success) or "FAIL" to the pipe. The jobworker 
reads from the pipe. If an error message is received, the jobhelper terminates early, without starting the job process. The
jobworker returns the error to the client and updates the status "Server Error" in the JobMap status field.
The jobhelper also waits for the exit status of the job uses the same pipe to write the exit status to the jobworker.

### Library Functions
#### SetupCgroups() error
* Creates new memory, cpu and IO cgroup files (cpu.max, memory.max) and sets standard values in them. All jobs managed by the
jobworker Service will be bounded by these limits.

#### StartJob(command string, args []string, cpu_limit float, mem_limit int, io_limit int) (jobId string, error)
* Creates a unique JobId using uuidv4, adds the job to the jobMap under a mutex with the initial status "Running".
* Creates a pipe before invoking jobhelper, used to collect any setup failures from jobhelper.
* invokes jobhelper, passing the client’s job command and JobID as arguments. It also passes the pipe file descriptor to jobhelper
  for setup status communication.
* Starts the jobhelper process using os.Exec.
* Starts a new goroutine to monitor the jobhelper process. If jobhelper exits, it signals the pipe monitoring goroutine to terminate.
  and ends itself.
* Starts a new goroutine to monitor the job setup status and the job exit status reported by the jobhelper over the pipe. 
  if the process has errored it receives the error code or the terminating signal number from jobhelper. Once the process ends the goroutine
  acquires mutex and updates the status of the job in JobMap. It also notifies in the Job's exit channel that the process has ended.
* Returns the JobID for client interaction.

#### StopJob(JobID string) error
* Looks up the jobMap, if the job is "Running", gets the PID of jobhelper, sends a SIGTERM signal to the jobhelper process.
* The jobhelper terminates the job process with SIGKILL.
* The jobhelper writes the exit status to the pipe.
* The goroutine monitoring the pipe, started by StartJob() function, updates the job status in JobMap when the job process exits.
* The log file associated with the job is cleaned up.

#### GetJobStatus(JobID string) (string, error)
* Returns the current status of the job ("Running", "Exited", "Terminated", or "Server Error" with "exit_code" or "signal_number")
  and other job details like command and jobId.
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

#### CleanAllJobs() error
* Terminated any running jobs and removes all jobs from the jobMap

## TradeOffs
* The jobworker service runs with root priviledges so that it can isolate the processes into separate namespaces and can access 
  /proc. Rootless mode is possible but scoped out for this project
* UUIDv4 generates a 36 character jobID which may be too long for a good user experience
* Job log files rotation and cleanup is not considered
* No persistent connection between client and server. Every request is a new connection
* Client, Server and CA certs and keys are pre-generated
* Version management between the jobhelper and Server needs to be considered.

