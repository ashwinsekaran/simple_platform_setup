package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ashwinsekaran/simple_platform_app/ingest/ent"
	"github.com/ashwinsekaran/simple_platform_app/ingest/repo"
	"github.com/ashwinsekaran/simple_platform_app/monitoring/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
)

type IngestServer struct {
	repo repo.EventRepository
}

var ingestMetrics struct {
	once       sync.Once
	requests   metric.Int64Counter
	successes  metric.Int64Counter
	errors     metric.Int64Counter
	durationMS metric.Float64Histogram
}

func NewIngestServer(eventRepo repo.EventRepository) *IngestServer {
	return &IngestServer{
		repo: eventRepo,
	}
}

func (s *IngestServer) Handler() http.Handler {
	return s.routes()
}

func (s *IngestServer) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.HandleHealth)
	mux.HandleFunc("GET /ready", s.HandleReady)
	mux.HandleFunc("POST /events", s.HandlePostEvent)
	mux.HandleFunc("GET /events/{id}", s.HandleGetEvent)
	return mux
}

func (s *IngestServer) HandlePostEvent(w http.ResponseWriter, r *http.Request) {
	ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
	ctx, span := otel.Tracer("simple_platform_setup/ingest").Start(ctx, "ingest.post_event")
	defer span.End()

	instruments := ingestInstruments()
	instruments.requests.Add(ctx, 1)
	start := time.Now()
	defer instruments.durationMS.Record(ctx, float64(time.Since(start).Milliseconds()))

	var request struct {
		ID      string          `json:"id"`
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		instruments.errors.Add(ctx, 1)
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid JSON body")
		WriteJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid JSON body",
		})
		return
	}

	if request.ID == "" || request.Type == "" || len(request.Payload) == 0 {
		instruments.errors.Add(ctx, 1)
		span.SetStatus(codes.Error, "missing required fields")
		WriteJSON(w, http.StatusBadRequest, map[string]string{
			"error": "id, type, and payload are required",
		})
		return
	}

	if !json.Valid(request.Payload) {
		instruments.errors.Add(ctx, 1)
		span.SetStatus(codes.Error, "payload must be valid JSON")
		WriteJSON(w, http.StatusBadRequest, map[string]string{
			"error": "payload must be valid JSON",
		})
		return
	}

	event := ent.Event{
		ID:      request.ID,
		Type:    request.Type,
		Payload: request.Payload,
	}
	span.SetAttributes(
		attribute.String("event.id", event.ID),
		attribute.String("event.type", event.Type),
	)

	created, err := s.repo.SaveEvent(ctx, event)
	if err != nil {
		if err == repo.ErrEventConflict {
			instruments.errors.Add(ctx, 1)
			span.SetStatus(codes.Error, "event conflict")
			WriteJSON(w, http.StatusConflict, map[string]string{
				"error": "event id already exists with different content",
			})
			return
		}

		instruments.errors.Add(ctx, 1)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to publish event")
		telemetry.Log(ctx, "publish event failed: %v", err)
		WriteJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to publish event",
		})
		return
	}

	if created {
		instruments.successes.Add(ctx, 1)
		telemetry.Log(ctx, "stored and published event: id=%s type=%s payload=%s", event.ID, event.Type, event.Payload)
		WriteJSON(w, http.StatusAccepted, event)
		return
	}

	instruments.successes.Add(ctx, 1)
	telemetry.Log(ctx, "idempotent replay ignored for event: id=%s", event.ID)
	WriteJSON(w, http.StatusOK, event)
}

func (s *IngestServer) HandleGetEvent(w http.ResponseWriter, r *http.Request) {
	ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
	ctx, span := otel.Tracer("simple_platform_setup/ingest").Start(ctx, "ingest.get_event")
	defer span.End()

	instruments := ingestInstruments()
	instruments.requests.Add(ctx, 1)
	start := time.Now()
	defer instruments.durationMS.Record(ctx, float64(time.Since(start).Milliseconds()))

	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		instruments.errors.Add(ctx, 1)
		span.SetStatus(codes.Error, "missing event id")
		WriteJSON(w, http.StatusBadRequest, map[string]string{
			"error": "event id is required",
		})
		return
	}

	span.SetAttributes(attribute.String("event.id", id))

	event, err := s.repo.GetEvent(ctx, id)
	if err != nil {
		if err == repo.ErrEventNotFound {
			instruments.errors.Add(ctx, 1)
			span.SetStatus(codes.Error, "event not found")
			WriteJSON(w, http.StatusNotFound, map[string]string{
				"error": "event not found",
			})
			return
		}

		instruments.errors.Add(ctx, 1)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get event")
		telemetry.Log(ctx, "get event failed: %v", err)
		WriteJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to get event",
		})
		return
	}

	instruments.successes.Add(ctx, 1)
	span.SetAttributes(attribute.String("event.type", event.Type))
	telemetry.Log(ctx, "fetched event: id=%s status=%s", event.ID, event.ProcessingStatus)
	WriteJSON(w, http.StatusOK, event)
}

func (s *IngestServer) HandleHealth(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

func (s *IngestServer) HandleReady(w http.ResponseWriter, r *http.Request) {
	if err := s.repo.Ready(r.Context()); err != nil {
		log.Printf("readiness check failed: %v", err)
		WriteJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "not_ready",
		})
		return
	}

	WriteJSON(w, http.StatusOK, map[string]string{
		"status": "ready",
	})
}

func ingestInstruments() *struct {
	once       sync.Once
	requests   metric.Int64Counter
	successes  metric.Int64Counter
	errors     metric.Int64Counter
	durationMS metric.Float64Histogram
} {
	ingestMetrics.once.Do(func() {
		meter := otel.Meter("simple_platform_setup/ingest")
		ingestMetrics.requests, _ = meter.Int64Counter("ingest_requests_total")
		ingestMetrics.successes, _ = meter.Int64Counter("ingest_success_total")
		ingestMetrics.errors, _ = meter.Int64Counter("ingest_error_total")
		ingestMetrics.durationMS, _ = meter.Float64Histogram("ingest_request_duration_ms")
	})

	return &ingestMetrics
}

func WriteJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write response: %v", err)
	}
}
