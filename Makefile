SHELL := /bin/bash

.PHONY: demo stop-demo

ifneq (,$(wildcard .env))
include .env
export
endif

AWS_REGION ?= us-east-1
TF_DIR ?= infra/tf
TF_MIN_VERSION ?= 1.6.0
RUNTIME_DIR ?= .runtime
RUNTIME_ENV_FILE ?= $(RUNTIME_DIR)/ingest.env
TF_VARS_FILE ?= $(RUNTIME_DIR)/terraform.auto.tfvars

demo:
	@test -n "$(AWS_ACCESS_KEY_ID)" || (echo "Missing required secrets. Please set:" && echo "  AWS_ACCESS_KEY_ID" && echo "  AWS_SECRET_ACCESS_KEY" && exit 1)
	@test -n "$(AWS_SECRET_ACCESS_KEY)" || (echo "Missing required secrets. Please set:" && echo "  AWS_ACCESS_KEY_ID" && echo "  AWS_SECRET_ACCESS_KEY" && exit 1)
	@TF_VERSION="$$(terraform version -json | awk -F'"' '/terraform_version/ {print $$4}')"; \
	awk -v current="$$TF_VERSION" -v required="$(TF_MIN_VERSION)" 'BEGIN { split(current, c, "."); split(required, r, "."); for (i = 1; i <= 3; i++) { cv = (c[i] == "" ? 0 : c[i]); rv = (r[i] == "" ? 0 : r[i]); if (cv > rv) exit 0; if (cv < rv) exit 1 } exit 0 }' || (echo "Terraform >= $(TF_MIN_VERSION) is required (found $$TF_VERSION)" && exit 1)
	mkdir -p $(RUNTIME_DIR) $(TF_DIR)/build
	printf "aws_access_key_id = \"%s\"\naws_secret_access_key = \"%s\"\naws_region = \"%s\"\n" "$(AWS_ACCESS_KEY_ID)" "$(AWS_SECRET_ACCESS_KEY)" "$(AWS_REGION)" > $(TF_VARS_FILE)
	printf "INGEST_DYNAMODB_TABLE=\nINGEST_SQS_QUEUE_URL=\n" > $(RUNTIME_ENV_FILE)
	docker compose up -d localstack jaeger prometheus grafana otel-collector
	docker compose build worker-build ingest
	docker compose run --rm worker-build
	terraform -chdir=$(TF_DIR) init
	terraform -chdir=$(TF_DIR) apply -auto-approve -var-file=$(abspath $(TF_VARS_FILE))
	@QUEUE_URL="$$(terraform -chdir=$(TF_DIR) output -raw ingest_queue_url)"; \
	TABLE_NAME="$$(terraform -chdir=$(TF_DIR) output -raw ingest_table_name)"; \
	API_URL="http://localhost:8080"; \
	EVENT_ID="demo-evt-1"; \
	printf "INGEST_DYNAMODB_TABLE=%s\nINGEST_SQS_QUEUE_URL=%s\n" "$$TABLE_NAME" "$$QUEUE_URL" > "$(RUNTIME_ENV_FILE)"; \
	docker compose up -d ingest; \
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
	echo "POST sample:"; \
	echo "curl -X POST $$API_URL/events -H 'Content-Type: application/json' -d '{\"id\":\"1\",\"type\":\"user.created\",\"payload\":{\"name\":\"Ada\"}}'"; \
	echo "GET sample:"; \
	echo "curl $$API_URL/events/1"

stop-demo:
	-test -f $(TF_VARS_FILE) && terraform -chdir=$(TF_DIR) destroy -auto-approve -var-file=$(abspath $(TF_VARS_FILE))
	docker compose down
