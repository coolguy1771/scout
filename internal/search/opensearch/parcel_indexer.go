package opensearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	opensearchapi "github.com/opensearch-project/opensearch-go/v4/opensearchapi"
	"go.uber.org/zap"

	"github.com/coolguy1771/scout/internal/models"
)

// ParcelIndexer handles indexing parcel documents
type ParcelIndexer struct {
	client *Client
	logger *zap.Logger
}

// NewParcelIndexer creates a new parcel indexer
func NewParcelIndexer(client *Client, logger *zap.Logger) *ParcelIndexer {
	return &ParcelIndexer{
		client: client,
		logger: logger,
	}
}

// ParcelDocument represents a parcel document in OpenSearch
type ParcelDocument struct {
	ParcelID     string    `json:"parcelId"`
	Centroid     []float64 `json:"centroid"` // [lon, lat]
	BBox         []float64 `json:"bbox"`     // [minLon, minLat, maxLon, maxLat]
	Acres        float64   `json:"acres"`
	ZoningTags   []string  `json:"zoningTags"`
	Jurisdiction string    `json:"jurisdiction"`
	StateFIPS    string    `json:"stateFips"`
	TenantID     *string   `json:"tenantId,omitempty"`
}

// convertParcelToDocument converts a models.Parcel to a ParcelDocument
func convertParcelToDocument(parcel *models.Parcel) (*ParcelDocument, error) {
	doc := &ParcelDocument{
		ParcelID:     parcel.ParcelID.String(),
		Acres:        parcel.Acres,
		ZoningTags:   parcel.ZoningTags,
		Jurisdiction: parcel.Jurisdiction,
		StateFIPS:    parcel.StateFIPS,
	}

	if parcel.TenantID != nil {
		tenantIDStr := parcel.TenantID.String()
		doc.TenantID = &tenantIDStr
	}

	// Parse centroid from GeoJSON
	if parcel.Centroid != "" {
		var centroidGeoJSON map[string]interface{}
		if err := json.Unmarshal([]byte(parcel.Centroid), &centroidGeoJSON); err == nil {
			if coords, ok := centroidGeoJSON["coordinates"].([]interface{}); ok && len(coords) >= 2 {
				lon, _ := coords[0].(float64)
				lat, _ := coords[1].(float64)
				doc.Centroid = []float64{lon, lat}
			}
		}
	}

	// Parse bbox from geometry
	if parcel.Geom != "" {
		var geomGeoJSON map[string]interface{}
		if err := json.Unmarshal([]byte(parcel.Geom), &geomGeoJSON); err == nil {
			bbox := extractBBoxFromGeoJSON(geomGeoJSON)
			if len(bbox) == 4 {
				doc.BBox = bbox
			}
		}
	}

	return doc, nil
}

// extractBBoxFromGeoJSON extracts bounding box from GeoJSON geometry
func extractBBoxFromGeoJSON(geom map[string]interface{}) []float64 {
	var coords []interface{}

	switch geom["type"] {
	case "Point":
		if c, ok := geom["coordinates"].([]interface{}); ok {
			coords = []interface{}{c, c} // Point bbox is same point twice
		}
	case "Polygon":
		if c, ok := geom["coordinates"].([]interface{}); ok && len(c) > 0 {
			if ring, ok := c[0].([]interface{}); ok {
				coords = ring
			}
		}
	case "MultiPolygon":
		if c, ok := geom["coordinates"].([]interface{}); ok {
			// Flatten all coordinates from all polygons
			for _, poly := range c {
				if polyArr, ok := poly.([]interface{}); ok && len(polyArr) > 0 {
					if ring, ok := polyArr[0].([]interface{}); ok {
						coords = append(coords, ring...)
					}
				}
			}
		}
	}

	if len(coords) == 0 {
		return nil
	}

	// Calculate bbox
	minLon, maxLon := coords[0].([]interface{})[0].(float64), coords[0].([]interface{})[0].(float64)
	minLat, maxLat := coords[0].([]interface{})[1].(float64), coords[0].([]interface{})[1].(float64)

	for _, coord := range coords {
		if coordArr, ok := coord.([]interface{}); ok && len(coordArr) >= 2 {
			lon, _ := coordArr[0].(float64)
			lat, _ := coordArr[1].(float64)
			if lon < minLon {
				minLon = lon
			}
			if lon > maxLon {
				maxLon = lon
			}
			if lat < minLat {
				minLat = lat
			}
			if lat > maxLat {
				maxLat = lat
			}
		}
	}

	return []float64{minLon, minLat, maxLon, maxLat}
}

