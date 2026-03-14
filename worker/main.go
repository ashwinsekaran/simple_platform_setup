package main

import (
	"context"
	"errors"
	"log"
	"os/signal"
	"syscall"

	"github.com/ashwinsekaran/simple_platform_app/ingest/config"
	"github.com/ashwinsekaran/simple_platform_app/worker/processor"
	"github.com/ashwinsekaran/simple_platform_app/worker/repo"
)

func main() {
	cfg := config.Load()

	eventRepo, err := repo.NewSQSRepository(context.Background(), cfg)
	if err != nil {
		log.Fatal(err)
	}

	eventProcessor := processor.New(eventRepo)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := eventProcessor.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}
