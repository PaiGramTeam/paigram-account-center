package observability

import (
	"context"
	"net/http"
	"time"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/certificateexpiry"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gorm.io/gorm"

	"paigram/internal/model"
)

var ticketReasonCodes = map[string]struct{}{
	"authorization_state_unavailable": {},
	"bot_identity_not_found":          {},
	"consumer_grant_not_found":        {},
	"consumer_grant_revoked":          {},
	"consumer_not_supported":          {},
	"inactive_binding":                {},
	"internal_error":                  {},
	"invalid_ticket_config":           {},
	"platform_account_missing":        {},
	"platform_service_unavailable":    {},
	"scope_not_granted":               {},
	"signing_key_unavailable":         {},
}

var accountMetricsRegistry = prometheus.NewRegistry()
var accountTicketMetrics = newTicketMetrics(accountMetricsRegistry)

const (
	metricsMaxRequestsInFlight = 2
	metricsHandlerTimeout      = 3 * time.Second
)

type CertificateTarget struct {
	Identity        string
	CertificateFile string
}

type ticketMetrics struct {
	rejections *prometheus.CounterVec
}

func newTicketMetrics(registerer prometheus.Registerer) *ticketMetrics {
	rejections := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "paigram_service_ticket_rejections_total",
		Help: "Total number of rejected service ticket operations.",
	}, []string{"service", "surface", "reason"})
	registerer.MustRegister(rejections)
	return &ticketMetrics{rejections: rejections}
}

func (m *ticketMetrics) RecordTicketRejection(reason string) {
	if _, ok := ticketReasonCodes[reason]; !ok {
		reason = "other"
	}
	m.rejections.WithLabelValues("account-center", "issuer", reason).Inc()
}

func RecordTicketRejection(reason string) {
	accountTicketMetrics.RecordTicketRejection(reason)
}

func NewMetricsHandler(db *gorm.DB, certificates []CertificateTarget) http.Handler {
	registry := prometheus.NewRegistry()
	registry.MustRegister(newReconciliationCollector(db), newKeyMaterialCollector(certificates))
	return newPrometheusHandler(prometheus.Gatherers{accountMetricsRegistry, registry})
}

func newPrometheusHandler(gatherer prometheus.Gatherer) http.Handler {
	return promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{
		EnableOpenMetrics:   true,
		MaxRequestsInFlight: metricsMaxRequestsInFlight,
		Timeout:             metricsHandlerTimeout,
	})
}

type keyMaterialCollector struct {
	targets []CertificateTarget
	desc    *prometheus.Desc
	readErr *prometheus.Desc
}

func newKeyMaterialCollector(targets []CertificateTarget) *keyMaterialCollector {
	return &keyMaterialCollector{
		targets: targets,
		desc: prometheus.NewDesc(
			"paigram_key_material_expiry_timestamp_seconds",
			"Unix timestamp when configured certificate key material expires.",
			[]string{"service", "identity"}, nil,
		),
		readErr: prometheus.NewDesc(
			"paigram_key_material_read_error",
			"Whether configured certificate key material cannot be read or parsed.",
			[]string{"service", "identity"}, nil,
		),
	}
}

func (c *keyMaterialCollector) Describe(output chan<- *prometheus.Desc) {
	output <- c.desc
	output <- c.readErr
}

func (c *keyMaterialCollector) Collect(output chan<- prometheus.Metric) {
	for _, target := range c.targets {
		if target.Identity == "" || target.CertificateFile == "" {
			continue
		}
		notAfter, err := certificateexpiry.ReadNotAfter(target.CertificateFile)
		if err != nil {
			output <- prometheus.MustNewConstMetric(c.readErr, prometheus.GaugeValue, 1, "account-center", target.Identity)
			continue
		}
		output <- prometheus.MustNewConstMetric(c.readErr, prometheus.GaugeValue, 0, "account-center", target.Identity)
		output <- prometheus.MustNewConstMetric(
			c.desc, prometheus.GaugeValue, float64(notAfter.Unix()), "account-center", target.Identity,
		)
	}
}

