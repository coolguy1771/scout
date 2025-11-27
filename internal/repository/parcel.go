package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/coolguy1771/scout/internal/models"
)

type ParcelRepository struct {
	db     *sqlx.DB
	logger *zap.Logger
}

func NewParcelRepository(db *sqlx.DB, logger *zap.Logger) *ParcelRepository {
	return &ParcelRepository{db: db, logger: logger}
}

// GetDB returns the underlying database connection
func (r *ParcelRepository) GetDB() *sqlx.DB {
	return r.db
}

// GetByID retrieves a parcel by ID with tenant check
func (r *ParcelRepository) GetByID(ctx context.Context, parcelID uuid.UUID, tenantID *uuid.UUID) (*models.Parcel, error) {
	var parcel models.Parcel
	query := `
		SELECT parcel_id, tenant_id, apn, acres, zoning_raw, zoning_tags, 
		       jurisdiction, state_fips, 
		       ST_AsGeoJSON(geom)::jsonb->>'type' as geom_type,
		       ST_AsGeoJSON(centroid)::jsonb->>'type' as centroid_type,
		       created_at, updated_at
		FROM parcels
		WHERE parcel_id = $1
	`

	args := []interface{}{parcelID}
	if tenantID != nil {
		query += " AND (tenant_id = $2 OR tenant_id IS NULL)"
		args = append(args, *tenantID)
	}

	err := r.db.GetContext(ctx, &parcel, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get parcel: %w", err)
	}

	// Fetch geometry separately as PostGIS geometry
	var geomStr, centroidStr string
	err = r.db.GetContext(ctx, &geomStr, "SELECT ST_AsGeoJSON(geom) FROM parcels WHERE parcel_id = $1", parcelID)
	if err == nil {
		parcel.Geom = geomStr
	}
	err = r.db.GetContext(ctx, &centroidStr, "SELECT ST_AsGeoJSON(centroid) FROM parcels WHERE parcel_id = $1", parcelID)
	if err == nil {
		parcel.Centroid = centroidStr
	}

	return &parcel, nil
}

// FindNearby finds nearby infrastructure features for a parcel
func (r *ParcelRepository) FindNearby(ctx context.Context, parcelID uuid.UUID, featureTypes []string, maxDistanceM float64, limit int) (map[string][]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 10
	}
	if maxDistanceM <= 0 {
		maxDistanceM = 5000 // default 5km
	}

	result := make(map[string][]map[string]interface{})

	// Get parcel centroid
	var centroid string
	err := r.db.GetContext(ctx, &centroid, "SELECT centroid FROM parcels WHERE parcel_id = $1", parcelID)
	if err != nil {
		return nil, fmt.Errorf("failed to get parcel centroid: %w", err)
	}

	// Query each feature type
	for _, featureType := range featureTypes {
		var query string
		var rows *sqlx.Rows
		var err error

		switch featureType {
		case "rail":
			query = fmt.Sprintf(`
				SELECT feature_id, name, 
				       ST_Distance(geom::geography, $1::geography) as distance_m,
				       ST_AsGeoJSON(geom) as geom
				FROM rail_lines
				WHERE ST_DWithin(geom::geography, $1::geography, $2)
				ORDER BY distance_m
				LIMIT $3
			`)
			rows, err = r.db.QueryxContext(ctx, query, centroid, maxDistanceM, limit)
		case "highway":
			query = fmt.Sprintf(`
				SELECT feature_id, name, 
				       ST_Distance(geom::geography, $1::geography) as distance_m,
				       ST_AsGeoJSON(geom) as geom
				FROM highways
				WHERE ST_DWithin(geom::geography, $1::geography, $2)
				ORDER BY distance_m
				LIMIT $3
			`)
			rows, err = r.db.QueryxContext(ctx, query, centroid, maxDistanceM, limit)
		case "airport":
			query = fmt.Sprintf(`
				SELECT feature_id, name, 
				       ST_Distance(geom::geography, $1::geography) as distance_m,
				       ST_AsGeoJSON(geom) as geom
				FROM airports
				WHERE ST_DWithin(geom::geography, $1::geography, $2)
				ORDER BY distance_m
				LIMIT $3
			`)
			rows, err = r.db.QueryxContext(ctx, query, centroid, maxDistanceM, limit)
		case "power_line":
			query = fmt.Sprintf(`
				SELECT feature_id, name, 
				       ST_Distance(geom::geography, $1::geography) as distance_m,
				       ST_AsGeoJSON(geom) as geom
				FROM power_lines
				WHERE ST_DWithin(geom::geography, $1::geography, $2)
				ORDER BY distance_m
				LIMIT $3
			`)
			rows, err = r.db.QueryxContext(ctx, query, centroid, maxDistanceM, limit)
		case "substation":
			query = fmt.Sprintf(`
				SELECT feature_id, name, 
				       ST_Distance(geom::geography, $1::geography) as distance_m,
				       ST_AsGeoJSON(geom) as geom
				FROM substations
				WHERE ST_DWithin(geom::geography, $1::geography, $2)
				ORDER BY distance_m
				LIMIT $3
			`)
			rows, err = r.db.QueryxContext(ctx, query, centroid, maxDistanceM, limit)
		default:
			continue
		}

		if err != nil {
			r.logger.Warn("Failed to query nearby features", zap.String("type", featureType), zap.Error(err))
			continue
		}
		defer rows.Close()

		features := []map[string]interface{}{}
		for rows.Next() {
			var feature map[string]interface{}
			if err := rows.MapScan(feature); err != nil {
				r.logger.Warn("Failed to scan feature", zap.Error(err))
				continue
			}
			features = append(features, feature)
		}
		result[featureType] = features
	}

	return result, nil
}

