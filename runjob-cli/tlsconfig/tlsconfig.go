// Package tlsconfig provides utilities for establishing gRPC connections
// using mutual TLS (mTLS) credentials.
package tlsconfig

import (
	"log"

	"crypto/tls"
	"crypto/x509"
	"io/ioutil"

	pb "github.com/job-worker-service/jobworkergrpc/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// GetClient establishes a gRPC connection with mutual TLS (mTLS) credentials
// and returns the connection along with a JobWorkerClient instance.
// It terminates the program if the connection or credentials cannot be initialized.
func GetClient() (*grpc.ClientConn, pb.JobWorkerClient) {
	// Load the TLS credentials required for mTLS.
	creds, err := loadTLSCredentials()
	if err != nil {
		log.Fatalf("Failed to load TLS credentials: %v", err)
	}

	// Establish a secure gRPC connection with the server.
	conn, err := grpc.Dial("localhost:60001", grpc.WithTransportCredentials(creds))
	if err != nil {
		log.Fatalf("Failed to connect to server: %v", err)
	}

	// Create a new JobWorkerClient using the established connection.
	client := pb.NewJobWorkerClient(conn)
	return conn, client
}

// loadTLSCredentials loads the client's certificate, private key, and
// the CA certificate required for mutual TLS (mTLS).
// It returns the configured TransportCredentials or an error if the certificates
// cannot be loaded or configured properly.
func loadTLSCredentials() (credentials.TransportCredentials, error) {
	// Load the CA's certificate to verify the server's certificate.
	pem, err := ioutil.ReadFile("../certs/ca.crt")
	if err != nil {
		return nil, err
	}
	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(pem) {
		return nil, err
	}

	// Load the client's certificate and private key for authentication.
	clientCert, err := tls.LoadX509KeyPair("../certs/client.crt", "../certs/client.key")
	if err != nil {
		return nil, err
	}

	// Create a TLS configuration using the loaded credentials.
	config := &tls.Config{
		Certificates: []tls.Certificate{clientCert}, // Client's certificate and key
		RootCAs:      certPool,                      // CA certificate pool for verification
		ServerName:   "localhost",                   // Must match the server certificate's CN
	}

	// Return the configured TLS credentials.
	return credentials.NewTLS(config), nil
}
