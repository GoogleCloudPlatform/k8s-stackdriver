package gcm

import (
	"context"
	"strings"
	"testing"

	monitoringgrpc "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	monitoringpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"gke-internal/gke-metrics/gke-metrics-library/test/gcmtestserver"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

func TestCreateClient(t *testing.T) {
	testcases := []struct {
		desc         string
		options      []option.ClientOption
		wantTLSError bool
		wantErr      bool
	}{
		{
			desc: "insecure connection",
			options: []option.ClientOption{
				option.WithoutAuthentication(),
				option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
			},
			wantTLSError: false,
			wantErr:      false,
		},
		{
			desc: "secure connection to insecure server should fail with TLS error",
			// we want a TLS error here as the test server is insecure and the client
			// won't be able to establish a secure connection
			wantTLSError: true,
			wantErr:      false,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			gcmServer := &testGCMServer{
				expectCredentials: true,
			}
			endpoint := gcmtestserver.New(t, gcmServer)
			client, err := CreateClient(context.TODO(), endpoint, tc.options...)
			if tc.wantErr && err == nil {
				t.Fatalf("CreateClient(ctx, %s, %v) = nil, want error", endpoint, tc.options)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("CreateClient(ctx, %s, %v) = %q, want nil", endpoint, tc.options, err)
			}

			// make a call using the client to check credentials
			if _, err := client.CreateServiceTimeSeries(context.TODO(), &monitoringpb.CreateTimeSeriesRequest{}); err != nil {
				if tc.wantTLSError && !isTLSError(err) {
					t.Errorf("CreateServiceTimeSeries(...) = %v, want TLS error", err)
				} else if !tc.wantTLSError {
					t.Errorf("CreateServiceTimeSeries(...) = %v, want nil", err)
				}
			}
		})
	}
}

func isTLSError(err error) bool {
	statusErr, ok := status.FromError(err)
	if !ok {
		return false
	}
	if statusErr.Code() != codes.Unavailable {
		return false
	}
	if strings.Contains(statusErr.Message(), "first record does not look like a TLS handshake") {
		return true
	}
	return false
}

type testGCMServer struct {
	expectCredentials bool
	monitoringgrpc.UnimplementedMetricServiceServer
}

func (svc *testGCMServer) CreateServiceTimeSeries(ctx context.Context, req *monitoringpb.CreateTimeSeriesRequest) (*emptypb.Empty, error) {
	if !svc.expectCredentials {
		return nil, nil
	}
	_, ok := peer.FromContext(ctx)
	if !ok {
		return nil, status.Error(codes.PermissionDenied, "permission denied: no peer info")
	}

	return &emptypb.Empty{}, nil
}
