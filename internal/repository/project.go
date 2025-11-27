package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/coolguy1771/scout/internal/models"
)

type ProjectRepository struct {
	db     *sqlx.DB
	logger *zap.Logger
}

func NewProjectRepository(db *sqlx.DB, logger *zap.Logger) *ProjectRepository {
	return &ProjectRepository{db: db, logger: logger}
}

// Create creates a new project
func (r *ProjectRepository) Create(ctx context.Context, project *models.Project) error {
	query := `
		INSERT INTO projects (project_id, tenant_id, name, created_by, created_at)
		VALUES (:project_id, :tenant_id, :name, :created_by, :created_at)
		RETURNING project_id
	`
	_, err := r.db.NamedExecContext(ctx, query, project)
	if err != nil {
		return fmt.Errorf("failed to create project: %w", err)
	}
	return nil
}

// GetByID retrieves a project by ID
func (r *ProjectRepository) GetByID(ctx context.Context, projectID uuid.UUID, tenantID uuid.UUID) (*models.Project, error) {
	var project models.Project
	err := r.db.GetContext(ctx, &project, `
		SELECT project_id, tenant_id, name, created_by, created_at
		FROM projects
		WHERE project_id = $1 AND tenant_id = $2
	`, projectID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}
	return &project, nil
}

// ListByTenant lists all projects for a tenant
func (r *ProjectRepository) ListByTenant(ctx context.Context, tenantID uuid.UUID) ([]models.Project, error) {
	var projects []models.Project
	err := r.db.SelectContext(ctx, &projects, `
		SELECT project_id, tenant_id, name, created_by, created_at
		FROM projects
		WHERE tenant_id = $1
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}
	return projects, nil
}

// Update updates a project
func (r *ProjectRepository) Update(ctx context.Context, project *models.Project) error {
	_, err := r.db.NamedExecContext(ctx, `
		UPDATE projects
		SET name = :name
		WHERE project_id = :project_id AND tenant_id = :tenant_id
	`, project)
	if err != nil {
		return fmt.Errorf("failed to update project: %w", err)
	}
	return nil
}

// Delete deletes a project
func (r *ProjectRepository) Delete(ctx context.Context, projectID uuid.UUID, tenantID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM projects
		WHERE project_id = $1 AND tenant_id = $2
	`, projectID, tenantID)
	if err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}
	return nil
}

// CreateSavedSearch creates a new saved search
func (r *ProjectRepository) CreateSavedSearch(ctx context.Context, search *models.SavedSearch) error {
	query := `
		INSERT INTO saved_searches (
			saved_search_id, tenant_id, project_id, name, query_json, 
			scoring_profile_id, dataset_version_id, created_by, created_at
		)
		VALUES (
			:saved_search_id, :tenant_id, :project_id, :name, :query_json, 
			:scoring_profile_id, :dataset_version_id, :created_by, :created_at
		)
	`
	_, err := r.db.NamedExecContext(ctx, query, search)
	if err != nil {
		return fmt.Errorf("failed to create saved search: %w", err)
	}
	return nil
}

// GetSavedSearchByID retrieves a saved search by ID
func (r *ProjectRepository) GetSavedSearchByID(ctx context.Context, savedSearchID uuid.UUID, tenantID uuid.UUID) (*models.SavedSearch, error) {
	var search models.SavedSearch
	err := r.db.GetContext(ctx, &search, `
		SELECT saved_search_id, tenant_id, project_id, name, query_json, 
		       scoring_profile_id, dataset_version_id, created_by, created_at
		FROM saved_searches
		WHERE saved_search_id = $1 AND tenant_id = $2
	`, savedSearchID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get saved search: %w", err)
	}
	return &search, nil
}

// ListSavedSearches lists saved searches for a tenant
func (r *ProjectRepository) ListSavedSearches(ctx context.Context, tenantID uuid.UUID, projectID *uuid.UUID) ([]models.SavedSearch, error) {
	query := `
		SELECT saved_search_id, tenant_id, project_id, name, query_json, 
		       scoring_profile_id, dataset_version_id, created_by, created_at
		FROM saved_searches
		WHERE tenant_id = $1
	`
	args := []interface{}{tenantID}
	if projectID != nil {
		query += " AND project_id = $2"
		args = append(args, *projectID)
	}
	query += " ORDER BY created_at DESC"

	var searches []models.SavedSearch
	err := r.db.SelectContext(ctx, &searches, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list saved searches: %w", err)
	}
	return searches, nil
}

// UpdateSavedSearch updates a saved search
func (r *ProjectRepository) UpdateSavedSearch(ctx context.Context, search *models.SavedSearch) error {
	query := `
		UPDATE saved_searches
		SET name = $1,
		    query_json = $2,
		    project_id = $3,
		    scoring_profile_id = $4,
		    dataset_version_id = $5
		WHERE saved_search_id = $6 AND tenant_id = $7
	`
	result, err := r.db.ExecContext(ctx, query,
		search.Name,
		search.QueryJSON,
		search.ProjectID,
		search.ScoringProfileID,
		search.DatasetVersionID,
		search.SavedSearchID,
		search.TenantID,
	)
	if err != nil {
		return fmt.Errorf("failed to update saved search: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("saved search not found")
	}
	return nil
}

// DeleteSavedSearch deletes a saved search
func (r *ProjectRepository) DeleteSavedSearch(ctx context.Context, savedSearchID uuid.UUID, tenantID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM saved_searches
		WHERE saved_search_id = $1 AND tenant_id = $2
	`, savedSearchID, tenantID)
	if err != nil {
		return fmt.Errorf("failed to delete saved search: %w", err)
	}
	return nil
}
