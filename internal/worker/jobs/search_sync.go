package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	"github.com/coolguy1771/scout/internal/models"
	"github.com/coolguy1771/scout/internal/repository"
	opensearch "github.com/coolguy1771/scout/internal/search/opensearch"
)

type SearchSyncJob struct {
	jobRepo      *repository.JobRepository
	parcelRepo   *repository.ParcelRepository
	indexer      *opensearch.ParcelIndexer
	indexManager *opensearch.IndexManager
	logger       *zap.Logger
}

func NewSearchSyncJob(
	jobRepo *repository.JobRepository,
	parcelRepo *repository.ParcelRepository,
	indexer *opensearch.ParcelIndexer,
	indexManager *opensearch.IndexManager,
	logger *zap.Logger,
) *SearchSyncJob {
	return &SearchSyncJob{
		jobRepo:      jobRepo,
		parcelRepo:   parcelRepo,
		indexer:      indexer,
		indexManager: indexManager,
		logger:       logger,
	}
}

type SearchSyncInput struct {
	BatchSize     int    `json:"batchSize,omitempty"`     // Default 1000
	StateFIPS     string `json:"stateFips,omitempty"`     // Optional: sync specific state
	RecreateIndex bool   `json:"recreateIndex,omitempty"` // Recreate index before syncing
}

func (j *SearchSyncJob) Process(ctx context.Context, job *models.Job) error {
	// Parse input
	var input SearchSyncInput
	if err := json.Unmarshal([]byte(job.InputJSON), &input); err != nil {
		return fmt.Errorf("failed to parse search sync input: %w", err)
	}

	// Set defaults
	batchSize := input.BatchSize
	if batchSize <= 0 {
		batchSize = 1000
	}

	// Recreate index if requested
	if input.RecreateIndex {
		j.logger.Info("Recreating OpenSearch index")
		if err := j.indexManager.DeleteParcelIndex(ctx); err != nil {
			j.logger.Warn("Failed to delete existing index, continuing", zap.Error(err))
		}
		if err := j.indexManager.CreateParcelIndex(ctx); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	} else {
		// Ensure index exists
		exists, err := j.indexManager.IndexExists(ctx, opensearch.ParcelIndexName)
		if err != nil {
			return fmt.Errorf("failed to check if index exists: %w", err)
		}
		if !exists {
			j.logger.Info("Index does not exist, creating it")
			if err := j.indexManager.CreateParcelIndex(ctx); err != nil {
				return fmt.Errorf("failed to create index: %w", err)
			}
		}
	}

	// Fetch parcels in batches and index them
	offset := 0
	totalIndexed := 0

	for {
		// Fetch batch of parcels
		parcels, err := j.fetchParcelsBatch(ctx, batchSize, offset, input.StateFIPS)
		if err != nil {
			return fmt.Errorf("failed to fetch parcels batch: %w", err)
		}

		if len(parcels) == 0 {
			break // No more parcels
		}

		// Index batch
		if err := j.indexer.BulkIndexParcels(ctx, parcels); err != nil {
			return fmt.Errorf("failed to index parcels batch: %w", err)
		}

		totalIndexed += len(parcels)
		j.logger.Info("Indexed batch",
			zap.Int("batchSize", len(parcels)),
			zap.Int("totalIndexed", totalIndexed),
			zap.Int("offset", offset))

		// Check if we've processed all parcels
		if len(parcels) < batchSize {
			break
		}

		offset += batchSize
	}

	// Update job as completed
	outputJSON, _ := json.Marshal(map[string]interface{}{
		"totalIndexed": totalIndexed,
		"batchSize":    batchSize,
	})
	return j.jobRepo.UpdateStatus(ctx, job.JobID, "completed", string(outputJSON), nil)
}

// fetchParcelsBatch fetches a batch of parcels from the database
func (j *SearchSyncJob) fetchParcelsBatch(ctx context.Context, limit, offset int, stateFIPS string) ([]models.Parcel, error) {
	query := `
		SELECT parcel_id, tenant_id, apn, acres, zoning_raw, zoning_tags,
		       jurisdiction, state_fips,
		       ST_AsGeoJSON(geom) as geom,
		       ST_AsGeoJSON(centroid) as centroid,
		       created_at, updated_at
		FROM parcels
		WHERE 1=1
	`
	args := []interface{}{}
	argIdx := 1

	if stateFIPS != "" {
		query += fmt.Sprintf(" AND state_fips = $%d", argIdx)
		args = append(args, stateFIPS)
		argIdx++
	}

	query += fmt.Sprintf(" ORDER BY parcel_id LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	var parcels []models.Parcel
	rows, err := j.parcelRepo.GetDB().QueryxContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query parcels: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var parcel models.Parcel
		if err := rows.StructScan(&parcel); err != nil {
			j.logger.Warn("Failed to scan parcel", zap.Error(err))
			continue
		}
		parcels = append(parcels, parcel)
	}

	return parcels, nil
}


