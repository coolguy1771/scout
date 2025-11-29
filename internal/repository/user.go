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

type UserRepository struct {
	db     *sqlx.DB
	logger *zap.Logger
}

func NewUserRepository(db *sqlx.DB, logger *zap.Logger) *UserRepository {
	return &UserRepository{db: db, logger: logger}
}

// Create creates a new user
func (r *UserRepository) Create(ctx context.Context, user *models.User) error {
	query := `
		INSERT INTO users (user_id, email, name, created_at)
		VALUES (:user_id, :email, :name, :created_at)
	`
	_, err := r.db.NamedExecContext(ctx, query, user)
	if err != nil {
		var pgErr *pq.Error
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return fmt.Errorf("user with email already exists: %w", err)
		}
		return fmt.Errorf("failed to create user: %w", err)
	}
	return nil
}

// GetByID retrieves a user by ID
func (r *UserRepository) GetByID(ctx context.Context, userID uuid.UUID) (*models.User, error) {
	var user models.User
	err := r.db.GetContext(ctx, &user, `
		SELECT user_id, email, name, created_at
		FROM users
		WHERE user_id = $1
	`, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("user not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return &user, nil
}

// GetByEmail retrieves a user by email
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	var user models.User
	err := r.db.GetContext(ctx, &user, `
		SELECT user_id, email, name, created_at
		FROM users
		WHERE email = $1
	`, email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("user not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return &user, nil
}

// List lists users with pagination (tenant-scoped via memberships)
func (r *UserRepository) List(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]models.User, error) {
	var users []models.User
	err := r.db.SelectContext(ctx, &users, `
		SELECT DISTINCT u.user_id, u.email, u.name, u.created_at
		FROM users u
		INNER JOIN memberships m ON u.user_id = m.user_id
		WHERE m.tenant_id = $1
		ORDER BY u.created_at DESC
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	return users, nil
}

// Update updates a user
func (r *UserRepository) Update(ctx context.Context, user *models.User) error {
	query := `
		UPDATE users
		SET email = :email, name = :name
		WHERE user_id = :user_id
	`
	result, err := r.db.NamedExecContext(ctx, query, user)
	if err != nil {
		var pgErr *pq.Error
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return fmt.Errorf("user with email already exists: %w", err)
		}
		return fmt.Errorf("failed to update user: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

// Delete deletes a user
func (r *UserRepository) Delete(ctx context.Context, userID uuid.UUID) error {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM users
		WHERE user_id = $1
	`, userID)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}
