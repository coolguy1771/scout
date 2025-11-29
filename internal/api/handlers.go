package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
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
	favoriteRepo       *repository.FavoriteRepository
	userRepo           *repository.UserRepository
	tenantRepo         *repository.TenantRepository
	membershipRepo     *repository.MembershipRepository
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
	favoriteRepo := repository.NewFavoriteRepository(db.DB, logger)
	userRepo := repository.NewUserRepository(db.DB, logger)
	tenantRepo := repository.NewTenantRepository(db.DB, logger)
	membershipRepo := repository.NewMembershipRepository(db.DB, logger)

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
		favoriteRepo:       favoriteRepo,
		userRepo:           userRepo,
		tenantRepo:         tenantRepo,
		membershipRepo:     membershipRepo,
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

	// Check if parcel is favorited by current user (optional)
	userID := h.getUserID(r.Context())
	if userID != "" && tenantID != "" {
		userUUID, err := uuid.Parse(userID)
		if err == nil {
			tenantUUID, err := uuid.Parse(tenantID)
			if err == nil {
				exists, err := h.favoriteRepo.Exists(r.Context(), userUUID, parcelID, tenantUUID)
				if err == nil {
					response.IsFavorited = &exists
				}
			}
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

	// Validate request
	if err := ValidateRequest(&req); err != nil {
		errors := GetValidationErrors(err)
		respondJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error":   "Validation failed",
			"details": errors,
		})
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

	// Validate request
	if err := ValidateRequest(&req); err != nil {
		errors := GetValidationErrors(err)
		respondJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error":   "Validation failed",
			"details": errors,
		})
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

	// Validate request
	if err := ValidateRequest(&req); err != nil {
		errors := GetValidationErrors(err)
		respondJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error":   "Validation failed",
			"details": errors,
		})
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

	// Validate request
	if err := ValidateRequest(&req); err != nil {
		errors := GetValidationErrors(err)
		respondJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error":   "Validation failed",
			"details": errors,
		})
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

	// Validate request
	if err := ValidateRequest(&req); err != nil {
		errors := GetValidationErrors(err)
		respondJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error":   "Validation failed",
			"details": errors,
		})
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

// ScoringProfileHandler handles scoring profile requests
type ScoringProfileHandler struct {
	*Handler
}

func NewScoringProfileHandler(db *database.DB, logger *zap.Logger, cfg *config.Config) *ScoringProfileHandler {
	return &ScoringProfileHandler{Handler: NewHandler(db, logger, cfg)}
}

// CreateScoringProfile creates a new scoring profile
// @Summary Create scoring profile
// @Description Create a new scoring profile for the tenant
// @Tags scoringProfiles
// @Accept json
// @Produce json
// @Param profile body CreateScoringProfileRequest true "Scoring profile information"
// @Success 201 {object} ScoringProfileResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/scoringProfiles [post]
func (h *ScoringProfileHandler) CreateScoringProfile(w http.ResponseWriter, r *http.Request) {
	// Check RBAC - only analyst and admin can create
	role := auth.GetRole(r.Context())
	if role != "analyst" && role != "admin" {
		respondError(w, http.StatusForbidden, "Only analysts and admins can create scoring profiles")
		return
	}

	var req CreateScoringProfileRequest
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

	// Validate weights JSON structure
	if _, err := scoring.ParseProfile(string(req.WeightsJSON)); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid weights JSON: %v", err))
		return
	}

	// Validate thresholds JSON if provided
	if len(req.ThresholdsJSON) > 0 {
		var thresholds map[string]interface{}
		if err := json.Unmarshal(req.ThresholdsJSON, &thresholds); err != nil {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid thresholds JSON: %v", err))
			return
		}
	}

	// Validate hard constraints JSON if provided
	if len(req.HardConstraintsJSON) > 0 {
		var constraints map[string]interface{}
		if err := json.Unmarshal(req.HardConstraintsJSON, &constraints); err != nil {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid hard constraints JSON: %v", err))
			return
		}
	}

	tenantID := h.getTenantID(r.Context())
	userID := h.getUserID(r.Context())
	if tenantID == "" || userID == "" {
		respondError(w, http.StatusUnauthorized, "Missing tenant or user ID")
		return
	}

	tenantUUID, _ := uuid.Parse(tenantID)
	userUUID, _ := uuid.Parse(userID)

	version := 1
	if req.Version != nil {
		version = *req.Version
	}

	profile := &models.ScoringProfile{
		ScoringProfileID:    uuid.New(),
		TenantID:            tenantUUID,
		Name:                req.Name,
		Version:             version,
		WeightsJSON:         string(req.WeightsJSON),
		ThresholdsJSON:      string(req.ThresholdsJSON),
		HardConstraintsJSON: string(req.HardConstraintsJSON),
		CreatedBy:           userUUID,
		CreatedAt:           time.Now(),
	}

	if err := h.scoringProfileRepo.Create(r.Context(), profile); err != nil {
		h.logger.Error("Failed to create scoring profile", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Failed to create scoring profile")
		return
	}

	// Parse JSON fields for response
	var weights map[string]float64
	json.Unmarshal(req.WeightsJSON, &weights)

	var thresholds map[string]interface{}
	if len(req.ThresholdsJSON) > 0 {
		json.Unmarshal(req.ThresholdsJSON, &thresholds)
	}

	var constraints map[string]interface{}
	if len(req.HardConstraintsJSON) > 0 {
		json.Unmarshal(req.HardConstraintsJSON, &constraints)
	}

	response := ScoringProfileResponse{
		ScoringProfileID: profile.ScoringProfileID.String(),
		TenantID:         profile.TenantID.String(),
		Name:             profile.Name,
		Version:          profile.Version,
		Weights:          weights,
		Thresholds:       thresholds,
		HardConstraints:  constraints,
		CreatedBy:        profile.CreatedBy.String(),
		CreatedAt:        profile.CreatedAt.Format(time.RFC3339),
	}

	respondJSON(w, http.StatusCreated, response)
}

