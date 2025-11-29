package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/coolguy1771/scout/internal/models"
	"github.com/coolguy1771/scout/internal/repository"
	"github.com/coolguy1771/scout/internal/storage"
)

type ExportJob struct {
	jobRepo     *repository.JobRepository
	exportRepo  *repository.ExportRepository
	parcelRepo  *repository.ParcelRepository
	tileStorage *storage.TileStorage
	logger      *zap.Logger
}

func NewExportJob(jobRepo *repository.JobRepository, exportRepo *repository.ExportRepository, parcelRepo *repository.ParcelRepository, tileStorage *storage.TileStorage, logger *zap.Logger) *ExportJob {
	return &ExportJob{
		jobRepo:     jobRepo,
		exportRepo:  exportRepo,
		parcelRepo:  parcelRepo,
		tileStorage: tileStorage,
		logger:      logger,
	}
}

type ExportInput struct {
	ParcelIDs        []string `json:"parcelIds,omitempty"`
	SavedSearchID    *string  `json:"savedSearchId,omitempty"`
	Kind             string   `json:"kind"` // pdf, csv, geojson
	ScoringProfileID *string  `json:"scoringProfileId,omitempty"`
}

func (j *ExportJob) Process(ctx context.Context, job *models.Job) error {
	// Parse input
	var input ExportInput
	if err := json.Unmarshal([]byte(job.InputJSON), &input); err != nil {
		return fmt.Errorf("failed to parse export input: %w", err)
	}

	switch input.Kind {
	case "csv":
		return j.generateCSV(ctx, job, &input)
	case "geojson":
		return j.generateGeoJSON(ctx, job, &input)
	case "pdf":
		return j.generatePDF(ctx, job, &input)
	default:
		return fmt.Errorf("unsupported export kind: %s", input.Kind)
	}
}

