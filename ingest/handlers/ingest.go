package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/ashwinsekaran/simple_platform_app/common"
	"github.com/ashwinsekaran/simple_platform_app/ingest/ent"
	"github.com/ashwinsekaran/simple_platform_app/ingest/repo"
)

type IngestServer struct {
	repo repo.EventRepository
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
	var request struct {
		ID      string          `json:"id"`
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		common.WriteJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid JSON body",
		})
		return
	}

	if request.ID == "" || request.Type == "" || len(request.Payload) == 0 {
		common.WriteJSON(w, http.StatusBadRequest, map[string]string{
			"error": "id, type, and payload are required",
		})
		return
	}

	if !json.Valid(request.Payload) {
		common.WriteJSON(w, http.StatusBadRequest, map[string]string{
			"error": "payload must be valid JSON",
		})
		return
	}

	event := ent.Event{
		ID:      request.ID,
		Type:    request.Type,
		Payload: request.Payload,
	}

	if err := s.repo.PublishEvent(r.Context(), event); err != nil {
		log.Printf("publish event failed: %v", err)
		common.WriteJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to publish event",
		})
		return
	}

	log.Printf("published event: id=%s type=%s payload=%s", event.ID, event.Type, event.Payload)
	common.WriteJSON(w, http.StatusAccepted, event)
}

func (s *IngestServer) HandleGetEvent(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		common.WriteJSON(w, http.StatusBadRequest, map[string]string{
			"error": "event id is required",
		})
		return
	}

	event, err := s.repo.GetEvent(r.Context(), id)
	if err != nil {
		if err == repo.ErrEventNotFound {
			common.WriteJSON(w, http.StatusNotFound, map[string]string{
				"error": "event not found",
			})
			return
		}

		log.Printf("get event failed: %v", err)
		common.WriteJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to get event",
		})
		return
	}

	common.WriteJSON(w, http.StatusOK, event)
}

func (s *IngestServer) HandleHealth(w http.ResponseWriter, _ *http.Request) {
	common.WriteJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

func (s *IngestServer) HandleReady(w http.ResponseWriter, r *http.Request) {
	if err := s.repo.Ready(r.Context()); err != nil {
		log.Printf("readiness check failed: %v", err)
		common.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "not_ready",
		})
		return
	}

	common.WriteJSON(w, http.StatusOK, map[string]string{
		"status": "ready",
	})
}
