package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/coolguy1771/scout/internal/models"
)

type ExportRepository struct {
	db     *sqlx.DB
	logger *zap.Logger
}

func NewExportRepository(db *sqlx.DB, logger *zap.Logger) *ExportRepository {
	return &ExportRepository{db: db, logger: logger}
}

// Create creates a new export
func (r *ExportRepository) Create(ctx context.Context, export *models.Export) error {
	query := `
		INSERT INTO exports (
			export_id, tenant_id, job_id, kind, object_key, content_type, created_at
		)
		VALUES (
			:export_id, :tenant_id, :job_id, :kind, :object_key, :content_type, :created_at
		)
	`
	_, err := r.db.NamedExecContext(ctx, query, export)
	if err != nil {
		return fmt.Errorf("failed to create export: %w", err)
	}
	return nil
}

// GetByID retrieves an export by ID
func (r *ExportRepository) GetByID(ctx context.Context, exportID uuid.UUID, tenantID uuid.UUID) (*models.Export, error) {
	var export models.Export
	err := r.db.GetContext(ctx, &export, `
		SELECT export_id, tenant_id, job_id, kind, object_key, content_type, created_at
		FROM exports
		WHERE export_id = $1 AND tenant_id = $2
	`, exportID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get export: %w", err)
	}
	return &export, nil
}

// GetByJobID retrieves exports for a job
func (r *ExportRepository) GetByJobID(ctx context.Context, jobID uuid.UUID) ([]models.Export, error) {
	var exports []models.Export
	err := r.db.SelectContext(ctx, &exports, `
		SELECT export_id, tenant_id, job_id, kind, object_key, content_type, created_at
		FROM exports
		WHERE job_id = $1
		ORDER BY created_at DESC
	`, jobID)
	if err != nil {
		return nil, fmt.Errorf("failed to get exports: %w", err)
	}
	return exports, nil
}

// ListByTenant lists all exports for a tenant
func (r *ExportRepository) ListByTenant(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]models.Export, error) {
	var exports []models.Export
	err := r.db.SelectContext(ctx, &exports, `
		SELECT export_id, tenant_id, job_id, kind, object_key, content_type, created_at
		FROM exports
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list exports: %w", err)
	}
	return exports, nil
}
