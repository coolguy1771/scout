package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/coolguy1771/scout/internal/config"
	"github.com/coolguy1771/scout/internal/database"
	"github.com/coolguy1771/scout/internal/features"
	"github.com/coolguy1771/scout/internal/models"
	"github.com/coolguy1771/scout/internal/repository"
	"github.com/coolguy1771/scout/internal/storage"
	"github.com/coolguy1771/scout/internal/worker/jobs"
)

// Worker processes background jobs
type Worker struct {
	config         *config.Config
	db             *database.DB
	logger         *zap.Logger
	jobRepo        *repository.JobRepository
	exportJob      *jobs.ExportJob
	tileBuildJob   *jobs.TileBuildJob
	featureJob     *jobs.FeatureRecomputeJob
	layerUploadJob *jobs.LayerUploadJob
	stop           chan struct{}
	pollInterval   time.Duration
	maxRetries     int
}

// New creates a new worker instance
func New(cfg *config.Config, db *database.DB, logger *zap.Logger) (*Worker, error) {
	jobRepo := repository.NewJobRepository(db.DB, logger)
	exportRepo := repository.NewExportRepository(db.DB, logger)

	// Initialize storage (may be nil if not configured)
	var tileStorage *storage.TileStorage
	if s3Storage, err := storage.NewS3Storage(cfg.S3, logger); err == nil {
		tileStorage = storage.NewTileStorage(s3Storage)
	}

	// Initialize feature service
	parcelRepo := repository.NewParcelRepository(db.DB, logger)
	parcelFeatureRepo := repository.NewParcelFeatureRepository(db.DB, logger)
	featureService := features.NewService(parcelRepo, parcelFeatureRepo, logger)

	exportJob := jobs.NewExportJob(jobRepo, exportRepo, tileStorage, logger)
	tileBuildJob := jobs.NewTileBuildJob(jobRepo, tileStorage, logger)
	featureJob := jobs.NewFeatureRecomputeJob(jobRepo, featureService, logger)

	// Initialize layer upload job
	var layerUploadJob *jobs.LayerUploadJob
	if s3Storage, err := storage.NewS3Storage(cfg.S3, logger); err == nil {
		layerRepo := repository.NewLayerRepository(db.DB, logger)
		layerUploadJob = jobs.NewLayerUploadJob(jobRepo, layerRepo, s3Storage, logger)
	}

	return &Worker{
		config:         cfg,
		db:             db,
		logger:         logger,
		jobRepo:        jobRepo,
		exportJob:      exportJob,
		tileBuildJob:   tileBuildJob,
		featureJob:     featureJob,
		layerUploadJob: layerUploadJob,
		stop:           make(chan struct{}),
		pollInterval:   5 * time.Second,
		maxRetries:     3,
	}, nil
}

// Start begins processing jobs
func (w *Worker) Start(ctx context.Context) error {
	w.logger.Info("Worker started")

	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	// Main worker loop
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("Worker context cancelled")
			return ctx.Err()
		case <-w.stop:
			w.logger.Info("Worker stop signal received")
			return nil
		case <-ticker.C:
			// Poll for pending jobs
			if err := w.processPendingJobs(ctx); err != nil {
				w.logger.Error("Error processing jobs", zap.Error(err))
			}
		}
	}
}

// Shutdown gracefully shuts down the worker
func (w *Worker) Shutdown(ctx context.Context) error {
	w.logger.Info("Shutting down worker...")
	close(w.stop)
	return nil
}

// processPendingJobs polls for and processes pending jobs
func (w *Worker) processPendingJobs(ctx context.Context) error {
	// Poll for pending jobs (limit to 10 at a time)
	pendingJobs, err := w.jobRepo.PollPending(ctx, 10)
	if err != nil {
		return fmt.Errorf("failed to poll pending jobs: %w", err)
	}

	if len(pendingJobs) == 0 {
		return nil
	}

	w.logger.Info("Processing jobs", zap.Int("count", len(pendingJobs)))

	// Process each job
	for i := range pendingJobs {
		job := &pendingJobs[i]
		if err := w.processJob(ctx, job); err != nil {
			w.logger.Error("Failed to process job",
				zap.String("jobId", job.JobID.String()),
				zap.Error(err))

			// Increment attempts
			if err := w.jobRepo.IncrementAttempts(ctx, job.JobID); err != nil {
				w.logger.Error("Failed to increment attempts", zap.Error(err))
			}

			// Mark as failed if max retries reached
			if job.Attempts >= w.maxRetries {
				errorJSON, _ := json.Marshal(map[string]interface{}{
					"error":    err.Error(),
					"attempts": job.Attempts,
				})
				if err := w.jobRepo.UpdateStatus(ctx, job.JobID, "failed", nil, string(errorJSON)); err != nil {
					w.logger.Error("Failed to update job status", zap.Error(err))
				}
			} else {
				// Reset to pending for retry
				if err := w.jobRepo.UpdateStatus(ctx, job.JobID, "pending", nil, nil); err != nil {
					w.logger.Error("Failed to reset job status", zap.Error(err))
				}
			}
		}
	}

	return nil
}

// processJob processes a single job
func (w *Worker) processJob(ctx context.Context, job *models.Job) error {
	w.logger.Info("Processing job",
		zap.String("jobId", job.JobID.String()),
		zap.String("type", job.Type))

	switch job.Type {
	case "export":
		return w.exportJob.Process(ctx, job)
	case "tile_build":
		return w.tileBuildJob.Process(ctx, job)
	case "feature_recompute":
		return w.featureJob.Process(ctx, job)
	case "layer_upload":
		if w.layerUploadJob == nil {
			return fmt.Errorf("layer upload job not initialized (S3 storage not configured)")
		}
		return w.layerUploadJob.Process(ctx, job)
	default:
		return fmt.Errorf("unknown job type: %s", job.Type)
	}
}
