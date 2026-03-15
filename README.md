# Simple Platform Setup

Small event-driven system for local development on LocalStack.

It provides:
- an HTTP ingest API
- durable event state in DynamoDB
- asynchronous processing through SQS + Lambda
- DLQ tooling for failed messages
- end-to-end observability with OpenTelemetry, Jaeger, Prometheus, and Grafana

## Services

Runtime services involved:
- `ingest`
  Go HTTP API for write/read endpoints
- `SQS`
  Main async queue for events
- `Lambda`
  Downstream async processor
- `DynamoDB`
  Durable event state and processing result store
- `DLQ`
  Isolation queue for repeatedly failing messages
- `OTel Collector`
  Receives OTLP telemetry from app components
- `Jaeger`
  Trace UI
- `Prometheus`
  Metrics store/query engine
- `Grafana`
  RED dashboard UI
- `LocalStack`
  Local AWS emulation

## Structure

Main folders:
- [ingest](/Users/ashwinsekaran/Work/github/simple_platform_setup/ingest)
  HTTP API, validation, idempotent write path, read path
- [worker](/Users/ashwinsekaran/Work/github/simple_platform_setup/worker)
  Lambda handler, DLQ inspect/replay helpers, worker-side repo code
- [infra/tf](/Users/ashwinsekaran/Work/github/simple_platform_setup/infra/tf)
  Terraform for SQS, DLQ, DynamoDB, Lambda, IAM, event source mapping
- [monitoring](/Users/ashwinsekaran/Work/github/simple_platform_setup/monitoring)
  OTel Collector, Prometheus, Grafana, dashboard provisioning

## How It Works

Write flow:
1. Client calls `POST /events`
2. Ingest validates `id`, `type`, `payload`
3. Ingest stores the event in DynamoDB with processing status `queued`
4. Ingest publishes the event to SQS
5. Lambda consumes from SQS asynchronously
6. Lambda updates DynamoDB with processing result

Read flow:
1. Client calls `GET /events/{id}`
2. Ingest reads the durable event record from DynamoDB
3. Response includes event data plus processing status/result

## Run Locally

Prerequisites:
- Docker / Docker Compose
- Terraform `>= 1.6.0`

Optional local credentials in `.env` or shell:

```bash
AWS_ACCESS_KEY_ID=test
AWS_SECRET_ACCESS_KEY=test
```

Start the full demo:

```bash
make demo
```

No host Go installation is required. The ingest app and Lambda artifact are both built with Docker.

This will:
- start LocalStack and the observability stack
- build the ingest container and Lambda artifact with Docker
- apply Terraform against LocalStack
- start the ingest API container
- generate sample traffic end-to-end
- print local URLs and sample curl commands

Stop and destroy:

```bash
make stop-demo
```

Useful local URLs after `make demo`:
- API: `http://localhost:8080`
- Jaeger: `http://localhost:16686`
- Prometheus: `http://localhost:9090`
- Grafana: `http://localhost:3000`

## API

Create event:

```bash
curl -X POST http://localhost:8080/events \
  -H "Content-Type: application/json" \
  -d '{"id":"1","type":"user.created","payload":{"name":"Ada"}}'
```

Read event:

```bash
curl http://localhost:8080/events/1
```

Health:

```bash
curl http://localhost:8080/health
curl http://localhost:8080/ready
```

## Requirements Coverage

### Ingestion API

Implemented in [ingest/handlers/ingest.go](/Users/ashwinsekaran/Work/github/simple_platform_setup/ingest/handlers/ingest.go).

Accepted fields:
- `id`
- `type`
- `payload`

Validation rules:
- missing required fields -> `400`
- invalid JSON body -> `400`
- invalid payload JSON -> `400`

### Query API

Implemented in [ingest/handlers/ingest.go](/Users/ashwinsekaran/Work/github/simple_platform_setup/ingest/handlers/ingest.go).

`GET /events/{id}` returns:
- original event data
- processing status
- processing result

### Asynchronous Processing

Implemented with:
- SQS in [infra/tf/main.tf](/Users/ashwinsekaran/Work/github/simple_platform_setup/infra/tf/main.tf)
- Lambda in [infra/tf/main.tf](/Users/ashwinsekaran/Work/github/simple_platform_setup/infra/tf/main.tf)
- Lambda handler in [worker/lambda/main.go](/Users/ashwinsekaran/Work/github/simple_platform_setup/worker/lambda/main.go)

### Fault Tolerance

