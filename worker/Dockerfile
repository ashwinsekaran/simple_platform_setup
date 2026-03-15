FROM golang:1.25-alpine AS builder

RUN apk add --no-cache zip

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY worker ./worker
COPY monitoring ./monitoring

RUN mkdir -p /artifacts && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /artifacts/bootstrap ./worker/lambda && \
    cd /artifacts && zip -q worker-lambda.zip bootstrap

FROM alpine:3.22

WORKDIR /artifacts

COPY --from=builder /artifacts /artifacts

ENTRYPOINT ["/bin/sh", "-c", "mkdir -p /output && cp /artifacts/bootstrap /output/bootstrap && cp /artifacts/worker-lambda.zip /output/worker-lambda.zip"]
