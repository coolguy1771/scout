package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-playground/validator/v10"
)

var validate *validator.Validate

func init() {
	validate = validator.New()
}

// CreateProjectRequest represents a request to create a project
type CreateProjectRequest struct {
	Name string `json:"name" validate:"required,min=1,max=255"`
}

// UpdateProjectRequest represents a request to update a project
type UpdateProjectRequest struct {
	Name string `json:"name" validate:"required,min=1,max=255"`
}

// CreateSavedSearchRequest represents a request to create a saved search
type CreateSavedSearchRequest struct {
	ProjectID        *string         `json:"projectId,omitempty" validate:"omitempty,uuid"`
	Name             string          `json:"name" validate:"required,min=1,max=255"`
	QueryJSON        json.RawMessage `json:"queryJson" validate:"required"`
	ScoringProfileID *string         `json:"scoringProfileId,omitempty" validate:"omitempty,uuid"`
	DatasetVersionID *string         `json:"datasetVersionId,omitempty" validate:"omitempty,uuid"`
}

// CreateExportRequest represents a request to create an export
type CreateExportRequest struct {
	ParcelIDs        []string `json:"parcelIds,omitempty" validate:"omitempty,dive,uuid"`
	SavedSearchID    *string  `json:"savedSearchId,omitempty" validate:"omitempty,uuid"`
	Kind             string   `json:"kind" validate:"required,oneof=pdf csv geojson"` // pdf, csv, geojson
	ScoringProfileID *string  `json:"scoringProfileId,omitempty" validate:"omitempty,uuid"`
}

// UpdateSavedSearchRequest represents a request to update a saved search
type UpdateSavedSearchRequest struct {
	Name             string          `json:"name" validate:"required,min=1,max=255"`
	QueryJSON        json.RawMessage `json:"queryJson,omitempty"`
	ProjectID        *string         `json:"projectId,omitempty" validate:"omitempty,uuid"`
	ScoringProfileID *string         `json:"scoringProfileId,omitempty" validate:"omitempty,uuid"`
	DatasetVersionID *string         `json:"datasetVersionId,omitempty" validate:"omitempty,uuid"`
}

// UpdateLayerRequest represents a request to update a layer
type UpdateLayerRequest struct {
	Name string `json:"name" validate:"required,min=1,max=255"`
}

// CreateScoringProfileRequest represents a request to create a scoring profile
type CreateScoringProfileRequest struct {
	Name                string          `json:"name" validate:"required,min=1,max=255"`
	WeightsJSON         json.RawMessage `json:"weightsJson" validate:"required"`
	ThresholdsJSON      json.RawMessage `json:"thresholdsJson,omitempty"`
	HardConstraintsJSON json.RawMessage `json:"hardConstraintsJson,omitempty"`
	Version             *int            `json:"version,omitempty"`
}

// UpdateScoringProfileRequest represents a request to update a scoring profile
type UpdateScoringProfileRequest struct {
	Name                string          `json:"name" validate:"required,min=1,max=255"`
	WeightsJSON         json.RawMessage `json:"weightsJson,omitempty"`
	ThresholdsJSON      json.RawMessage `json:"thresholdsJson,omitempty"`
	HardConstraintsJSON json.RawMessage `json:"hardConstraintsJson,omitempty"`
	Version             *int            `json:"version,omitempty"`
}

// ValidateRequest validates a request struct
func ValidateRequest(v interface{}) error {
	return validate.Struct(v)
}

// GetValidationErrors returns a map of validation errors
func GetValidationErrors(err error) map[string]string {
	errors := make(map[string]string)
	if ve, ok := err.(validator.ValidationErrors); ok {
		for _, fe := range ve {
			errors[fe.Field()] = getValidationError(fe)
		}
	}
	return errors
}

// getValidationError returns a human-readable validation error message
func getValidationError(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return fe.Field() + " is required"
	case "email":
		return fe.Field() + " must be a valid email address"
	case "min":
		return fe.Field() + " must be at least " + fe.Param() + " characters"
	case "max":
		return fe.Field() + " must be at most " + fe.Param() + " characters"
	case "uuid":
		return fe.Field() + " must be a valid UUID"
	case "gte":
		return fe.Field() + " must be greater than or equal to " + fe.Param()
	case "lte":
		return fe.Field() + " must be less than or equal to " + fe.Param()
	case "oneof":
		return fe.Field() + " must be one of: " + fe.Param()
	default:
		return fe.Field() + " is invalid"
	}
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
