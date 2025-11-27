package search

import (
	"encoding/json"
	"fmt"
)

// SearchQuery represents a search request
type SearchQuery struct {
	BBox             []float64              `json:"bbox,omitempty"` // [minLon, minLat, maxLon, maxLat]
	AOI              [][]float64            `json:"aoi,omitempty"`  // Polygon coordinates [[lon, lat], ...]
	MinAcres         *float64               `json:"minAcres,omitempty"`
	MaxAcres         *float64               `json:"maxAcres,omitempty"`
	ZoningTags       []string               `json:"zoningTags,omitempty"`
	Jurisdiction     string                 `json:"jurisdiction,omitempty"`
	StateFips        string                 `json:"stateFips,omitempty"`
	ScoringProfileID *string                `json:"scoringProfileId,omitempty"`
	Page             int                    `json:"page,omitempty"`
	PageSize         int                    `json:"pageSize,omitempty"`
	Exclusions       map[string]interface{} `json:"exclusions,omitempty"`
}

// Validate validates the search query
func (q *SearchQuery) Validate() error {
	if len(q.BBox) > 0 && len(q.BBox) != 4 {
		return fmt.Errorf("bbox must have 4 elements [minLon, minLat, maxLon, maxLat]")
	}

	if len(q.BBox) == 0 && len(q.AOI) == 0 {
		return fmt.Errorf("either bbox or aoi must be provided")
	}

	if len(q.AOI) > 0 && len(q.AOI) < 3 {
		return fmt.Errorf("aoi polygon must have at least 3 points")
	}

	if q.Page < 0 {
		return fmt.Errorf("page must be >= 0")
	}

	if q.PageSize < 0 {
		return fmt.Errorf("pageSize must be >= 0")
	}

	if q.PageSize > 1000 {
		return fmt.Errorf("pageSize must be <= 1000")
	}

	return nil
}

// ToFilters converts query to filter map for repository
func (q *SearchQuery) ToFilters() map[string]interface{} {
	filters := make(map[string]interface{})

	if q.MinAcres != nil {
		filters["minAcres"] = *q.MinAcres
	}
	if q.MaxAcres != nil {
		filters["maxAcres"] = *q.MaxAcres
	}
	if len(q.ZoningTags) > 0 {
		filters["zoningTags"] = q.ZoningTags
	}
	if q.Jurisdiction != "" {
		filters["jurisdiction"] = q.Jurisdiction
	}
	if q.StateFips != "" {
		filters["stateFips"] = q.StateFips
	}

	return filters
}

// ParseSearchQuery parses JSON into SearchQuery
func ParseSearchQuery(data []byte) (*SearchQuery, error) {
	var query SearchQuery
	if err := json.Unmarshal(data, &query); err != nil {
		return nil, fmt.Errorf("failed to parse search query: %w", err)
	}

	if err := query.Validate(); err != nil {
		return nil, err
	}

	// Set defaults
	if query.Page <= 0 {
		query.Page = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 50
	}

	return &query, nil
}