// ListScoringProfiles lists all scoring profiles for the tenant
// @Summary List scoring profiles
// @Description List all scoring profiles for the tenant
// @Tags scoringProfiles
// @Accept json
// @Produce json
// @Success 200 {array} ScoringProfileResponse
// @Failure 401 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/scoringProfiles [get]
func (h *ScoringProfileHandler) ListScoringProfiles(w http.ResponseWriter, r *http.Request) {
	tenantID := h.getTenantID(r.Context())
	if tenantID == "" {
		respondError(w, http.StatusUnauthorized, "Missing tenant ID")
		return
	}

	tenantUUID, _ := uuid.Parse(tenantID)
	profiles, err := h.scoringProfileRepo.ListByTenant(r.Context(), tenantUUID)
	if err != nil {
		h.logger.Error("Failed to list scoring profiles", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Failed to list scoring profiles")
		return
	}

	// Ensure we return an empty array instead of null
	if profiles == nil {
		profiles = []models.ScoringProfile{}
	}

	responses := make([]ScoringProfileResponse, len(profiles))
	for i, profile := range profiles {
		var weights map[string]float64
		json.Unmarshal([]byte(profile.WeightsJSON), &weights)

		var thresholds map[string]interface{}
		if profile.ThresholdsJSON != "" {
			json.Unmarshal([]byte(profile.ThresholdsJSON), &thresholds)
		}

		var constraints map[string]interface{}
		if profile.HardConstraintsJSON != "" {
			json.Unmarshal([]byte(profile.HardConstraintsJSON), &constraints)
		}

		responses[i] = ScoringProfileResponse{
			ScoringProfileID: profile.ScoringProfileID.String(),
			TenantID:         profile.TenantID.String(),
			Name:             profile.Name,
			Version:          profile.Version,
			Weights:          weights,
			Thresholds:       thresholds,
			HardConstraints:  constraints,
			CreatedBy:        profile.CreatedBy.String(),
			CreatedAt:        profile.CreatedAt.Format(time.RFC3339),
		}
	}

	respondJSON(w, http.StatusOK, responses)
}

// GetScoringProfile gets a scoring profile by ID
// @Summary Get scoring profile
// @Description Get a scoring profile by ID
// @Tags scoringProfiles
// @Accept json
// @Produce json
// @Param scoringProfileId path string true "Scoring Profile ID" format(uuid)
// @Success 200 {object} ScoringProfileResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/scoringProfiles/{scoringProfileId} [get]
func (h *ScoringProfileHandler) GetScoringProfile(w http.ResponseWriter, r *http.Request) {
	scoringProfileIDStr := chi.URLParam(r, "scoringProfileId")
	scoringProfileID, err := uuid.Parse(scoringProfileIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid scoring profile ID")
		return
	}

	tenantID := h.getTenantID(r.Context())
	if tenantID == "" {
		respondError(w, http.StatusUnauthorized, "Missing tenant ID")
		return
	}

	tenantUUID, _ := uuid.Parse(tenantID)
	profile, err := h.scoringProfileRepo.GetByID(r.Context(), scoringProfileID, tenantUUID)
	if err != nil {
		respondError(w, http.StatusNotFound, "Scoring profile not found")
		return
	}

	// Parse JSON fields for response
	var weights map[string]float64
	json.Unmarshal([]byte(profile.WeightsJSON), &weights)

	var thresholds map[string]interface{}
	if profile.ThresholdsJSON != "" {
		json.Unmarshal([]byte(profile.ThresholdsJSON), &thresholds)
	}

	var constraints map[string]interface{}
	if profile.HardConstraintsJSON != "" {
		json.Unmarshal([]byte(profile.HardConstraintsJSON), &constraints)
	}

	response := ScoringProfileResponse{
		ScoringProfileID: profile.ScoringProfileID.String(),
		TenantID:         profile.TenantID.String(),
		Name:             profile.Name,
		Version:          profile.Version,
		Weights:          weights,
		Thresholds:       thresholds,
		HardConstraints:  constraints,
		CreatedBy:        profile.CreatedBy.String(),
		CreatedAt:        profile.CreatedAt.Format(time.RFC3339),
	}

	respondJSON(w, http.StatusOK, response)
}

// UpdateScoringProfile updates a scoring profile
// @Summary Update scoring profile
// @Description Update a scoring profile
// @Tags scoringProfiles
// @Accept json
// @Produce json
// @Param scoringProfileId path string true "Scoring Profile ID" format(uuid)
// @Param profile body UpdateScoringProfileRequest true "Scoring profile information"
// @Success 200 {object} ScoringProfileResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/scoringProfiles/{scoringProfileId} [put]
func (h *ScoringProfileHandler) UpdateScoringProfile(w http.ResponseWriter, r *http.Request) {
	// Check RBAC - only analyst and admin can update
	role := auth.GetRole(r.Context())
	if role != "analyst" && role != "admin" {
		respondError(w, http.StatusForbidden, "Only analysts and admins can update scoring profiles")
		return
	}

	scoringProfileIDStr := chi.URLParam(r, "scoringProfileId")
	scoringProfileID, err := uuid.Parse(scoringProfileIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid scoring profile ID")
		return
	}

	var req UpdateScoringProfileRequest
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

	tenantID := h.getTenantID(r.Context())
	if tenantID == "" {
		respondError(w, http.StatusUnauthorized, "Missing tenant ID")
		return
	}

	tenantUUID, _ := uuid.Parse(tenantID)

	// Get existing profile
	existing, err := h.scoringProfileRepo.GetByID(r.Context(), scoringProfileID, tenantUUID)
	if err != nil {
		respondError(w, http.StatusNotFound, "Scoring profile not found")
		return
	}

	// Validate version increment if provided
	if req.Version != nil {
		if *req.Version <= existing.Version {
			respondError(w, http.StatusBadRequest, "Version must be greater than existing version")
			return
		}
		existing.Version = *req.Version
	}

	// Update fields
	existing.Name = req.Name

	// Validate and update weights JSON if provided
	if len(req.WeightsJSON) > 0 {
		if _, err := scoring.ParseProfile(string(req.WeightsJSON)); err != nil {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid weights JSON: %v", err))
			return
		}
		existing.WeightsJSON = string(req.WeightsJSON)
	}

	// Validate and update thresholds JSON if provided
	if len(req.ThresholdsJSON) > 0 {
		var thresholds map[string]interface{}
		if err := json.Unmarshal(req.ThresholdsJSON, &thresholds); err != nil {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid thresholds JSON: %v", err))
			return
		}
		existing.ThresholdsJSON = string(req.ThresholdsJSON)
	}

	// Validate and update hard constraints JSON if provided
	if len(req.HardConstraintsJSON) > 0 {
		var constraints map[string]interface{}
		if err := json.Unmarshal(req.HardConstraintsJSON, &constraints); err != nil {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid hard constraints JSON: %v", err))
			return
		}
		existing.HardConstraintsJSON = string(req.HardConstraintsJSON)
	}

	if err := h.scoringProfileRepo.Update(r.Context(), existing); err != nil {
		h.logger.Error("Failed to update scoring profile", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Failed to update scoring profile")
		return
	}

	// Parse JSON fields for response
	var weights map[string]float64
	json.Unmarshal([]byte(existing.WeightsJSON), &weights)

	var thresholds map[string]interface{}
	if existing.ThresholdsJSON != "" {
		json.Unmarshal([]byte(existing.ThresholdsJSON), &thresholds)
	}

	var constraints map[string]interface{}
	if existing.HardConstraintsJSON != "" {
		json.Unmarshal([]byte(existing.HardConstraintsJSON), &constraints)
	}

	response := ScoringProfileResponse{
		ScoringProfileID: existing.ScoringProfileID.String(),
		TenantID:         existing.TenantID.String(),
		Name:             existing.Name,
		Version:          existing.Version,
		Weights:          weights,
		Thresholds:       thresholds,
		HardConstraints:  constraints,
		CreatedBy:        existing.CreatedBy.String(),
		CreatedAt:        existing.CreatedAt.Format(time.RFC3339),
	}

	respondJSON(w, http.StatusOK, response)
}

// DeleteScoringProfile deletes a scoring profile
// @Summary Delete scoring profile
// @Description Delete a scoring profile
// @Tags scoringProfiles
// @Accept json
// @Produce json
// @Param scoringProfileId path string true "Scoring Profile ID" format(uuid)
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/scoringProfiles/{scoringProfileId} [delete]
func (h *ScoringProfileHandler) DeleteScoringProfile(w http.ResponseWriter, r *http.Request) {
	// Check RBAC - only analyst and admin can delete
	role := auth.GetRole(r.Context())
	if role != "analyst" && role != "admin" {
		respondError(w, http.StatusForbidden, "Only analysts and admins can delete scoring profiles")
		return
	}

	scoringProfileIDStr := chi.URLParam(r, "scoringProfileId")
	scoringProfileID, err := uuid.Parse(scoringProfileIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid scoring profile ID")
		return
	}

	tenantID := h.getTenantID(r.Context())
	if tenantID == "" {
		respondError(w, http.StatusUnauthorized, "Missing tenant ID")
		return
	}

	tenantUUID, _ := uuid.Parse(tenantID)
	if err := h.scoringProfileRepo.Delete(r.Context(), scoringProfileID, tenantUUID); err != nil {
		h.logger.Error("Failed to delete scoring profile", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Failed to delete scoring profile")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// FavoriteHandler handles favorite-related requests
type FavoriteHandler struct {
	*Handler
}

func NewFavoriteHandler(db *database.DB, logger *zap.Logger, cfg *config.Config) *FavoriteHandler {
	return &FavoriteHandler{Handler: NewHandler(db, logger, cfg)}
}

// CreateFavorite creates a new favorite
// @Summary Create favorite
// @Description Add a parcel to favorites
// @Tags favorites
// @Accept json
// @Produce json
// @Param favorite body CreateFavoriteRequest true "Favorite information"
// @Success 201 {object} FavoriteResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/favorites [post]
func (h *FavoriteHandler) CreateFavorite(w http.ResponseWriter, r *http.Request) {
	var req CreateFavoriteRequest
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

	tenantID := h.getTenantID(r.Context())
	userID := h.getUserID(r.Context())
	if tenantID == "" || userID == "" {
		respondError(w, http.StatusUnauthorized, "Missing tenant or user ID")
		return
	}

	tenantUUID, _ := uuid.Parse(tenantID)
	userUUID, _ := uuid.Parse(userID)
	parcelUUID, err := uuid.Parse(req.ParcelID)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid parcel ID")
		return
	}

	// Validate parcel exists and user has access
	var tenantUUIDPtr *uuid.UUID
	if tenantID != "" {
		tenantUUIDPtr = &tenantUUID
	}
	parcel, err := h.parcelRepo.GetByID(r.Context(), parcelUUID, tenantUUIDPtr)
	if err != nil {
		h.logger.Error("Failed to get parcel", zap.Error(err))
		respondError(w, http.StatusNotFound, "Parcel not found")
		return
	}

	// Check if favorite already exists
	exists, err := h.favoriteRepo.Exists(r.Context(), userUUID, parcelUUID, tenantUUID)
	if err != nil {
		h.logger.Error("Failed to check favorite existence", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Failed to check favorite")
		return
	}
	if exists {
		respondError(w, http.StatusConflict, "Parcel is already favorited")
		return
	}

	// Create favorite
	favorite := &models.Favorite{
		FavoriteID: uuid.New(),
		TenantID:   tenantUUID,
		UserID:     userUUID,
		ParcelID:   parcelUUID,
		CreatedAt:  time.Now(),
	}

	if err := h.favoriteRepo.Create(r.Context(), favorite); err != nil {
		// Check if it's a duplicate constraint error
		if strings.Contains(err.Error(), "favorite already exists") {
			respondError(w, http.StatusConflict, "Parcel is already favorited")
			return
		}
		h.logger.Error("Failed to create favorite", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Failed to create favorite")
		return
	}

	// Get parcel features for response
	features, _ := h.parcelFeatureRepo.GetByParcelID(r.Context(), parcelUUID)

	parcelResponse := ParcelResponse{
		ParcelID:     parcel.ParcelID.String(),
		APN:          parcel.APN,
		Acres:        parcel.Acres,
		ZoningRaw:    parcel.ZoningRaw,
		ZoningTags:   parcel.ZoningTags,
		Jurisdiction: parcel.Jurisdiction,
		StateFips:    parcel.StateFIPS,
	}

	if features != nil {
		parcelResponse.Features = &ParcelFeatureResponse{
			InFloodplain:    features.InFloodplain,
			InWetlands:      features.InWetlands,
			InProtectedLand: features.InProtectedLand,
		}
		if features.DistToHighwayM > 0 {
			parcelResponse.Features.DistToHighwayM = &features.DistToHighwayM
		}
		if features.DistToRailM > 0 {
			parcelResponse.Features.DistToRailM = &features.DistToRailM
		}
		if features.DistToAirportM > 0 {
			parcelResponse.Features.DistToAirportM = &features.DistToAirportM
		}
		if features.DistToPowerLineM > 0 {
			parcelResponse.Features.DistToPowerLineM = &features.DistToPowerLineM
		}
		if features.DistToSubstationM > 0 {
			parcelResponse.Features.DistToSubstationM = &features.DistToSubstationM
		}
	}

	response := FavoriteResponse{
		FavoriteID: favorite.FavoriteID.String(),
		ParcelID:   favorite.ParcelID.String(),
		Parcel:     parcelResponse,
		CreatedAt:  favorite.CreatedAt.Format(time.RFC3339),
	}

	respondJSON(w, http.StatusCreated, response)
}

// ListFavorites lists all favorites for the current user
// @Summary List favorites
// @Description List all favorites for the current user with pagination
// @Tags favorites
// @Accept json
// @Produce json
// @Param limit query int false "Limit" default(50)
// @Param offset query int false "Offset" default(0)
// @Success 200 {object} ListFavoritesResponse
// @Failure 401 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/favorites [get]
func (h *FavoriteHandler) ListFavorites(w http.ResponseWriter, r *http.Request) {
	tenantID := h.getTenantID(r.Context())
	userID := h.getUserID(r.Context())
	if tenantID == "" || userID == "" {
		respondError(w, http.StatusUnauthorized, "Missing tenant or user ID")
		return
	}

	tenantUUID, _ := uuid.Parse(tenantID)
	userUUID, _ := uuid.Parse(userID)

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

	favorites, err := h.favoriteRepo.ListByUser(r.Context(), userUUID, tenantUUID, limit, offset)
	if err != nil {
		h.logger.Error("Failed to list favorites", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Failed to list favorites")
		return
	}

	// Get total count
	total, err := h.favoriteRepo.CountByUser(r.Context(), userUUID, tenantUUID)
	if err != nil {
		h.logger.Error("Failed to count favorites", zap.Error(err))
		// Continue without total count
		total = len(favorites)
	}

	// Build response with parcel details
	responses := make([]FavoriteResponse, 0, len(favorites))
	for _, favorite := range favorites {
		// Get parcel details
		var tenantUUIDPtr *uuid.UUID
		if tenantID != "" {
			tenantUUIDPtr = &tenantUUID
		}
		parcel, err := h.parcelRepo.GetByID(r.Context(), favorite.ParcelID, tenantUUIDPtr)
		if err != nil {
			h.logger.Warn("Failed to get parcel for favorite", zap.String("parcelId", favorite.ParcelID.String()), zap.Error(err))
			continue
		}

		// Get parcel features
		features, _ := h.parcelFeatureRepo.GetByParcelID(r.Context(), favorite.ParcelID)

		parcelResponse := ParcelResponse{
			ParcelID:     parcel.ParcelID.String(),
			APN:          parcel.APN,
			Acres:        parcel.Acres,
			ZoningRaw:    parcel.ZoningRaw,
			ZoningTags:   parcel.ZoningTags,
			Jurisdiction: parcel.Jurisdiction,
			StateFips:    parcel.StateFIPS,
		}

		if features != nil {
			parcelResponse.Features = &ParcelFeatureResponse{
				InFloodplain:    features.InFloodplain,
				InWetlands:      features.InWetlands,
				InProtectedLand: features.InProtectedLand,
			}
			if features.DistToHighwayM > 0 {
				parcelResponse.Features.DistToHighwayM = &features.DistToHighwayM
			}
			if features.DistToRailM > 0 {
				parcelResponse.Features.DistToRailM = &features.DistToRailM
			}
			if features.DistToAirportM > 0 {
				parcelResponse.Features.DistToAirportM = &features.DistToAirportM
			}
			if features.DistToPowerLineM > 0 {
				parcelResponse.Features.DistToPowerLineM = &features.DistToPowerLineM
			}
			if features.DistToSubstationM > 0 {
				parcelResponse.Features.DistToSubstationM = &features.DistToSubstationM
			}
		}

		responses = append(responses, FavoriteResponse{
			FavoriteID: favorite.FavoriteID.String(),
			ParcelID:   favorite.ParcelID.String(),
			Parcel:     parcelResponse,
			CreatedAt:  favorite.CreatedAt.Format(time.RFC3339),
		})
	}

	respondJSON(w, http.StatusOK, ListFavoritesResponse{
		Favorites: responses,
		Total:     total,
		Limit:     limit,
		Offset:    offset,
	})
}

// GetFavorite gets a favorite by ID
// @Summary Get favorite
// @Description Get a favorite by ID
// @Tags favorites
// @Accept json
// @Produce json
// @Param favoriteId path string true "Favorite ID" format(uuid)
// @Success 200 {object} FavoriteResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/favorites/{favoriteId} [get]
func (h *FavoriteHandler) GetFavorite(w http.ResponseWriter, r *http.Request) {
	favoriteIDStr := chi.URLParam(r, "favoriteId")
	favoriteID, err := uuid.Parse(favoriteIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid favorite ID")
		return
	}

	tenantID := h.getTenantID(r.Context())
	if tenantID == "" {
		respondError(w, http.StatusUnauthorized, "Missing tenant ID")
		return
	}

	tenantUUID, _ := uuid.Parse(tenantID)
	favorite, err := h.favoriteRepo.GetByID(r.Context(), favoriteID, tenantUUID)
	if err != nil {
		h.logger.Error("Failed to get favorite", zap.Error(err))
		respondError(w, http.StatusNotFound, "Favorite not found")
		return
	}

	// Get parcel details
	var tenantUUIDPtr *uuid.UUID
	if tenantID != "" {
		tenantUUIDPtr = &tenantUUID
	}
	parcel, err := h.parcelRepo.GetByID(r.Context(), favorite.ParcelID, tenantUUIDPtr)
	if err != nil {
		h.logger.Error("Failed to get parcel", zap.Error(err))
		respondError(w, http.StatusNotFound, "Parcel not found")
		return
	}

	// Get parcel features
	features, _ := h.parcelFeatureRepo.GetByParcelID(r.Context(), favorite.ParcelID)

	parcelResponse := ParcelResponse{
		ParcelID:     parcel.ParcelID.String(),
		APN:          parcel.APN,
		Acres:        parcel.Acres,
		ZoningRaw:    parcel.ZoningRaw,
		ZoningTags:   parcel.ZoningTags,
		Jurisdiction: parcel.Jurisdiction,
		StateFips:    parcel.StateFIPS,
	}

	if features != nil {
		parcelResponse.Features = &ParcelFeatureResponse{
			InFloodplain:    features.InFloodplain,
			InWetlands:      features.InWetlands,
			InProtectedLand: features.InProtectedLand,
		}
		if features.DistToHighwayM > 0 {
			parcelResponse.Features.DistToHighwayM = &features.DistToHighwayM
		}
		if features.DistToRailM > 0 {
			parcelResponse.Features.DistToRailM = &features.DistToRailM
		}
		if features.DistToAirportM > 0 {
			parcelResponse.Features.DistToAirportM = &features.DistToAirportM
		}
		if features.DistToPowerLineM > 0 {
			parcelResponse.Features.DistToPowerLineM = &features.DistToPowerLineM
		}
		if features.DistToSubstationM > 0 {
			parcelResponse.Features.DistToSubstationM = &features.DistToSubstationM
		}
	}

	response := FavoriteResponse{
		FavoriteID: favorite.FavoriteID.String(),
		ParcelID:   favorite.ParcelID.String(),
		Parcel:     parcelResponse,
		CreatedAt:  favorite.CreatedAt.Format(time.RFC3339),
	}

	respondJSON(w, http.StatusOK, response)
}

// DeleteFavorite deletes a favorite
// @Summary Delete favorite
// @Description Remove a favorite
// @Tags favorites
// @Accept json
// @Produce json
// @Param favoriteId path string true "Favorite ID" format(uuid)
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/favorites/{favoriteId} [delete]
func (h *FavoriteHandler) DeleteFavorite(w http.ResponseWriter, r *http.Request) {
	favoriteIDStr := chi.URLParam(r, "favoriteId")
	favoriteID, err := uuid.Parse(favoriteIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid favorite ID")
		return
	}

	tenantID := h.getTenantID(r.Context())
	if tenantID == "" {
		respondError(w, http.StatusUnauthorized, "Missing tenant ID")
		return
	}

	tenantUUID, _ := uuid.Parse(tenantID)
	if err := h.favoriteRepo.Delete(r.Context(), favoriteID, tenantUUID); err != nil {
		h.logger.Error("Failed to delete favorite", zap.Error(err))
		if err.Error() == "favorite not found" {
			respondError(w, http.StatusNotFound, "Favorite not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "Failed to delete favorite")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// CheckParcelFavorite checks if a parcel is favorited by the current user
// @Summary Check parcel favorite
// @Description Check if the current user has favorited a parcel
// @Tags parcels
// @Accept json
// @Produce json
// @Param parcelId path string true "Parcel ID" format(uuid)
// @Success 200 {object} map[string]bool
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/parcels/{parcelId}/favorites [get]
func (h *ParcelHandler) CheckParcelFavorite(w http.ResponseWriter, r *http.Request) {
	parcelIDStr := chi.URLParam(r, "parcelId")
	parcelID, err := uuid.Parse(parcelIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid parcel ID")
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

	exists, err := h.favoriteRepo.Exists(r.Context(), userUUID, parcelID, tenantUUID)
	if err != nil {
		h.logger.Error("Failed to check favorite", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Failed to check favorite")
		return
	}

	respondJSON(w, http.StatusOK, map[string]bool{"isFavorited": exists})
}

// UserHandler handles user-related requests
type UserHandler struct {
	*Handler
}

func NewUserHandler(db *database.DB, logger *zap.Logger, cfg *config.Config) *UserHandler {
	return &UserHandler{Handler: NewHandler(db, logger, cfg)}
}

// CreateUser creates a new user (admin only)
// @Summary Create user
// @Description Create a new user (admin only)
// @Tags users
// @Accept json
// @Produce json
// @Param user body CreateUserRequest true "User information"
// @Success 201 {object} UserResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/users [post]
func (h *UserHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	// Check RBAC - only admin can create users
	role := auth.GetRole(r.Context())
	if role != "admin" {
		respondError(w, http.StatusForbidden, "Only admins can create users")
		return
	}

	var req CreateUserRequest
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

	// Check if user already exists
	existing, err := h.userRepo.GetByEmail(r.Context(), req.Email)
	if err == nil && existing != nil {
		respondError(w, http.StatusConflict, "User with email already exists")
		return
	}

	user := &models.User{
		UserID:    uuid.New(),
		Email:     req.Email,
		Name:      req.Name,
		CreatedAt: time.Now(),
	}

	if err := h.userRepo.Create(r.Context(), user); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			respondError(w, http.StatusConflict, "User with email already exists")
			return
		}
		h.logger.Error("Failed to create user", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Failed to create user")
		return
	}

	response := UserResponse{
		UserID:    user.UserID.String(),
		Email:     user.Email,
		Name:      user.Name,
		CreatedAt: user.CreatedAt.Format(time.RFC3339),
	}

	respondJSON(w, http.StatusCreated, response)
}

// ListUsers lists all users for the tenant (admin only)
// @Summary List users
// @Description List all users in the tenant (admin only)
// @Tags users
// @Accept json
// @Produce json
// @Param limit query int false "Limit" default(50)
// @Param offset query int false "Offset" default(0)
// @Success 200 {object} ListUsersResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/users [get]
func (h *UserHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	// Check RBAC - only admin can list users
	role := auth.GetRole(r.Context())
	if role != "admin" {
		respondError(w, http.StatusForbidden, "Only admins can list users")
		return
	}

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

	users, err := h.userRepo.List(r.Context(), tenantUUID, limit, offset)
	if err != nil {
		h.logger.Error("Failed to list users", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Failed to list users")
		return
	}

	responses := make([]UserResponse, len(users))
	for i, user := range users {
		responses[i] = UserResponse{
			UserID:    user.UserID.String(),
			Email:     user.Email,
			Name:      user.Name,
			CreatedAt: user.CreatedAt.Format(time.RFC3339),
		}
	}

	respondJSON(w, http.StatusOK, ListUsersResponse{
		Users:  responses,
		Limit:  limit,
		Offset: offset,
	})
}

// GetUser gets a user by ID
// @Summary Get user
// @Description Get a user by ID
// @Tags users
// @Accept json
// @Produce json
// @Param userId path string true "User ID" format(uuid)
// @Success 200 {object} UserResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/users/{userId} [get]
func (h *UserHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	userIDStr := chi.URLParam(r, "userId")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	// Users can view their own profile, admins can view any user
	currentUserID := h.getUserID(r.Context())
	role := auth.GetRole(r.Context())
	if currentUserID == "" {
		respondError(w, http.StatusUnauthorized, "Missing user ID")
		return
	}

	currentUserUUID, _ := uuid.Parse(currentUserID)
	if role != "admin" && currentUserUUID != userID {
		respondError(w, http.StatusForbidden, "You can only view your own profile")
		return
	}

	user, err := h.userRepo.GetByID(r.Context(), userID)
	if err != nil {
		h.logger.Error("Failed to get user", zap.Error(err))
		respondError(w, http.StatusNotFound, "User not found")
		return
	}

	response := UserResponse{
		UserID:    user.UserID.String(),
		Email:     user.Email,
		Name:      user.Name,
		CreatedAt: user.CreatedAt.Format(time.RFC3339),
	}

	respondJSON(w, http.StatusOK, response)
}

// GetCurrentUser gets the current user's profile
// @Summary Get current user
// @Description Get the current user's profile
// @Tags users
// @Accept json
// @Produce json
// @Success 200 {object} UserResponse
// @Failure 401 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/users/me [get]
func (h *UserHandler) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	userID := h.getUserID(r.Context())
	if userID == "" {
		respondError(w, http.StatusUnauthorized, "Missing user ID")
		return
	}

	userUUID, _ := uuid.Parse(userID)
	user, err := h.userRepo.GetByID(r.Context(), userUUID)
	if err != nil {
		h.logger.Error("Failed to get user", zap.Error(err))
		respondError(w, http.StatusNotFound, "User not found")
		return
	}

	response := UserResponse{
		UserID:    user.UserID.String(),
		Email:     user.Email,
		Name:      user.Name,
		CreatedAt: user.CreatedAt.Format(time.RFC3339),
	}

	respondJSON(w, http.StatusOK, response)
}

// UpdateUser updates a user
// @Summary Update user
// @Description Update a user (users can update their own profile, admins can update any user)
// @Tags users
// @Accept json
// @Produce json
// @Param userId path string true "User ID" format(uuid)
// @Param user body UpdateUserRequest true "User information"
// @Success 200 {object} UserResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/users/{userId} [put]
func (h *UserHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	userIDStr := chi.URLParam(r, "userId")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	var req UpdateUserRequest
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

	// Users can update their own profile, admins can update any user
	currentUserID := h.getUserID(r.Context())
	role := auth.GetRole(r.Context())
	if currentUserID == "" {
		respondError(w, http.StatusUnauthorized, "Missing user ID")
		return
	}

	currentUserUUID, _ := uuid.Parse(currentUserID)
	if role != "admin" && currentUserUUID != userID {
		respondError(w, http.StatusForbidden, "You can only update your own profile")
		return
	}

	// Get existing user
	user, err := h.userRepo.GetByID(r.Context(), userID)
	if err != nil {
		respondError(w, http.StatusNotFound, "User not found")
		return
	}

	// Check if email is being changed and if new email already exists
	if user.Email != req.Email {
		existing, err := h.userRepo.GetByEmail(r.Context(), req.Email)
		if err == nil && existing != nil && existing.UserID != userID {
			respondError(w, http.StatusConflict, "User with email already exists")
			return
		}
	}

	user.Email = req.Email
	user.Name = req.Name

	if err := h.userRepo.Update(r.Context(), user); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			respondError(w, http.StatusConflict, "User with email already exists")
			return
		}
		h.logger.Error("Failed to update user", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Failed to update user")
		return
	}

	response := UserResponse{
		UserID:    user.UserID.String(),
		Email:     user.Email,
		Name:      user.Name,
		CreatedAt: user.CreatedAt.Format(time.RFC3339),
	}

	respondJSON(w, http.StatusOK, response)
}

// UpdateCurrentUser updates the current user's profile
// @Summary Update current user
// @Description Update the current user's profile
// @Tags users
// @Accept json
// @Produce json
// @Param user body UpdateUserRequest true "User information"
// @Success 200 {object} UserResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/users/me [put]
func (h *UserHandler) UpdateCurrentUser(w http.ResponseWriter, r *http.Request) {
	userID := h.getUserID(r.Context())
	if userID == "" {
		respondError(w, http.StatusUnauthorized, "Missing user ID")
		return
	}

	userUUID, _ := uuid.Parse(userID)

	var req UpdateUserRequest
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

	// Get existing user
	user, err := h.userRepo.GetByID(r.Context(), userUUID)
	if err != nil {
		respondError(w, http.StatusNotFound, "User not found")
		return
	}

	// Check if email is being changed and if new email already exists
	if user.Email != req.Email {
		existing, err := h.userRepo.GetByEmail(r.Context(), req.Email)
		if err == nil && existing != nil && existing.UserID != userUUID {
			respondError(w, http.StatusConflict, "User with email already exists")
			return
		}
	}

	user.Email = req.Email
	user.Name = req.Name

	if err := h.userRepo.Update(r.Context(), user); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			respondError(w, http.StatusConflict, "User with email already exists")
			return
		}
		h.logger.Error("Failed to update user", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Failed to update user")
		return
	}

	response := UserResponse{
		UserID:    user.UserID.String(),
		Email:     user.Email,
		Name:      user.Name,
		CreatedAt: user.CreatedAt.Format(time.RFC3339),
	}

	respondJSON(w, http.StatusOK, response)
}

// DeleteUser deletes a user (admin only)
// @Summary Delete user
// @Description Delete a user (admin only)
// @Tags users
// @Accept json
// @Produce json
// @Param userId path string true "User ID" format(uuid)
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/users/{userId} [delete]
func (h *UserHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	// Check RBAC - only admin can delete users
	role := auth.GetRole(r.Context())
	if role != "admin" {
		respondError(w, http.StatusForbidden, "Only admins can delete users")
		return
	}

	userIDStr := chi.URLParam(r, "userId")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	if err := h.userRepo.Delete(r.Context(), userID); err != nil {
		h.logger.Error("Failed to delete user", zap.Error(err))
		if err.Error() == "user not found" {
			respondError(w, http.StatusNotFound, "User not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "Failed to delete user")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// TenantHandler handles tenant-related requests
type TenantHandler struct {
	*Handler
}

func NewTenantHandler(db *database.DB, logger *zap.Logger, cfg *config.Config) *TenantHandler {
	return &TenantHandler{Handler: NewHandler(db, logger, cfg)}
}

// CreateTenant creates a new tenant (super admin only - for now, admin can create)
// @Summary Create tenant
// @Description Create a new tenant (admin only)
// @Tags tenants
// @Accept json
// @Produce json
// @Param tenant body CreateTenantRequest true "Tenant information"
// @Success 201 {object} TenantResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/tenants [post]
func (h *TenantHandler) CreateTenant(w http.ResponseWriter, r *http.Request) {
	// Check RBAC - only admin can create tenants
	role := auth.GetRole(r.Context())
	if role != "admin" {
		respondError(w, http.StatusForbidden, "Only admins can create tenants")
		return
	}

	var req CreateTenantRequest
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

	tenant := &models.Tenant{
		TenantID:  uuid.New(),
		Name:      req.Name,
		CreatedAt: time.Now(),
	}

	if err := h.tenantRepo.Create(r.Context(), tenant); err != nil {
		h.logger.Error("Failed to create tenant", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Failed to create tenant")
		return
	}

	response := TenantResponse{
		TenantID:  tenant.TenantID.String(),
		Name:      tenant.Name,
		CreatedAt: tenant.CreatedAt.Format(time.RFC3339),
	}

	respondJSON(w, http.StatusCreated, response)
}

// ListTenants lists all tenants (admin only)
// @Summary List tenants
// @Description List all tenants (admin only)
// @Tags tenants
// @Accept json
// @Produce json
// @Param limit query int false "Limit" default(50)
// @Param offset query int false "Offset" default(0)
// @Success 200 {object} ListTenantsResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/tenants [get]
func (h *TenantHandler) ListTenants(w http.ResponseWriter, r *http.Request) {
	// Check RBAC - only admin can list tenants
	role := auth.GetRole(r.Context())
	if role != "admin" {
		respondError(w, http.StatusForbidden, "Only admins can list tenants")
		return
	}

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

	tenants, err := h.tenantRepo.List(r.Context(), limit, offset)
	if err != nil {
		h.logger.Error("Failed to list tenants", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Failed to list tenants")
		return
	}

	responses := make([]TenantResponse, len(tenants))
	for i, tenant := range tenants {
		responses[i] = TenantResponse{
			TenantID:  tenant.TenantID.String(),
			Name:      tenant.Name,
			CreatedAt: tenant.CreatedAt.Format(time.RFC3339),
		}
	}

	respondJSON(w, http.StatusOK, ListTenantsResponse{
		Tenants: responses,
		Limit:   limit,
		Offset:  offset,
	})
}

// GetTenant gets a tenant by ID
// @Summary Get tenant
// @Description Get a tenant by ID
// @Tags tenants
// @Accept json
// @Produce json
// @Param tenantId path string true "Tenant ID" format(uuid)
// @Success 200 {object} TenantResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/tenants/{tenantId} [get]
func (h *TenantHandler) GetTenant(w http.ResponseWriter, r *http.Request) {
	tenantIDStr := chi.URLParam(r, "tenantId")
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid tenant ID")
		return
	}

	// Users can view their own tenant, admins can view any tenant
	currentTenantID := h.getTenantID(r.Context())
	role := auth.GetRole(r.Context())
	if currentTenantID == "" {
		respondError(w, http.StatusUnauthorized, "Missing tenant ID")
		return
	}

	currentTenantUUID, _ := uuid.Parse(currentTenantID)
	if role != "admin" && currentTenantUUID != tenantID {
		respondError(w, http.StatusForbidden, "You can only view your own tenant")
		return
	}

	tenant, err := h.tenantRepo.GetByID(r.Context(), tenantID)
	if err != nil {
		h.logger.Error("Failed to get tenant", zap.Error(err))
		respondError(w, http.StatusNotFound, "Tenant not found")
		return
	}

	response := TenantResponse{
		TenantID:  tenant.TenantID.String(),
		Name:      tenant.Name,
		CreatedAt: tenant.CreatedAt.Format(time.RFC3339),
	}

	respondJSON(w, http.StatusOK, response)
}

// UpdateTenant updates a tenant (admin only)
// @Summary Update tenant
// @Description Update a tenant (admin only)
// @Tags tenants
// @Accept json
// @Produce json
// @Param tenantId path string true "Tenant ID" format(uuid)
// @Param tenant body UpdateTenantRequest true "Tenant information"
// @Success 200 {object} TenantResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/tenants/{tenantId} [put]
func (h *TenantHandler) UpdateTenant(w http.ResponseWriter, r *http.Request) {
	// Check RBAC - only admin can update tenants
	role := auth.GetRole(r.Context())
	if role != "admin" {
		respondError(w, http.StatusForbidden, "Only admins can update tenants")
		return
	}

	tenantIDStr := chi.URLParam(r, "tenantId")
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid tenant ID")
		return
	}

	var req UpdateTenantRequest
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

	tenant, err := h.tenantRepo.GetByID(r.Context(), tenantID)
	if err != nil {
		respondError(w, http.StatusNotFound, "Tenant not found")
		return
	}

	tenant.Name = req.Name

	if err := h.tenantRepo.Update(r.Context(), tenant); err != nil {
		h.logger.Error("Failed to update tenant", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Failed to update tenant")
		return
	}

	response := TenantResponse{
		TenantID:  tenant.TenantID.String(),
		Name:      tenant.Name,
		CreatedAt: tenant.CreatedAt.Format(time.RFC3339),
	}

	respondJSON(w, http.StatusOK, response)
}

// DeleteTenant deletes a tenant (admin only)
// @Summary Delete tenant
// @Description Delete a tenant (admin only)
// @Tags tenants
// @Accept json
// @Produce json
// @Param tenantId path string true "Tenant ID" format(uuid)
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/tenants/{tenantId} [delete]
func (h *TenantHandler) DeleteTenant(w http.ResponseWriter, r *http.Request) {
	// Check RBAC - only admin can delete tenants
	role := auth.GetRole(r.Context())
	if role != "admin" {
		respondError(w, http.StatusForbidden, "Only admins can delete tenants")
		return
	}

	tenantIDStr := chi.URLParam(r, "tenantId")
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid tenant ID")
		return
	}

	if err := h.tenantRepo.Delete(r.Context(), tenantID); err != nil {
		h.logger.Error("Failed to delete tenant", zap.Error(err))
		if err.Error() == "tenant not found" {
			respondError(w, http.StatusNotFound, "Tenant not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "Failed to delete tenant")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListTenantMembers lists all members of a tenant
// @Summary List tenant members
// @Description List all members of a tenant
// @Tags tenants
// @Accept json
// @Produce json
// @Param tenantId path string true "Tenant ID" format(uuid)
// @Param limit query int false "Limit" default(50)
// @Param offset query int false "Offset" default(0)
// @Success 200 {object} ListMembershipsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/tenants/{tenantId}/members [get]
func (h *TenantHandler) ListTenantMembers(w http.ResponseWriter, r *http.Request) {
	tenantIDStr := chi.URLParam(r, "tenantId")
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid tenant ID")
		return
	}

	// Verify user has access to this tenant
	currentTenantID := h.getTenantID(r.Context())
	if currentTenantID == "" {
		respondError(w, http.StatusUnauthorized, "Missing tenant ID")
		return
	}

	currentTenantUUID, _ := uuid.Parse(currentTenantID)
	role := auth.GetRole(r.Context())
	if role != "admin" && currentTenantUUID != tenantID {
		respondError(w, http.StatusForbidden, "You can only view members of your own tenant")
		return
	}

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

	memberships, err := h.tenantRepo.ListMembers(r.Context(), tenantID, limit, offset)
	if err != nil {
		h.logger.Error("Failed to list members", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Failed to list members")
		return
	}

	responses := make([]MembershipResponse, len(memberships))
	for i, membership := range memberships {
		responses[i] = MembershipResponse{
			MembershipID: membership.MembershipID.String(),
			TenantID:     membership.TenantID.String(),
			UserID:       membership.UserID.String(),
			Role:         membership.Role,
			CreatedAt:    membership.CreatedAt.Format(time.RFC3339),
		}
	}

	respondJSON(w, http.StatusOK, ListMembershipsResponse{
		Memberships: responses,
		Limit:       limit,
		Offset:      offset,
	})
}

// MembershipHandler handles membership-related requests
type MembershipHandler struct {
	*Handler
}

func NewMembershipHandler(db *database.DB, logger *zap.Logger, cfg *config.Config) *MembershipHandler {
	return &MembershipHandler{Handler: NewHandler(db, logger, cfg)}
}

// CreateMembership creates a new membership (admin only)
// @Summary Create membership
// @Description Add a user to a tenant with a role (admin only)
// @Tags memberships
// @Accept json
// @Produce json
// @Param membership body CreateMembershipRequest true "Membership information"
// @Success 201 {object} MembershipResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/memberships [post]
func (h *MembershipHandler) CreateMembership(w http.ResponseWriter, r *http.Request) {
	// Check RBAC - only admin can create memberships
	role := auth.GetRole(r.Context())
	if role != "admin" {
		respondError(w, http.StatusForbidden, "Only admins can create memberships")
		return
	}

	var req CreateMembershipRequest
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

	tenantID := h.getTenantID(r.Context())
	if tenantID == "" {
		respondError(w, http.StatusUnauthorized, "Missing tenant ID")
		return
	}

	tenantUUID, _ := uuid.Parse(tenantID)
	userUUID, err := uuid.Parse(req.UserID)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	// Verify user exists
	_, err = h.userRepo.GetByID(r.Context(), userUUID)
	if err != nil {
		respondError(w, http.StatusNotFound, "User not found")
		return
	}

	// Check if membership already exists
	existing, err := h.membershipRepo.GetByUserTenant(r.Context(), userUUID, tenantUUID)
	if err == nil && existing != nil {
		respondError(w, http.StatusConflict, "Membership already exists")
		return
	}

	membership := &models.Membership{
		MembershipID: uuid.New(),
		TenantID:     tenantUUID,
		UserID:       userUUID,
		Role:         req.Role,
		CreatedAt:    time.Now(),
	}

	if err := h.membershipRepo.Create(r.Context(), membership); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			respondError(w, http.StatusConflict, "Membership already exists")
			return
		}
		h.logger.Error("Failed to create membership", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Failed to create membership")
		return
	}

	response := MembershipResponse{
		MembershipID: membership.MembershipID.String(),
		TenantID:     membership.TenantID.String(),
		UserID:       membership.UserID.String(),
		Role:         membership.Role,
		CreatedAt:    membership.CreatedAt.Format(time.RFC3339),
	}

	respondJSON(w, http.StatusCreated, response)
}

// ListMemberships lists all memberships for the tenant
// @Summary List memberships
// @Description List all memberships for the tenant
// @Tags memberships
// @Accept json
// @Produce json
// @Param limit query int false "Limit" default(50)
// @Param offset query int false "Offset" default(0)
// @Success 200 {object} ListMembershipsResponse
// @Failure 401 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/memberships [get]
func (h *MembershipHandler) ListMemberships(w http.ResponseWriter, r *http.Request) {
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

	memberships, err := h.membershipRepo.ListByTenant(r.Context(), tenantUUID, limit, offset)
	if err != nil {
		h.logger.Error("Failed to list memberships", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Failed to list memberships")
		return
	}

	responses := make([]MembershipResponse, len(memberships))
	for i, membership := range memberships {
		responses[i] = MembershipResponse{
			MembershipID: membership.MembershipID.String(),
			TenantID:     membership.TenantID.String(),
			UserID:       membership.UserID.String(),
			Role:         membership.Role,
			CreatedAt:    membership.CreatedAt.Format(time.RFC3339),
		}
	}

	respondJSON(w, http.StatusOK, ListMembershipsResponse{
		Memberships: responses,
		Limit:       limit,
		Offset:      offset,
	})
}

// GetMembership gets a membership by ID
// @Summary Get membership
// @Description Get a membership by ID
// @Tags memberships
// @Accept json
// @Produce json
// @Param membershipId path string true "Membership ID" format(uuid)
// @Success 200 {object} MembershipResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/memberships/{membershipId} [get]
func (h *MembershipHandler) GetMembership(w http.ResponseWriter, r *http.Request) {
	membershipIDStr := chi.URLParam(r, "membershipId")
	membershipID, err := uuid.Parse(membershipIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid membership ID")
		return
	}

	tenantID := h.getTenantID(r.Context())
	if tenantID == "" {
		respondError(w, http.StatusUnauthorized, "Missing tenant ID")
		return
	}

	tenantUUID, _ := uuid.Parse(tenantID)
	membership, err := h.membershipRepo.GetByID(r.Context(), membershipID, tenantUUID)
	if err != nil {
		h.logger.Error("Failed to get membership", zap.Error(err))
		respondError(w, http.StatusNotFound, "Membership not found")
		return
	}

	response := MembershipResponse{
		MembershipID: membership.MembershipID.String(),
		TenantID:     membership.TenantID.String(),
		UserID:       membership.UserID.String(),
		Role:         membership.Role,
		CreatedAt:    membership.CreatedAt.Format(time.RFC3339),
	}

	respondJSON(w, http.StatusOK, response)
}

// UpdateMembership updates a membership role (admin only)
// @Summary Update membership
// @Description Update a membership role (admin only)
// @Tags memberships
// @Accept json
// @Produce json
// @Param membershipId path string true "Membership ID" format(uuid)
// @Param membership body UpdateMembershipRequest true "Membership information"
// @Success 200 {object} MembershipResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/memberships/{membershipId} [put]
func (h *MembershipHandler) UpdateMembership(w http.ResponseWriter, r *http.Request) {
	// Check RBAC - only admin can update memberships
	role := auth.GetRole(r.Context())
	if role != "admin" {
		respondError(w, http.StatusForbidden, "Only admins can update memberships")
		return
	}

	membershipIDStr := chi.URLParam(r, "membershipId")
	membershipID, err := uuid.Parse(membershipIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid membership ID")
		return
	}

	var req UpdateMembershipRequest
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

	tenantID := h.getTenantID(r.Context())
	if tenantID == "" {
		respondError(w, http.StatusUnauthorized, "Missing tenant ID")
		return
	}

	tenantUUID, _ := uuid.Parse(tenantID)
	membership, err := h.membershipRepo.GetByID(r.Context(), membershipID, tenantUUID)
	if err != nil {
		respondError(w, http.StatusNotFound, "Membership not found")
		return
	}

	membership.Role = req.Role

	if err := h.membershipRepo.Update(r.Context(), membership); err != nil {
		h.logger.Error("Failed to update membership", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "Failed to update membership")
		return
	}

	response := MembershipResponse{
		MembershipID: membership.MembershipID.String(),
		TenantID:     membership.TenantID.String(),
		UserID:       membership.UserID.String(),
		Role:         membership.Role,
		CreatedAt:    membership.CreatedAt.Format(time.RFC3339),
	}

	respondJSON(w, http.StatusOK, response)
}

// DeleteMembership deletes a membership (admin only)
// @Summary Delete membership
// @Description Remove a membership (admin only)
// @Tags memberships
// @Accept json
// @Produce json
// @Param membershipId path string true "Membership ID" format(uuid)
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/memberships/{membershipId} [delete]
func (h *MembershipHandler) DeleteMembership(w http.ResponseWriter, r *http.Request) {
	// Check RBAC - only admin can delete memberships
	role := auth.GetRole(r.Context())
	if role != "admin" {
		respondError(w, http.StatusForbidden, "Only admins can delete memberships")
		return
	}

	membershipIDStr := chi.URLParam(r, "membershipId")
	membershipID, err := uuid.Parse(membershipIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid membership ID")
		return
	}

	tenantID := h.getTenantID(r.Context())
	if tenantID == "" {
		respondError(w, http.StatusUnauthorized, "Missing tenant ID")
		return
	}

	tenantUUID, _ := uuid.Parse(tenantID)
	if err := h.membershipRepo.Delete(r.Context(), membershipID, tenantUUID); err != nil {
		h.logger.Error("Failed to delete membership", zap.Error(err))
		if err.Error() == "membership not found" {
			respondError(w, http.StatusNotFound, "Membership not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "Failed to delete membership")
		return
	}

	w.WriteHeader(http.StatusNoContent)
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
