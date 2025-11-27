package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/coolguy1771/scout/internal/config"
	"github.com/coolguy1771/scout/internal/database"
	"github.com/coolguy1771/scout/internal/middleware/auth"
	"github.com/coolguy1771/scout/internal/models"
	"github.com/coolguy1771/scout/internal/repository"
	"github.com/coolguy1771/scout/internal/scoring"
	"github.com/coolguy1771/scout/internal/search"
	"github.com/coolguy1771/scout/internal/storage"
)

// Base handler with common dependencies
type Handler struct {
	db                 *database.DB
	logger             *zap.Logger
	parcelRepo         *repository.ParcelRepository
	parcelFeatureRepo  *repository.ParcelFeatureRepository
	projectRepo        *repository.ProjectRepository
	jobRepo            *repository.JobRepository
	exportRepo         *repository.ExportRepository
	scoringProfileRepo *repository.ScoringProfileRepository
	searchService      *search.SearchService
	scoringService     *scoring.Service
	tileStorage        *storage.TileStorage
	config             *config.Config
}

func NewHandler(db *database.DB, logger *zap.Logger, cfg *config.Config) *Handler {
	parcelRepo := repository.NewParcelRepository(db.DB, logger)
	parcelFeatureRepo := repository.NewParcelFeatureRepository(db.DB, logger)
	projectRepo := repository.NewProjectRepository(db.DB, logger)
	jobRepo := repository.NewJobRepository(db.DB, logger)
	exportRepo := repository.NewExportRepository(db.DB, logger)
	scoringProfileRepo := repository.NewScoringProfileRepository(db.DB, logger)

	scoringService := scoring.NewService(scoringProfileRepo, logger)
	searchService := search.NewSearchService(parcelRepo, parcelFeatureRepo, scoringService, cfg, logger)

	// Initialize storage (may fail if S3 not configured)
	var tileStorage *storage.TileStorage
	if s3Storage, err := storage.NewS3Storage(cfg.S3, logger); err == nil {
		tileStorage = storage.NewTileStorage(s3Storage)
	}

	return &Handler{
		db:                 db,
		logger:             logger,
		parcelRepo:         parcelRepo,
		parcelFeatureRepo:  parcelFeatureRepo,
		projectRepo:        projectRepo,
		jobRepo:            jobRepo,
		exportRepo:         exportRepo,
		scoringProfileRepo: scoringProfileRepo,
		searchService:      searchService,
		scoringService:     scoringService,
		tileStorage:        tileStorage,
		config:             cfg,
	}
}

// ParcelHandler handles parcel-related requests
type ParcelHandler struct {
	*Handler
}

func NewParcelHandler(db *database.DB, logger *zap.Logger, cfg *config.Config) *ParcelHandler {
	return &ParcelHandler{Handler: NewHandler(db, logger, cfg)}
}

