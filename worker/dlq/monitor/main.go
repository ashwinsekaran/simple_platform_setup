package main

import (
	"context"
	"log"
	"time"

	"github.com/ashwinsekaran/simple_platform_app/monitoring/telemetry"
	"github.com/ashwinsekaran/simple_platform_app/worker/config"
	workerrepo "github.com/ashwinsekaran/simple_platform_app/worker/repo"
)

func main() {
	cfg := config.Load()

	shutdownTelemetry, err := telemetry.Init(context.Background(), "dlq-monitor", cfg.OTELEndpoint)
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

	ctx := context.Background()
	if err := eventRepo.StartDLQMonitoring(ctx, 15*time.Second); err != nil {
		log.Fatal(err)
	}
}
