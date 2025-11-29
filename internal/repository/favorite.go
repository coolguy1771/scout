package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"go.uber.org/zap"

	"github.com/coolguy1771/scout/internal/models"
)

type FavoriteRepository struct {
	db     *sqlx.DB
	logger *zap.Logger
}

func NewFavoriteRepository(db *sqlx.DB, logger *zap.Logger) *FavoriteRepository {
	return &FavoriteRepository{db: db, logger: logger}
}

// Create creates a new favorite
func (r *FavoriteRepository) Create(ctx context.Context, favorite *models.Favorite) error {
	query := `
		INSERT INTO favorites (favorite_id, tenant_id, user_id, parcel_id, created_at)
		VALUES (:favorite_id, :tenant_id, :user_id, :parcel_id, :created_at)
	`
	_, err := r.db.NamedExecContext(ctx, query, favorite)
	if err != nil {
		// Check for unique constraint violation (duplicate favorite)
		var pgErr *pq.Error
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return fmt.Errorf("favorite already exists: %w", err)
		}
		return fmt.Errorf("failed to create favorite: %w", err)
	}
	return nil
}

// GetByID retrieves a favorite by ID with tenant check
func (r *FavoriteRepository) GetByID(ctx context.Context, favoriteID uuid.UUID, tenantID uuid.UUID) (*models.Favorite, error) {
	var favorite models.Favorite
	err := r.db.GetContext(ctx, &favorite, `
		SELECT favorite_id, tenant_id, user_id, parcel_id, created_at
		FROM favorites
		WHERE favorite_id = $1 AND tenant_id = $2
	`, favoriteID, tenantID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("favorite not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get favorite: %w", err)
	}
	return &favorite, nil
}

// ListByUser lists all favorites for a user with pagination
func (r *FavoriteRepository) ListByUser(ctx context.Context, userID uuid.UUID, tenantID uuid.UUID, limit, offset int) ([]models.Favorite, error) {
	var favorites []models.Favorite
	err := r.db.SelectContext(ctx, &favorites, `
		SELECT favorite_id, tenant_id, user_id, parcel_id, created_at
		FROM favorites
		WHERE user_id = $1 AND tenant_id = $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`, userID, tenantID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list favorites: %w", err)
	}
	return favorites, nil
}

// ListByParcel lists all users who favorited a parcel (optional)
func (r *FavoriteRepository) ListByParcel(ctx context.Context, parcelID uuid.UUID, tenantID uuid.UUID) ([]models.Favorite, error) {
	var favorites []models.Favorite
	err := r.db.SelectContext(ctx, &favorites, `
		SELECT favorite_id, tenant_id, user_id, parcel_id, created_at
		FROM favorites
		WHERE parcel_id = $1 AND tenant_id = $2
		ORDER BY created_at DESC
	`, parcelID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to list favorites by parcel: %w", err)
	}
	return favorites, nil
}

// Delete deletes a favorite
func (r *FavoriteRepository) Delete(ctx context.Context, favoriteID uuid.UUID, tenantID uuid.UUID) error {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM favorites
		WHERE favorite_id = $1 AND tenant_id = $2
	`, favoriteID, tenantID)
	if err != nil {
		return fmt.Errorf("failed to delete favorite: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("favorite not found")
	}
	return nil
}

// Exists checks if a favorite exists for a user and parcel
func (r *FavoriteRepository) Exists(ctx context.Context, userID uuid.UUID, parcelID uuid.UUID, tenantID uuid.UUID) (bool, error) {
	var exists bool
	err := r.db.GetContext(ctx, &exists, `
		SELECT EXISTS(
			SELECT 1 FROM favorites
			WHERE user_id = $1 AND parcel_id = $2 AND tenant_id = $3
		)
	`, userID, parcelID, tenantID)
	if err != nil {
		return false, fmt.Errorf("failed to check favorite existence: %w", err)
	}
	return exists, nil
}

// CountByUser counts total favorites for a user
func (r *FavoriteRepository) CountByUser(ctx context.Context, userID uuid.UUID, tenantID uuid.UUID) (int, error) {
	var count int
	err := r.db.GetContext(ctx, &count, `
		SELECT COUNT(*) FROM favorites
		WHERE user_id = $1 AND tenant_id = $2
	`, userID, tenantID)
	if err != nil {
		return 0, fmt.Errorf("failed to count favorites: %w", err)
	}
	return count, nil
}