// GetParcel gets a parcel by ID
// @Summary Get parcel
// @Description Get a parcel by ID with features
// @Tags parcels
// @Accept json
// @Produce json
// @Param parcelId path string true "Parcel ID" format(uuid)
// @Success 200 {object} ParcelResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/parcels/{parcelId} [get]
func (h *ParcelHandler) GetParcel(w http.ResponseWriter, r *http.Request) {
	parcelIDStr := chi.URLParam(r, "parcelId")
	parcelID, err := uuid.Parse(parcelIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid parcel ID")
		return
	}

	tenantID := h.getTenantID(r.Context())
	var tenantUUID *uuid.UUID
	if tenantID != "" {
		id, err := uuid.Parse(tenantID)
		if err == nil {
			tenantUUID = &id
		}
	}

	parcel, err := h.parcelRepo.GetByID(r.Context(), parcelID, tenantUUID)
	if err != nil {
		h.logger.Error("Failed to get parcel", zap.Error(err))
		respondError(w, http.StatusNotFound, "Parcel not found")
		return
	}

	// Get features
	features, _ := h.parcelFeatureRepo.GetByParcelID(r.Context(), parcelID)

	response := ParcelResponse{
		ParcelID:     parcel.ParcelID.String(),
		APN:          parcel.APN,
		Acres:        parcel.Acres,
		ZoningRaw:    parcel.ZoningRaw,
		ZoningTags:   parcel.ZoningTags,
		Jurisdiction: parcel.Jurisdiction,
		StateFips:    parcel.StateFIPS,
	}

	if features != nil {
		response.Features = &ParcelFeatureResponse{
			InFloodplain:    features.InFloodplain,
			InWetlands:      features.InWetlands,
			InProtectedLand: features.InProtectedLand,
		}
		if features.DistToHighwayM > 0 {
			response.Features.DistToHighwayM = &features.DistToHighwayM
		}
		if features.DistToRailM > 0 {
			response.Features.DistToRailM = &features.DistToRailM
		}
		if features.DistToAirportM > 0 {
			response.Features.DistToAirportM = &features.DistToAirportM
		}
		if features.DistToPowerLineM > 0 {
			response.Features.DistToPowerLineM = &features.DistToPowerLineM
		}
		if features.DistToSubstationM > 0 {
			response.Features.DistToSubstationM = &features.DistToSubstationM
		}
	}

	respondJSON(w, http.StatusOK, response)
}

