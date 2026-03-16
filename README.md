# Simple Platform Setup

Small event-driven system for local development on LocalStack.

## Overview

Architecture:
- `ingest`
  Go HTTP API with `POST /events` and `GET /events/{id}`
- `DynamoDB`
  Durable event state and processing result store
- `SQS`
  Main async queue between ingest and worker
- `Lambda`
  Downstream async processor
- `DLQ`
  Isolation queue for repeatedly failing messages
- `OTel Collector`, `Jaeger`, `Prometheus`, `Grafana`
  Local observability stack
- `LocalStack`
  Local AWS emulation

Main folders:
- [ingest](/Users/ashwinsekaran/Work/github/simple_platform_setup/ingest)
- [worker](/Users/ashwinsekaran/Work/github/simple_platform_setup/worker)
- [infra/tf](/Users/ashwinsekaran/Work/github/simple_platform_setup/infra/tf)
- [monitoring](/Users/ashwinsekaran/Work/github/simple_platform_setup/monitoring)

## Run

Prerequisites:
- Docker / Docker Compose
- Terraform `>= 1.6.0`

Set local credentials in `.env` or shell:

```bash
AWS_ACCESS_KEY_ID=test
AWS_SECRET_ACCESS_KEY=test
```

Start the full demo:

```bash
make demo
```

Stop and destroy:

```bash
make stop-demo
```

`make demo`:
- starts LocalStack and the observability stack
- builds the ingest container and Lambda artifact with Docker
- applies Terraform against LocalStack
- starts ingest and DLQ monitor
- generates sample traffic
- prints API, Jaeger, Prometheus, and Grafana URLs
- prints sample `curl`, DLQ inspect, and DLQ replay commands

No host Go installation is required.

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

## Design Notes

Assumptions:
- local development only; no real AWS deployment required
- event ids are caller-supplied and identify immutable event submissions
- same `id` with different content is treated as conflict, not update

Requirements coverage:
- write API: `POST /events`
- read API: `GET /events/{id}`
- async processing: `ingest -> SQS -> Lambda`
- durable state: DynamoDB
- DLQ inspect/replay: yes
- LocalStack + Terraform: yes

Idempotency model:
- first submission with a new `id` stores and publishes
- same `id` + same content returns success without creating a duplicate
- same `id` + different content returns `409 Conflict`

Important note:
- this is correct for normal sequential retries
- it is not fully hardened for highly concurrent same-id racing requests

Fault tolerance:
- healthy events continue through SQS + Lambda
- repeatedly failing events are isolated by SQS redrive into the DLQ
- failed items can be inspected and replayed later
- replay can be done unchanged or with corrected payload/type

Real failing-event rule:
- `user.created` requires `payload.name`
- a valid JSON event without `payload.name` is accepted by ingest, then fails in Lambda, is retried, and eventually moves to the DLQ

Trace propagation:
- ingest extracts HTTP trace context from headers
- ingest injects OTel context into SQS message attributes
- Lambda extracts that context and continues the trace
- Jaeger shows a single trace spanning `ingest.post_event` and `worker.process_event`

Security and cost notes:
- secrets are env-driven, not hardcoded in app or Terraform
- Lambda IAM policy is scoped to the specific table and queue it uses
- DynamoDB uses `PAY_PER_REQUEST`
- SQS + Lambda keep the async path simple and cost-friendly
- LocalStack avoids real cloud spend for local development

## Observability

Traces:
- Jaeger: [http://localhost:16686](http://localhost:16686)

Metrics:
- Prometheus: [http://localhost:9090](http://localhost:9090)
- Grafana: [http://localhost:3000](http://localhost:3000)

Dashboard:
- [monitoring/grafana/dashboards/red-dashboard.json](/Users/ashwinsekaran/Work/github/simple_platform_setup/monitoring/grafana/dashboards/red-dashboard.json)

Key panels:
- `Ingest Rate`
- `Ingest Errors`
- `Ingest Duration P95`
- `Ingest Replays`
- `Ingest Conflicts`
- `Ingest Validation Failures`
- `Ingest Dependency Duration P95`
- `Pipeline Comparison`
  Compare ingest accepted, worker processed, and worker failed in one graph
- `Queue Overview`
  Main queue depth, DLQ depth, and DLQ replay count
- `Worker Processed Count`
- `Worker Error Count`
- `Worker Error Percentage`
- `Worker End-to-End Latency`

## DLQ Operations

Inspect failed events:

```bash
docker compose run --rm dlq-inspect
```

Replay unchanged:

```bash
docker compose run --rm -e EVENT_ID=3 dlq-replay
```

Replay with corrected payload:

```bash
docker compose run --rm \
  -e EVENT_ID=3 \
  -e FIXED_PAYLOAD='{"name":"Ada"}' \
  dlq-replay
```

Note:
- DLQ movement is not immediate
- a failing message may take around a minute or more to appear in the DLQ because of queue visibility timeout and retry/redrive timing

## Validation Flow

Duplicate submission:
- post the same `id` and same content twice
- second request should be idempotent, not a new event

Conflict:
- post the same `id` with different content
- should return `409 Conflict`

Deliberately failing event:

```bash
curl -X POST http://localhost:8080/events \
  -H "Content-Type: application/json" \
  -d '{"id":"3","type":"user.created","payload":{"email":"ada@example.com"}}'
```

Then verify:
- `GET /events/3` shows `failed`
- Grafana shows worker failure activity and queue movement
- `docker compose run --rm dlq-inspect` shows the failed message once it reaches the DLQ

Trace validation:
- send a request with `traceparent` header
- inspect Jaeger to confirm one trace spans ingest and Lambda
