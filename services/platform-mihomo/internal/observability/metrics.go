package observability

import (
	"net/http"
	"strings"
	"time"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/certificateexpiry"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var upstreamOperations = map[string]struct{}{
	"discover": {}, "refresh": {}, "issue_authkey": {}, "revoke_authkey": {},
}

var ticketSurfaces = map[string]struct{}{"control": {}, "runtime": {}}

const (
	metricsMaxRequestsInFlight = 2
	metricsHandlerTimeout      = 3 * time.Second
)

type Metrics struct {
	registry         *prometheus.Registry
	upstreamDegraded *prometheus.GaugeVec
	ticketRejections *prometheus.CounterVec
}

type CertificateTarget struct {
	Identity        string
	CertificateFile string
}

func NewMetrics(certificates []CertificateTarget) *Metrics {
	registry := prometheus.NewRegistry()
	metrics := &Metrics{
		registry: registry,
		upstreamDegraded: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "paigram_platform_upstream_degraded",
			Help: "Whether the most recent platform upstream capability call was degraded.",
		}, []string{"operation"}),
		ticketRejections: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "paigram_service_ticket_rejections_total",
			Help: "Total number of rejected service ticket operations.",
		}, []string{"service", "surface", "reason"}),
	}
	registry.MustRegister(
		metrics.upstreamDegraded,
		metrics.ticketRejections,
		newKeyMaterialCollector(certificates),
	)
	return metrics
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
			output <- prometheus.MustNewConstMetric(c.readErr, prometheus.GaugeValue, 1, "platform-mihomo", target.Identity)
			continue
		}
		output <- prometheus.MustNewConstMetric(c.readErr, prometheus.GaugeValue, 0, "platform-mihomo", target.Identity)
		output <- prometheus.MustNewConstMetric(
			c.desc, prometheus.GaugeValue, float64(notAfter.Unix()), "platform-mihomo", target.Identity,
		)
	}
}

func (m *Metrics) Handler() http.Handler {
	return newPrometheusHandler(m.registry)
}

func newPrometheusHandler(gatherer prometheus.Gatherer) http.Handler {
	return promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{
		EnableOpenMetrics:   true,
		MaxRequestsInFlight: metricsMaxRequestsInFlight,
		Timeout:             metricsHandlerTimeout,
	})
}

func (m *Metrics) RecordUpstreamResult(operation string, degraded bool) {
	if m == nil {
		return
	}
	if _, ok := upstreamOperations[operation]; !ok {
		operation = "other"
	}
	value := float64(0)
	if degraded {
		value = 1
	}
	m.upstreamDegraded.WithLabelValues(operation).Set(value)
}

func (m *Metrics) RecordTicketRejection(surface, reason string) {
	if m == nil {
		return
	}
	if _, ok := ticketSurfaces[surface]; !ok {
		surface = "other"
	}
	reason = strings.ToLower(strings.TrimSpace(reason))
	switch reason {
	case "unauthenticated", "unavailable":
	case "permissiondenied":
		reason = "permission_denied"
	case "invalidargument":
		reason = "invalid_argument"
	case "failedprecondition":
		reason = "failed_precondition"
	default:
		reason = "other"
	}
	m.ticketRejections.WithLabelValues("platform-mihomo", surface, reason).Inc()
}
