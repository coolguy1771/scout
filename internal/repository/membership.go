package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
	"github.com/lib/pq"

	"github.com/coolguy1771/scout/internal/models"
)

type MembershipRepository struct {
	db     *sqlx.DB
	logger *zap.Logger
}

func NewMembershipRepository(db *sqlx.DB, logger *zap.Logger) *MembershipRepository {
	return &MembershipRepository{db: db, logger: logger}
}

// Create creates a new membership
func (r *MembershipRepository) Create(ctx context.Context, membership *models.Membership) error {
	query := `
		INSERT INTO memberships (membership_id, tenant_id, user_id, role, created_at)
		VALUES (:membership_id, :tenant_id, :user_id, :role, :created_at)
	`
	_, err := r.db.NamedExecContext(ctx, query, membership)
	if err != nil {
		var pgErr *pq.Error
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return fmt.Errorf("membership already exists: %w", err)
		}
		return fmt.Errorf("failed to create membership: %w", err)
	}
	return nil
}

// GetByID retrieves a membership by ID with tenant check
func (r *MembershipRepository) GetByID(ctx context.Context, membershipID uuid.UUID, tenantID uuid.UUID) (*models.Membership, error) {
	var membership models.Membership
	err := r.db.GetContext(ctx, &membership, `
		SELECT membership_id, tenant_id, user_id, role, created_at
		FROM memberships
		WHERE membership_id = $1 AND tenant_id = $2
	`, membershipID, tenantID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("membership not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get membership: %w", err)
	}
	return &membership, nil
}

// GetByUserTenant retrieves a membership by user and tenant
func (r *MembershipRepository) GetByUserTenant(ctx context.Context, userID uuid.UUID, tenantID uuid.UUID) (*models.Membership, error) {
	var membership models.Membership
	err := r.db.GetContext(ctx, &membership, `
		SELECT membership_id, tenant_id, user_id, role, created_at
		FROM memberships
		WHERE user_id = $1 AND tenant_id = $2
	`, userID, tenantID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("membership not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get membership: %w", err)
	}
	return &membership, nil
}

// ListByTenant lists all memberships for a tenant
func (r *MembershipRepository) ListByTenant(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]models.Membership, error) {
	var memberships []models.Membership
	err := r.db.SelectContext(ctx, &memberships, `
		SELECT membership_id, tenant_id, user_id, role, created_at
		FROM memberships
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list memberships: %w", err)
	}
	return memberships, nil
}

// ListByUser lists all memberships for a user
func (r *MembershipRepository) ListByUser(ctx context.Context, userID uuid.UUID) ([]models.Membership, error) {
	var memberships []models.Membership
	err := r.db.SelectContext(ctx, &memberships, `
		SELECT membership_id, tenant_id, user_id, role, created_at
		FROM memberships
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list memberships: %w", err)
	}
	return memberships, nil
}

// Update updates a membership role
func (r *MembershipRepository) Update(ctx context.Context, membership *models.Membership) error {
	query := `
		UPDATE memberships
		SET role = :role
		WHERE membership_id = :membership_id AND tenant_id = :tenant_id
	`
	result, err := r.db.NamedExecContext(ctx, query, membership)
	if err != nil {
		return fmt.Errorf("failed to update membership: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("membership not found")
	}
	return nil
}

// Delete deletes a membership
func (r *MembershipRepository) Delete(ctx context.Context, membershipID uuid.UUID, tenantID uuid.UUID) error {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM memberships
		WHERE membership_id = $1 AND tenant_id = $2
	`, membershipID, tenantID)
	if err != nil {
		return fmt.Errorf("failed to delete membership: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("membership not found")
	}
	return nil
}
