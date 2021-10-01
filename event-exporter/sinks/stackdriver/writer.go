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

package stackdriver

import (
	"net/http"
	"strconv"
	"time"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/api/googleapi"
	sd "google.golang.org/api/logging/v2"
)

const (
	retryDelay = 10 * time.Second
)

var (
	requestCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      "request_count",
			Help:      "Number of request, issued to Stackdriver API",
			Subsystem: "stackdriver_sink",
		},
		[]string{"code"},
	)

	successfullySentEntryCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name:      "successfully_sent_entry_count",
			Help:      "Number of entries successfully ingested by Stackdriver",
			Subsystem: "stackdriver_sink",
		},
	)
)

type sdWriter interface {
	Write([]*sd.LogEntry, string, *sd.MonitoredResource)
}

type sdWriterImpl struct {
	service *sd.Service
}

func newSdWriter(service *sd.Service) sdWriter {
	return &sdWriterImpl{
		service: service,
	}
}

// Writer writes log entries to Stackdriver. It retries writing logs forever
// unless the API returns BadRequest error.
func (w sdWriterImpl) Write(entries []*sd.LogEntry, logName string, resource *sd.MonitoredResource) {
	req := &sd.WriteLogEntriesRequest{
		Entries:  entries,
		LogName:  logName,
		Resource: resource,
	}

	// We retry forever, until request either succeeds or API returns
	// BadRequest, which means that the request is malformed, e.g. because
	// it contains too large entries. This behavior mirrors the way logging
	// agent pushes logs to Stackdriver.
	for {
		res, err := w.service.Entries.Write(req).Do()

		// The entry is successfully sent to Stackdriver.
		if err == nil {
			requestCount.WithLabelValues(strconv.Itoa(res.HTTPStatusCode)).Inc()
			successfullySentEntryCount.Add(float64(len(entries)))
			break
		}

		apiErr, ok := err.(*googleapi.Error)
		if ok {
			requestCount.WithLabelValues(strconv.Itoa(apiErr.Code)).Inc()

			// Bad request from Stackdriver most probably indicates that some entries
			// are in bad format, which means they won't be ingested after retry also,
			// so it doesn't make sense to try again.
			// TODO: Check response properly and return the actual number of
			// successfully ingested entries, parsed out from the response body.
			if apiErr.Code == http.StatusBadRequest {
				glog.Warningf("Received bad request response from server, "+
					"assuming some entries were rejected: %v", err)
				break
			}
		}

		glog.Warningf("Failed to send request to Stackdriver: %v", err)
		time.Sleep(retryDelay)
	}
}
