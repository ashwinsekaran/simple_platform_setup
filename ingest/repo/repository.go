package repo

import (
	"context"

	"github.com/ashwinsekaran/simple_platform_app/ingest/ent"
)

type EventRepository interface {
	PublishEvent(ctx context.Context, event ent.Event) error
}
