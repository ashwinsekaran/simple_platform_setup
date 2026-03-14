package repo

import (
	"context"

	"github.com/ashwinsekaran/simple_platform_app/ingest/ent"
)

type EventRepository interface {
	ReceiveEvents(ctx context.Context, maxMessages int32) ([]ReceivedEvent, error)
	DeleteEvent(ctx context.Context, receiptHandle string) error
}

type ReceivedEvent struct {
	Event         ent.Event
	ReceiptHandle string
}
