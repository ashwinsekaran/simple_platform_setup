FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY ingest ./ingest
COPY monitoring ./monitoring

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/ingest ./ingest

FROM alpine:3.22

RUN adduser -D -H appuser

COPY --from=builder /out/ingest /usr/local/bin/ingest

USER appuser
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/ingest"]
