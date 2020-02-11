/*
Copyright 2017 Google Inc.

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

package monitor

import (
	"time"

	log "github.com/golang/glog"
	v3 "google.golang.org/api/monitoring/v3"
)

const maxTimeSeriesPerRequest = 200

// SourceConfig is the set of data required to configure a kubernetes
// data source (e.g., kubelet or kube-controller).
type SourceConfig struct {
	Zone, Project, Cluster, ClusterLocation, Host, Instance, InstanceID, SchemaPrefix string
	MonitoredResourceLabels                                                           map[string]string
	Port                                                                              uint
	Resolution                                                                        time.Duration
}

// MetricsSource is an object that provides kubernetes metrics in
// Stackdriver format, probably from a backend like the kubelet.
type MetricsSource interface {
	GetTimeSeriesReq() (*v3.CreateTimeSeriesRequest, error)
	Name() string
	ProjectPath() string
}

// Once polls the backend and puts the data to the given service one time.
func Once(src MetricsSource, gcm *v3.Service) {
	scrapeTimestamp := time.Now()
	req, err := src.GetTimeSeriesReq()
	if err != nil {
		observeFailedScrape(src.Name())
		log.Warningf("Failed to create time series request: %v", err)
		return
	}
	observeSuccessfullScrape(src.Name())

	for _, subReq := range subRequests(req) {
		// Push that data to GCM's v3 API.
		createCall := gcm.Projects.TimeSeries.Create(src.ProjectPath(), subReq)
		if empty, err := createCall.Do(); err != nil {
			log.Warningf("Failed to write time series data, empty: %v, err: %v", empty, err)
			observeFailedRequest(len(subReq.TimeSeries))
			jsonReq, err := subReq.MarshalJSON()
			if err != nil {
				log.Warningf("Failed to marshal time series as JSON")
				return
			}
			log.Warningf("JSON GCM: %s", string(jsonReq[:]))
			return
		}
		log.V(4).Infof("Successfully wrote TimeSeries data for %s to GCM v3 API.", src.Name())
		observeSuccessfullRequest(len(subReq.TimeSeries))
		observeIngestionLatency(len(subReq.TimeSeries), time.Now().Sub(scrapeTimestamp).Seconds())
	}
}

func subRequests(req *v3.CreateTimeSeriesRequest) []*v3.CreateTimeSeriesRequest {
	tsCount := len(req.TimeSeries)
	if tsCount <= maxTimeSeriesPerRequest {
		return []*v3.CreateTimeSeriesRequest{req}
	}
	subRequestsCount := (tsCount-1)/maxTimeSeriesPerRequest + 1
	log.V(2).Infof("Splitting CreateTimeSeriesRequest into %v requests", subRequestsCount)
	subReqs := make([]*v3.CreateTimeSeriesRequest, subRequestsCount)
	for i := 0; i < subRequestsCount; i++ {
		startIdx := i * maxTimeSeriesPerRequest
		endIdx := (i + 1) * maxTimeSeriesPerRequest
		if endIdx > tsCount {
			endIdx = tsCount
		}
		subReqs[i] = &v3.CreateTimeSeriesRequest{
			TimeSeries: req.TimeSeries[startIdx:endIdx],
		}
	}
	return subReqs
}
