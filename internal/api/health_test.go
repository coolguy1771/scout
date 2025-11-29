package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/coolguy1771/scout/internal/config"
	"go.uber.org/zap"
)

func TestHealthHandler_Live(t *testing.T) {
	handler := NewHealthHandler(nil, nil, nil, &config.Config{}, zap.NewNop())

	req := httptest.NewRequest("GET", "/health/live", nil)
	w := httptest.NewRecorder()

	handler.Live(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestHealthHandler_Health(t *testing.T) {
	handler := NewHealthHandler(nil, nil, nil, &config.Config{}, zap.NewNop())

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	handler.Health(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	if w.Body.String() != "OK" {
		t.Errorf("Expected body 'OK', got '%s'", w.Body.String())
	}
}

