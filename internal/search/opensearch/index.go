package opensearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	opensearchapi "github.com/opensearch-project/opensearch-go/v4/opensearchapi"
	"go.uber.org/zap"
)

const (
	// ParcelIndexName is the name of the parcels index
	ParcelIndexName = "parcels"
)

// IndexManager handles index operations
type IndexManager struct {
	client *Client
	logger *zap.Logger
}

// NewIndexManager creates a new index manager
func NewIndexManager(client *Client, logger *zap.Logger) *IndexManager {
	return &IndexManager{
		client: client,
		logger: logger,
	}
}

// ParcelMapping defines the mapping for parcel documents
var ParcelMapping = map[string]interface{}{
	"mappings": map[string]interface{}{
		"properties": map[string]interface{}{
			"parcelId": map[string]interface{}{
				"type": "keyword",
			},
			"centroid": map[string]interface{}{
				"type": "geo_point",
			},
			"bbox": map[string]interface{}{
				"type": "geo_shape",
			},
			"acres": map[string]interface{}{
				"type": "float",
			},
			"zoningTags": map[string]interface{}{
				"type": "keyword",
			},
			"jurisdiction": map[string]interface{}{
				"type": "keyword",
			},
			"stateFips": map[string]interface{}{
				"type": "keyword",
			},
			"tenantId": map[string]interface{}{
				"type": "keyword",
			},
		},
	},
	"settings": map[string]interface{}{
		"number_of_shards":   1,
		"number_of_replicas": 0,
	},
}

// CreateParcelIndex creates the parcels index with the proper mapping
func (im *IndexManager) CreateParcelIndex(ctx context.Context) error {
	if !im.client.IsEnabled() {
		return fmt.Errorf("OpenSearch is not enabled")
	}

	// Check if index already exists
	exists, err := im.IndexExists(ctx, ParcelIndexName)
	if err != nil {
		return fmt.Errorf("failed to check if index exists: %w", err)
	}

	if exists {
		im.logger.Info("Parcel index already exists", zap.String("index", ParcelIndexName))
		return nil
	}

	// Create index with mapping
	mappingJSON, err := json.Marshal(ParcelMapping)
	if err != nil {
		return fmt.Errorf("failed to marshal mapping: %w", err)
	}

	createReq := opensearchapi.IndicesCreateReq{
		Index: ParcelIndexName,
		Body:  bytes.NewReader(mappingJSON),
	}
	var createRes opensearchapi.IndicesCreateResp

	_, err = im.client.GetClient().Do(ctx, &createReq, &createRes)
	if err != nil {
		return fmt.Errorf("failed to create index: %w", err)
	}

	statusCode := createRes.Inspect().Response.StatusCode
	if statusCode != http.StatusOK && statusCode != http.StatusCreated {
		return fmt.Errorf("failed to create index: status %d", statusCode)
	}

	im.logger.Info("Created parcel index", zap.String("index", ParcelIndexName))
	return nil
}

// DeleteParcelIndex deletes the parcels index
func (im *IndexManager) DeleteParcelIndex(ctx context.Context) error {
	if !im.client.IsEnabled() {
		return fmt.Errorf("OpenSearch is not enabled")
	}

	deleteReq := opensearchapi.IndicesDeleteReq{
		Indices: []string{ParcelIndexName},
	}
	var deleteRes opensearchapi.IndicesDeleteResp

	_, err := im.client.GetClient().Do(ctx, &deleteReq, &deleteRes)
	if err != nil {
		return fmt.Errorf("failed to delete index: %w", err)
	}

	statusCode := deleteRes.Inspect().Response.StatusCode
	if statusCode != http.StatusOK {
		return fmt.Errorf("failed to delete index: status %d", statusCode)
	}

	im.logger.Info("Deleted parcel index", zap.String("index", ParcelIndexName))
	return nil
}

// IndexExists checks if an index exists
func (im *IndexManager) IndexExists(ctx context.Context, indexName string) (bool, error) {
	if !im.client.IsEnabled() {
		return false, fmt.Errorf("OpenSearch is not enabled")
	}

	existsReq := opensearchapi.IndicesExistsReq{
		Indices: []string{indexName},
	}

	resp, err := im.client.GetClient().Do(ctx, &existsReq, nil)
	if err != nil {
		return false, fmt.Errorf("failed to check if index exists: %w", err)
	}

	return resp.StatusCode == http.StatusOK, nil
}

// GetIndexStats returns statistics about the index
func (im *IndexManager) GetIndexStats(ctx context.Context, indexName string) (map[string]interface{}, error) {
	if !im.client.IsEnabled() {
		return nil, fmt.Errorf("OpenSearch is not enabled")
	}

	statsReq := opensearchapi.IndicesStatsReq{
		Indices: []string{indexName},
	}
	var statsRes opensearchapi.IndicesStatsResp

	_, err := im.client.GetClient().Do(ctx, &statsReq, &statsRes)
	if err != nil {
		return nil, fmt.Errorf("failed to get index stats: %w", err)
	}

	if statsRes.Inspect().Response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get index stats: status %d", statsRes.Inspect().Response.StatusCode)
	}

	// Read response body
	body := statsRes.Inspect().Response.Body
	defer body.Close()
	var stats map[string]interface{}
	if err := json.NewDecoder(body).Decode(&stats); err != nil {
		return nil, fmt.Errorf("failed to decode stats: %w", err)
	}

	return stats, nil
}