type reconciliationCollector struct {
	db         *gorm.DB
	due        *prometheus.Desc
	backlog    *prometheus.Desc
	oldestAge  *prometheus.Desc
	deadLetter *prometheus.Desc
}

func newReconciliationCollector(db *gorm.DB) *reconciliationCollector {
	return &reconciliationCollector{
		db:         db,
		due:        prometheus.NewDesc("paigram_reconciliation_outbox_due", "Number of due reconciliation outbox records.", nil, nil),
		backlog:    prometheus.NewDesc("paigram_reconciliation_outbox_backlog", "Number of pending reconciliation records excluding user input waits.", nil, nil),
		oldestAge:  prometheus.NewDesc("paigram_reconciliation_outbox_oldest_backlog_age_seconds", "Age of the oldest pending reconciliation record excluding user input waits.", nil, nil),
		deadLetter: prometheus.NewDesc("paigram_reconciliation_outbox_dead_letter", "Number of reconciliation outbox records in dead letter state.", nil, nil),
	}
}

func (c *reconciliationCollector) Describe(output chan<- *prometheus.Desc) {
	output <- c.due
	output <- c.backlog
	output <- c.oldestAge
	output <- c.deadLetter
}

func (c *reconciliationCollector) Collect(output chan<- prometheus.Metric) {
	if c.db == nil {
		output <- prometheus.NewInvalidMetric(c.due, gorm.ErrInvalidDB)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	db := c.db.WithContext(ctx)
	now := time.Now().UTC()
	var dueCount int64
	dueQuery := db.Model(&model.PlatformOperationOutbox{}).
		Where("status = ? AND available_at <= ?", model.PlatformOperationOutboxStatusPending, now)
	if err := dueQuery.Count(&dueCount).Error; err != nil {
		output <- prometheus.NewInvalidMetric(c.due, err)
		return
	}
	var backlogCount int64
	if err := reconciliationBacklogQuery(db).Count(&backlogCount).Error; err != nil {
		output <- prometheus.NewInvalidMetric(c.backlog, err)
		return
	}
	var deadLetterCount int64
	if err := db.Model(&model.PlatformOperationOutbox{}).
		Where("status = ?", model.PlatformOperationOutboxStatusDeadLetter).
		Count(&deadLetterCount).Error; err != nil {
		output <- prometheus.NewInvalidMetric(c.deadLetter, err)
		return
	}
	output <- prometheus.MustNewConstMetric(c.due, prometheus.GaugeValue, float64(dueCount))
	output <- prometheus.MustNewConstMetric(c.backlog, prometheus.GaugeValue, float64(backlogCount))
	output <- prometheus.MustNewConstMetric(c.deadLetter, prometheus.GaugeValue, float64(deadLetterCount))
	if backlogCount == 0 {
		output <- prometheus.MustNewConstMetric(c.oldestAge, prometheus.GaugeValue, 0)
		return
	}
	var oldest struct {
		CreatedAt time.Time
	}
	if err := reconciliationBacklogQuery(db).
		Select("platform_operation_outbox.created_at").
		Order("platform_operation_outbox.created_at ASC").
		Take(&oldest).Error; err != nil {
		output <- prometheus.NewInvalidMetric(c.oldestAge, err)
		return
	}
	output <- prometheus.MustNewConstMetric(c.oldestAge, prometheus.GaugeValue, max(0, now.Sub(oldest.CreatedAt).Seconds()))
}

func reconciliationBacklogQuery(db *gorm.DB) *gorm.DB {
	return db.Table("platform_operation_outbox").
		Joins("JOIN platform_operation_intents ON platform_operation_intents.operation_id = platform_operation_outbox.operation_id").
		Where("platform_operation_outbox.status = ? AND platform_operation_intents.state <> ?",
			model.PlatformOperationOutboxStatusPending, model.PlatformOperationIntentStateInputRequired)
}
