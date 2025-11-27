package api

import (
	"encoding/json"
	"io"
	"net/http"
)

// CreateProjectRequest represents a request to create a project
type CreateProjectRequest struct {
	Name string `json:"name"`
}

// UpdateProjectRequest represents a request to update a project
type UpdateProjectRequest struct {
	Name string `json:"name"`
}

// CreateSavedSearchRequest represents a request to create a saved search
type CreateSavedSearchRequest struct {
	ProjectID        *string         `json:"projectId,omitempty"`
	Name             string          `json:"name"`
	QueryJSON        json.RawMessage `json:"queryJson"`
	ScoringProfileID *string         `json:"scoringProfileId,omitempty"`
	DatasetVersionID *string         `json:"datasetVersionId,omitempty"`
}

// CreateExportRequest represents a request to create an export
type CreateExportRequest struct {
	ParcelIDs        []string `json:"parcelIds,omitempty"`
	SavedSearchID    *string  `json:"savedSearchId,omitempty"`
	Kind             string   `json:"kind"` // pdf, csv, geojson
	ScoringProfileID *string  `json:"scoringProfileId,omitempty"`
}

// UpdateSavedSearchRequest represents a request to update a saved search
type UpdateSavedSearchRequest struct {
	Name             string          `json:"name"`
	QueryJSON        json.RawMessage `json:"queryJson,omitempty"`
	ProjectID        *string         `json:"projectId,omitempty"`
	ScoringProfileID *string         `json:"scoringProfileId,omitempty"`
	DatasetVersionID *string         `json:"datasetVersionId,omitempty"`
}

// UpdateLayerRequest represents a request to update a layer
type UpdateLayerRequest struct {
	Name string `json:"name"`
}

// ParseJSONRequest parses JSON request body
func ParseJSONRequest(r *http.Request, v interface{}) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	if err := json.Unmarshal(body, v); err != nil {
		return err
	}

	return nil
}