func (j *ExportJob) generateCSV(ctx context.Context, job *models.Job, input *ExportInput) error {
	exportID := uuid.New()
	objectKey := fmt.Sprintf("exports/%s/%s.csv", job.TenantID.String(), exportID.String())

	// Fetch parcels
	parcelIDs, err := j.resolveParcelIDs(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to resolve parcel IDs: %w", err)
	}

	if len(parcelIDs) == 0 {
		return fmt.Errorf("no parcels to export")
	}

	j.logger.Info("Generating CSV export", zap.Int("parcelCount", len(parcelIDs)))

	// Query parcels with features
	query := `
		SELECT
			p.parcel_id,
			p.apn,
			p.acres,
			p.zoning_raw,
			p.jurisdiction,
			p.state_fips,
			ST_Y(ST_Centroid(p.centroid)) as latitude,
			ST_X(ST_Centroid(p.centroid)) as longitude,
			COALESCE(pf.in_floodplain, false) as in_floodplain,
			COALESCE(pf.in_wetlands, false) as in_wetlands,
			COALESCE(pf.in_protected_land, false) as in_protected_land,
			COALESCE(pf.dist_to_highway_m, 0) as dist_to_highway_m,
			COALESCE(pf.dist_to_rail_m, 0) as dist_to_rail_m,
			COALESCE(pf.dist_to_airport_m, 0) as dist_to_airport_m,
			COALESCE(pf.dist_to_power_line_m, 0) as dist_to_power_line_m,
			COALESCE(pf.dist_to_substation_m, 0) as dist_to_substation_m
		FROM parcels p
		LEFT JOIN parcel_features pf ON p.parcel_id = pf.parcel_id
		WHERE p.parcel_id = ANY($1)
		ORDER BY p.parcel_id
	`

	db := j.parcelRepo.GetDB()
	rows, err := db.QueryxContext(ctx, query, parcelIDs)
	if err != nil {
		return fmt.Errorf("failed to query parcels: %w", err)
	}
	defer rows.Close()

	// Build CSV content
	var csvBuilder strings.Builder
	csvBuilder.WriteString("parcel_id,apn,acres,zoning,jurisdiction,state_fips,latitude,longitude,")
	csvBuilder.WriteString("in_floodplain,in_wetlands,in_protected_land,")
	csvBuilder.WriteString("dist_to_highway_m,dist_to_rail_m,dist_to_airport_m,dist_to_power_line_m,dist_to_substation_m\n")

	rowCount := 0
	for rows.Next() {
		var row struct {
			ParcelID          string  `db:"parcel_id"`
			APN               string  `db:"apn"`
			Acres             float64 `db:"acres"`
			ZoningRaw         string  `db:"zoning_raw"`
			Jurisdiction      string  `db:"jurisdiction"`
			StateFips         string  `db:"state_fips"`
			Latitude          float64 `db:"latitude"`
			Longitude         float64 `db:"longitude"`
			InFloodplain      bool    `db:"in_floodplain"`
			InWetlands        bool    `db:"in_wetlands"`
			InProtectedLand   bool    `db:"in_protected_land"`
			DistToHighwayM    float64 `db:"dist_to_highway_m"`
			DistToRailM       float64 `db:"dist_to_rail_m"`
			DistToAirportM    float64 `db:"dist_to_airport_m"`
			DistToPowerLineM  float64 `db:"dist_to_power_line_m"`
			DistToSubstationM float64 `db:"dist_to_substation_m"`
		}

		if err := rows.StructScan(&row); err != nil {
			j.logger.Warn("Failed to scan row", zap.Error(err))
			continue
		}

		// Write CSV row
		csvBuilder.WriteString(fmt.Sprintf("%s,%s,%.2f,%s,%s,%s,%.6f,%.6f,%t,%t,%t,%.2f,%.2f,%.2f,%.2f,%.2f\n",
			row.ParcelID, row.APN, row.Acres, row.ZoningRaw, row.Jurisdiction, row.StateFips,
			row.Latitude, row.Longitude,
			row.InFloodplain, row.InWetlands, row.InProtectedLand,
			row.DistToHighwayM, row.DistToRailM, row.DistToAirportM,
			row.DistToPowerLineM, row.DistToSubstationM))
		rowCount++
	}

	csvContent := csvBuilder.String()

	// Upload to storage
	if j.tileStorage != nil {
		if err := j.tileStorage.UploadExport(ctx, job.TenantID.String(), exportID.String(), "csv", []byte(csvContent), "text/csv"); err != nil {
			return fmt.Errorf("failed to upload export: %w", err)
		}
	}

	// Create export record
	export := &models.Export{
		ExportID:    exportID,
		TenantID:    job.TenantID,
		JobID:       job.JobID,
		Kind:        "csv",
		ObjectKey:   objectKey,
		ContentType: "text/csv",
		CreatedAt:   time.Now(),
	}

	if err := j.exportRepo.Create(ctx, export); err != nil {
		return fmt.Errorf("failed to create export record: %w", err)
	}

	// Update job with output
	outputJSON, _ := json.Marshal(map[string]interface{}{
		"exportId":  exportID.String(),
		"objectKey": objectKey,
		"rowCount":  rowCount,
	})
	return j.jobRepo.UpdateStatus(ctx, job.JobID, "completed", string(outputJSON), nil)
}

