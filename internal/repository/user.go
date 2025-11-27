package repository

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

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
		ON CONFLICT (email) DO NOTHING
	`
	_, err := r.db.NamedExecContext(ctx, query, user)
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}
	return nil
}
