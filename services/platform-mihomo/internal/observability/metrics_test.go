package observability

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/tlstest"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetricsReportKeyExpiryAndDegradedOperations(t *testing.T) {
	tlsBundle := tlstest.New(t, "platform-metrics.internal")
	metrics := NewMetrics([]CertificateTarget{{Identity: "runtime-server", CertificateFile: tlsBundle.ServerCertFile}})
	metrics.RecordUpstreamResult("refresh", true)
	metrics.RecordTicketRejection("control", "Unauthenticated")

	response := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, string(body), `paigram_platform_upstream_degraded{operation="refresh"} 1`)
	assert.Contains(t, string(body), `paigram_service_ticket_rejections_total{reason="unauthenticated",service="platform-mihomo",surface="control"} 1`)
	assert.Contains(t, string(body), `paigram_key_material_expiry_timestamp_seconds{identity="runtime-server",service="platform-mihomo"}`)
	assert.Contains(t, string(body), `paigram_key_material_read_error{identity="runtime-server",service="platform-mihomo"} 0`)
}

func TestMetricsHandlerLimitsConcurrentScrapes(t *testing.T) {
	collector := newBlockingCollector()
	registry := prometheus.NewRegistry()
	registry.MustRegister(collector)
	handler := newPrometheusHandler(registry)
	statuses := make(chan int, metricsMaxRequestsInFlight)
	for range metricsMaxRequestsInFlight {
		go func() {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
			statuses <- response.Code
		}()
	}
	for range metricsMaxRequestsInFlight {
		select {
		case <-collector.started:
		case <-time.After(time.Second):
			t.Fatal("metrics collection did not start")
		}
	}

	overflow := httptest.NewRecorder()
	handler.ServeHTTP(overflow, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	assert.Equal(t, http.StatusServiceUnavailable, overflow.Code)
	close(collector.release)
	for range metricsMaxRequestsInFlight {
		select {
		case status := <-statuses:
			assert.Equal(t, http.StatusOK, status)
		case <-time.After(time.Second):
			t.Fatal("metrics collection did not finish")
		}
	}
}

type blockingCollector struct {
	desc    *prometheus.Desc
	started chan struct{}
	release chan struct{}
}

func newBlockingCollector() *blockingCollector {
	return &blockingCollector{
		desc:    prometheus.NewDesc("paigram_test_blocking_metric", "Test metric.", nil, nil),
		started: make(chan struct{}, metricsMaxRequestsInFlight),
		release: make(chan struct{}),
	}
}

func (c *blockingCollector) Describe(output chan<- *prometheus.Desc) {
	output <- c.desc
}

func (c *blockingCollector) Collect(output chan<- prometheus.Metric) {
	c.started <- struct{}{}
	<-c.release
	output <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, 1)
}

func TestMetricsBoundLabels(t *testing.T) {
	metrics := NewMetrics(nil)
	metrics.RecordUpstreamResult("dynamic-operation", true)
	metrics.RecordTicketRejection("dynamic-surface", "dynamic reason")
	metrics.RecordTicketRejection("runtime", "PermissionDenied")

	response := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	assert.Contains(t, string(body), `paigram_platform_upstream_degraded{operation="other"} 1`)
	assert.Contains(t, string(body), `paigram_service_ticket_rejections_total{reason="other",service="platform-mihomo",surface="other"} 1`)
	assert.Contains(t, string(body), `paigram_service_ticket_rejections_total{reason="permission_denied",service="platform-mihomo",surface="runtime"} 1`)
}