func (j *ExportJob) generateGeoJSON(ctx context.Context, job *models.Job, input *ExportInput) error {
	exportID := uuid.New()
	objectKey := fmt.Sprintf("exports/%s/%s.geojson", job.TenantID.String(), exportID.String())

	// Fetch parcels
	parcelIDs, err := j.resolveParcelIDs(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to resolve parcel IDs: %w", err)
	}

	if len(parcelIDs) == 0 {
		return fmt.Errorf("no parcels to export")
	}

	j.logger.Info("Generating GeoJSON export", zap.Int("parcelCount", len(parcelIDs)))

	// Query parcels with geometries and features
	query := `
		SELECT jsonb_build_object(
			'type', 'Feature',
			'id', p.parcel_id::text,
			'geometry', ST_AsGeoJSON(p.geom)::jsonb,
			'properties', jsonb_build_object(
				'parcelId', p.parcel_id::text,
				'apn', p.apn,
				'acres', p.acres,
				'zoning', p.zoning_raw,
				'jurisdiction', p.jurisdiction,
				'stateFips', p.state_fips,
				'inFloodplain', COALESCE(pf.in_floodplain, false),
				'inWetlands', COALESCE(pf.in_wetlands, false),
				'inProtectedLand', COALESCE(pf.in_protected_land, false),
				'distToHighwayM', COALESCE(pf.dist_to_highway_m, 0),
				'distToRailM', COALESCE(pf.dist_to_rail_m, 0),
				'distToAirportM', COALESCE(pf.dist_to_airport_m, 0),
				'distToPowerLineM', COALESCE(pf.dist_to_power_line_m, 0),
				'distToSubstationM', COALESCE(pf.dist_to_substation_m, 0)
			)
		) as feature
		FROM parcels p
		LEFT JOIN parcel_features pf ON p.parcel_id = pf.parcel_id
		WHERE p.parcel_id = ANY($1)
		ORDER BY p.parcel_id
	`

	db := j.parcelRepo.GetDB()
	rows, err := db.QueryxContext(ctx, query, parcelIDs)
	if err != nil {
		return fmt.Errorf("failed to query parcels: %w", err)
	}
	defer rows.Close()

	// Collect features
	features := []json.RawMessage{}
	for rows.Next() {
		var featureJSON string
		if err := rows.Scan(&featureJSON); err != nil {
			j.logger.Warn("Failed to scan feature", zap.Error(err))
			continue
		}
		features = append(features, json.RawMessage(featureJSON))
	}

	// Build GeoJSON FeatureCollection
	featureCollection := map[string]interface{}{
		"type":     "FeatureCollection",
		"features": features,
	}

	geojsonBytes, err := json.Marshal(featureCollection)
	if err != nil {
		return fmt.Errorf("failed to marshal GeoJSON: %w", err)
	}

	// Upload to storage
	if j.tileStorage != nil {
		if err := j.tileStorage.UploadExport(ctx, job.TenantID.String(), exportID.String(), "geojson", geojsonBytes, "application/geo+json"); err != nil {
			return fmt.Errorf("failed to upload export: %w", err)
		}
	}

	// Create export record
	export := &models.Export{
		ExportID:    exportID,
		TenantID:    job.TenantID,
		JobID:       job.JobID,
		Kind:        "geojson",
		ObjectKey:   objectKey,
		ContentType: "application/geo+json",
		CreatedAt:   time.Now(),
	}

	if err := j.exportRepo.Create(ctx, export); err != nil {
		return fmt.Errorf("failed to create export record: %w", err)
	}

	// Update job with output
	outputJSON, _ := json.Marshal(map[string]interface{}{
		"exportId":     exportID.String(),
		"objectKey":    objectKey,
		"featureCount": len(features),
	})
	return j.jobRepo.UpdateStatus(ctx, job.JobID, "completed", string(outputJSON), nil)
}

// resolveParcelIDs resolves parcel IDs from input (either direct IDs or from saved search)
func (j *ExportJob) resolveParcelIDs(_ context.Context, input *ExportInput) ([]uuid.UUID, error) {
	// If parcel IDs are provided directly, parse and return them
	if len(input.ParcelIDs) > 0 {
		parcelUUIDs := make([]uuid.UUID, 0, len(input.ParcelIDs))
		for _, idStr := range input.ParcelIDs {
			id, err := uuid.Parse(idStr)
			if err != nil {
				j.logger.Warn("Invalid parcel ID, skipping", zap.String("id", idStr), zap.Error(err))
				continue
			}
			parcelUUIDs = append(parcelUUIDs, id)
		}
		return parcelUUIDs, nil
	}

	// If saved search ID is provided, execute the search to get parcels
	if input.SavedSearchID != nil {
		// For MVP, we don't have saved search execution implemented
		// In production, this would:
		// 1. Load the saved search
		// 2. Execute the search query
		// 3. Return the parcel IDs
		return nil, fmt.Errorf("saved search export not yet implemented")
	}

	return nil, fmt.Errorf("must provide either parcelIds or savedSearchId")
}

