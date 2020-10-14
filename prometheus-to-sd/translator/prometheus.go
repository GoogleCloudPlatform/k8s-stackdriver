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

package translator

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/prometheus-to-sd/config"
)

const customMetricsPrefix = "custom.googleapis.com"

// PrometheusResponse represents unprocessed response from Prometheus endpoint.
type PrometheusResponse struct {
	rawResponse string
}

var prometheusClient *http.Client

func init() {
	// Copy a DefaultTransport that is used by default http client.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Allow insecure https connections.
	transport.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: true,
	}
	prometheusClient = &http.Client{
		Timeout:   time.Second * 30,
		Transport: transport,
	}
}

// GetPrometheusMetrics scrapes metrics from the given host and port using /metrics handler.
func GetPrometheusMetrics(config *config.SourceConfig) (*PrometheusResponse, error) {
	res, err := getPrometheusMetrics(config)
	if err != nil {
		componentMetricsAvailable.WithLabelValues(config.Component).Set(0.0)
	} else {
		componentMetricsAvailable.WithLabelValues(config.Component).Set(1.0)
	}
	return res, err
}

func getPrometheusMetrics(config *config.SourceConfig) (*PrometheusResponse, error) {
	url := fmt.Sprintf("%s://%s:%d%s", config.Protocol, config.Host, config.Port, config.Path)
	resp, err := doPrometheusRequest(url, config.AuthConfig)
	if err != nil {
		return nil, fmt.Errorf("request %s failed: %v", url, err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body - %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed - %q, response: %q", resp.Status, string(body))
	}
	return &PrometheusResponse{rawResponse: string(body)}, nil
}

func doPrometheusRequest(url string, auth config.AuthConfig) (resp *http.Response, err error) {
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if len(auth.Username) > 0 {
		request.SetBasicAuth(auth.Username, auth.Password)
	} else if len(auth.Token) > 0 {
		request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", auth.Token))
	}
	return prometheusClient.Do(request)
}

// Build performs parsing and processing of the prometheus metrics response.
func (p *PrometheusResponse) Build() (map[string]*dto.MetricFamily, error) {
	parser := &expfmt.TextParser{}
	return parser.TextToMetricFamilies(strings.NewReader(p.rawResponse))
}
