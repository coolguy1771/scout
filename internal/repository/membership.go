package repository

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

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
		ON CONFLICT (tenant_id, user_id) DO NOTHING
	`
	_, err := r.db.NamedExecContext(ctx, query, membership)
	if err != nil {
		return fmt.Errorf("failed to create membership: %w", err)
	}
	return nil
}
