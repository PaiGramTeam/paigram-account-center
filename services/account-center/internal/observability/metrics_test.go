package observability

import (
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/tlstest"
	"github.com/glebarez/sqlite"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"paigram/internal/email"
	"paigram/internal/model"
)

func TestMetricsHandlerReportsStableBacklogAgeAndDeadLetters(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PlatformOperationIntent{}, &model.PlatformOperationOutbox{}))
	now := time.Now().UTC()
	require.NoError(t, db.Create(&[]model.PlatformOperationIntent{
		{OperationID: "due", BindingRef: "binding-due", Platform: "mihomo", Kind: "refresh", RequestFingerprint: "fingerprint-due", DeliveryMode: model.PlatformOperationDeliveryModeOutbox, State: model.PlatformOperationIntentStatePendingDelivery, ActorType: "system", ActorID: "metrics", CreatedAt: now.Add(-5 * time.Minute), UpdatedAt: now},
		{OperationID: "future", BindingRef: "binding-future", Platform: "mihomo", Kind: "refresh", RequestFingerprint: "fingerprint-future", DeliveryMode: model.PlatformOperationDeliveryModeOutbox, State: model.PlatformOperationIntentStateUncertain, ActorType: "system", ActorID: "metrics", CreatedAt: now.Add(-time.Hour), UpdatedAt: now},
		{OperationID: "input", BindingRef: "binding-input", Platform: "mihomo", Kind: "bind", RequestFingerprint: "fingerprint-input", DeliveryMode: model.PlatformOperationDeliveryModeOutbox, State: model.PlatformOperationIntentStateInputRequired, ActorType: "system", ActorID: "metrics", CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now},
		{OperationID: "dead", BindingRef: "binding-dead", Platform: "mihomo", Kind: "refresh", RequestFingerprint: "fingerprint-dead", DeliveryMode: model.PlatformOperationDeliveryModeOutbox, State: model.PlatformOperationIntentStateFailed, ActorType: "system", ActorID: "metrics", CreatedAt: now, UpdatedAt: now},
	}).Error)
	require.NoError(t, db.Create(&[]model.PlatformOperationOutbox{
		{OperationID: "due", Status: model.PlatformOperationOutboxStatusPending, AvailableAt: now.Add(-2 * time.Minute), CreatedAt: now.Add(-5 * time.Minute), UpdatedAt: now},
		{OperationID: "future", Status: model.PlatformOperationOutboxStatusPending, AvailableAt: now.Add(2 * time.Minute), CreatedAt: now.Add(-time.Hour), UpdatedAt: now},
		{OperationID: "input", Status: model.PlatformOperationOutboxStatusPending, AvailableAt: now.Add(time.Hour), CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now},
		{OperationID: "dead", Status: model.PlatformOperationOutboxStatusDeadLetter, AvailableAt: now, CreatedAt: now, UpdatedAt: now},
	}).Error)

	tlsBundle := tlstest.New(t, "metrics.internal")
	email.EmailRateLimitExceeded.Inc()
	handler := NewMetricsHandler(db, []CertificateTarget{{Identity: "grpc-server", CertificateFile: tlsBundle.ServerCertFile}})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, string(body), "paigram_reconciliation_outbox_due 1")
	assert.Contains(t, string(body), "paigram_reconciliation_outbox_backlog 2")
	assert.Contains(t, string(body), "paigram_reconciliation_outbox_dead_letter 1")
	ageMatch := regexp.MustCompile(`paigram_reconciliation_outbox_oldest_backlog_age_seconds ([0-9.e+-]+)`).FindStringSubmatch(string(body))
	require.Len(t, ageMatch, 2)
	age, err := strconv.ParseFloat(ageMatch[1], 64)
	require.NoError(t, err)
	assert.Greater(t, age, float64(59*time.Minute/time.Second))
	assert.Contains(t, string(body), `paigram_key_material_expiry_timestamp_seconds{identity="grpc-server",service="account-center"}`)
	assert.Contains(t, string(body), `paigram_key_material_read_error{identity="grpc-server",service="account-center"} 0`)
	assert.NotContains(t, string(body), "email_rate_limit_exceeded_total")
	assert.NotContains(t, string(body), "recipient=")
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

func TestTicketRejectionMetricUsesBoundedReasonCodes(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := newTicketMetrics(registry)
	metrics.RecordTicketRejection("scope_not_granted")
	metrics.RecordTicketRejection("unexpected dynamic reason")

	families, err := registry.Gather()
	require.NoError(t, err)
	require.Len(t, families, 1)
	labels := map[string]float64{}
	for _, metric := range families[0].Metric {
		for _, label := range metric.Label {
			if label.GetName() == "reason" {
				labels[label.GetValue()] = metric.Counter.GetValue()
			}
		}
	}
	assert.Equal(t, float64(1), labels["scope_not_granted"])
	assert.Equal(t, float64(1), labels["other"])
}
