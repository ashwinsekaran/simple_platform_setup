package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/ashwinsekaran/simple_platform_app/worker/config"
	workerrepo "github.com/ashwinsekaran/simple_platform_app/worker/repo"
)

func main() {
	eventID := os.Getenv("EVENT_ID")
	if eventID == "" {
		log.Fatal("EVENT_ID is required")
	}

	cfg := config.Load()

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

		if err := eventRepo.ReplayDLQEvent(ctx, event); err != nil {
			log.Fatal(err)
		}

		log.Printf("replayed dlq event: id=%s", eventID)
		return
	}

	log.Fatalf("event not found in dlq: %s", eventID)
}
