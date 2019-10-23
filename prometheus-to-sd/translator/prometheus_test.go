package translator

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/prometheus-to-sd/config"
)

func TestScrapePrometheusMetrics(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer s.Close()
	for n := 0; n < 100000; n++ {
		resp, err := doPrometheusRequest(s.URL, config.AuthConfig{})
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		resp.Body.Close()
	}
}
