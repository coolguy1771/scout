package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/coolguy1771/scout/internal/config"
	"github.com/coolguy1771/scout/internal/database"
	"github.com/coolguy1771/scout/internal/middleware/auth"
	"github.com/coolguy1771/scout/internal/models"
	"github.com/coolguy1771/scout/internal/repository"
	"github.com/coolguy1771/scout/internal/storage"
)

// LayerHandler handles layer-related requests
type LayerHandler struct {
	*Handler
	layerRepo *repository.LayerRepository
}

func NewLayerHandler(db *database.DB, logger *zap.Logger, cfg *config.Config) *LayerHandler {
	layerRepo := repository.NewLayerRepository(db.DB, logger)
	return &LayerHandler{
		Handler:   NewHandler(db, logger, cfg),
		layerRepo: layerRepo,
	}
}

// UploadLayer handles file upload for private layers
func (h *LayerHandler) UploadLayer(w http.ResponseWriter, r *http.Request) {
	// Check RBAC - only analyst and admin can upload
	role := auth.GetRole(r.Context())
	if role != "analyst" && role != "admin" {
		respondError(w, http.StatusForbidden, "Only analysts and admins can upload layers")
		return
	}

	tenantIDStr := auth.GetTenantID(r.Context())
	if tenantIDStr == "" {
		respondError(w, http.StatusBadRequest, "Tenant ID required")
		return
	}

	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid tenant ID")
		return
	}

	userIDStr := auth.GetUserID(r.Context())
	if userIDStr == "" {
		respondError(w, http.StatusBadRequest, "User ID required")
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	// Parse multipart form
	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10MB memory limit
		respondError(w, http.StatusBadRequest, "Failed to parse multipart form")
		return
	}

	// Get file
	file, header, err := r.FormFile("file")
	if err != nil {
		respondError(w, http.StatusBadRequest, "File is required")
		return
	}
	defer file.Close()

	// Validate file size
	maxSize := h.config.LayerUpload.MaxFileSize
	if maxSize <= 0 {
		maxSize = 500 * 1024 * 1024 // Default 500MB
	}

	if header.Size > maxSize {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("File size exceeds maximum of %d bytes", maxSize))
		return
	}

	// Validate file format
	ext := strings.ToLower(filepath.Ext(header.Filename))
	allowedFormats := h.config.LayerUpload.AllowedFormats
	if len(allowedFormats) == 0 {
		allowedFormats = []string{"geojson"}
	}

	formatValid := false
	for _, format := range allowedFormats {
		if ext == "."+strings.ToLower(format) || ext == "."+strings.ToLower(format)+".json" {
			formatValid = true
			break
		}
	}

	if !formatValid {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("File format not allowed. Allowed formats: %v", allowedFormats))
		return
	}

	// Get layer name from form
	layerName := r.FormValue("name")
	if layerName == "" {
		layerName = strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename))
	}

	// Upload file to S3 staging area
	var s3Storage *storage.S3Storage
	if s3Storage, err = storage.NewS3Storage(h.config.S3, h.logger); err != nil {
		respondError(w, http.StatusInternalServerError, "Storage not configured")
		return
	}

	layerID := uuid.New()
	objectKey := fmt.Sprintf("uploads/%s/%s/%s", tenantIDStr, layerID.String(), header.Filename)

	// Read file content
	fileContent, err := io.ReadAll(file)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to read file")
		return
	}

	// Upload to S3
	ctx := r.Context()
	if err := s3Storage.Upload(ctx, objectKey, bytes.NewReader(fileContent), "application/json"); err != nil {
		h.logger.Error("Failed to upload file to S3", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Failed to upload file")
		return
	}

	// Create layer record
	layer := &models.PrivateLayer{
		LayerID:   layerID,
		TenantID:  tenantID,
		Name:      layerName,
		LayerType: "mixed", // Will be determined during processing
		FileName:  header.Filename,
		FileSize:  header.Size,
		Status:    "uploading",
		ObjectKey: objectKey,
		CreatedBy: userID,
	}

	if err := h.layerRepo.Create(ctx, layer); err != nil {
		h.logger.Error("Failed to create layer record", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Failed to create layer record")
		return
	}

	// Create processing job
	jobInput := map[string]interface{}{
		"layerId":   layerID.String(),
		"objectKey": objectKey,
		"fileName":  header.Filename,
	}
	jobInputJSON, _ := json.Marshal(jobInput)

	job := &models.Job{
		TenantID:  tenantID,
		Type:      "layer_upload",
		Status:    "pending",
		InputJSON: string(jobInputJSON),
	}

	if err := h.jobRepo.Create(ctx, job); err != nil {
		h.logger.Error("Failed to create processing job", zap.Error(err))
		// Update layer status to failed
		h.layerRepo.UpdateStatus(ctx, layerID, "failed", nil, "Failed to create processing job")
		respondError(w, http.StatusInternalServerError, "Failed to create processing job")
		return
	}

	// Update layer status to processing
	h.layerRepo.UpdateStatus(ctx, layerID, "processing", nil, "")

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"layerId": layerID.String(),
		"jobId":   job.JobID.String(),
		"status":  "processing",
	})
}

