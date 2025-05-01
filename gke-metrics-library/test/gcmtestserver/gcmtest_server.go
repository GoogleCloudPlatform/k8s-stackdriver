// Package gcmtestserver provides helpers for testing GCM clients against mock/fake servers.
package gcmtestserver

import (
	"fmt"
	"path"
	"testing"

	monitoringgrpc "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"gke-internal/gke-metrics/gke-metrics-library/test/grpctestserver"
	"google.golang.org/grpc"
	
)

// New creates a new GCM test server and returns its endpoint.
func New(t *testing.T, gcmServer monitoringgrpc.MetricServiceServer) string {
	setFakeServiceAccountDefaultCredentials(t)
	port := grpctestserver.NewWithAddr(t, "", func(srv *grpc.Server) {
		monitoringgrpc.RegisterMetricServiceServer(srv, gcmServer)
	})
	return fmt.Sprintf("localhost:%d", port)
}

func setFakeServiceAccountDefaultCredentials(t *testing.T) {
	packagePath := path.Join("..", "test/gcmtestserver")
	// we use a service_account.json with invalid credentials (but valid structure)
	// to make the client believe it can use it to authenticate to our test server
	serviceAccountPath := path.Join(packagePath, "invalid_test_service_account.json")
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", serviceAccountPath)
}
