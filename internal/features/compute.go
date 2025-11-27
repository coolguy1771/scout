package features

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/coolguy1771/scout/internal/models"
	"github.com/coolguy1771/scout/internal/repository"
)

// Service provides feature computation functionality
type Service struct {
	parcelRepo        *repository.ParcelRepository
	parcelFeatureRepo *repository.ParcelFeatureRepository
	logger            *zap.Logger
}

// NewService creates a new feature service
func NewService(
	parcelRepo *repository.ParcelRepository,
	parcelFeatureRepo *repository.ParcelFeatureRepository,
	logger *zap.Logger,
) *Service {
	return &Service{
		parcelRepo:        parcelRepo,
		parcelFeatureRepo: parcelFeatureRepo,
		logger:            logger,
	}
}

// RecomputeInput represents input for feature recomputation
type RecomputeInput struct {
	ParcelIDs []string `json:"parcelIds,omitempty"`
	StateFips string   `json:"stateFips,omitempty"`
	All       bool     `json:"all,omitempty"`
}

// Recompute recomputes features for parcels
func (s *Service) Recompute(ctx context.Context, input *RecomputeInput) error {
	// For MVP, this is a placeholder
	// In production, this would:
	// 1. Query parcels (by IDs, state, or all)
	// 2. For each parcel, compute distances to infrastructure using PostGIS
	// 3. Check intersections with constraints (floodplain, wetlands, protected land)
	// 4. Batch update parcel_features table

	s.logger.Info("Recomputing features",
		zap.Bool("all", input.All),
		zap.String("stateFips", input.StateFips),
		zap.Int("parcelCount", len(input.ParcelIDs)))

	// Placeholder implementation
	// TODO: Implement actual feature computation
	// This would involve:
	// - Querying parcels from database
	// - Using PostGIS ST_Distance for infrastructure distances
	// - Using ST_Intersects for constraint checks
	// - Batch updating parcel_features

	return nil
}

// ComputeFeaturesForParcel computes features for a single parcel
func (s *Service) ComputeFeaturesForParcel(ctx context.Context, parcelID uuid.UUID) (*models.ParcelFeature, error) {
	// Get parcel (for future use in actual computation)
	_, err := s.parcelRepo.GetByID(ctx, parcelID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get parcel: %w", err)
	}

	// For MVP, return placeholder features
	// In production, compute actual distances and intersections
	feature := &models.ParcelFeature{
		ParcelID:          parcelID,
		FeaturesVersionID: uuid.New(),
		InFloodplain:      false,
		InWetlands:        false,
		InProtectedLand:   false,
		DistToHighwayM:    0,
		DistToRailM:       0,
		DistToAirportM:    0,
		DistToPowerLineM:  0,
		DistToSubstationM: 0,
		ComputedAt:        time.Now(),
	}

	// Upsert features
	if err := s.parcelFeatureRepo.Upsert(ctx, feature); err != nil {
		return nil, fmt.Errorf("failed to upsert features: %w", err)
	}

	return feature, nil
}
