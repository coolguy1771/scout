package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/coolguy1771/scout/internal/ingestion"
	"github.com/coolguy1771/scout/internal/models"
	"github.com/coolguy1771/scout/internal/repository"
	"github.com/coolguy1771/scout/internal/storage"
)

type LayerUploadJob struct {
	jobRepo   *repository.JobRepository
	layerRepo *repository.LayerRepository
	s3Storage *storage.S3Storage
	parser    *ingestion.Parser
	logger    *zap.Logger
}

func NewLayerUploadJob(
	jobRepo *repository.JobRepository,
	layerRepo *repository.LayerRepository,
	s3Storage *storage.S3Storage,
	logger *zap.Logger,
) *LayerUploadJob {
	return &LayerUploadJob{
		jobRepo:   jobRepo,
		layerRepo: layerRepo,
		s3Storage: s3Storage,
		parser:    ingestion.NewParser(logger),
		logger:    logger,
	}
}

type LayerUploadInput struct {
	LayerID   string `json:"layerId"`
	ObjectKey string `json:"objectKey"`
	FileName  string `json:"fileName"`
}

func (j *LayerUploadJob) Process(ctx context.Context, job *models.Job) error {
	// Parse input
	var input LayerUploadInput
	if err := json.Unmarshal([]byte(job.InputJSON), &input); err != nil {
		return fmt.Errorf("failed to parse layer upload input: %w", err)
	}

	layerID, err := uuid.Parse(input.LayerID)
	if err != nil {
		return fmt.Errorf("invalid layer ID: %w", err)
	}

	// Verify layer exists and belongs to tenant
	_, err = j.layerRepo.GetByID(ctx, layerID, job.TenantID)
	if err != nil {
		return fmt.Errorf("failed to get layer: %w", err)
	}

	// Download file from S3
	fileReader, err := j.s3Storage.Download(ctx, input.ObjectKey)
	if err != nil {
		j.layerRepo.UpdateStatus(ctx, layerID, "failed", nil, fmt.Sprintf("Failed to download file: %v", err))
		return fmt.Errorf("failed to download file: %w", err)
	}
	defer fileReader.Close()

	// Read file content
	fileContent, err := io.ReadAll(fileReader)
	if err != nil {
		j.layerRepo.UpdateStatus(ctx, layerID, "failed", nil, fmt.Sprintf("Failed to read file: %v", err))
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Parse GeoJSON
	features, err := j.parser.ParseGeoJSON(bytes.NewReader(fileContent))
	if err != nil {
		j.layerRepo.UpdateStatus(ctx, layerID, "failed", nil, fmt.Sprintf("Failed to parse GeoJSON: %v", err))
		return fmt.Errorf("failed to parse GeoJSON: %w", err)
	}

	if len(features) == 0 {
		j.layerRepo.UpdateStatus(ctx, layerID, "failed", nil, "No features found in file")
		return fmt.Errorf("no features found in file")
	}

	// Determine layer type
	layerType := j.parser.DetermineLayerType(features)
	j.logger.Info("Processing layer",
		zap.String("layerId", layerID.String()),
		zap.String("layerType", layerType),
		zap.Int("featureCount", len(features)))

	// Create data run (simplified - just generate ID for now)
	dataRunID := uuid.New()

	// Load features into appropriate table based on layer type
	var loadErr error
	switch layerType {
	case "point":
		loadErr = j.loadPointFeatures(ctx, features, job.TenantID, dataRunID)
	case "line":
		loadErr = j.loadLineFeatures(ctx, features, job.TenantID, dataRunID)
	case "polygon":
		loadErr = j.loadPolygonFeatures(ctx, features, job.TenantID, dataRunID)
	default:
		loadErr = fmt.Errorf("unsupported layer type: %s", layerType)
	}

	if loadErr != nil {
		j.layerRepo.UpdateStatus(ctx, layerID, "failed", &dataRunID, loadErr.Error())
		return fmt.Errorf("failed to load features: %w", loadErr)
	}

	// Update layer status
	j.layerRepo.UpdateStatus(ctx, layerID, "completed", &dataRunID, "")

	// Update job status
	outputJSON, _ := json.Marshal(map[string]interface{}{
		"layerId":      layerID.String(),
		"dataRunId":    dataRunID.String(),
		"featureCount": len(features),
		"layerType":    layerType,
	})
	return j.jobRepo.UpdateStatus(ctx, job.JobID, "completed", string(outputJSON), nil)
}

// loadPointFeatures loads point features into airports or substations table
func (j *LayerUploadJob) loadPointFeatures(ctx context.Context, features []ingestion.ParsedFeature, tenantID uuid.UUID, dataRunID uuid.UUID) error {
	// For simplicity, we'll use a generic approach and load into a staging table
	// In production, you might want to route to specific tables based on properties
	query := `
		INSERT INTO airports (feature_id, tenant_id, name, geom, data_run_id, updated_at)
		SELECT gen_random_uuid(), $1, 
		       COALESCE(properties->>'name', ''),
		       ST_GeomFromGeoJSON(geometry),
		       $2,
		       NOW()
		FROM (
			SELECT jsonb_array_elements($3::jsonb) as feature
		) features,
		LATERAL (
			SELECT 
				feature->>'geometry' as geometry,
				feature->'properties' as properties
		) parsed
		WHERE ST_GeometryType(ST_GeomFromGeoJSON(geometry)) IN ('ST_Point', 'ST_MultiPoint')
	`

	// Convert features to JSON array
	featuresJSON := make([]map[string]interface{}, len(features))
	for i, f := range features {
		featuresJSON[i] = map[string]interface{}{
			"geometry":   json.RawMessage(f.Geometry),
			"properties": f.Properties,
		}
	}

	featuresBytes, _ := json.Marshal(featuresJSON)
	db := j.jobRepo.GetDB()
	_, err := db.ExecContext(ctx, query, tenantID, dataRunID, string(featuresBytes))
	return err
}

// loadLineFeatures loads line features into highways, rail_lines, or power_lines table
func (j *LayerUploadJob) loadLineFeatures(ctx context.Context, features []ingestion.ParsedFeature, tenantID uuid.UUID, dataRunID uuid.UUID) error {
	// Similar to loadPointFeatures but for line geometries
	query := `
		INSERT INTO highways (feature_id, tenant_id, name, geom, data_run_id, updated_at)
		SELECT gen_random_uuid(), $1,
		       COALESCE(properties->>'name', ''),
		       ST_GeomFromGeoJSON(geometry),
		       $2,
		       NOW()
		FROM (
			SELECT jsonb_array_elements($3::jsonb) as feature
		) features,
		LATERAL (
			SELECT 
				feature->>'geometry' as geometry,
				feature->'properties' as properties
		) parsed
		WHERE ST_GeometryType(ST_GeomFromGeoJSON(geometry)) IN ('ST_LineString', 'ST_MultiLineString')
	`

	featuresJSON := make([]map[string]interface{}, len(features))
	for i, f := range features {
		featuresJSON[i] = map[string]interface{}{
			"geometry":   json.RawMessage(f.Geometry),
			"properties": f.Properties,
		}
	}

	featuresBytes, _ := json.Marshal(featuresJSON)
	db := j.jobRepo.GetDB()
	_, err := db.ExecContext(ctx, query, tenantID, dataRunID, string(featuresBytes))
	return err
}

// loadPolygonFeatures loads polygon features into floodplains, wetlands, or protected_land table
func (j *LayerUploadJob) loadPolygonFeatures(ctx context.Context, features []ingestion.ParsedFeature, tenantID uuid.UUID, dataRunID uuid.UUID) error {
	// Similar to loadPointFeatures but for polygon geometries
	query := `
		INSERT INTO floodplains (feature_id, tenant_id, name, geom, data_run_id, updated_at)
		SELECT gen_random_uuid(), $1,
		       COALESCE(properties->>'name', ''),
		       ST_GeomFromGeoJSON(geometry),
		       $2,
		       NOW()
		FROM (
			SELECT jsonb_array_elements($3::jsonb) as feature
		) features,
		LATERAL (
			SELECT 
				feature->>'geometry' as geometry,
				feature->'properties' as properties
		) parsed
		WHERE ST_GeometryType(ST_GeomFromGeoJSON(geometry)) IN ('ST_Polygon', 'ST_MultiPolygon')
	`

	featuresJSON := make([]map[string]interface{}, len(features))
	for i, f := range features {
		featuresJSON[i] = map[string]interface{}{
			"geometry":   json.RawMessage(f.Geometry),
			"properties": f.Properties,
		}
	}

	featuresBytes, _ := json.Marshal(featuresJSON)
	db := j.jobRepo.GetDB()
	_, err := db.ExecContext(ctx, query, tenantID, dataRunID, string(featuresBytes))
	return err
}
