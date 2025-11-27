package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/coolguy1771/scout/internal/models"
)

type LayerRepository struct {
	db     *sqlx.DB
	logger *zap.Logger
}

func NewLayerRepository(db *sqlx.DB, logger *zap.Logger) *LayerRepository {
	return &LayerRepository{db: db, logger: logger}
}

// Create creates a new private layer record
func (r *LayerRepository) Create(ctx context.Context, layer *models.PrivateLayer) error {
	query := `
		INSERT INTO private_layers (
			layer_id, tenant_id, name, layer_type, file_name, file_size,
			status, object_key, created_by, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW()
		)
		RETURNING created_at, updated_at
	`

	err := r.db.QueryRowContext(ctx, query,
		layer.LayerID,
		layer.TenantID,
		layer.Name,
		layer.LayerType,
		layer.FileName,
		layer.FileSize,
		layer.Status,
		layer.ObjectKey,
		layer.CreatedBy,
	).Scan(&layer.CreatedAt, &layer.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create layer: %w", err)
	}

	return nil
}

// GetByID retrieves a layer by ID with tenant check
func (r *LayerRepository) GetByID(ctx context.Context, layerID uuid.UUID, tenantID uuid.UUID) (*models.PrivateLayer, error) {
	var layer models.PrivateLayer
	query := `
		SELECT layer_id, tenant_id, name, layer_type, file_name, file_size,
		       status, data_run_id, object_key, error_message,
		       created_by, created_at, updated_at
		FROM private_layers
		WHERE layer_id = $1 AND tenant_id = $2
	`

	err := r.db.GetContext(ctx, &layer, query, layerID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get layer: %w", err)
	}

	return &layer, nil
}

// ListByTenant lists all layers for a tenant
func (r *LayerRepository) ListByTenant(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]models.PrivateLayer, error) {
	query := `
		SELECT layer_id, tenant_id, name, layer_type, file_name, file_size,
		       status, data_run_id, object_key, error_message,
		       created_by, created_at, updated_at
		FROM private_layers
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	var layers []models.PrivateLayer
	err := r.db.SelectContext(ctx, &layers, query, tenantID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list layers: %w", err)
	}

	return layers, nil
}

// UpdateStatus updates the status and related fields of a layer
func (r *LayerRepository) UpdateStatus(ctx context.Context, layerID uuid.UUID, status string, dataRunID *uuid.UUID, errorMessage string) error {
	query := `
		UPDATE private_layers
		SET status = $1,
		    data_run_id = $2,
		    error_message = $3,
		    updated_at = NOW()
		WHERE layer_id = $4
	`

	result, err := r.db.ExecContext(ctx, query, status, dataRunID, errorMessage, layerID)
	if err != nil {
		return fmt.Errorf("failed to update layer status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("layer not found")
	}

	return nil
}

// Update updates a layer's name and type
func (r *LayerRepository) Update(ctx context.Context, layerID uuid.UUID, tenantID uuid.UUID, name string) error {
	query := `
		UPDATE private_layers
		SET name = $1,
		    updated_at = NOW()
		WHERE layer_id = $2 AND tenant_id = $3
	`
	result, err := r.db.ExecContext(ctx, query, name, layerID, tenantID)
	if err != nil {
		return fmt.Errorf("failed to update layer: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("layer not found")
	}
	return nil
}

// Delete deletes a layer (only by tenant admin)
func (r *LayerRepository) Delete(ctx context.Context, layerID uuid.UUID, tenantID uuid.UUID) error {
	query := `
		DELETE FROM private_layers
		WHERE layer_id = $1 AND tenant_id = $2
	`

	result, err := r.db.ExecContext(ctx, query, layerID, tenantID)
	if err != nil {
		return fmt.Errorf("failed to delete layer: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("layer not found")
	}

	return nil
}
