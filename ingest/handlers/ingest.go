package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/ashwinsekaran/simple_platform_app/common"
	"github.com/ashwinsekaran/simple_platform_app/ingest/ent"
)

type IngestServer struct{}

func NewIngestServer() *IngestServer {
	return &IngestServer{}
}

func (s *IngestServer) Handler() http.Handler {
	return s.routes()
}

func (s *IngestServer) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /events", s.HandlePostEvent)
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

	log.Printf("received event: id=%s type=%s payload=%s", event.ID, event.Type, event.Payload)
	common.WriteJSON(w, http.StatusAccepted, event)
}