// generatePDF generates a PDF export (HTML-based for MVP)
func (j *ExportJob) generatePDF(ctx context.Context, job *models.Job, input *ExportInput) error {
	exportID := uuid.New()
	objectKey := fmt.Sprintf("exports/%s/%s.html", job.TenantID.String(), exportID.String())

	// Fetch parcels
	parcelIDs, err := j.resolveParcelIDs(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to resolve parcel IDs: %w", err)
	}

	if len(parcelIDs) == 0 {
		return fmt.Errorf("no parcels to export")
	}

	j.logger.Info("Generating PDF export (HTML format)", zap.Int("parcelCount", len(parcelIDs)))

	// Query parcels with features
	query := `
		SELECT
			p.parcel_id,
			p.apn,
			p.acres,
			p.zoning_raw,
			p.jurisdiction,
			p.state_fips,
			ST_Y(ST_Centroid(p.centroid)) as latitude,
			ST_X(ST_Centroid(p.centroid)) as longitude,
			COALESCE(pf.in_floodplain, false) as in_floodplain,
			COALESCE(pf.in_wetlands, false) as in_wetlands,
			COALESCE(pf.in_protected_land, false) as in_protected_land,
			COALESCE(pf.dist_to_highway_m, 0) as dist_to_highway_m,
			COALESCE(pf.dist_to_rail_m, 0) as dist_to_rail_m,
			COALESCE(pf.dist_to_airport_m, 0) as dist_to_airport_m,
			COALESCE(pf.dist_to_power_line_m, 0) as dist_to_power_line_m,
			COALESCE(pf.dist_to_substation_m, 0) as dist_to_substation_m
		FROM parcels p
		LEFT JOIN parcel_features pf ON p.parcel_id = pf.parcel_id
		WHERE p.parcel_id = ANY($1)
		ORDER BY p.parcel_id
	`

	db := j.parcelRepo.GetDB()
	rows, err := db.QueryxContext(ctx, query, parcelIDs)
	if err != nil {
		return fmt.Errorf("failed to query parcels: %w", err)
	}
	defer rows.Close()

	// Build HTML content
	var htmlBuilder strings.Builder
	htmlBuilder.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Scout CRE Parcel Report</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; }
        h1 { color: #333; border-bottom: 2px solid #0066cc; padding-bottom: 10px; }
        .parcel { page-break-inside: avoid; margin-bottom: 30px; border: 1px solid #ddd; padding: 15px; border-radius: 5px; }
        .parcel h2 { color: #0066cc; margin-top: 0; }
        .info-grid { display: grid; grid-template-columns: 200px auto; gap: 10px; margin-bottom: 15px; }
        .info-label { font-weight: bold; color: #555; }
        .info-value { color: #333; }
        .constraints { background-color: #fff3cd; padding: 10px; border-left: 4px solid #ffc107; margin-top: 10px; }
        .distances { background-color: #d1ecf1; padding: 10px; border-left: 4px solid #17a2b8; margin-top: 10px; }
        .warning { color: #856404; }
        .distance { color: #0c5460; }
        @media print {
            .parcel { page-break-inside: avoid; }
            body { margin: 0; }
        }
    </style>
</head>
<body>
    <h1>Scout CRE - Parcel Analysis Report</h1>
    <p>Generated: ` + time.Now().Format("January 2, 2006 3:04 PM MST") + `</p>
    <p>Total Parcels: ` + fmt.Sprintf("%d", len(parcelIDs)) + `</p>
`)

	rowCount := 0
	for rows.Next() {
		var row struct {
			ParcelID          string  `db:"parcel_id"`
			APN               string  `db:"apn"`
			Acres             float64 `db:"acres"`
			ZoningRaw         string  `db:"zoning_raw"`
			Jurisdiction      string  `db:"jurisdiction"`
			StateFips         string  `db:"state_fips"`
			Latitude          float64 `db:"latitude"`
			Longitude         float64 `db:"longitude"`
			InFloodplain      bool    `db:"in_floodplain"`
			InWetlands        bool    `db:"in_wetlands"`
			InProtectedLand   bool    `db:"in_protected_land"`
			DistToHighwayM    float64 `db:"dist_to_highway_m"`
			DistToRailM       float64 `db:"dist_to_rail_m"`
			DistToAirportM    float64 `db:"dist_to_airport_m"`
			DistToPowerLineM  float64 `db:"dist_to_power_line_m"`
			DistToSubstationM float64 `db:"dist_to_substation_m"`
		}

		if err := rows.StructScan(&row); err != nil {
			j.logger.Warn("Failed to scan row", zap.Error(err))
			continue
		}

		// Build parcel section
		htmlBuilder.WriteString(fmt.Sprintf(`
    <div class="parcel">
        <h2>Parcel: %s</h2>
        <div class="info-grid">
            <div class="info-label">APN:</div>
            <div class="info-value">%s</div>
            <div class="info-label">Acres:</div>
            <div class="info-value">%.2f</div>
            <div class="info-label">Zoning:</div>
            <div class="info-value">%s</div>
            <div class="info-label">Jurisdiction:</div>
            <div class="info-value">%s</div>
            <div class="info-label">State:</div>
            <div class="info-value">%s</div>
            <div class="info-label">Location:</div>
            <div class="info-value">%.6f, %.6f</div>
        </div>
        <div class="constraints">
            <strong>Constraints:</strong><br>
            <span class="warning">Floodplain: %s</span> |
            <span class="warning">Wetlands: %s</span> |
            <span class="warning">Protected Land: %s</span>
        </div>
        <div class="distances">
            <strong>Infrastructure Proximity:</strong><br>
            <span class="distance">Highway: %.0f m (%.2f mi)</span> |
            <span class="distance">Rail: %.0f m (%.2f mi)</span><br>
            <span class="distance">Airport: %.0f m (%.2f mi)</span> |
            <span class="distance">Power Line: %.0f m (%.2f mi)</span> |
            <span class="distance">Substation: %.0f m (%.2f mi)</span>
        </div>
    </div>
`, row.APN, row.APN, row.Acres, row.ZoningRaw, row.Jurisdiction, row.StateFips,
			row.Latitude, row.Longitude,
			boolToYesNo(row.InFloodplain), boolToYesNo(row.InWetlands), boolToYesNo(row.InProtectedLand),
			row.DistToHighwayM, metersToMiles(row.DistToHighwayM),
			row.DistToRailM, metersToMiles(row.DistToRailM),
			row.DistToAirportM, metersToMiles(row.DistToAirportM),
			row.DistToPowerLineM, metersToMiles(row.DistToPowerLineM),
			row.DistToSubstationM, metersToMiles(row.DistToSubstationM)))
		rowCount++
	}

	htmlBuilder.WriteString(`
</body>
</html>`)

	htmlContent := htmlBuilder.String()

	// Upload to storage as HTML (can be converted to PDF by browser/tools)
	if j.tileStorage != nil {
		if err := j.tileStorage.UploadExport(ctx, job.TenantID.String(), exportID.String(), "html", []byte(htmlContent), "text/html"); err != nil {
			return fmt.Errorf("failed to upload export: %w", err)
		}
	}

	// Create export record
	export := &models.Export{
		ExportID:    exportID,
		TenantID:    job.TenantID,
		JobID:       job.JobID,
		Kind:        "pdf", // Keep as pdf in metadata
		ObjectKey:   objectKey,
		ContentType: "text/html", // But store as HTML for now
		CreatedAt:   time.Now(),
	}

	if err := j.exportRepo.Create(ctx, export); err != nil {
		return fmt.Errorf("failed to create export record: %w", err)
	}

	// Update job with output
	outputJSON, _ := json.Marshal(map[string]interface{}{
		"exportId":  exportID.String(),
		"objectKey": objectKey,
		"rowCount":  rowCount,
		"format":    "html", // Note that it's HTML, not true PDF
		"note":      "HTML format - can be printed to PDF. For native PDF, add a PDF library dependency.",
	})
	return j.jobRepo.UpdateStatus(ctx, job.JobID, "completed", string(outputJSON), nil)
}

// boolToYesNo converts boolean to Yes/No string
func boolToYesNo(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

// metersToMiles converts meters to miles
func metersToMiles(meters float64) float64 {
	return meters * 0.000621371
}


