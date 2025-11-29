package features

import (
	"context"
	"fmt"

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
	s.logger.Info("Recomputing features",
		zap.Bool("all", input.All),
		zap.String("stateFips", input.StateFips),
		zap.Int("parcelCount", len(input.ParcelIDs)))

	// Get database connection
	db := s.parcelRepo.GetDB()

	// Build parcel selection criteria
	var parcelIDsFilter string
	var args []interface{}
	argIdx := 1

	if len(input.ParcelIDs) > 0 {
		// Convert string IDs to UUIDs
		parcelUUIDs := make([]uuid.UUID, 0, len(input.ParcelIDs))
		for _, idStr := range input.ParcelIDs {
			id, err := uuid.Parse(idStr)
			if err != nil {
				s.logger.Warn("Invalid parcel ID, skipping", zap.String("id", idStr))
				continue
			}
			parcelUUIDs = append(parcelUUIDs, id)
		}
		parcelIDsFilter = fmt.Sprintf("AND p.parcel_id = ANY($%d)", argIdx)
		args = append(args, parcelUUIDs)
		argIdx++
	} else if input.StateFips != "" {
		parcelIDsFilter = fmt.Sprintf("AND p.state_fips = $%d", argIdx)
		args = append(args, input.StateFips)
		argIdx++
	}
	// If input.All is true and no other filters, process all parcels

	// Generate a new features version ID
	featuresVersionID := uuid.New()

	// Compute features using PostGIS
	// This query computes all distances and intersections in a single operation
	query := fmt.Sprintf(`
		INSERT INTO parcel_features (
			parcel_id,
			features_version_id,
			in_floodplain,
			in_wetlands,
			in_protected_land,
			dist_to_highway_m,
			dist_to_rail_m,
			dist_to_airport_m,
			dist_to_power_line_m,
			dist_to_substation_m,
			computed_at
		)
		SELECT
			p.parcel_id,
			$%d::uuid as features_version_id,
			-- Constraint intersections
			EXISTS(
				SELECT 1 FROM floodplains f
				WHERE f.tenant_id IS NULL
				  AND ST_Intersects(p.geom, f.geom)
			) as in_floodplain,
			EXISTS(
				SELECT 1 FROM wetlands w
				WHERE w.tenant_id IS NULL
				  AND ST_Intersects(p.geom, w.geom)
			) as in_wetlands,
			EXISTS(
				SELECT 1 FROM protected_land pl
				WHERE pl.tenant_id IS NULL
				  AND ST_Intersects(p.geom, pl.geom)
			) as in_protected_land,
			-- Distance calculations (in meters)
			COALESCE((
				SELECT MIN(ST_Distance(p.centroid::geography, h.geom::geography))
				FROM highways h
				WHERE h.tenant_id IS NULL
			), 0) as dist_to_highway_m,
			COALESCE((
				SELECT MIN(ST_Distance(p.centroid::geography, r.geom::geography))
				FROM rail_lines r
				WHERE r.tenant_id IS NULL
			), 0) as dist_to_rail_m,
			COALESCE((
				SELECT MIN(ST_Distance(p.centroid::geography, a.geom::geography))
				FROM airports a
				WHERE a.tenant_id IS NULL
			), 0) as dist_to_airport_m,
			COALESCE((
				SELECT MIN(ST_Distance(p.centroid::geography, pl.geom::geography))
				FROM power_lines pl
				WHERE pl.tenant_id IS NULL
			), 0) as dist_to_power_line_m,
			COALESCE((
				SELECT MIN(ST_Distance(p.centroid::geography, s.geom::geography))
				FROM substations s
				WHERE s.tenant_id IS NULL
			), 0) as dist_to_substation_m,
			NOW() as computed_at
		FROM parcels p
		WHERE 1=1 %s
		ON CONFLICT (parcel_id) DO UPDATE SET
			features_version_id = EXCLUDED.features_version_id,
			in_floodplain = EXCLUDED.in_floodplain,
			in_wetlands = EXCLUDED.in_wetlands,
			in_protected_land = EXCLUDED.in_protected_land,
			dist_to_highway_m = EXCLUDED.dist_to_highway_m,
			dist_to_rail_m = EXCLUDED.dist_to_rail_m,
			dist_to_airport_m = EXCLUDED.dist_to_airport_m,
			dist_to_power_line_m = EXCLUDED.dist_to_power_line_m,
			dist_to_substation_m = EXCLUDED.dist_to_substation_m,
			computed_at = EXCLUDED.computed_at
	`, argIdx, parcelIDsFilter)

	// Add features version ID to args
	args = append([]interface{}{featuresVersionID}, args...)

	// Execute the computation
	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to compute features: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	s.logger.Info("Feature recomputation completed",
		zap.Int64("rowsAffected", rowsAffected),
		zap.String("featuresVersionId", featuresVersionID.String()))

	return nil
}

// ComputeFeaturesForParcel computes features for a single parcel
func (s *Service) ComputeFeaturesForParcel(ctx context.Context, parcelID uuid.UUID) (*models.ParcelFeature, error) {
	// Use the Recompute method with a single parcel ID
	err := s.Recompute(ctx, &RecomputeInput{
		ParcelIDs: []string{parcelID.String()},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to recompute features: %w", err)
	}

	// Fetch the computed features
	db := s.parcelFeatureRepo.GetDB()
	var feature models.ParcelFeature
	query := `
		SELECT parcel_id, features_version_id, in_floodplain, in_wetlands, in_protected_land,
		       dist_to_highway_m, dist_to_rail_m, dist_to_airport_m,
		       dist_to_power_line_m, dist_to_substation_m, computed_at
		FROM parcel_features
		WHERE parcel_id = $1
	`
	err = db.GetContext(ctx, &feature, query, parcelID)
	if err != nil {
		return nil, fmt.Errorf("failed to get computed features: %w", err)
	}

	return &feature, nil
}