// ListLayers lists all layers for the tenant
func (h *LayerHandler) ListLayers(w http.ResponseWriter, r *http.Request) {
	tenantIDStr := auth.GetTenantID(r.Context())
	if tenantIDStr == "" {
		respondError(w, http.StatusBadRequest, "Tenant ID required")
		return
	}

	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid tenant ID")
		return
	}

	limit := 50
	offset := 0
	// Parse query params for pagination
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := parseInt(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := parseInt(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	layers, err := h.layerRepo.ListByTenant(r.Context(), tenantID, limit, offset)
	if err != nil {
		h.logger.Error("Failed to list layers", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Failed to list layers")
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"layers": layers,
		"limit":  limit,
		"offset": offset,
	})
}

// GetLayer gets a single layer by ID
func (h *LayerHandler) GetLayer(w http.ResponseWriter, r *http.Request) {
	layerIDStr := chi.URLParam(r, "layerId")
	layerID, err := uuid.Parse(layerIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid layer ID")
		return
	}

	tenantIDStr := auth.GetTenantID(r.Context())
	if tenantIDStr == "" {
		respondError(w, http.StatusBadRequest, "Tenant ID required")
		return
	}

	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid tenant ID")
		return
	}

	layer, err := h.layerRepo.GetByID(r.Context(), layerID, tenantID)
	if err != nil {
		respondError(w, http.StatusNotFound, "Layer not found")
		return
	}

	respondJSON(w, http.StatusOK, layer)
}

// UpdateLayer updates a layer's name
func (h *LayerHandler) UpdateLayer(w http.ResponseWriter, r *http.Request) {
	layerIDStr := chi.URLParam(r, "layerId")
	layerID, err := uuid.Parse(layerIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid layer ID")
		return
	}

	var req UpdateLayerRequest
	if err := ParseJSONRequest(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate request
	if err := ValidateRequest(&req); err != nil {
		errors := GetValidationErrors(err)
		respondJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error":   "Validation failed",
			"details": errors,
		})
		return
	}

	tenantIDStr := auth.GetTenantID(r.Context())
	if tenantIDStr == "" {
		respondError(w, http.StatusBadRequest, "Tenant ID required")
		return
	}

	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid tenant ID")
		return
	}

	if err := h.layerRepo.Update(r.Context(), layerID, tenantID, req.Name); err != nil {
		h.logger.Error("Failed to update layer", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Failed to update layer")
		return
	}

	// Fetch updated layer
	layer, err := h.layerRepo.GetByID(r.Context(), layerID, tenantID)
	if err != nil {
		respondError(w, http.StatusNotFound, "Layer not found")
		return
	}

	respondJSON(w, http.StatusOK, layer)
}

// DeleteLayer deletes a layer (admin only)
func (h *LayerHandler) DeleteLayer(w http.ResponseWriter, r *http.Request) {
	// Check RBAC - only admin can delete
	role := auth.GetRole(r.Context())
	if role != "admin" {
		respondError(w, http.StatusForbidden, "Only admins can delete layers")
		return
	}

	layerIDStr := chi.URLParam(r, "layerId")
	layerID, err := uuid.Parse(layerIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid layer ID")
		return
	}

	tenantIDStr := auth.GetTenantID(r.Context())
	if tenantIDStr == "" {
		respondError(w, http.StatusBadRequest, "Tenant ID required")
		return
	}

	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid tenant ID")
		return
	}

	if err := h.layerRepo.Delete(r.Context(), layerID, tenantID); err != nil {
		respondError(w, http.StatusNotFound, "Layer not found")
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Layer deleted successfully",
	})
}

// Helper function to parse integer from string
func parseInt(s string) (int, error) {
	var result int
	_, err := fmt.Sscanf(s, "%d", &result)
	return result, err
}
