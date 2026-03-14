package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ashwinsekaran/simple_platform_app/ingest/ent"
)

func TestPostEventCreatesEvent(t *testing.T) {
	server := NewIngestServer(stubEventRepository{})

	request := httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(`{"id":"evt-1","type":"user.created","payload":{"name":"Ada"}}`))
	recorder := httptest.NewRecorder()

	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, recorder.Code)
	}
}

func TestPostEventAllowsRepeatedPosts(t *testing.T) {
	server := NewIngestServer(stubEventRepository{})
	handler := server.Handler()
	body := `{"id":"evt-1","type":"user.created","payload":{"name":"Ada","role":"admin"}}`

	firstRequest := httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(body))
	firstRecorder := httptest.NewRecorder()
	handler.ServeHTTP(firstRecorder, firstRequest)

	secondRequest := httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(`{"id":"evt-1","type":"user.created","payload":{"role":"admin","name":"Ada"}}`))
	secondRecorder := httptest.NewRecorder()
	handler.ServeHTTP(secondRecorder, secondRequest)

	if firstRecorder.Code != http.StatusAccepted {
		t.Fatalf("expected first status %d, got %d", http.StatusAccepted, firstRecorder.Code)
	}

	if secondRecorder.Code != http.StatusAccepted {
		t.Fatalf("expected second status %d, got %d", http.StatusAccepted, secondRecorder.Code)
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

type stubEventRepository struct {
	err error
}

func (s stubEventRepository) PublishEvent(_ context.Context, _ ent.Event) error {
	return s.err
}
