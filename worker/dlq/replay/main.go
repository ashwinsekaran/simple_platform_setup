package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"time"

	"github.com/ashwinsekaran/simple_platform_app/monitoring/telemetry"
	"github.com/ashwinsekaran/simple_platform_app/worker/config"
	workerrepo "github.com/ashwinsekaran/simple_platform_app/worker/repo"
)

func main() {
	eventID := os.Getenv("EVENT_ID")
	if eventID == "" {
		log.Fatal("EVENT_ID is required")
	}
	fixedPayload := os.Getenv("FIXED_PAYLOAD")
	fixedType := os.Getenv("FIXED_TYPE")

	cfg := config.Load()

	shutdownTelemetry, err := telemetry.Init(context.Background(), "dlq-replay", cfg.OTELEndpoint)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTelemetry(ctx); err != nil {
			log.Printf("shutdown telemetry: %v", err)
		}
	}()

	eventRepo, err := workerrepo.NewSQSRepository(context.Background(), cfg)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	events, err := eventRepo.ReceiveDLQEvents(ctx, 10)
	if err != nil {
		log.Fatal(err)
	}

	for _, event := range events {
		if event.Event.ID != eventID {
			continue
		}

		if fixedType != "" {
			event.Event.Type = fixedType
		}

		if fixedPayload != "" {
			if !json.Valid([]byte(fixedPayload)) {
				log.Fatal("FIXED_PAYLOAD must be valid JSON")
			}

			event.Event.Payload = json.RawMessage(fixedPayload)
		}

		if fixedPayload != "" || fixedType != "" {
			if err := eventRepo.PrepareReplayEvent(ctx, event.Event); err != nil {
				log.Fatal(err)
			}
		}

		if err := eventRepo.ReplayDLQEvent(ctx, event); err != nil {
			log.Fatal(err)
		}

		log.Printf("replayed dlq event: id=%s", eventID)
		return
	}

	log.Fatalf("event not found in dlq: %s", eventID)
}
