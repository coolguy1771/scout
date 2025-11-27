package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/coolguy1771/scout/internal/models"
)

type JobRepository struct {
	db     *sqlx.DB
	logger *zap.Logger
}

func NewJobRepository(db *sqlx.DB, logger *zap.Logger) *JobRepository {
	return &JobRepository{db: db, logger: logger}
}

// GetDB returns the underlying database connection
func (r *JobRepository) GetDB() *sqlx.DB {
	return r.db
}

// Create creates a new job
func (r *JobRepository) Create(ctx context.Context, job *models.Job) error {
	query := `
		INSERT INTO jobs (
			job_id, tenant_id, type, status, input_json, output_json, 
			error_json, attempts, created_at, updated_at
		)
		VALUES (
			:job_id, :tenant_id, :type, :status, :input_json, :output_json, 
			:error_json, :attempts, :created_at, :updated_at
		)
	`
	_, err := r.db.NamedExecContext(ctx, query, job)
	if err != nil {
		return fmt.Errorf("failed to create job: %w", err)
	}
	return nil
}

// GetByID retrieves a job by ID
func (r *JobRepository) GetByID(ctx context.Context, jobID uuid.UUID, tenantID uuid.UUID) (*models.Job, error) {
	var job models.Job
	err := r.db.GetContext(ctx, &job, `
		SELECT job_id, tenant_id, type, status, input_json, output_json, 
		       error_json, attempts, created_at, updated_at
		FROM jobs
		WHERE job_id = $1 AND tenant_id = $2
	`, jobID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get job: %w", err)
	}
	return &job, nil
}

// PollPending polls for pending jobs (for worker)
func (r *JobRepository) PollPending(ctx context.Context, limit int) ([]models.Job, error) {
	if limit <= 0 {
		limit = 10
	}

	var jobs []models.Job
	err := r.db.SelectContext(ctx, &jobs, `
		SELECT job_id, tenant_id, type, status, input_json, output_json, 
		       error_json, attempts, created_at, updated_at
		FROM jobs
		WHERE status = 'pending'
		ORDER BY created_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to poll pending jobs: %w", err)
	}

	// Mark as processing
	if len(jobs) > 0 {
		jobIDs := make([]uuid.UUID, len(jobs))
		for i := range jobs {
			jobIDs[i] = jobs[i].JobID
		}

		query, args, err := sqlx.In(`
			UPDATE jobs
			SET status = 'processing', updated_at = NOW()
			WHERE job_id IN (?)
		`, jobIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to build update query: %w", err)
		}

		query = r.db.Rebind(query)
		_, err = r.db.ExecContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("failed to update job status: %w", err)
		}

		// Update status in returned jobs
		for i := range jobs {
			jobs[i].Status = "processing"
			jobs[i].UpdatedAt = time.Now()
		}
	}

	return jobs, nil
}

// UpdateStatus updates job status
func (r *JobRepository) UpdateStatus(ctx context.Context, jobID uuid.UUID, status string, outputJSON, errorJSON interface{}) error {
	var outputStr, errorStr sql.NullString

	if outputJSON != nil {
		outputBytes, err := json.Marshal(outputJSON)
		if err != nil {
			return fmt.Errorf("failed to marshal output: %w", err)
		}
		outputStr = sql.NullString{String: string(outputBytes), Valid: true}
	}

	if errorJSON != nil {
		errorBytes, err := json.Marshal(errorJSON)
		if err != nil {
			return fmt.Errorf("failed to marshal error: %w", err)
		}
		errorStr = sql.NullString{String: string(errorBytes), Valid: true}
	}

	_, err := r.db.ExecContext(ctx, `
		UPDATE jobs
		SET status = $1, 
		    output_json = $2, 
		    error_json = $3, 
		    updated_at = NOW()
		WHERE job_id = $4
	`, status, outputStr, errorStr, jobID)
	if err != nil {
		return fmt.Errorf("failed to update job status: %w", err)
	}
	return nil
}

// IncrementAttempts increments the attempt count for a job
func (r *JobRepository) IncrementAttempts(ctx context.Context, jobID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE jobs
		SET attempts = attempts + 1, updated_at = NOW()
		WHERE job_id = $1
	`, jobID)
	if err != nil {
		return fmt.Errorf("failed to increment attempts: %w", err)
	}
	return nil
}

// ListByTenant lists all jobs for a tenant
func (r *JobRepository) ListByTenant(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]models.Job, error) {
	var jobs []models.Job
	err := r.db.SelectContext(ctx, &jobs, `
		SELECT job_id, tenant_id, type, status, input_json, output_json, 
		       error_json, attempts, created_at, updated_at
		FROM jobs
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list jobs: %w", err)
	}
	return jobs, nil
}
