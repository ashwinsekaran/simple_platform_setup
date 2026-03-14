.PHONY: demo demo-deps run-ingest test fmt stop-demo

AWS_REGION ?= us-east-1
AWS_ENDPOINT_URL ?= http://localhost:4566
AWS_ACCESS_KEY_ID ?= test
AWS_SECRET_ACCESS_KEY ?= test
INGEST_HTTP_ADDR ?= :8080
TF_DIR ?= infra/tf

demo: demo-deps
	INGEST_SQS_QUEUE_URL="$$(terraform -chdir=$(TF_DIR) output -raw ingest_queue_url)" \
	AWS_REGION="$(AWS_REGION)" \
	AWS_ENDPOINT_URL="$(AWS_ENDPOINT_URL)" \
	AWS_ACCESS_KEY_ID="$(AWS_ACCESS_KEY_ID)" \
	AWS_SECRET_ACCESS_KEY="$(AWS_SECRET_ACCESS_KEY)" \
	INGEST_HTTP_ADDR="$(INGEST_HTTP_ADDR)" \
	go run ./ingest

demo-deps:
	docker compose up -d localstack
	terraform -chdir=$(TF_DIR) init
	terraform -chdir=$(TF_DIR) apply -auto-approve

run-ingest:
	INGEST_SQS_QUEUE_URL="$$(terraform -chdir=$(TF_DIR) output -raw ingest_queue_url)" \
	AWS_REGION="$(AWS_REGION)" \
	AWS_ENDPOINT_URL="$(AWS_ENDPOINT_URL)" \
	AWS_ACCESS_KEY_ID="$(AWS_ACCESS_KEY_ID)" \
	AWS_SECRET_ACCESS_KEY="$(AWS_SECRET_ACCESS_KEY)" \
	INGEST_HTTP_ADDR="$(INGEST_HTTP_ADDR)" \
	go run ./ingest

test:
	go test ./...

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')
	terraform fmt -recursive $(TF_DIR)

stop-demo:
	-terraform -chdir=$(TF_DIR) destroy -auto-approve
	docker compose down
