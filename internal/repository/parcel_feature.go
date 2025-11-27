package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/coolguy1771/scout/internal/models"
)

type ParcelFeatureRepository struct {
	db     *sqlx.DB
	logger *zap.Logger
}

func NewParcelFeatureRepository(db *sqlx.DB, logger *zap.Logger) *ParcelFeatureRepository {
	return &ParcelFeatureRepository{db: db, logger: logger}
}

// GetByParcelID retrieves features for a parcel
func (r *ParcelFeatureRepository) GetByParcelID(ctx context.Context, parcelID uuid.UUID) (*models.ParcelFeature, error) {
	var feature models.ParcelFeature
	err := r.db.GetContext(ctx, &feature, `
		SELECT parcel_id, features_version_id, in_floodplain, in_wetlands, 
		       in_protected_land, dist_to_highway_m, dist_to_rail_m, 
		       dist_to_airport_m, dist_to_power_line_m, dist_to_substation_m, 
		       computed_at
		FROM parcel_features
		WHERE parcel_id = $1
	`, parcelID)
	if err != nil {
		return nil, fmt.Errorf("failed to get parcel features: %w", err)
	}
	return &feature, nil
}

// GetByParcelIDs retrieves features for multiple parcels
func (r *ParcelFeatureRepository) GetByParcelIDs(ctx context.Context, parcelIDs []uuid.UUID) (map[uuid.UUID]*models.ParcelFeature, error) {
	if len(parcelIDs) == 0 {
		return make(map[uuid.UUID]*models.ParcelFeature), nil
	}

	query, args, err := sqlx.In(`
		SELECT parcel_id, features_version_id, in_floodplain, in_wetlands, 
		       in_protected_land, dist_to_highway_m, dist_to_rail_m, 
		       dist_to_airport_m, dist_to_power_line_m, dist_to_substation_m, 
		       computed_at
		FROM parcel_features
		WHERE parcel_id IN (?)
	`, parcelIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	query = r.db.Rebind(query)
	var features []models.ParcelFeature
	err = r.db.SelectContext(ctx, &features, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get parcel features: %w", err)
	}

	result := make(map[uuid.UUID]*models.ParcelFeature)
	for i := range features {
		result[features[i].ParcelID] = &features[i]
	}

	return result, nil
}

// Upsert creates or updates parcel features
func (r *ParcelFeatureRepository) Upsert(ctx context.Context, feature *models.ParcelFeature) error {
	_, err := r.db.NamedExecContext(ctx, `
		INSERT INTO parcel_features (
			parcel_id, features_version_id, in_floodplain, in_wetlands, 
			in_protected_land, dist_to_highway_m, dist_to_rail_m, 
			dist_to_airport_m, dist_to_power_line_m, dist_to_substation_m, 
			computed_at
		) VALUES (
			:parcel_id, :features_version_id, :in_floodplain, :in_wetlands, 
			:in_protected_land, :dist_to_highway_m, :dist_to_rail_m, 
			:dist_to_airport_m, :dist_to_power_line_m, :dist_to_substation_m, 
			:computed_at
		)
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
	`, feature)
	if err != nil {
		return fmt.Errorf("failed to upsert parcel features: %w", err)
	}
	return nil
}

// BatchUpsert creates or updates multiple parcel features in a transaction
func (r *ParcelFeatureRepository) BatchUpsert(ctx context.Context, features []models.ParcelFeature) error {
	if len(features) == 0 {
		return nil
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareNamedContext(ctx, `
		INSERT INTO parcel_features (
			parcel_id, features_version_id, in_floodplain, in_wetlands, 
			in_protected_land, dist_to_highway_m, dist_to_rail_m, 
			dist_to_airport_m, dist_to_power_line_m, dist_to_substation_m, 
			computed_at
		) VALUES (
			:parcel_id, :features_version_id, :in_floodplain, :in_wetlands, 
			:in_protected_land, :dist_to_highway_m, :dist_to_rail_m, 
			:dist_to_airport_m, :dist_to_power_line_m, :dist_to_substation_m, 
			:computed_at
		)
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
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for i := range features {
		_, err := stmt.ExecContext(ctx, &features[i])
		if err != nil {
			return fmt.Errorf("failed to upsert feature for parcel %s: %w", features[i].ParcelID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
