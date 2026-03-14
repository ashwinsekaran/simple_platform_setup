package repo

import (
	"context"
	"errors"

	"github.com/ashwinsekaran/simple_platform_app/ingest/ent"
)

var ErrEventNotFound = errors.New("event not found")
var ErrEventConflict = errors.New("event already exists with different content")

type EventRepository interface {
	SaveEvent(ctx context.Context, event ent.Event) (created bool, err error)
	GetEvent(ctx context.Context, id string) (ent.EventRecord, error)
	Ready(ctx context.Context) error
}
