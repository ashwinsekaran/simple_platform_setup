FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY worker ./worker
COPY monitoring ./monitoring

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/dlq-monitor ./worker/dlq/monitor && \
    CGO_ENABLED=0 GOOS=linux go build -o /out/dlq-inspect ./worker/dlq/inspect && \
    CGO_ENABLED=0 GOOS=linux go build -o /out/dlq-replay ./worker/dlq/replay

FROM alpine:3.22

RUN adduser -D -H appuser

COPY --from=builder /out/dlq-monitor /usr/local/bin/dlq-monitor
COPY --from=builder /out/dlq-inspect /usr/local/bin/dlq-inspect
COPY --from=builder /out/dlq-replay /usr/local/bin/dlq-replay

USER appuser
