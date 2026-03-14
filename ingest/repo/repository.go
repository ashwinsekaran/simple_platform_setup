package repo

import (
	"context"
	"errors"

	"github.com/ashwinsekaran/simple_platform_app/ingest/ent"
)

var ErrEventNotFound = errors.New("event not found")

type EventRepository interface {
	PublishEvent(ctx context.Context, event ent.Event) error
	GetEvent(ctx context.Context, id string) (ent.Event, error)
	Ready(ctx context.Context) error
}
