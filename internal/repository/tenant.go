package repository

import (
	"context"
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

// GetMembership retrieves a user's membership for a tenant
func (r *TenantRepository) GetMembership(ctx context.Context, userID uuid.UUID, tenantID uuid.UUID) (*models.Membership, error) {
	var membership models.Membership
	err := r.db.GetContext(ctx, &membership, `
		SELECT membership_id, tenant_id, user_id, role, created_at
		FROM memberships
		WHERE user_id = $1 AND tenant_id = $2
	`, userID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get membership: %w", err)
	}
	return &membership, nil
}

// GetUserByEmail retrieves a user by email
func (r *TenantRepository) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	var user models.User
	err := r.db.GetContext(ctx, &user, `
		SELECT user_id, email, name, created_at
		FROM users
		WHERE email = $1
	`, email)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return &user, nil
}

// Create creates a new tenant
func (r *TenantRepository) Create(ctx context.Context, tenant *models.Tenant) error {
	query := `
		INSERT INTO tenants (tenant_id, name, created_at)
		VALUES (:tenant_id, :name, :created_at)
		ON CONFLICT (tenant_id) DO NOTHING
	`
	_, err := r.db.NamedExecContext(ctx, query, tenant)
	if err != nil {
		return fmt.Errorf("failed to create tenant: %w", err)
	}
	return nil
}
