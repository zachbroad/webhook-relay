# Webhook Relay — MVP

Resume project showcasing Kubernetes, Helm, Terraform, observability (Prometheus/Grafana/OTel), and production-grade Go service design.

## What's Already Built

- Go API + Worker with Redis Streams fan-out, retry logic, HMAC signing
- Transform scripts (sandboxed JS runtime)
- Server-rendered UI with htmx + Monaco editor
- Helm chart (API, Worker, Postgres, Redis)
- Docker Compose for local dev
- Database migrations (5)

## Action Ideas (Useful Webhook Integrations)

Real-world "actions" that fire when a webhook is received and transformed:

| Action | Trigger Example | Why It's Good for Demo |
|--------|----------------|----------------------|
| **Slack notification** | GitHub push → post to Slack channel | Most relatable, easy to demo live |
| **Discord message** | Stripe payment → Discord alert | Same idea, different transport |
| **Email (SMTP/SES)** | Any event → send formatted email | Classic action type |
| **HTTP relay** (already built) | Fan-out to N subscriber URLs | Core feature |
| **Webhook-to-webhook transform** | Receive GitHub format → re-emit as Slack-compatible payload | Shows transform scripts in action |
| **Dead letter queue** | Failed deliveries → DLQ topic for inspection | Shows reliability thinking |
| **PagerDuty/OpsGenie alert** | Error threshold → create incident | Ties into observability story |
| **Log to S3/object store** | Archive every payload to MinIO/S3 | Demonstrates async processing |

**MVP scope: HTTP relay (done) + Slack notification + dead letter queue.** The rest are stretch goals.

## MVP Requirements

### 1. Core Features (Mostly Done)

- [x] Ingest webhooks via HTTP
- [x] Fan-out to subscriber endpoints
- [x] Retry with exponential backoff
- [x] HMAC signing
- [x] Idempotency (dedup by key)
- [x] Transform scripts (JS sandbox)
- [x] Record mode vs Active mode
- [x] Web UI (sources, subscriptions, deliveries, script editor)
- [ ] Slack notification action type
- [ ] Dead letter queue (move permanently-failed deliveries)
- [ ] Delivery detail page in UI (attempt history, request/response bodies)
- [ ] Source-level webhook event log with payload inspection

### 2. Observability (Not Started)

This is the main resume differentiator. Instrument everything.

- [ ] **OpenTelemetry SDK** — traces + metrics in Go services
  - Trace: full request lifecycle (ingest → store → stream → fanout → deliver)
  - Metrics: `webhooks_received_total`, `deliveries_attempted_total`, `delivery_duration_seconds`, `delivery_errors_total`, `retry_queue_depth`
- [ ] **Prometheus** — scrape `/metrics` endpoints on API + Worker
- [ ] **Grafana dashboards** — pre-built JSON dashboards deployed via Helm
  - Webhook throughput (rate of ingestion)
  - Delivery success/failure rates
  - P50/P95/P99 delivery latency
  - Retry queue depth over time
  - Active subscriptions per source
- [ ] **Grafana Loki** (stretch) — aggregate structured logs
- [ ] **Grafana Tempo** (stretch) — distributed tracing backend
- [ ] **Alerting rules** — Prometheus alert when error rate > threshold

### 3. Kubernetes / Helm

- [x] Helm chart with API + Worker + Postgres + Redis
- [ ] **HPA** — Horizontal Pod Autoscaler for Worker (scale on Redis stream lag or CPU)
- [ ] **PodDisruptionBudgets** — for Worker and API
- [ ] **Health checks** — liveness + readiness probes (API has `/health`, wire into Helm)
- [ ] **ServiceMonitor** CRDs — for Prometheus Operator auto-discovery
- [ ] **Network Policies** — restrict traffic (API→Postgres/Redis, Worker→Postgres/Redis/external)
- [ ] **Resource quotas** — right-sized requests/limits based on load testing
- [ ] **Secrets management** — use k8s Secrets properly (DB password, signing keys)
- [ ] **Ingress TLS** — cert-manager + Let's Encrypt (or self-signed for demo)

### 4. Terraform

Provision the infrastructure that the Helm chart runs on.

- [ ] **Local/Minikube module** — terraform apply to spin up minikube cluster + install chart
- [ ] **AWS module** (stretch) — EKS cluster, RDS Postgres, ElastiCache Redis, S3 for backups
  - VPC, subnets, security groups
  - IAM roles for service accounts (IRSA)
  - terraform outputs: kubeconfig, DB endpoint, Redis endpoint
- [ ] **Helm provider** — `helm_release` resource to deploy the chart from Terraform
- [ ] **State management** — remote state in S3 + DynamoDB lock (for AWS module)

### 5. CI/CD

- [ ] **GitHub Actions** workflow
  - Lint (`golangci-lint`)
  - Test (`go test ./...`)
  - Build + push Docker images (GHCR)
  - Helm chart lint + template validation
  - (Stretch) Deploy to staging cluster on merge to main

### 6. Load Testing & Reliability

- [ ] **k6 or vegeta** load test script — blast webhooks, measure throughput
- [ ] Document max throughput numbers (e.g., "sustained 1k webhooks/sec on 2 worker replicas")
- [ ] Graceful shutdown — drain in-flight deliveries on SIGTERM
- [ ] Connection pooling tuning (pgx pool, Redis pool)

## Architecture Diagram (Target)

```
                         ┌──────────────┐
  Webhook Sources        │   Ingress    │
  (GitHub, Stripe,  ───▶ │  (nginx/TLS) │
   etc.)                 └──────┬───────┘
                                │
                         ┌──────▼───────┐
                         │   API (Go)   │──── /metrics ──▶ Prometheus
                         │   :8080      │
                         └──┬───────┬───┘
                            │       │
                    ┌───────▼──┐ ┌──▼────────┐
                    │ Postgres │ │   Redis    │
                    │ (state)  │ │  (stream)  │
                    └───────▲──┘ └──┬────────┘
                            │       │
                         ┌──┴───────▼───┐
                         │ Worker (Go)  │──── /metrics ──▶ Prometheus
                         │ N replicas   │
                         └──────┬───────┘        ┌──────────┐
                                │           ┌───▶│ Grafana  │
                         ┌──────▼───────┐   │    └──────────┘
                         │  Subscriber  │   │
                         │  Endpoints   │   │
                         └──────────────┘   │
                                      Prometheus
```

## Priority Order

1. **Observability** — OTel instrumentation + Prometheus + Grafana dashboards (biggest resume impact)
2. **Terraform** — even a local minikube module shows IaC competence
3. **CI/CD** — GitHub Actions pipeline
4. **HPA + PDB + Network Policies** — Helm chart hardening
5. **Slack action + DLQ** — feature completeness
6. **Load testing** — concrete performance numbers
7. **AWS module** — only if going for max impact
