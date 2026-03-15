package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ashwinsekaran/simple_platform_app/ingest/config"
	"github.com/ashwinsekaran/simple_platform_app/ingest/handlers"
	"github.com/ashwinsekaran/simple_platform_app/ingest/repo"
	"github.com/ashwinsekaran/simple_platform_app/monitoring/telemetry"
)

func main() {
	cfg := config.Load()

	shutdownTelemetry, err := telemetry.Init(context.Background(), "ingest", cfg.OTELEndpoint)
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

	ingestServer := handlers.NewIngestServer(eventRepo)
	httpServer := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: ingestServer.Handler(),
	}

	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("ingest server listening on %s", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
		close(serverErrors)
	}()

	shutdownSignals := make(chan os.Signal, 1)
	signal.Notify(shutdownSignals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(shutdownSignals)

	select {
	case err = <-serverErrors:
		if err != nil {
			log.Fatal(err)
		}
	case sig := <-shutdownSignals:
		log.Printf("shutdown signal received: %s", sig)

		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.ShutdownTimeoutS)*time.Second)
		defer cancel()

		if err := httpServer.Shutdown(ctx); err != nil {
			log.Fatal(err)
		}

		log.Print("ingest server stopped gracefully")
	}
}