Implemented with:
- SQS DLQ in [infra/tf/main.tf](/Users/ashwinsekaran/Work/github/simple_platform_setup/infra/tf/main.tf)
- redrive policy from main queue to DLQ
- inspect helper in [worker/dlq/inspect/main.go](/Users/ashwinsekaran/Work/github/simple_platform_setup/worker/dlq/inspect/main.go)
- replay helper in [worker/dlq/replay/main.go](/Users/ashwinsekaran/Work/github/simple_platform_setup/worker/dlq/replay/main.go)

## Assumptions

- Local development only; deployment to real AWS is not required here
- Event ids are caller-supplied and expected to uniquely identify one immutable event
- Same `id` with changed content is treated as conflict, not update
- Observability infrastructure is local Docker infrastructure, while AWS-like resources are provisioned through Terraform into LocalStack
- Lambda is the async compute boundary used for the challenge

## Idempotency Model

Idempotency behavior:
- first submission with a new `id` -> store + publish
- repeated submission with same `id` and same content -> return success without creating a new logical event
- repeated submission with same `id` but different content -> `409 Conflict`

Why:
- event ids identify immutable event submissions
- allowing changed payloads under the same id would blur retry vs update semantics

Important note:
- the implementation is correct for standard sequential retries
- it is not fully hardened for highly concurrent same-id racing requests

## Fault-Tolerance Approach

Approach:
- healthy messages are processed asynchronously through SQS + Lambda
- repeatedly failing messages are isolated by SQS redrive into a DLQ
- failed messages can be inspected later
- failed messages can be replayed back to the main queue later

DLQ helpers:

Inspect:

```bash
INGEST_DLQ_QUEUE_URL=$(terraform -chdir=infra/tf output -raw ingest_dlq_queue_url) \
go run ./worker/dlq/inspect
```

Replay:

```bash
EVENT_ID=evt-1 \
INGEST_DLQ_QUEUE_URL=$(terraform -chdir=infra/tf output -raw ingest_dlq_queue_url) \
go run ./worker/dlq/replay
```

Important note:
- DLQ infrastructure is in place
- a realistic business-failure scenario for deliberately driving events into the DLQ is still a follow-up improvement

## Trace Propagation Plan

HTTP boundary:
- ingest extracts incoming `traceparent` headers

Async boundary:
- ingest injects OTel trace context into SQS message attributes
- Lambda extracts context from SQS message attributes

Result:
- Jaeger can show one logical trace that spans:
  - `ingest.post_event`
  - `worker.process_event`

## Observability

### Traces

- OTel SDK in app code
- OTel Collector receives OTLP
- Jaeger stores and visualizes traces

### Metrics

Metrics emitted:
- ingest request rate
- ingest success/error counts
- ingest request duration
- worker processed count
- worker error count
- worker end-to-end latency

### Logs

- logs are trace-correlated through [monitoring/telemetry/log.go](/Users/ashwinsekaran/Work/github/simple_platform_setup/monitoring/telemetry/log.go)
- Lambda runtime logs are visible through LocalStack logs

### Dashboard

RED dashboard:
- [monitoring/grafana/dashboards/red-dashboard.json](/Users/ashwinsekaran/Work/github/simple_platform_setup/monitoring/grafana/dashboards/red-dashboard.json)

Data path:
- app -> OTel Collector -> Prometheus -> Grafana

Trace path:
- app -> OTel Collector -> Jaeger

## Key Metrics / Alerts

Key metrics to watch:
- `ingest_requests_total`
- `ingest_error_total`
- `ingest_request_duration_ms`
- `worker_processed_total`
- `worker_error_total`
- `worker_end_to_end_latency_ms`

Reasonable alert ideas:
- sustained non-zero ingest error rate
- worker error rate spike
- end-to-end latency increase
- DLQ depth above zero
- ingest request rate drops unexpectedly

## Security Notes

Secrets handling:
- local credentials are supplied via environment variables / `.env`
- secrets are not hardcoded into Terraform resources

Least privilege:
- Lambda IAM policy is scoped to the specific SQS queue and DynamoDB table
- only currently needed actions are granted

Production-oriented improvements:
- move secrets to Secrets Manager or SSM
- tighten IAM further if additional access patterns become known
- add authn/authz in front of the ingest API

## Cost Notes

Cost-aware choices:
- SQS and Lambda are pay-per-use
- DynamoDB uses `PAY_PER_REQUEST`
- LocalStack avoids real cloud spend during development

Cost levers to tune later:
- Lambda memory and timeout
- SQS batch size
- DynamoDB capacity mode
- trace sampling rate
- log verbosity and retention

## Current Gaps / Follow-Ups

- realistic poison-message processing scenario is not yet modeled in Lambda business logic
- idempotency is not fully race-safe for concurrent duplicate submissions
- logs are correlated but not stored in Grafana because Loki is not part of the stack
