/*
Copyright 2026 Google Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package gcm is the only package that talks to the Cloud Monitoring API:
// a thin client wrapper (bounded concurrency, instrumentation hooks), a
// periodically refreshed metric-descriptor cache, and an in-memory fake.
package gcm

import (
	"context"
	"fmt"
	"strings"
	"time"

	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"google.golang.org/api/iterator"
	"google.golang.org/genproto/googleapis/api/metric"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SeriesQuery asks for the recent points of exactly one metric type — the
// ListTimeSeries filter grammar does not allow more than one per call.
type SeriesQuery struct {
	MetricType string
	// ExtraFilters are ANDed onto the metric.type equality (cluster scope
	// restriction, user-supplied filter). Each must already be valid
	// Monitoring filter syntax.
	ExtraFilters []string
	Start, End   time.Time
	PageSize     int32
}

// Client is the surface the rest of the proxy uses; Fake implements it too.
type Client interface {
	// ListDescriptors returns metric descriptors matching filter
	// ("" = all in the project).
	ListDescriptors(ctx context.Context, filter string) ([]*metric.MetricDescriptor, error)
	// ListSeries returns all series (full points) for one query.
	ListSeries(ctx context.Context, q SeriesQuery) ([]*monitoringpb.TimeSeries, error)
	Close() error
}

// Hooks receives instrumentation callbacks; any field may be nil.
type Hooks struct {
	// OnAPICall fires once per API method invocation with the gRPC code.
	OnAPICall func(method string, code codes.Code)
	// OnSeriesReturned fires with the number of series a ListSeries call
	// returned — the unit GCM bills reads by (min 1 per call).
	OnSeriesReturned func(metricType string, n int)
}

func (h Hooks) apiCall(method string, err error) {
	if h.OnAPICall != nil {
		h.OnAPICall(method, status.Code(err))
	}
}

type client struct {
	mc      *monitoring.MetricClient
	project string
	sem     chan struct{}
	hooks   Hooks
}

// New dials the Monitoring API using ADC. concurrency bounds in-flight
// calls across all callers of this client.
func New(ctx context.Context, projectID string, concurrency int, hooks Hooks) (Client, error) {
	if concurrency < 1 {
		concurrency = 1
	}
	mc, err := monitoring.NewMetricClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating monitoring client: %w", err)
	}
	return &client{
		mc:      mc,
		project: "projects/" + projectID,
		sem:     make(chan struct{}, concurrency),
		hooks:   hooks,
	}, nil
}

func (c *client) acquire(ctx context.Context) error {
	select {
	case c.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *client) ListDescriptors(ctx context.Context, filter string) ([]*metric.MetricDescriptor, error) {
	if err := c.acquire(ctx); err != nil {
		return nil, err
	}
	defer func() { <-c.sem }()

	it := c.mc.ListMetricDescriptors(ctx, &monitoringpb.ListMetricDescriptorsRequest{
		Name:   c.project,
		Filter: filter,
	})
	var out []*metric.MetricDescriptor
	for {
		d, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			c.hooks.apiCall("ListMetricDescriptors", err)
			return nil, fmt.Errorf("listing metric descriptors (filter %q): %w", filter, err)
		}
		out = append(out, d)
	}
	c.hooks.apiCall("ListMetricDescriptors", nil)
	return out, nil
}

func (c *client) ListSeries(ctx context.Context, q SeriesQuery) ([]*monitoringpb.TimeSeries, error) {
	if err := c.acquire(ctx); err != nil {
		return nil, err
	}
	defer func() { <-c.sem }()

	it := c.mc.ListTimeSeries(ctx, &monitoringpb.ListTimeSeriesRequest{
		Name:   c.project,
		Filter: BuildFilter(q.MetricType, q.ExtraFilters),
		Interval: &monitoringpb.TimeInterval{
			StartTime: timestamppb.New(q.Start),
			EndTime:   timestamppb.New(q.End),
		},
		View:     monitoringpb.ListTimeSeriesRequest_FULL,
		PageSize: q.PageSize,
	})
	var out []*monitoringpb.TimeSeries
	for {
		ts, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			c.hooks.apiCall("ListTimeSeries", err)
			return nil, fmt.Errorf("listing time series for %s: %w", q.MetricType, err)
		}
		out = append(out, ts)
	}
	c.hooks.apiCall("ListTimeSeries", nil)
	if c.hooks.OnSeriesReturned != nil {
		c.hooks.OnSeriesReturned(q.MetricType, len(out))
	}
	return out, nil
}

func (c *client) Close() error { return c.mc.Close() }

// BuildFilter assembles the ListTimeSeries filter: the mandatory single
// metric.type equality plus ANDed extras.
func BuildFilter(metricType string, extras []string) string {
	parts := make([]string, 0, 1+len(extras))
	parts = append(parts, fmt.Sprintf("metric.type = %q", metricType))
	for _, e := range extras {
		if e = strings.TrimSpace(e); e != "" {
			parts = append(parts, "("+e+")")
		}
	}
	return strings.Join(parts, " AND ")
}

// IsQuotaErr reports whether err is a quota/rate exhaustion the poller
// should respond to by backing off a cycle rather than retrying hot.
func IsQuotaErr(err error) bool {
	return status.Code(err) == codes.ResourceExhausted
}
