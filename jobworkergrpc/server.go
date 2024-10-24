// Package main implements a gRPC server for managing jobs through the JobWorker service.
// It provides operations to start, stop, monitor, and clean jobs, using mutual TLS (mTLS)
// for authentication and authorization.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net"
	"os"

	"github.com/job-worker-service/jobworker"              // JobWorker library
	pb "github.com/job-worker-service/jobworkergrpc/proto" // Protobuf definitions

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

var logger *log.Logger

// initLogger initializes the logger used for the server. Logs are written
// to "server.log" with timestamps and file locations.
func initLogger() {
	logFilePath := "server.log"
	logFile, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("[ERROR] Failed to open log file: %v", err)
	}
	logger = log.New(logFile, "server: ", log.LstdFlags|log.Lshortfile)
}

// JobWorkerServer implements the JobWorker gRPC service.
type JobWorkerServer struct {
	pb.UnimplementedJobWorkerServer
	jobManager *jobworker.JobManager
}

// NewJobWorkerServer creates and returns a new JobWorkerServer instance.
func NewJobWorkerServer() *JobWorkerServer {
	return &JobWorkerServer{jobManager: jobworker.NewJobManager()}
}

// StartJob handles a gRPC request to start a new job with the specified command and arguments.
func (s *JobWorkerServer) StartJob(ctx context.Context, req *pb.StartJobRequest) (*pb.StartJobResponse, error) {
	jobID, err := s.jobManager.StartJob(req.Command, req.Args)
	if err != nil {
		logger.Printf("[ERROR] Failed to start job: %v", err)
		return nil, err
	}
	logger.Printf("[INFO] Started job with ID: %s", jobID)
	return &pb.StartJobResponse{JobId: jobID}, nil
}

// StopJob handles a gRPC request to stop a running job with the given job ID.
func (s *JobWorkerServer) StopJob(ctx context.Context, req *pb.StopJobRequest) (*pb.StopJobResponse, error) {
	err := s.jobManager.StopJob(req.JobId)
	if err != nil {
		logger.Printf("[ERROR] Failed to stop job: %v", err)
		return nil, err
	}
	logger.Printf("[INFO] Stopped job with ID: %s", req.JobId)
	return &pb.StopJobResponse{}, nil
}

// GetJobStatus retrieves the current status of a job with the specified job ID.
func (s *JobWorkerServer) GetJobStatus(ctx context.Context, req *pb.GetJobStatusRequest) (*pb.JobStatus, error) {
	status, exists := s.jobManager.GetJobStatus(req.JobId)
	if !exists {
		logger.Printf("[ERROR] Job not found: %s", req.JobId)
		return nil, fmt.Errorf("job not found")
	}
	logger.Printf("[INFO] Retrieved status for job ID: %s", req.JobId)
	return &pb.JobStatus{
		JobId:    req.JobId,
		Status:   string(status.Status),
		Command:  status.Cmd,
		ExitCode: fmt.Sprintf("%d", status.ExitCode),
		SigNum:   fmt.Sprintf("%d", status.SignalNum),
	}, nil
}

// CleanAllJobs stops all running jobs and clears the job map.
func (s *JobWorkerServer) CleanAllJobs(ctx context.Context, req *pb.CleanAllJobsRequest) (*pb.CleanAllJobsResponse, error) {
	logger.Printf("[INFO] Cleaning all jobs")

	defer func() {
		if r := recover(); r != nil {
			logger.Printf("[ERROR] Panic occurred during CleanAllJobs: %v", r)
		}
	}()

	s.jobManager.CleanAllJobs()
	logger.Printf("[INFO] All jobs cleaned successfully")

	return &pb.CleanAllJobsResponse{
		Success: true,
	}, nil
}

// StreamJobOutput streams the output of a specified job to the client.
func (s *JobWorkerServer) StreamJobOutput(req *pb.StreamJobOutputRequest, stream pb.JobWorker_StreamJobOutputServer) error {
	ctx := stream.Context()
	logger.Printf("[INFO] Starting output stream for job ID: %s", req.JobId)

	// Pass the streamFunc to the library to send data back to the client.
	streamFunc := func(data []byte) error {
		// Send the data to the client via the gRPC stream.
		if err := stream.Send(&pb.JobOutputResponse{Output: data}); err != nil {
			logger.Printf("[ERROR] Failed to send data to client: %v", err)
			return err
		}
		return nil
	}

	// Call the library API with the provided context, job ID, and stream function.
	return s.jobManager.StreamJobOutput(ctx, req.JobId, streamFunc)
}

