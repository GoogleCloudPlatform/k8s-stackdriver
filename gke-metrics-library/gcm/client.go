package gcm

import (
	"context"
	"fmt"

	monitoringgrpc "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"google.golang.org/api/option"
	"google.golang.org/api/transport"
)

// CreateClient for sending requests to the provided Google Cloud Monitoring endpoint.
func CreateClient(ctx context.Context, cloudMonarchEndpoint string, options ...option.ClientOption) (monitoringgrpc.MetricServiceClient, error) {
	conn, err := transport.DialGRPC(ctx, append(options, option.WithEndpoint(cloudMonarchEndpoint))...)
	if err != nil {
		return nil, fmt.Errorf("dialing cloud monarch %s: %q", cloudMonarchEndpoint, err)
	}

	return monitoringgrpc.NewMetricServiceClient(conn), nil
}