// IndexParcel indexes a single parcel
func (pi *ParcelIndexer) IndexParcel(ctx context.Context, parcel *models.Parcel) error {
	if !pi.client.IsEnabled() {
		return fmt.Errorf("OpenSearch is not enabled")
	}

	doc, err := convertParcelToDocument(parcel)
	if err != nil {
		return fmt.Errorf("failed to convert parcel to document: %w", err)
	}

	docJSON, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("failed to marshal document: %w", err)
	}

	indexReq := opensearchapi.IndexReq{
		Index:      ParcelIndexName,
		DocumentID: doc.ParcelID,
		Body:       bytes.NewReader(docJSON),
	}
	var indexRes opensearchapi.IndexResp

	_, err = pi.client.GetClient().Do(ctx, &indexReq, &indexRes)
	if err != nil {
		return fmt.Errorf("failed to index parcel: %w", err)
	}

	statusCode := indexRes.Inspect().Response.StatusCode
	if statusCode != http.StatusOK && statusCode != http.StatusCreated {
		return fmt.Errorf("failed to index parcel: status %d", statusCode)
	}

	return nil
}

// BulkIndexParcels indexes multiple parcels in a single bulk request
func (pi *ParcelIndexer) BulkIndexParcels(ctx context.Context, parcels []models.Parcel) error {
	if !pi.client.IsEnabled() {
		return fmt.Errorf("OpenSearch is not enabled")
	}

	if len(parcels) == 0 {
		return nil
	}

	var bulkBody strings.Builder

	for _, parcel := range parcels {
		doc, err := convertParcelToDocument(&parcel)
		if err != nil {
			pi.logger.Warn("Failed to convert parcel to document", zap.Error(err), zap.String("parcelId", parcel.ParcelID.String()))
			continue
		}

		// Action line
		action := map[string]interface{}{
			"index": map[string]interface{}{
				"_index": ParcelIndexName,
				"_id":    doc.ParcelID,
			},
		}
		actionJSON, _ := json.Marshal(action)
		bulkBody.WriteString(string(actionJSON))
		bulkBody.WriteString("\n")

		// Document line
		docJSON, _ := json.Marshal(doc)
		bulkBody.WriteString(string(docJSON))
		bulkBody.WriteString("\n")
	}

	bulkReq := opensearchapi.BulkReq{
		Body: bytes.NewReader([]byte(bulkBody.String())),
	}
	var bulkRes opensearchapi.BulkResp

	_, err := pi.client.GetClient().Do(ctx, &bulkReq, &bulkRes)
	if err != nil {
		return fmt.Errorf("failed to bulk index parcels: %w", err)
	}

	statusCode := bulkRes.Inspect().Response.StatusCode
	if statusCode != http.StatusOK {
		return fmt.Errorf("failed to bulk index parcels: status %d", statusCode)
	}

	// Check for errors in bulk response
	if bulkRes.Errors {
		pi.logger.Warn("Some documents failed to index in bulk operation")
	}

	return nil
}

// UpdateParcel updates an existing parcel document
func (pi *ParcelIndexer) UpdateParcel(ctx context.Context, parcel *models.Parcel) error {
	// Update is same as index (upsert behavior)
	return pi.IndexParcel(ctx, parcel)
}

// DeleteParcel deletes a parcel document
func (pi *ParcelIndexer) DeleteParcel(ctx context.Context, parcelID uuid.UUID) error {
	if !pi.client.IsEnabled() {
		return fmt.Errorf("OpenSearch is not enabled")
	}

	deleteReq := opensearchapi.DocumentDeleteReq{
		Index:      ParcelIndexName,
		DocumentID: parcelID.String(),
	}
	var deleteRes opensearchapi.DocumentDeleteResp

	_, err := pi.client.GetClient().Do(ctx, &deleteReq, &deleteRes)
	if err != nil {
		return fmt.Errorf("failed to delete parcel: %w", err)
	}

	statusCode := deleteRes.Inspect().Response.StatusCode
	if statusCode != http.StatusOK && statusCode != http.StatusNotFound {
		return fmt.Errorf("failed to delete parcel: status %d", statusCode)
	}

	return nil
}
