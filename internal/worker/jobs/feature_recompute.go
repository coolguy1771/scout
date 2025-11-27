package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	"github.com/coolguy1771/scout/internal/features"
	"github.com/coolguy1771/scout/internal/models"
	"github.com/coolguy1771/scout/internal/repository"
)

type FeatureRecomputeJob struct {
	jobRepo        *repository.JobRepository
	featureService *features.Service
	logger         *zap.Logger
}

func NewFeatureRecomputeJob(jobRepo *repository.JobRepository, featureService *features.Service, logger *zap.Logger) *FeatureRecomputeJob {
	return &FeatureRecomputeJob{
		jobRepo:        jobRepo,
		featureService: featureService,
		logger:         logger,
	}
}

func (j *FeatureRecomputeJob) Process(ctx context.Context, job *models.Job) error {
	// Parse input
	var input features.RecomputeInput
	if err := json.Unmarshal([]byte(job.InputJSON), &input); err != nil {
		return fmt.Errorf("failed to parse feature recompute input: %w", err)
	}

	// Recompute features
	if err := j.featureService.Recompute(ctx, &input); err != nil {
		return fmt.Errorf("failed to recompute features: %w", err)
	}

	// Update job as completed
	outputJSON, _ := json.Marshal(map[string]interface{}{
		"message": "Feature recomputation completed",
	})
	return j.jobRepo.UpdateStatus(ctx, job.JobID, "completed", string(outputJSON), nil)
}
