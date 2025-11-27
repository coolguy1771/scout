package search

import (
	"context"
	"fmt"
	"math"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/coolguy1771/scout/internal/config"
	"github.com/coolguy1771/scout/internal/models"
	"github.com/coolguy1771/scout/internal/repository"
	"github.com/coolguy1771/scout/internal/scoring"
	opensearchsearch "github.com/coolguy1771/scout/internal/search/opensearch"
)

type SearchService struct {
	parcelRepo        *repository.ParcelRepository
	parcelFeatureRepo *repository.ParcelFeatureRepository
	scoringService    *scoring.Service
	config            *config.Config
	logger            *zap.Logger
	osSearchService   *opensearchsearch.SearchService
}

func NewSearchService(
	parcelRepo *repository.ParcelRepository,
	parcelFeatureRepo *repository.ParcelFeatureRepository,
	scoringService *scoring.Service,
	cfg *config.Config,
	logger *zap.Logger,
) *SearchService {
	service := &SearchService{
		parcelRepo:        parcelRepo,
		parcelFeatureRepo: parcelFeatureRepo,
		scoringService:    scoringService,
		config:            cfg,
		logger:            logger,
	}

	// Initialize OpenSearch if enabled
	if cfg.Search.Enabled {
		osClient, err := opensearchsearch.NewClient(&cfg.Search, logger)
		if err != nil {
			logger.Warn("Failed to initialize OpenSearch client, falling back to PostGIS", zap.Error(err))
		} else if osClient.IsEnabled() {
			indexer := opensearchsearch.NewParcelIndexer(osClient, logger)
			service.osSearchService = opensearchsearch.NewSearchService(indexer, logger)
			logger.Info("OpenSearch search service initialized")
		}
	}

	return service
}

// SearchResult represents a single search result
type SearchResult struct {
	ParcelID       string                 `json:"parcelId"`
	Score          float64                `json:"score"`
	ScoreBreakdown map[string]interface{} `json:"scoreBreakdown,omitempty"`
	ConstraintsHit []string               `json:"constraintsHit,omitempty"`
	Parcel         interface{}            `json:"parcel,omitempty"`
	Features       interface{}            `json:"features,omitempty"`
}

// SearchResponse represents the search response
type SearchResponse struct {
	Results    []SearchResult `json:"results"`
	Total      int            `json:"total"`
	Page       int            `json:"page"`
	PageSize   int            `json:"pageSize"`
	TotalPages int            `json:"totalPages"`
}

// Search performs a two-phase search
func (s *SearchService) Search(ctx context.Context, query *SearchQuery, tenantID *uuid.UUID, scoringProfileID *uuid.UUID) (*SearchResponse, error) {
	// Phase A: Fast candidate selection
	candidates, err := s.phaseA(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("phase A failed: %w", err)
	}

	s.logger.Info("Phase A completed", zap.Int("candidates", len(candidates)))

	if len(candidates) == 0 {
		return &SearchResponse{
			Results:    []SearchResult{},
			Total:      0,
			Page:       query.Page,
			PageSize:   query.PageSize,
			TotalPages: 0,
		}, nil
	}

	// Phase B: Precise refinement and scoring
	results, err := s.phaseB(ctx, candidates, query, tenantID, scoringProfileID)
	if err != nil {
		return nil, fmt.Errorf("phase B failed: %w", err)
	}

	// Pagination
	total := len(results)
	totalPages := int(math.Ceil(float64(total) / float64(query.PageSize)))
	start := (query.Page - 1) * query.PageSize
	end := start + query.PageSize

	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	var paginatedResults []SearchResult
	if start < end {
		paginatedResults = results[start:end]
	}

	return &SearchResponse{
		Results:    paginatedResults,
		Total:      total,
		Page:       query.Page,
		PageSize:   query.PageSize,
		TotalPages: totalPages,
	}, nil
}

