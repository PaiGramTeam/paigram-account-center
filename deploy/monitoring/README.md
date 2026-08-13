# Monitoring

The Account Center and Platform Mihomo expose Prometheus metrics only on their container networks. The production frontend explicitly returns `404` for `/metrics`, and neither application deployment publishes a metrics port on the host.

Attach the Prometheus scraper to the shared `paigram-platform-backplane` network, or the configured `PAI_PLATFORM_NETWORK`, and use these targets:

| Target | Metrics endpoint |
| --- | --- |
| Account Center | `http://account-center:8080/metrics` |
| Platform Mihomo | `http://platform-mihomo:9090/metrics` |

Do not route either endpoint through public ingress. If a remote monitoring system cannot join the private network, deploy an authenticated metrics gateway on that network instead of publishing the application listeners directly.

The checked-in Compose project runs the current Prometheus 3.13 LTS image pinned by manifest digest, joins only the shared application backplane, loads `prometheus.yaml` and `paigram-alerts.yaml`, and keeps its TSDB in a named volume. This avoids granting the scraper network access to either service's PostgreSQL or Redis network. Start it after Platform has created the shared backplane and both application services are running:

Use the same pinned `podman-compose==1.6.0` provider required by the application deployment projects.

```powershell annotate
# Starts the private scraper and binds its operator UI to host loopback only.
cd deploy/monitoring
podman-compose up -d
```

The operator UI defaults to `http://127.0.0.1:19090`. Keep that binding on loopback; use an authenticated monitoring ingress if remote access is required. Configure an Alertmanager separately when notifications are required; Prometheus still evaluates and exposes firing alerts without one.

The rule file covers five failure signals:

- reconciliation work that is stale for more than five minutes or has entered dead-letter state;
- more than twenty service-ticket rejections in ten minutes, grouped by the bounded `service` and `surface` labels;
- an Account or Platform TLS identity or configured CA trust bundle entering its final 14 days, passing expiry, or becoming unreadable;
- the latest discover, refresh, AuthKey issue, or AuthKey revoke result remaining degraded for five minutes.
- an Account Center or Platform metrics target remaining unavailable, including a missing target series.

The exporters deliberately omit request, trace, operation, user, binding, account, profile, and credential values from metric labels. Use the correlated structured logs and audit records for per-operation diagnosis.

When an alert fires:

1. Check Account Center and Platform readiness before restarting either service.
2. For reconciliation alerts, inspect pending/dead-letter intent reason codes and retry state without copying credential payloads into tickets or logs.
3. For ticket rejections, compare issuer/verifier clock, key ID, audience, action, generation, and authorization-fence state.
4. For key-material expiry, follow the certificate overlap and container recreation procedure; never overwrite a live key in place.
5. For degraded upstream operations, inspect the typed upstream failure and retry guidance. A third-party outage should not make local liveness fail.
6. For a metrics-target alert, check the private backplane, service readiness, and exporter logs before trusting the absence of any other alert.

Thresholds are conservative defaults. Tune them from observed traffic after deployment, but keep label sets bounded and retain the alert categories as release gates.

Before deploying a rule change, validate both configuration files with the `promtool` shipped in the pinned image:

```powershell annotate
# Uses the same parser and PromQL implementation as the deployed Prometheus release.
podman run --rm --entrypoint /bin/promtool `
  -v "${PWD}/paigram-alerts.yaml:/rules.yaml:ro" `
  docker.io/prom/prometheus:v3.13.1@sha256:3c42b892cf723fa54d2f262c37a0e1f80aa8c8ddb1da7b9b0df9455a35a7f893 `
  check rules /rules.yaml
podman run --rm --entrypoint /bin/promtool `
  -v "${PWD}/prometheus.yaml:/prometheus.yaml:ro" `
  docker.io/prom/prometheus:v3.13.1@sha256:3c42b892cf723fa54d2f262c37a0e1f80aa8c8ddb1da7b9b0df9455a35a7f893 `
  check config /prometheus.yaml
podman run --rm --entrypoint /bin/promtool `
  -v "${PWD}:/etc/prometheus:ro" `
  docker.io/prom/prometheus:v3.13.1@sha256:3c42b892cf723fa54d2f262c37a0e1f80aa8c8ddb1da7b9b0df9455a35a7f893 `
  test rules /etc/prometheus/paigram-alerts.test.yaml
```
