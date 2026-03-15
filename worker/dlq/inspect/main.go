package main

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/ashwinsekaran/simple_platform_app/worker/config"
	workerrepo "github.com/ashwinsekaran/simple_platform_app/worker/repo"
)

func main() {
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

	if len(events) == 0 {
		log.Print("no messages found in dlq")
		return
	}

	output, err := json.MarshalIndent(events, "", "  ")
	if err != nil {
		log.Fatal(err)
	}

	log.Print(string(output))
}
