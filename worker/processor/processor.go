package processor

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/ashwinsekaran/simple_platform_app/monitoring/telemetry"
	"github.com/ashwinsekaran/simple_platform_app/worker/repo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
)

const (
	defaultBatchSize   int32 = 10
	defaultConcurrency       = 5
)

type Processor struct {
	repo        repo.EventRepository
	batchSize   int32
	concurrency int
}

var processorMetrics struct {
	once      sync.Once
	processed metric.Int64Counter
	errors    metric.Int64Counter
	latencyMS metric.Float64Histogram
}

func New(eventRepo repo.EventRepository) *Processor {
	return &Processor{
		repo:        eventRepo,
		batchSize:   defaultBatchSize,
		concurrency: defaultConcurrency,
	}
}

func (p *Processor) Run(ctx context.Context) error {
	sem := make(chan struct{}, p.concurrency)
	var wg sync.WaitGroup

	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return ctx.Err()
		default:
		}

		events, err := p.repo.ReceiveEvents(ctx, p.batchSize)
		if err != nil {
			if ctx.Err() != nil {
				wg.Wait()
				return ctx.Err()
			}

			log.Printf("receive events failed: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}

		for _, event := range events {
			sem <- struct{}{}
			wg.Add(1)

			go func(received repo.ReceivedEvent) {
				defer wg.Done()
				defer func() { <-sem }()

				if err := p.processEvent(ctx, received); err != nil {
					processorInstruments().errors.Add(ctx, 1)
					telemetry.Log(ctx, "process event failed: %v", err)
				}
			}(event)
		}
	}
}

func (p *Processor) processEvent(ctx context.Context, received repo.ReceivedEvent) error {
	ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier(received.Attributes))
	ctx, span := otel.Tracer("simple_platform_setup/worker").Start(ctx, "worker.process_event")
	defer span.End()

	span.SetAttributes(
		attribute.String("event.id", received.Event.ID),
		attribute.String("event.type", received.Event.Type),
	)

	telemetry.Log(
		ctx,
		"processed event: id=%s type=%s payload=%s",
		received.Event.ID,
		received.Event.Type,
		received.Event.Payload,
	)

	instruments := processorInstruments()
	instruments.processed.Add(ctx, 1)
	if !received.IngestedAt.IsZero() {
		instruments.latencyMS.Record(ctx, float64(time.Since(received.IngestedAt).Milliseconds()))
	}

	if err := p.repo.UpdateProcessingResult(ctx, received.Event.ID, "processed", "worker processed event successfully"); err != nil {
		return err
	}

	return p.repo.DeleteEvent(ctx, received.ReceiptHandle)
}

func processorInstruments() *struct {
	once      sync.Once
	processed metric.Int64Counter
	errors    metric.Int64Counter
	latencyMS metric.Float64Histogram
} {
	processorMetrics.once.Do(func() {
		meter := otel.Meter("simple_platform_setup/worker")
		processorMetrics.processed, _ = meter.Int64Counter("worker_processed_total")
		processorMetrics.errors, _ = meter.Int64Counter("worker_error_total")
		processorMetrics.latencyMS, _ = meter.Float64Histogram("worker_end_to_end_latency_ms")
	})

	return &processorMetrics
}