// phaseA performs fast candidate selection
func (s *SearchService) phaseA(ctx context.Context, query *SearchQuery, tenantID *uuid.UUID) ([]uuid.UUID, error) {
	filters := query.ToFilters()
	maxCandidates := s.config.FeatureFlags.MaxSearchCandidates
	if maxCandidates <= 0 {
		maxCandidates = 50000
	}

	// Try OpenSearch first if enabled
	if s.osSearchService != nil {
		bbox := query.BBox
		if len(bbox) == 0 && len(query.AOI) > 0 {
			// Convert AOI polygon to bbox for OpenSearch
			bbox = polygonToBBox(query.AOI)
		}

		if len(bbox) > 0 {
			osQuery := &opensearchsearch.SearchQuery{
				BBox:          bbox,
				Filters:       filters,
				TenantID:      tenantID,
				MaxCandidates: maxCandidates,
			}

			candidates, err := s.osSearchService.SearchCandidates(ctx, osQuery)
			if err != nil {
				s.logger.Warn("OpenSearch query failed, falling back to PostGIS", zap.Error(err))
				// Fall through to PostGIS
			} else {
				s.logger.Info("Phase A completed using OpenSearch", zap.Int("candidates", len(candidates)))
				return candidates, nil
			}
		}
	}

	// Fallback to PostGIS
	var candidates []uuid.UUID
	var err error

	if len(query.BBox) > 0 {
		candidates, err = s.parcelRepo.SearchCandidates(ctx, query.BBox, filters, tenantID, maxCandidates)
		if err != nil {
			return nil, err
		}
	} else if len(query.AOI) > 0 {
		// Convert AOI polygon to bbox for initial filter
		// For MVP, we'll use bbox approximation
		bbox := polygonToBBox(query.AOI)
		candidates, err = s.parcelRepo.SearchCandidates(ctx, bbox, filters, tenantID, maxCandidates)
		if err != nil {
			return nil, err
		}
	}

	s.logger.Info("Phase A completed using PostGIS", zap.Int("candidates", len(candidates)))
	return candidates, nil
}

// phaseB performs precise refinement and scoring
func (s *SearchService) phaseB(ctx context.Context, candidates []uuid.UUID, query *SearchQuery, tenantID *uuid.UUID, scoringProfileID *uuid.UUID) ([]SearchResult, error) {
	// Fetch parcels
	parcels, err := s.parcelRepo.GetByIDs(ctx, candidates)
	if err != nil {
		return nil, err
	}

	// Fetch features
	parcelIDs := make([]uuid.UUID, len(parcels))
	for i := range parcels {
		parcelIDs[i] = parcels[i].ParcelID
	}
	features, err := s.parcelFeatureRepo.GetByParcelIDs(ctx, parcelIDs)
	if err != nil {
		return nil, err
	}

	// Get scoring profile if provided
	var scorer *scoring.Scorer
	if scoringProfileID != nil {
		// This will be implemented when we have the scoring profile repository
		// For now, we'll use default scoring
		scorer = scoring.NewDefaultScorer()
	} else {
		scorer = scoring.NewDefaultScorer()
	}

	// Score each parcel
	results := make([]SearchResult, 0, len(parcels))
	for i := range parcels {
		parcel := &parcels[i]
		feature := features[parcel.ParcelID]

		// Apply hard constraints from query exclusions
		if s.shouldExclude(parcel, feature, query.Exclusions) {
			continue
		}

		// Score the parcel
		score, breakdown, constraintsHit := scorer.Score(parcel, feature)

		results = append(results, SearchResult{
			ParcelID:       parcel.ParcelID.String(),
			Score:          score,
			ScoreBreakdown: breakdown,
			ConstraintsHit: constraintsHit,
			Parcel:         parcel,
			Features:       feature,
		})
	}

	// Sort by score (descending)
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[i].Score < results[j].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	return results, nil
}

// shouldExclude checks if a parcel should be excluded based on exclusions
func (s *SearchService) shouldExclude(parcel interface{}, feature interface{}, exclusions map[string]interface{}) bool {
	if exclusions == nil || len(exclusions) == 0 {
		return false
	}

	// Check floodplain exclusion
	if excludeFloodplain, ok := exclusions["floodplain"].(bool); ok && excludeFloodplain {
		if f, ok := feature.(*models.ParcelFeature); ok && f != nil && f.InFloodplain {
			return true
		}
	}

	// Check wetlands exclusion
	if excludeWetlands, ok := exclusions["wetlands"].(bool); ok && excludeWetlands {
		if f, ok := feature.(*models.ParcelFeature); ok && f != nil && f.InWetlands {
			return true
		}
	}

	// Check protected land exclusion
	if excludeProtected, ok := exclusions["protectedLand"].(bool); ok && excludeProtected {
		if f, ok := feature.(*models.ParcelFeature); ok && f != nil && f.InProtectedLand {
			return true
		}
	}

	return false
}

// polygonToBBox converts a polygon to bounding box
func polygonToBBox(polygon [][]float64) []float64 {
	if len(polygon) == 0 {
		return []float64{}
	}

	minLon := polygon[0][0]
	maxLon := polygon[0][0]
	minLat := polygon[0][1]
	maxLat := polygon[0][1]

	for _, point := range polygon {
		if point[0] < minLon {
			minLon = point[0]
		}
		if point[0] > maxLon {
			maxLon = point[0]
		}
		if point[1] < minLat {
			minLat = point[1]
		}
		if point[1] > maxLat {
			maxLat = point[1]
		}
	}

	return []float64{minLon, minLat, maxLon, maxLat}
}
