package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/coolguy1771/scout/internal/models"
)

type TenantRepository struct {
	db     *sqlx.DB
	logger *zap.Logger
}

func NewTenantRepository(db *sqlx.DB, logger *zap.Logger) *TenantRepository {
	return &TenantRepository{db: db, logger: logger}
}

// Create creates a new tenant
func (r *TenantRepository) Create(ctx context.Context, tenant *models.Tenant) error {
	query := `
		INSERT INTO tenants (tenant_id, name, created_at)
		VALUES (:tenant_id, :name, :created_at)
	`
	_, err := r.db.NamedExecContext(ctx, query, tenant)
	if err != nil {
		return fmt.Errorf("failed to create tenant: %w", err)
	}
	return nil
}

// GetByID retrieves a tenant by ID
func (r *TenantRepository) GetByID(ctx context.Context, tenantID uuid.UUID) (*models.Tenant, error) {
	var tenant models.Tenant
	err := r.db.GetContext(ctx, &tenant, `
		SELECT tenant_id, name, created_at
		FROM tenants
		WHERE tenant_id = $1
	`, tenantID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("tenant not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}
	return &tenant, nil
}

// List lists all tenants (for super admin)
func (r *TenantRepository) List(ctx context.Context, limit, offset int) ([]models.Tenant, error) {
	var tenants []models.Tenant
	err := r.db.SelectContext(ctx, &tenants, `
		SELECT tenant_id, name, created_at
		FROM tenants
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list tenants: %w", err)
	}
	return tenants, nil
}

// Update updates a tenant
func (r *TenantRepository) Update(ctx context.Context, tenant *models.Tenant) error {
	query := `
		UPDATE tenants
		SET name = :name
		WHERE tenant_id = :tenant_id
	`
	result, err := r.db.NamedExecContext(ctx, query, tenant)
	if err != nil {
		return fmt.Errorf("failed to update tenant: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("tenant not found")
	}
	return nil
}

// Delete deletes a tenant
func (r *TenantRepository) Delete(ctx context.Context, tenantID uuid.UUID) error {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM tenants
		WHERE tenant_id = $1
	`, tenantID)
	if err != nil {
		return fmt.Errorf("failed to delete tenant: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("tenant not found")
	}
	return nil
}

// ListMembers lists all members of a tenant
func (r *TenantRepository) ListMembers(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]models.Membership, error) {
	var memberships []models.Membership
	err := r.db.SelectContext(ctx, &memberships, `
		SELECT membership_id, tenant_id, user_id, role, created_at
		FROM memberships
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list members: %w", err)
	}
	return memberships, nil
}
