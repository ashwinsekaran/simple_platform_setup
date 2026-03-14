package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ashwinsekaran/simple_platform_app/ingest/ent"
	"github.com/ashwinsekaran/simple_platform_app/ingest/repo"
)

func TestPostEventCreatesEvent(t *testing.T) {
	server := NewIngestServer(stubEventRepository{created: true})

	request := httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(`{"id":"evt-1","type":"user.created","payload":{"name":"Ada"}}`))
	recorder := httptest.NewRecorder()

	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, recorder.Code)
	}
}

func TestPostEventReturnsOKForIdempotentReplay(t *testing.T) {
	server := NewIngestServer(stubEventRepository{created: false})
	handler := server.Handler()
	body := `{"id":"evt-1","type":"user.created","payload":{"name":"Ada","role":"admin"}}`

	firstRequest := httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(body))
	firstRecorder := httptest.NewRecorder()
	handler.ServeHTTP(firstRecorder, firstRequest)

	secondRequest := httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(`{"id":"evt-1","type":"user.created","payload":{"role":"admin","name":"Ada"}}`))
	secondRecorder := httptest.NewRecorder()
	handler.ServeHTTP(secondRecorder, secondRequest)

	if firstRecorder.Code != http.StatusOK {
		t.Fatalf("expected first status %d, got %d", http.StatusOK, firstRecorder.Code)
	}

	if secondRecorder.Code != http.StatusOK {
		t.Fatalf("expected second status %d, got %d", http.StatusOK, secondRecorder.Code)
	}
}

func TestPostEventValidatesRequiredFields(t *testing.T) {
	server := NewIngestServer(stubEventRepository{})

	request := httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(`{"id":"","type":"user.created","payload":{"name":"Ada"}}`))
	recorder := httptest.NewRecorder()

	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
	}
}

func TestPostEventReturnsInternalServerErrorWhenPublishFails(t *testing.T) {
	server := NewIngestServer(stubEventRepository{err: errors.New("publish failed")})

	request := httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(`{"id":"evt-1","type":"user.created","payload":{"name":"Ada"}}`))
	recorder := httptest.NewRecorder()

	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, recorder.Code)
	}
}

func TestPostEventReturnsConflictWhenExistingEventDiffers(t *testing.T) {
	server := NewIngestServer(stubEventRepository{err: repo.ErrEventConflict})

	request := httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(`{"id":"evt-1","type":"user.created","payload":{"name":"Ada"}}`))
	recorder := httptest.NewRecorder()

	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, recorder.Code)
	}
}

func TestGetEventReturnsEventByID(t *testing.T) {
	server := NewIngestServer(stubEventRepository{
		events: map[string]ent.Event{
			"evt-1": {
				ID:      "evt-1",
				Type:    "user.created",
				Payload: json.RawMessage(`{"name":"Ada"}`),
			},
		},
	})

	request := httptest.NewRequest(http.MethodGet, "/events/evt-1", nil)
	recorder := httptest.NewRecorder()

	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
}

func TestGetEventReturnsNotFound(t *testing.T) {
	server := NewIngestServer(stubEventRepository{})

	request := httptest.NewRequest(http.MethodGet, "/events/missing", nil)
	recorder := httptest.NewRecorder()

	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, recorder.Code)
	}
}

func TestHealthReturnsOK(t *testing.T) {
	server := NewIngestServer(stubEventRepository{})

	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	recorder := httptest.NewRecorder()

	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
}

func TestReadyReturnsOK(t *testing.T) {
	server := NewIngestServer(stubEventRepository{})

	request := httptest.NewRequest(http.MethodGet, "/ready", nil)
	recorder := httptest.NewRecorder()

	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
}

func TestReadyReturnsServiceUnavailableWhenRepoIsNotReady(t *testing.T) {
	server := NewIngestServer(stubEventRepository{readyErr: errors.New("not ready")})

	request := httptest.NewRequest(http.MethodGet, "/ready", nil)
	recorder := httptest.NewRecorder()

	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, recorder.Code)
	}
}

type stubEventRepository struct {
	err      error
	readyErr error
	events   map[string]ent.Event
	created  bool
}

func (s stubEventRepository) SaveEvent(_ context.Context, event ent.Event) (bool, error) {
	if s.events == nil {
		return s.created, s.err
	}

	s.events[event.ID] = event
	return s.created, s.err
}

func (s stubEventRepository) GetEvent(_ context.Context, id string) (ent.Event, error) {
	event, ok := s.events[id]
	if !ok {
		return ent.Event{}, repo.ErrEventNotFound
	}

	return event, nil
}

func (s stubEventRepository) Ready(_ context.Context) error {
	return s.readyErr
}
