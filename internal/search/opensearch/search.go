package opensearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	opensearchapi "github.com/opensearch-project/opensearch-go/v4/opensearchapi"
	"go.uber.org/zap"
)

// SearchService provides search functionality using OpenSearch
type SearchService struct {
	indexer *ParcelIndexer
	logger  *zap.Logger
}

// NewSearchService creates a new OpenSearch search service
func NewSearchService(indexer *ParcelIndexer, logger *zap.Logger) *SearchService {
	return &SearchService{
		indexer: indexer,
		logger:  logger,
	}
}

// SearchQuery represents a search query
type SearchQuery struct {
	BBox          []float64              `json:"bbox,omitempty"`
	Filters       map[string]interface{} `json:"filters,omitempty"`
	TenantID      *uuid.UUID             `json:"tenantId,omitempty"`
	MaxCandidates int                    `json:"maxCandidates,omitempty"`
}

// SearchCandidates performs a search and returns candidate parcel IDs
func (s *SearchService) SearchCandidates(ctx context.Context, query *SearchQuery) ([]uuid.UUID, error) {
	if !s.indexer.client.IsEnabled() {
		return nil, fmt.Errorf("OpenSearch is not enabled")
	}

	// Build OpenSearch query
	esQuery := s.buildQuery(query)

	queryJSON, err := json.Marshal(esQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query: %w", err)
	}

	searchReq := opensearchapi.SearchReq{
		Indices: []string{ParcelIndexName},
		Body:    bytes.NewReader(queryJSON),
	}
	var searchRes opensearchapi.SearchResp

	_, err = s.indexer.client.GetClient().Do(ctx, &searchReq, &searchRes)
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}

	if searchRes.Inspect().Response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search failed: status %d", searchRes.Inspect().Response.StatusCode)
	}

	// Extract parcel IDs from hits
	var parcelIDs []uuid.UUID
	if len(searchRes.Hits.Hits) == 0 {
		return parcelIDs, nil
	}

	for _, hit := range searchRes.Hits.Hits {
		if hit.Source != nil {
			// Parse source JSON to get parcelId
			var source map[string]interface{}
			if err := json.Unmarshal(hit.Source, &source); err == nil {
				if parcelIDStr, ok := source["parcelId"].(string); ok {
					if parcelID, err := uuid.Parse(parcelIDStr); err == nil {
						parcelIDs = append(parcelIDs, parcelID)
					}
				}
			}
		}
	}

	return parcelIDs, nil
}

// buildQuery builds an OpenSearch query from the search query
func (s *SearchService) buildQuery(query *SearchQuery) map[string]interface{} {
	mustClauses := []map[string]interface{}{}
	filterClauses := []map[string]interface{}{}

	// BBox filter using geo_shape
	if len(query.BBox) == 4 {
		bboxQuery := map[string]interface{}{
			"geo_shape": map[string]interface{}{
				"bbox": map[string]interface{}{
					"shape": map[string]interface{}{
						"type": "envelope",
						"coordinates": [][]float64{
							{query.BBox[0], query.BBox[3]}, // minLon, maxLat
							{query.BBox[2], query.BBox[1]}, // maxLon, minLat
						},
					},
					"relation": "intersects",
				},
			},
		}
		filterClauses = append(filterClauses, bboxQuery)
	}

	// Tenant filter
	if query.TenantID != nil {
		tenantFilter := map[string]interface{}{
			"bool": map[string]interface{}{
				"should": []map[string]interface{}{
					{
						"term": map[string]interface{}{
							"tenantId": query.TenantID.String(),
						},
					},
					{
						"bool": map[string]interface{}{
							"must_not": map[string]interface{}{
								"exists": map[string]interface{}{
									"field": "tenantId",
								},
							},
						},
					},
				},
			},
		}
		filterClauses = append(filterClauses, tenantFilter)
	}

	// Apply filters from query
	if query.Filters != nil {
		// Acres range filter
		if minAcres, ok := query.Filters["minAcres"].(float64); ok {
			filterClauses = append(filterClauses, map[string]interface{}{
				"range": map[string]interface{}{
					"acres": map[string]interface{}{
						"gte": minAcres,
					},
				},
			})
		}
		if maxAcres, ok := query.Filters["maxAcres"].(float64); ok {
			filterClauses = append(filterClauses, map[string]interface{}{
				"range": map[string]interface{}{
					"acres": map[string]interface{}{
						"lte": maxAcres,
					},
				},
			})
		}

		// Zoning tags filter
		if zoningTags, ok := query.Filters["zoningTags"].([]string); ok && len(zoningTags) > 0 {
			filterClauses = append(filterClauses, map[string]interface{}{
				"terms": map[string]interface{}{
					"zoningTags": zoningTags,
				},
			})
		}

		// Jurisdiction filter
		if jurisdiction, ok := query.Filters["jurisdiction"].(string); ok && jurisdiction != "" {
			filterClauses = append(filterClauses, map[string]interface{}{
				"term": map[string]interface{}{
					"jurisdiction": jurisdiction,
				},
			})
		}

		// State FIPS filter
		if stateFips, ok := query.Filters["stateFips"].(string); ok && stateFips != "" {
			filterClauses = append(filterClauses, map[string]interface{}{
				"term": map[string]interface{}{
					"stateFips": stateFips,
				},
			})
		}
	}

	// Build final query
	esQuery := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must":   mustClauses,
				"filter": filterClauses,
			},
		},
		"size":    query.MaxCandidates,
		"_source": []string{"parcelId"},
	}

	if query.MaxCandidates <= 0 {
		esQuery["size"] = 50000 // Default max
	}

	return esQuery
}
