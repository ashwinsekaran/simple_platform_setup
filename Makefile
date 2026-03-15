SHELL := /bin/bash

.PHONY: demo stop-demo

ifneq (,$(wildcard .env))
include .env
export
endif

AWS_REGION ?= us-east-1
AWS_ENDPOINT_URL ?= http://localhost:4566
INGEST_HTTP_ADDR ?= :8080
TF_DIR ?= infra/tf

demo:
	@test -n "$(AWS_ACCESS_KEY_ID)" || (echo "AWS_ACCESS_KEY_ID is required" && exit 1)
	@test -n "$(AWS_SECRET_ACCESS_KEY)" || (echo "AWS_SECRET_ACCESS_KEY is required" && exit 1)
	docker compose up -d localstack jaeger prometheus grafana otel-collector
	mkdir -p $(TF_DIR)/build
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $(TF_DIR)/build/bootstrap ./worker/lambda
	rm -f $(TF_DIR)/build/worker-lambda.zip
	cd $(TF_DIR)/build && zip -q worker-lambda.zip bootstrap
	TF_VAR_aws_access_key_id="$(AWS_ACCESS_KEY_ID)" \
	TF_VAR_aws_secret_access_key="$(AWS_SECRET_ACCESS_KEY)" \
	terraform -chdir=$(TF_DIR) init
	TF_VAR_aws_access_key_id="$(AWS_ACCESS_KEY_ID)" \
	TF_VAR_aws_secret_access_key="$(AWS_SECRET_ACCESS_KEY)" \
	terraform -chdir=$(TF_DIR) apply -auto-approve
	@QUEUE_URL="$$(terraform -chdir=$(TF_DIR) output -raw ingest_queue_url)"; \
	TABLE_NAME="$$(terraform -chdir=$(TF_DIR) output -raw ingest_table_name)"; \
	API_URL="http://localhost:8080"; \
	EVENT_ID="demo-evt-1"; \
	trap 'kill 0' INT TERM EXIT; \
	AWS_REGION="$(AWS_REGION)" \
	AWS_ENDPOINT_URL="$(AWS_ENDPOINT_URL)" \
	AWS_ACCESS_KEY_ID="$(AWS_ACCESS_KEY_ID)" \
	AWS_SECRET_ACCESS_KEY="$(AWS_SECRET_ACCESS_KEY)" \
	OTEL_EXPORTER_OTLP_ENDPOINT="localhost:4317" \
	INGEST_HTTP_ADDR="$(INGEST_HTTP_ADDR)" \
	INGEST_DYNAMODB_TABLE="$$TABLE_NAME" \
	INGEST_SQS_QUEUE_URL="$$QUEUE_URL" \
	go run ./ingest & \
	for attempt in {1..30}; do \
		if curl -fsS "$$API_URL/health" >/dev/null 2>&1; then \
			break; \
		fi; \
		sleep 1; \
	done; \
	curl -fsS -X POST "$$API_URL/events" \
		-H "Content-Type: application/json" \
		-H "traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01" \
		-d "{\"id\":\"$$EVENT_ID\",\"type\":\"user.created\",\"payload\":{\"name\":\"Test\"}}" >/dev/null; \
	for attempt in {1..20}; do \
		RESPONSE="$$(curl -fsS "$$API_URL/events/$$EVENT_ID" || true)"; \
		if printf '%s' "$$RESPONSE" | grep -q '"processing_status":"processed"'; then \
			break; \
		fi; \
		sleep 1; \
	done; \
	echo ""; \
	echo "Demo is ready."; \
	echo "API: $$API_URL"; \
	echo "Jaeger: http://localhost:16686"; \
	echo "Prometheus: http://localhost:9090"; \
	echo "Grafana: http://localhost:3000 (admin/admin)"; \
	echo "Sample event id: $$EVENT_ID"; \
	echo ""; \
	wait

stop-demo:
	-TF_VAR_aws_access_key_id="$(AWS_ACCESS_KEY_ID)" \
	TF_VAR_aws_secret_access_key="$(AWS_SECRET_ACCESS_KEY)" \
	terraform -chdir=$(TF_DIR) destroy -auto-approve
	docker compose down
