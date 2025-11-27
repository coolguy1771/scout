package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/coolguy1771/scout/internal/models"
)

type ScoringProfileRepository struct {
	db     *sqlx.DB
	logger *zap.Logger
}

func NewScoringProfileRepository(db *sqlx.DB, logger *zap.Logger) *ScoringProfileRepository {
	return &ScoringProfileRepository{db: db, logger: logger}
}

// Create creates a new scoring profile
func (r *ScoringProfileRepository) Create(ctx context.Context, profile *models.ScoringProfile) error {
	query := `
		INSERT INTO scoring_profiles (
			scoring_profile_id, tenant_id, name, version, weights_json, 
			thresholds_json, hard_constraints_json, created_by, created_at
		)
		VALUES (
			:scoring_profile_id, :tenant_id, :name, :version, :weights_json, 
			:thresholds_json, :hard_constraints_json, :created_by, :created_at
		)
	`
	_, err := r.db.NamedExecContext(ctx, query, profile)
	if err != nil {
		return fmt.Errorf("failed to create scoring profile: %w", err)
	}
	return nil
}

// GetByID retrieves a scoring profile by ID
func (r *ScoringProfileRepository) GetByID(ctx context.Context, profileID uuid.UUID, tenantID uuid.UUID) (*models.ScoringProfile, error) {
	var profile models.ScoringProfile
	err := r.db.GetContext(ctx, &profile, `
		SELECT scoring_profile_id, tenant_id, name, version, weights_json, 
		       thresholds_json, hard_constraints_json, created_by, created_at
		FROM scoring_profiles
		WHERE scoring_profile_id = $1 AND tenant_id = $2
	`, profileID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get scoring profile: %w", err)
	}
	return &profile, nil
}

// ListByTenant lists all scoring profiles for a tenant
func (r *ScoringProfileRepository) ListByTenant(ctx context.Context, tenantID uuid.UUID) ([]models.ScoringProfile, error) {
	var profiles []models.ScoringProfile
	err := r.db.SelectContext(ctx, &profiles, `
		SELECT scoring_profile_id, tenant_id, name, version, weights_json, 
		       thresholds_json, hard_constraints_json, created_by, created_at
		FROM scoring_profiles
		WHERE tenant_id = $1
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to list scoring profiles: %w", err)
	}
	return profiles, nil
}