// ListJobs returns the current list of jobs with their statuses.
func (s *JobWorkerServer) ListJobs(ctx context.Context, req *pb.ListJobsRequest) (*pb.ListJobsResponse, error) {
	jobs := s.jobManager.ListJobs()
	logger.Printf("[INFO] Retrieved list of jobs")
	var jobList []*pb.JobStatus
	for _, job := range jobs {
		jobList = append(jobList, &pb.JobStatus{
			JobId:    job.ID,
			Status:   string(job.Status),
			Command:  job.Cmd,
			ExitCode: fmt.Sprintf("%d", job.ExitCode),
			SigNum:   fmt.Sprintf("%d", job.SignalNum),
		})
	}
	return &pb.ListJobsResponse{JobList: jobList}, nil
}

// SetupTLSConfig loads the server's certificates and sets up a TLS configuration for mTLS authentication.
func SetupTLSConfig() (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair("../certs/server.crt", "../certs/server.key")
	if err != nil {
		logger.Printf("[ERROR] Failed to load server key pair: %v", err)
		return nil, fmt.Errorf("failed to load server key pair: %w", err)
	}

	caCert, err := os.ReadFile("../certs/ca.crt")
	if err != nil {
		logger.Printf("[ERROR] Failed to load CA certificate: %v", err)
		return nil, fmt.Errorf("failed to load CA certificate: %w", err)
	}
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		logger.Printf("[ERROR] Failed to append CA certificate")
		return nil, fmt.Errorf("failed to append CA certificate")
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    caCertPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
		VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			if len(verifiedChains) == 0 {
				logger.Printf("[ERROR] No verified chains found")
				return fmt.Errorf("client certificate verification failed")
			}
			logger.Printf("[INFO] Successfully verified client certificate")
			return nil
		},
		VerifyConnection: func(state tls.ConnectionState) error {
			logger.Printf("[INFO] TLS handshake completed with peer: %v", state.PeerCertificates)
			return nil
		},
	}
	logger.Printf("[INFO] TLS configuration set up successfully")
	return tlsConfig, nil
}

// AuthorizationInterceptor validates the client's certificate Common Name (CN).
func AuthorizationInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		p, ok := peer.FromContext(ctx)
		if !ok {
			logger.Printf("[ERROR] No peer info in context")
			return nil, status.Error(codes.Unauthenticated, "no peer info")
		}

		tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
		if !ok {
			logger.Printf("[ERROR] No TLS info found")
			return nil, status.Error(codes.Unauthenticated, "no TLS info")
		}

		cn := tlsInfo.State.VerifiedChains[0][0].Subject.CommonName
		if cn != "jobworkercli" {
			logger.Printf("[ERROR] Unauthorized client with CN: %s", cn)
			return nil, status.Error(codes.PermissionDenied, "unauthorized client")
		}

		logger.Printf("[INFO] Authenticated & Authorized the client with CN: %s", cn)
		return handler(ctx, req)
	}
}

// main initializes the gRPC server and starts listening for client requests on port 60001.
func main() {
	initLogger()

	tlsConfig, err := SetupTLSConfig()
	if err != nil {
		log.Fatalf("[ERROR] Failed to setup TLS: %v", err)
	}

	lis, err := net.Listen("tcp", ":60001")
	if err != nil {
		log.Fatalf("[ERROR] Failed to listen on port 60001: %v", err)
	}

	grpcServer := grpc.NewServer(
		grpc.Creds(credentials.NewTLS(tlsConfig)),
		grpc.UnaryInterceptor(AuthorizationInterceptor()),
	)

	pb.RegisterJobWorkerServer(grpcServer, NewJobWorkerServer())

	logger.Printf("[INFO] Starting gRPC server on port 60001")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("[ERROR] Failed to serve: %v", err)
	}
}
