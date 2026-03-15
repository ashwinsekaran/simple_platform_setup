package repo

import (
	"context"
	"time"

	"github.com/ashwinsekaran/simple_platform_app/worker/ent"
)

type EventRepository interface {
	ReceiveEvents(ctx context.Context, maxMessages int32) ([]ReceivedEvent, error)
	DeleteEvent(ctx context.Context, receiptHandle string) error
	UpdateProcessingResult(ctx context.Context, id, status, result string) error
}

type ReceivedEvent struct {
	Event         ent.Event
	Attributes    map[string]string
	IngestedAt    time.Time
	ReceiptHandle string
}
