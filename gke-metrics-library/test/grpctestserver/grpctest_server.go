// Package grpctestserver provides helpers for testing GRPC clients against mock/fake servers.
package grpctestserver

import (
	"net"
	"testing"

	"google.golang.org/grpc"
)

// NewWithAddr creates a GRPC server on the provided address.
// The registerServices function is called before the server is started and should
// be used to register services.
// If no address is specified a random port will be used.
// The function returns the port used.
// The passed in testing.T is used for logging and to fail the test in case of errors during
// server creation or shutdown.
// The gRPC server is shutdown and closed on t.Cleanup.
func NewWithAddr(t *testing.T, addr string, registerServices func(*grpc.Server), opt ...grpc.ServerOption) int {
	t.Helper()
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("server failed to listen: %v", err)
	}

	port := listener.Addr().(*net.TCPAddr).Port
	server := grpc.NewServer(opt...)

	registerServices(server)

	finishedShutdown := make(chan struct{})
	go func() {
		if err := server.Serve(listener); err != nil && err != grpc.ErrServerStopped {
			t.Errorf("grpc server returned: %v", err)
		}
		close(finishedShutdown)
	}()

	t.Cleanup(func() {
		server.Stop()
		<-finishedShutdown
		// server.Stop() should close this, but retry to avoid resource leak.
		listener.Close()
	})
	return port
}
