package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/coolguy1771/scout/internal/config"
	"github.com/coolguy1771/scout/internal/database"
	"github.com/coolguy1771/scout/internal/search/opensearch"
	"github.com/coolguy1771/scout/internal/storage"
)

type HealthHandler struct {
	db          *database.DB
	searchClient *opensearch.Client
	s3Storage   *storage.S3Storage
	cfg         *config.Config
	logger      *zap.Logger
}

func NewHealthHandler(db *database.DB, searchClient *opensearch.Client, s3Storage *storage.S3Storage, cfg *config.Config, logger *zap.Logger) *HealthHandler {
	return &HealthHandler{
		db:           db,
		searchClient: searchClient,
		s3Storage:    s3Storage,
		cfg:          cfg,
		logger:       logger,
	}
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status    string            `json:"status"`
	Timestamp string            `json:"timestamp"`
	Checks    map[string]string `json:"checks,omitempty"`
}

// Live handles liveness probe - indicates the service is running
func (h *HealthHandler) Live(w http.ResponseWriter, r *http.Request) {
	response := HealthResponse{
		Status:    "ok",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// Ready handles readiness probe - indicates the service is ready to accept traffic
func (h *HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	checks := make(map[string]string)
	allHealthy := true

	// Check database
	if h.db != nil {
		if err := h.db.Health(); err != nil {
			checks["database"] = "unhealthy: " + err.Error()
			allHealthy = false
		} else {
			checks["database"] = "healthy"
		}
	} else {
		checks["database"] = "not configured"
	}

	// Check OpenSearch (if enabled)
	if h.cfg.Search.Enabled && h.searchClient != nil {
		if err := h.searchClient.Ping(ctx); err != nil {
			checks["opensearch"] = "unhealthy: " + err.Error()
			allHealthy = false
		} else {
			checks["opensearch"] = "healthy"
		}
	} else {
		checks["opensearch"] = "disabled"
	}

	// Check S3 storage (if configured)
	if h.s3Storage != nil {
		if err := h.s3Storage.HealthCheck(ctx); err != nil {
			checks["s3"] = "unhealthy: " + err.Error()
			allHealthy = false
		} else {
			checks["s3"] = "healthy"
		}
	} else {
		checks["s3"] = "not configured"
	}

	response := HealthResponse{
		Status:    "ok",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Checks:    checks,
	}

	statusCode := http.StatusOK
	if !allHealthy {
		statusCode = http.StatusServiceUnavailable
		response.Status = "degraded"
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}

// Health is a simple health check endpoint (backwards compatible)
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}