// GetNearby gets nearby features for a parcel
// @Summary Get nearby features
// @Description Get nearby infrastructure features for a parcel
// @Tags parcels
// @Accept json
// @Produce json
// @Param parcelId path string true "Parcel ID" format(uuid)
// @Success 200 {object} NearbyResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/parcels/{parcelId}/nearby [get]
func (h *ParcelHandler) GetNearby(w http.ResponseWriter, r *http.Request) {
	parcelIDStr := chi.URLParam(r, "parcelId")
	parcelID, err := uuid.Parse(parcelIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid parcel ID")
		return
	}

	// Parse query params
	typesParam := r.URL.Query().Get("types")
	types := []string{}
	if typesParam != "" {
		// Split by comma
		for _, t := range []rune(typesParam) {
			if t == ',' {
				continue
			}
			types = append(types, string(t))
		}
		// Simple split for MVP
		if len(types) == 0 {
			types = []string{"rail", "highway", "airport"}
		}
	} else {
		types = []string{"rail", "highway", "airport"}
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 10
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	maxDistanceStr := r.URL.Query().Get("maxDistanceM")
	maxDistanceM := 5000.0
	if maxDistanceStr != "" {
		if d, err := strconv.ParseFloat(maxDistanceStr, 64); err == nil && d > 0 {
			maxDistanceM = d
		}
	}

	nearby, err := h.parcelRepo.FindNearby(r.Context(), parcelID, types, maxDistanceM, limit)
	if err != nil {
		h.logger.Error("Failed to find nearby features", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Failed to find nearby features")
		return
	}

	// Convert to response format
	response := NearbyResponse{
		ParcelID: parcelID.String(),
		Features: make(map[string][]NearbyFeatureResponse),
	}

	for featureType, features := range nearby {
		response.Features[featureType] = []NearbyFeatureResponse{}
		for _, f := range features {
			featureID, _ := f["feature_id"].(string)
			name, _ := f["name"].(string)
			distanceM, _ := f["distance_m"].(float64)
			geom, _ := f["geom"]

			response.Features[featureType] = append(response.Features[featureType], NearbyFeatureResponse{
				FeatureID: featureID,
				Name:      name,
				DistanceM: distanceM,
				Geometry:  geom,
			})
		}
	}

	respondJSON(w, http.StatusOK, response)
}

// SearchHandler handles search requests
type SearchHandler struct {
	*Handler
}

func NewSearchHandler(db *database.DB, logger *zap.Logger, cfg *config.Config) *SearchHandler {
	return &SearchHandler{Handler: NewHandler(db, logger, cfg)}
}

// Search searches for parcels based on suitability criteria
// @Summary Search parcels
// @Description Search for parcels based on suitability criteria using OpenSearch query DSL
// @Tags search
// @Accept json
// @Produce json
// @Param query body object true "OpenSearch query DSL" example({"query":{"match_all":{}},"size":10})
// @Success 200 {object} object "Search results"
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/suitability/search [post]
func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	// Parse request body
	body, err := readBody(r)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	query, err := search.ParseSearchQuery(body)
	if err != nil {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid search query: %v", err))
		return
	}

	// Get tenant ID
	tenantID := h.getTenantID(r.Context())
	var tenantUUID *uuid.UUID
	if tenantID != "" {
		id, err := uuid.Parse(tenantID)
		if err == nil {
			tenantUUID = &id
		}
	}

	// Get scoring profile ID if provided
	var scoringProfileUUID *uuid.UUID
	if query.ScoringProfileID != nil && *query.ScoringProfileID != "" {
		id, err := uuid.Parse(*query.ScoringProfileID)
		if err == nil {
			scoringProfileUUID = &id
		}
	}

	// Perform search
	results, err := h.searchService.Search(r.Context(), query, tenantUUID, scoringProfileUUID)
	if err != nil {
		h.logger.Error("Search failed", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Search failed")
		return
	}

	respondJSON(w, http.StatusOK, results)
}

// ProjectHandler handles project and saved search requests
type ProjectHandler struct {
	*Handler
}

func NewProjectHandler(db *database.DB, logger *zap.Logger, cfg *config.Config) *ProjectHandler {
	return &ProjectHandler{Handler: NewHandler(db, logger, cfg)}
}

// CreateProject creates a new project
// @Summary Create project
// @Description Create a new project for the tenant
// @Tags projects
// @Accept json
// @Produce json
// @Param project body CreateProjectRequest true "Project information"
// @Success 201 {object} models.Project
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/projects [post]
func (h *ProjectHandler) CreateProject(w http.ResponseWriter, r *http.Request) {
	var req CreateProjectRequest
	if err := ParseJSONRequest(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	tenantID := h.getTenantID(r.Context())
	userID := h.getUserID(r.Context())
	if tenantID == "" || userID == "" {
		respondError(w, http.StatusUnauthorized, "Missing tenant or user ID")
		return
	}

	tenantUUID, _ := uuid.Parse(tenantID)
	userUUID, _ := uuid.Parse(userID)

	project := &models.Project{
		ProjectID: uuid.New(),
		TenantID:  tenantUUID,
		Name:      req.Name,
		CreatedBy: userUUID,
		CreatedAt: time.Now(),
	}

	if err := h.projectRepo.Create(r.Context(), project); err != nil {
		h.logger.Error("Failed to create project", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Failed to create project")
		return
	}

	respondJSON(w, http.StatusCreated, project)
}

// ListProjects lists all projects for the tenant
// @Summary List projects
// @Description List all projects for the tenant
// @Tags projects
// @Accept json
// @Produce json
// @Success 200 {array} models.Project
// @Failure 401 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/projects [get]
func (h *ProjectHandler) ListProjects(w http.ResponseWriter, r *http.Request) {
	tenantID := h.getTenantID(r.Context())
	if tenantID == "" {
		respondError(w, http.StatusUnauthorized, "Missing tenant ID")
		return
	}

	tenantUUID, _ := uuid.Parse(tenantID)
	projects, err := h.projectRepo.ListByTenant(r.Context(), tenantUUID)
	if err != nil {
		h.logger.Error("Failed to list projects", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Failed to list projects")
		return
	}

	// Ensure we return an empty array instead of null
	if projects == nil {
		projects = []models.Project{}
	}

	respondJSON(w, http.StatusOK, projects)
}

// GetProject gets a project by ID
// @Summary Get project
// @Description Get a project by ID
// @Tags projects
// @Accept json
// @Produce json
// @Param projectId path string true "Project ID" format(uuid)
// @Success 200 {object} models.Project
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/projects/{projectId} [get]
func (h *ProjectHandler) GetProject(w http.ResponseWriter, r *http.Request) {
	projectIDStr := chi.URLParam(r, "projectId")
	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid project ID")
		return
	}

	tenantID := h.getTenantID(r.Context())
	if tenantID == "" {
		respondError(w, http.StatusUnauthorized, "Missing tenant ID")
		return
	}

	tenantUUID, _ := uuid.Parse(tenantID)
	project, err := h.projectRepo.GetByID(r.Context(), projectID, tenantUUID)
	if err != nil {
		respondError(w, http.StatusNotFound, "Project not found")
		return
	}

	respondJSON(w, http.StatusOK, project)
}

// UpdateProject updates a project
// @Summary Update project
// @Description Update a project
// @Tags projects
// @Accept json
// @Produce json
// @Param projectId path string true "Project ID" format(uuid)
// @Param project body UpdateProjectRequest true "Project information"
// @Success 200 {object} models.Project
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/projects/{projectId} [put]
func (h *ProjectHandler) UpdateProject(w http.ResponseWriter, r *http.Request) {
	projectIDStr := chi.URLParam(r, "projectId")
	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid project ID")
		return
	}

	var req UpdateProjectRequest
	if err := ParseJSONRequest(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	tenantID := h.getTenantID(r.Context())
	if tenantID == "" {
		respondError(w, http.StatusUnauthorized, "Missing tenant ID")
		return
	}

	tenantUUID, _ := uuid.Parse(tenantID)
	project, err := h.projectRepo.GetByID(r.Context(), projectID, tenantUUID)
	if err != nil {
		respondError(w, http.StatusNotFound, "Project not found")
		return
	}

	project.Name = req.Name
	if err := h.projectRepo.Update(r.Context(), project); err != nil {
		h.logger.Error("Failed to update project", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Failed to update project")
		return
	}

	respondJSON(w, http.StatusOK, project)
}

// DeleteProject deletes a project
// @Summary Delete project
// @Description Delete a project
// @Tags projects
// @Accept json
// @Produce json
// @Param projectId path string true "Project ID" format(uuid)
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/projects/{projectId} [delete]
func (h *ProjectHandler) DeleteProject(w http.ResponseWriter, r *http.Request) {
	projectIDStr := chi.URLParam(r, "projectId")
	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid project ID")
		return
	}

	tenantID := h.getTenantID(r.Context())
	if tenantID == "" {
		respondError(w, http.StatusUnauthorized, "Missing tenant ID")
		return
	}

	tenantUUID, _ := uuid.Parse(tenantID)
	if err := h.projectRepo.Delete(r.Context(), projectID, tenantUUID); err != nil {
		h.logger.Error("Failed to delete project", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Failed to delete project")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *ProjectHandler) CreateSavedSearch(w http.ResponseWriter, r *http.Request) {
	var req CreateSavedSearchRequest
	if err := ParseJSONRequest(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	tenantID := h.getTenantID(r.Context())
	userID := h.getUserID(r.Context())
	if tenantID == "" || userID == "" {
		respondError(w, http.StatusUnauthorized, "Missing tenant or user ID")
		return
	}

	tenantUUID, _ := uuid.Parse(tenantID)
	userUUID, _ := uuid.Parse(userID)

	var projectUUID *uuid.UUID
	if req.ProjectID != nil && *req.ProjectID != "" {
		id, _ := uuid.Parse(*req.ProjectID)
		projectUUID = &id
	}

	var scoringProfileUUID *uuid.UUID
	if req.ScoringProfileID != nil && *req.ScoringProfileID != "" {
		id, _ := uuid.Parse(*req.ScoringProfileID)
		scoringProfileUUID = &id
	}

	var datasetVersionUUID *uuid.UUID
	if req.DatasetVersionID != nil && *req.DatasetVersionID != "" {
		id, _ := uuid.Parse(*req.DatasetVersionID)
		datasetVersionUUID = &id
	}

	search := &models.SavedSearch{
		SavedSearchID:    uuid.New(),
		TenantID:         tenantUUID,
		ProjectID:        projectUUID,
		Name:             req.Name,
		QueryJSON:        string(req.QueryJSON),
		ScoringProfileID: scoringProfileUUID,
		DatasetVersionID: datasetVersionUUID,
		CreatedBy:        userUUID,
		CreatedAt:        time.Now(),
	}

	if err := h.projectRepo.CreateSavedSearch(r.Context(), search); err != nil {
		h.logger.Error("Failed to create saved search", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Failed to create saved search")
		return
	}

	respondJSON(w, http.StatusCreated, search)
}

func (h *ProjectHandler) ListSavedSearches(w http.ResponseWriter, r *http.Request) {
	tenantID := h.getTenantID(r.Context())
	if tenantID == "" {
		respondError(w, http.StatusUnauthorized, "Missing tenant ID")
		return
	}

	tenantUUID, _ := uuid.Parse(tenantID)
	projectIDStr := r.URL.Query().Get("projectId")
	var projectUUID *uuid.UUID
	if projectIDStr != "" {
		id, _ := uuid.Parse(projectIDStr)
		projectUUID = &id
	}

	searches, err := h.projectRepo.ListSavedSearches(r.Context(), tenantUUID, projectUUID)
	if err != nil {
		h.logger.Error("Failed to list saved searches", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Failed to list saved searches")
		return
	}

	respondJSON(w, http.StatusOK, searches)
}

func (h *ProjectHandler) GetSavedSearch(w http.ResponseWriter, r *http.Request) {
	savedSearchIDStr := chi.URLParam(r, "savedSearchId")
	savedSearchID, err := uuid.Parse(savedSearchIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid saved search ID")
		return
	}

	tenantID := h.getTenantID(r.Context())
	if tenantID == "" {
		respondError(w, http.StatusUnauthorized, "Missing tenant ID")
		return
	}

	tenantUUID, _ := uuid.Parse(tenantID)
	search, err := h.projectRepo.GetSavedSearchByID(r.Context(), savedSearchID, tenantUUID)
	if err != nil {
		respondError(w, http.StatusNotFound, "Saved search not found")
		return
	}

	respondJSON(w, http.StatusOK, search)
}

func (h *ProjectHandler) RunSavedSearch(w http.ResponseWriter, r *http.Request) {
	savedSearchIDStr := chi.URLParam(r, "savedSearchId")
	savedSearchID, err := uuid.Parse(savedSearchIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid saved search ID")
		return
	}

	tenantID := h.getTenantID(r.Context())
	if tenantID == "" {
		respondError(w, http.StatusUnauthorized, "Missing tenant ID")
		return
	}

	tenantUUID, _ := uuid.Parse(tenantID)
	savedSearch, err := h.projectRepo.GetSavedSearchByID(r.Context(), savedSearchID, tenantUUID)
	if err != nil {
		respondError(w, http.StatusNotFound, "Saved search not found")
		return
	}

	// Parse the saved query
	query, err := search.ParseSearchQuery([]byte(savedSearch.QueryJSON))
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid saved search query")
		return
	}

	// Perform search
	var scoringProfileUUID *uuid.UUID
	if savedSearch.ScoringProfileID != nil {
		scoringProfileUUID = savedSearch.ScoringProfileID
	}

	results, err := h.searchService.Search(r.Context(), query, &tenantUUID, scoringProfileUUID)
	if err != nil {
		h.logger.Error("Failed to run saved search", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Failed to run saved search")
		return
	}

	respondJSON(w, http.StatusOK, results)
}

func (h *ProjectHandler) UpdateSavedSearch(w http.ResponseWriter, r *http.Request) {
	savedSearchIDStr := chi.URLParam(r, "savedSearchId")
	savedSearchID, err := uuid.Parse(savedSearchIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid saved search ID")
		return
	}

	var req UpdateSavedSearchRequest
	if err := ParseJSONRequest(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	tenantID := h.getTenantID(r.Context())
	if tenantID == "" {
		respondError(w, http.StatusUnauthorized, "Missing tenant ID")
		return
	}

	tenantUUID, _ := uuid.Parse(tenantID)

	// Get existing saved search
	existing, err := h.projectRepo.GetSavedSearchByID(r.Context(), savedSearchID, tenantUUID)
	if err != nil {
		respondError(w, http.StatusNotFound, "Saved search not found")
		return
	}

	// Update fields
	existing.Name = req.Name
	if len(req.QueryJSON) > 0 {
		existing.QueryJSON = string(req.QueryJSON)
	}
	if req.ProjectID != nil {
		projectUUID, err := uuid.Parse(*req.ProjectID)
		if err != nil {
			respondError(w, http.StatusBadRequest, "Invalid project ID")
			return
		}
		existing.ProjectID = &projectUUID
	} else {
		existing.ProjectID = nil
	}
	if req.ScoringProfileID != nil {
		profileUUID, err := uuid.Parse(*req.ScoringProfileID)
		if err != nil {
			respondError(w, http.StatusBadRequest, "Invalid scoring profile ID")
			return
		}
		existing.ScoringProfileID = &profileUUID
	} else {
		existing.ScoringProfileID = nil
	}
	if req.DatasetVersionID != nil {
		versionUUID, err := uuid.Parse(*req.DatasetVersionID)
		if err != nil {
			respondError(w, http.StatusBadRequest, "Invalid dataset version ID")
			return
		}
		existing.DatasetVersionID = &versionUUID
	} else {
		existing.DatasetVersionID = nil
	}

	if err := h.projectRepo.UpdateSavedSearch(r.Context(), existing); err != nil {
		h.logger.Error("Failed to update saved search", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Failed to update saved search")
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"savedSearchId":    existing.SavedSearchID.String(),
		"name":             existing.Name,
		"projectId":        existing.ProjectID,
		"scoringProfileId": existing.ScoringProfileID,
		"datasetVersionId": existing.DatasetVersionID,
	})
}

func (h *ProjectHandler) DeleteSavedSearch(w http.ResponseWriter, r *http.Request) {
	savedSearchIDStr := chi.URLParam(r, "savedSearchId")
	savedSearchID, err := uuid.Parse(savedSearchIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid saved search ID")
		return
	}

	tenantID := h.getTenantID(r.Context())
	if tenantID == "" {
		respondError(w, http.StatusUnauthorized, "Missing tenant ID")
		return
	}

	tenantUUID, _ := uuid.Parse(tenantID)
	if err := h.projectRepo.DeleteSavedSearch(r.Context(), savedSearchID, tenantUUID); err != nil {
		h.logger.Error("Failed to delete saved search", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Failed to delete saved search")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ExportHandler handles export and job requests
type ExportHandler struct {
	*Handler
}

func NewExportHandler(db *database.DB, logger *zap.Logger, cfg *config.Config) *ExportHandler {
	return &ExportHandler{Handler: NewHandler(db, logger, cfg)}
}

func (h *ExportHandler) CreateExport(w http.ResponseWriter, r *http.Request) {
	var req CreateExportRequest
	if err := ParseJSONRequest(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	tenantID := h.getTenantID(r.Context())
	if tenantID == "" {
		respondError(w, http.StatusUnauthorized, "Missing tenant ID")
		return
	}

	tenantUUID, _ := uuid.Parse(tenantID)

	// Create job
	inputJSON, _ := json.Marshal(req)
	job := &models.Job{
		JobID:     uuid.New(),
		TenantID:  tenantUUID,
		Type:      "export",
		Status:    "pending",
		InputJSON: string(inputJSON),
		Attempts:  0,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := h.jobRepo.Create(r.Context(), job); err != nil {
		h.logger.Error("Failed to create export job", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Failed to create export job")
		return
	}

	respondJSON(w, http.StatusAccepted, JobResponse{
		JobID:     job.JobID.String(),
		Type:      job.Type,
		Status:    job.Status,
		Attempts:  job.Attempts,
		CreatedAt: job.CreatedAt.Format(time.RFC3339),
		UpdatedAt: job.UpdatedAt.Format(time.RFC3339),
	})
}

func (h *ExportHandler) GetExport(w http.ResponseWriter, r *http.Request) {
	exportIDStr := chi.URLParam(r, "exportId")
	exportID, err := uuid.Parse(exportIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid export ID")
		return
	}

	tenantID := h.getTenantID(r.Context())
	if tenantID == "" {
		respondError(w, http.StatusUnauthorized, "Missing tenant ID")
		return
	}

	tenantUUID, _ := uuid.Parse(tenantID)
	export, err := h.exportRepo.GetByID(r.Context(), exportID, tenantUUID)
	if err != nil {
		respondError(w, http.StatusNotFound, "Export not found")
		return
	}

	response := ExportResponse{
		ExportID:    export.ExportID.String(),
		JobID:       export.JobID.String(),
		Kind:        export.Kind,
		ObjectKey:   export.ObjectKey,
		ContentType: export.ContentType,
		CreatedAt:   export.CreatedAt.Format(time.RFC3339),
	}

	// Generate presigned URL if storage is available
	if h.tileStorage != nil {
		url, err := h.tileStorage.GetExportURL(r.Context(), tenantID, export.ExportID.String(), export.Kind, 1*time.Hour)
		if err == nil {
			response.URL = url
		}
	}

	respondJSON(w, http.StatusOK, response)
}

func (h *ExportHandler) ListExports(w http.ResponseWriter, r *http.Request) {
	tenantID := h.getTenantID(r.Context())
	if tenantID == "" {
		respondError(w, http.StatusUnauthorized, "Missing tenant ID")
		return
	}

	tenantUUID, _ := uuid.Parse(tenantID)

	limit := 50
	offset := 0
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	exports, err := h.exportRepo.ListByTenant(r.Context(), tenantUUID, limit, offset)
	if err != nil {
		h.logger.Error("Failed to list exports", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Failed to list exports")
		return
	}

	responses := make([]ExportResponse, len(exports))
	for i, export := range exports {
		response := ExportResponse{
			ExportID:    export.ExportID.String(),
			JobID:       export.JobID.String(),
			Kind:        export.Kind,
			ObjectKey:   export.ObjectKey,
			ContentType: export.ContentType,
			CreatedAt:   export.CreatedAt.Format(time.RFC3339),
		}

		// Generate presigned URL if storage is available
		if h.tileStorage != nil {
			url, err := h.tileStorage.GetExportURL(r.Context(), tenantID, export.ExportID.String(), export.Kind, 1*time.Hour)
			if err == nil {
				response.URL = url
			}
		}

		responses[i] = response
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"exports": responses,
		"limit":   limit,
		"offset":  offset,
	})
}

func (h *ExportHandler) GetJob(w http.ResponseWriter, r *http.Request) {
	jobIDStr := chi.URLParam(r, "jobId")
	jobID, err := uuid.Parse(jobIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid job ID")
		return
	}

	tenantID := h.getTenantID(r.Context())
	if tenantID == "" {
		respondError(w, http.StatusUnauthorized, "Missing tenant ID")
		return
	}

	tenantUUID, _ := uuid.Parse(tenantID)
	job, err := h.jobRepo.GetByID(r.Context(), jobID, tenantUUID)
	if err != nil {
		respondError(w, http.StatusNotFound, "Job not found")
		return
	}

	var input, output, errorData interface{}
	if job.InputJSON != "" {
		json.Unmarshal([]byte(job.InputJSON), &input)
	}
	if job.OutputJSON != "" {
		json.Unmarshal([]byte(job.OutputJSON), &output)
	}
	if job.ErrorJSON != "" {
		json.Unmarshal([]byte(job.ErrorJSON), &errorData)
	}

	respondJSON(w, http.StatusOK, JobResponse{
		JobID:     job.JobID.String(),
		Type:      job.Type,
		Status:    job.Status,
		Input:     input,
		Output:    output,
		Error:     errorData,
		Attempts:  job.Attempts,
		CreatedAt: job.CreatedAt.Format(time.RFC3339),
		UpdatedAt: job.UpdatedAt.Format(time.RFC3339),
	})
}

func (h *ExportHandler) ListJobs(w http.ResponseWriter, r *http.Request) {
	tenantID := h.getTenantID(r.Context())
	if tenantID == "" {
		respondError(w, http.StatusUnauthorized, "Missing tenant ID")
		return
	}

	tenantUUID, _ := uuid.Parse(tenantID)

	limit := 50
	offset := 0
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	jobs, err := h.jobRepo.ListByTenant(r.Context(), tenantUUID, limit, offset)
	if err != nil {
		h.logger.Error("Failed to list jobs", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Failed to list jobs")
		return
	}

	responses := make([]JobResponse, len(jobs))
	for i, job := range jobs {
		var input, output, errorData interface{}
		if job.InputJSON != "" {
			json.Unmarshal([]byte(job.InputJSON), &input)
		}
		if job.OutputJSON != "" {
			json.Unmarshal([]byte(job.OutputJSON), &output)
		}
		if job.ErrorJSON != "" {
			json.Unmarshal([]byte(job.ErrorJSON), &errorData)
		}

		responses[i] = JobResponse{
			JobID:     job.JobID.String(),
			Type:      job.Type,
			Status:    job.Status,
			Input:     input,
			Output:    output,
			Error:     errorData,
			Attempts:  job.Attempts,
			CreatedAt: job.CreatedAt.Format(time.RFC3339),
			UpdatedAt: job.UpdatedAt.Format(time.RFC3339),
		}
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"jobs":   responses,
		"limit":  limit,
		"offset": offset,
	})
}

// TileHandler handles tile requests
type TileHandler struct {
	*Handler
}

func NewTileHandler(db *database.DB, logger *zap.Logger, cfg *config.Config) *TileHandler {
	return &TileHandler{Handler: NewHandler(db, logger, cfg)}
}

func (h *TileHandler) GetTile(w http.ResponseWriter, r *http.Request) {
	layer := chi.URLParam(r, "layer")
	zStr := chi.URLParam(r, "z")
	xStr := chi.URLParam(r, "x")
	yStr := chi.URLParam(r, "y")

	z, err := strconv.Atoi(zStr)
	if err != nil || z < 0 || z > 20 {
		respondError(w, http.StatusBadRequest, "Invalid zoom level")
		return
	}

	x, err := strconv.Atoi(xStr)
	if err != nil || x < 0 {
		respondError(w, http.StatusBadRequest, "Invalid x coordinate")
		return
	}

	y, err := strconv.Atoi(yStr)
	if err != nil || y < 0 {
		respondError(w, http.StatusBadRequest, "Invalid y coordinate")
		return
	}

	// Get tenant ID for private layers
	tenantID := h.getTenantID(r.Context())
	var tenantIDPtr *string
	if tenantID != "" {
		tenantIDPtr = &tenantID
	}

	// Try to get tile from storage
	if h.tileStorage != nil {
		exists, err := h.tileStorage.TileExists(r.Context(), layer, z, x, y, tenantIDPtr)
		if err == nil && exists {
			// Generate presigned URL
			url, err := h.tileStorage.GetTileURL(r.Context(), layer, z, x, y, tenantIDPtr, 1*time.Hour)
			if err == nil {
				http.Redirect(w, r, url, http.StatusTemporaryRedirect)
				return
			}
		}
	}

	// Tile not found
	http.NotFound(w, r)
}

// Helper functions
func (h *Handler) getTenantID(ctx context.Context) string {
	return auth.GetTenantID(ctx)
}

func (h *Handler) getUserID(ctx context.Context) string {
	return auth.GetUserID(ctx)
}

func readBody(r *http.Request) ([]byte, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	return body, nil
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, ErrorResponse{Error: message})
}