// SearchCandidates performs Phase A: fast candidate selection
func (r *ParcelRepository) SearchCandidates(ctx context.Context, bbox []float64, filters map[string]interface{}, tenantID *uuid.UUID, maxCandidates int) ([]uuid.UUID, error) {
	query := `
		SELECT parcel_id
		FROM parcels
		WHERE 1=1
	`
	args := []interface{}{}
	argIdx := 1

	// Bbox filter
	if len(bbox) == 4 {
		bboxGeom := fmt.Sprintf("ST_MakeEnvelope(%f, %f, %f, %f, 4326)", bbox[0], bbox[1], bbox[2], bbox[3])
		query += fmt.Sprintf(" AND ST_Intersects(centroid, %s)", bboxGeom)
	}

	// Tenant filter
	if tenantID != nil {
		query += fmt.Sprintf(" AND (tenant_id = $%d OR tenant_id IS NULL)", argIdx)
		args = append(args, *tenantID)
		argIdx++
	}

	// Acres filter
	if minAcres, ok := filters["minAcres"].(float64); ok {
		query += fmt.Sprintf(" AND acres >= $%d", argIdx)
		args = append(args, minAcres)
		argIdx++
	}
	if maxAcres, ok := filters["maxAcres"].(float64); ok {
		query += fmt.Sprintf(" AND acres <= $%d", argIdx)
		args = append(args, maxAcres)
		argIdx++
	}

	// Zoning tags filter
	if zoningTags, ok := filters["zoningTags"].([]string); ok && len(zoningTags) > 0 {
		query += fmt.Sprintf(" AND zoning_tags && $%d", argIdx)
		args = append(args, zoningTags)
		argIdx++
	}

	// Jurisdiction filter
	if jurisdiction, ok := filters["jurisdiction"].(string); ok && jurisdiction != "" {
		query += fmt.Sprintf(" AND jurisdiction = $%d", argIdx)
		args = append(args, jurisdiction)
		argIdx++
	}

	// State FIPS filter
	if stateFips, ok := filters["stateFips"].(string); ok && stateFips != "" {
		query += fmt.Sprintf(" AND state_fips = $%d", argIdx)
		args = append(args, stateFips)
		argIdx++
	}

	// Limit
	if maxCandidates > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argIdx)
		args = append(args, maxCandidates)
	}

	var parcelIDs []uuid.UUID
	err := r.db.SelectContext(ctx, &parcelIDs, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to search candidates: %w", err)
	}

	return parcelIDs, nil
}

// GetByIDs retrieves multiple parcels by IDs
func (r *ParcelRepository) GetByIDs(ctx context.Context, parcelIDs []uuid.UUID) ([]models.Parcel, error) {
	if len(parcelIDs) == 0 {
		return []models.Parcel{}, nil
	}

	query, args, err := sqlx.In(`
		SELECT parcel_id, tenant_id, apn, acres, zoning_raw, zoning_tags, 
		       jurisdiction, state_fips, created_at, updated_at
		FROM parcels
		WHERE parcel_id IN (?)
	`, parcelIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	query = r.db.Rebind(query)
	var parcels []models.Parcel
	err = r.db.SelectContext(ctx, &parcels, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get parcels: %w", err)
	}

	return parcels, nil
}
