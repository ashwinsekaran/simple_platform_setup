package repo

import (
	"time"

	"github.com/ashwinsekaran/simple_platform_app/worker/ent"
)

type ReceivedEvent struct {
	Event         ent.Event
	Attributes    map[string]string
	IngestedAt    time.Time
	ReceiptHandle string
}
