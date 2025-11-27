package jobs

import (
	"context"
	"encoding/json"
	"fmt"
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
	tileStorage *storage.TileStorage
	logger      *zap.Logger
}

func NewExportJob(jobRepo *repository.JobRepository, exportRepo *repository.ExportRepository, tileStorage *storage.TileStorage, logger *zap.Logger) *ExportJob {
	return &ExportJob{
		jobRepo:     jobRepo,
		exportRepo:  exportRepo,
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

	// For MVP, we'll generate a simple CSV/GeoJSON export
	// PDF generation would require a template engine (not implemented in MVP)
	switch input.Kind {
	case "csv":
		return j.generateCSV(ctx, job, &input)
	case "geojson":
		return j.generateGeoJSON(ctx, job, &input)
	case "pdf":
		return fmt.Errorf("PDF export not yet implemented")
	default:
		return fmt.Errorf("unsupported export kind: %s", input.Kind)
	}
}

func (j *ExportJob) generateCSV(ctx context.Context, job *models.Job, input *ExportInput) error {
	// For MVP, create a placeholder export
	// In production, this would query parcels and generate CSV
	exportID := uuid.New()
	objectKey := fmt.Sprintf("exports/%s/%s.csv", job.TenantID.String(), exportID.String())

	// Create placeholder CSV content
	csvContent := "parcelId,apn,acres\n"

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
	})
	return j.jobRepo.UpdateStatus(ctx, job.JobID, "completed", string(outputJSON), nil)
}

func (j *ExportJob) generateGeoJSON(ctx context.Context, job *models.Job, input *ExportInput) error {
	// For MVP, create a placeholder export
	exportID := uuid.New()
	objectKey := fmt.Sprintf("exports/%s/%s.geojson", job.TenantID.String(), exportID.String())

	// Create placeholder GeoJSON
	geojsonContent := `{"type":"FeatureCollection","features":[]}`

	// Upload to storage
	if j.tileStorage != nil {
		if err := j.tileStorage.UploadExport(ctx, job.TenantID.String(), exportID.String(), "geojson", []byte(geojsonContent), "application/geo+json"); err != nil {
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
		"exportId":  exportID.String(),
		"objectKey": objectKey,
	})
	return j.jobRepo.UpdateStatus(ctx, job.JobID, "completed", string(outputJSON), nil)
}
