package processor

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/ashwinsekaran/simple_platform_app/worker/repo"
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
					log.Printf("process event failed: %v", err)
				}
			}(event)
		}
	}
}

func (p *Processor) processEvent(ctx context.Context, received repo.ReceivedEvent) error {
	log.Printf(
		"processed event: id=%s type=%s payload=%s",
		received.Event.ID,
		received.Event.Type,
		received.Event.Payload,
	)

	if err := p.repo.UpdateProcessingResult(ctx, received.Event.ID, "processed", "worker processed event successfully"); err != nil {
		return err
	}

	return p.repo.DeleteEvent(ctx, received.ReceiptHandle)
}
