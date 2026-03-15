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
	docker compose up -d localstack jaeger otel-collector
	TF_VAR_aws_access_key_id="$(AWS_ACCESS_KEY_ID)" \
	TF_VAR_aws_secret_access_key="$(AWS_SECRET_ACCESS_KEY)" \
	terraform -chdir=$(TF_DIR) init
	TF_VAR_aws_access_key_id="$(AWS_ACCESS_KEY_ID)" \
	TF_VAR_aws_secret_access_key="$(AWS_SECRET_ACCESS_KEY)" \
	terraform -chdir=$(TF_DIR) apply -auto-approve
	@QUEUE_URL="$$(terraform -chdir=$(TF_DIR) output -raw ingest_queue_url)"; \
	TABLE_NAME="$$(terraform -chdir=$(TF_DIR) output -raw ingest_table_name)"; \
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
	AWS_REGION="$(AWS_REGION)" \
	AWS_ENDPOINT_URL="$(AWS_ENDPOINT_URL)" \
	AWS_ACCESS_KEY_ID="$(AWS_ACCESS_KEY_ID)" \
	AWS_SECRET_ACCESS_KEY="$(AWS_SECRET_ACCESS_KEY)" \
	OTEL_EXPORTER_OTLP_ENDPOINT="localhost:4317" \
	INGEST_SQS_QUEUE_URL="$$QUEUE_URL" \
	go run ./worker & \
	wait

stop-demo:
	-TF_VAR_aws_access_key_id="$(AWS_ACCESS_KEY_ID)" \
	TF_VAR_aws_secret_access_key="$(AWS_SECRET_ACCESS_KEY)" \
	terraform -chdir=$(TF_DIR) destroy -auto-approve
	docker compose down
