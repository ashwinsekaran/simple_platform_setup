package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/ashwinsekaran/simple_platform_app/monitoring/telemetry"
	"github.com/ashwinsekaran/simple_platform_app/worker/ent"
	"github.com/aws/aws-lambda-go/events"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
)

type EventRepository interface {
	UpdateProcessingResult(ctx context.Context, id, status, result string) error
}

type Service struct {
	repo EventRepository
}

var workerMetrics struct {
	once      sync.Once
	processed metric.Int64Counter
	errors    metric.Int64Counter
	latencyMS metric.Float64Histogram
}

func New(eventRepo EventRepository) *Service {
	return &Service{repo: eventRepo}
}

func (s *Service) Process(ctx context.Context, sqsEvent events.SQSEvent) (events.SQSEventResponse, error) {
	response := events.SQSEventResponse{
		BatchItemFailures: make([]events.SQSBatchItemFailure, 0),
	}

	for _, record := range sqsEvent.Records {
		if err := s.processRecord(ctx, record); err != nil {
			response.BatchItemFailures = append(response.BatchItemFailures, events.SQSBatchItemFailure{
				ItemIdentifier: record.MessageId,
			})
		}
	}

	return response, nil
}

func (s *Service) processRecord(ctx context.Context, record events.SQSMessage) error {
	ctx = otel.GetTextMapPropagator().Extract(ctx, messageAttributeCarrier(record.MessageAttributes))
	ctx, span := otel.Tracer("simple_platform_setup/worker").Start(ctx, "worker.process_event")
	defer span.End()

	instruments := workerInstruments()

	var event ent.Event
	if err := json.Unmarshal([]byte(record.Body), &event); err != nil {
		instruments.errors.Add(ctx, 1)
		span.RecordError(err)
		span.SetStatus(codes.Error, "unmarshal event")
		telemetry.Log(ctx, "lambda process failed: message_id=%s error=%v", record.MessageId, err)
		return fmt.Errorf("unmarshal event: %w", err)
	}

	span.SetAttributes(
		attribute.String("event.id", event.ID),
		attribute.String("event.type", event.Type),
	)

	if err := s.repo.UpdateProcessingResult(ctx, event.ID, "processed", "lambda processed event successfully"); err != nil {
		instruments.errors.Add(ctx, 1)
		span.RecordError(err)
		span.SetStatus(codes.Error, "update processing result")
		telemetry.Log(ctx, "lambda process failed: message_id=%s error=%v", record.MessageId, err)
		return fmt.Errorf("update processing result: %w", err)
	}

	instruments.processed.Add(ctx, 1)
	if ingestedAt := ingestedAt(record.MessageAttributes); !ingestedAt.IsZero() {
		instruments.latencyMS.Record(ctx, float64(time.Since(ingestedAt).Milliseconds()))
	}

	telemetry.Log(ctx, "lambda processed event: id=%s type=%s payload=%s", event.ID, event.Type, event.Payload)
	return nil
}

func workerInstruments() *struct {
	once      sync.Once
	processed metric.Int64Counter
	errors    metric.Int64Counter
	latencyMS metric.Float64Histogram
} {
	workerMetrics.once.Do(func() {
		meter := otel.Meter("simple_platform_setup/worker")
		workerMetrics.processed, _ = meter.Int64Counter("worker_processed_total")
		workerMetrics.errors, _ = meter.Int64Counter("worker_error_total")
		workerMetrics.latencyMS, _ = meter.Float64Histogram("worker_end_to_end_latency_ms")
	})

	return &workerMetrics
}

func ingestedAt(attributes map[string]events.SQSMessageAttribute) time.Time {
	attribute, ok := attributes["ingested_at_unix_nano"]
	if !ok || attribute.StringValue == nil {
		return time.Time{}
	}

	nanos, err := strconv.ParseInt(*attribute.StringValue, 10, 64)
	if err != nil {
		return time.Time{}
	}

	return time.Unix(0, nanos).UTC()
}

type messageAttributeCarrier map[string]events.SQSMessageAttribute

func (c messageAttributeCarrier) Get(key string) string {
	attribute, ok := c[key]
	if !ok || attribute.StringValue == nil {
		return ""
	}

	return *attribute.StringValue
}

func (c messageAttributeCarrier) Set(key, value string) {
	c[key] = events.SQSMessageAttribute{
		StringValue: &value,
	}
}

func (c messageAttributeCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for key := range c {
		keys = append(keys, key)
	}

	return keys
}
