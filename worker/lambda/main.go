package main

import (
	"context"
	"log"
	"time"

	"github.com/ashwinsekaran/simple_platform_app/monitoring/telemetry"
	"github.com/ashwinsekaran/simple_platform_app/worker/config"
	"github.com/ashwinsekaran/simple_platform_app/worker/lambda/service"
	"github.com/ashwinsekaran/simple_platform_app/worker/repo"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

func main() {
	cfg := config.Load()

	shutdownTelemetry, err := telemetry.Init(context.Background(), "worker", cfg.OTELEndpoint)
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

	eventRepo, err := repo.NewSQSRepository(context.Background(), cfg)
	if err != nil {
		log.Fatal(err)
	}

	eventService := service.New(eventRepo)
	lambda.Start(func(ctx context.Context, event events.SQSEvent) (events.SQSEventResponse, error) {
		return eventService.Process(ctx, event)
	})
}
