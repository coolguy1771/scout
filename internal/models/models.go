package models

import (
	"time"

	"github.com/google/uuid"
)

// Tenant represents an organization/tenant
type Tenant struct {
	TenantID  uuid.UUID `db:"tenant_id" json:"tenantId"`
	Name      string    `db:"name" json:"name"`
	CreatedAt time.Time `db:"created_at" json:"createdAt"`
}

// User represents a user account
type User struct {
	UserID    uuid.UUID `db:"user_id" json:"userId"`
	Email     string    `db:"email" json:"email"`
	Name      string    `db:"name" json:"name"`
	CreatedAt time.Time `db:"created_at" json:"createdAt"`
}

// Membership links users to tenants with roles
type Membership struct {
	MembershipID uuid.UUID `db:"membership_id" json:"membershipId"`
	TenantID     uuid.UUID `db:"tenant_id" json:"tenantId"`
	UserID       uuid.UUID `db:"user_id" json:"userId"`
	Role         string    `db:"role" json:"role"` // viewer, analyst, admin
	CreatedAt    time.Time `db:"created_at" json:"createdAt"`
}

// Parcel represents a land parcel
type Parcel struct {
	ParcelID     uuid.UUID  `db:"parcel_id" json:"parcelId"`
	TenantID     *uuid.UUID `db:"tenant_id" json:"tenantId,omitempty"`
	APN          string     `db:"apn" json:"apn"`
	Acres        float64    `db:"acres" json:"acres"`
	ZoningRaw    string     `db:"zoning_raw" json:"zoningRaw"`
	ZoningTags   []string   `db:"zoning_tags" json:"zoningTags"`
	Jurisdiction string     `db:"jurisdiction" json:"jurisdiction"`
	StateFIPS    string     `db:"state_fips" json:"stateFips"`
	Geom         string     `db:"geom" json:"-"`     // PostGIS geometry, not serialized
	Centroid     string     `db:"centroid" json:"-"` // PostGIS point
	CreatedAt    time.Time  `db:"created_at" json:"createdAt"`
	UpdatedAt    time.Time  `db:"updated_at" json:"updatedAt"`
}

// ParcelFeature contains precomputed features for a parcel
type ParcelFeature struct {
	ParcelID          uuid.UUID `db:"parcel_id" json:"parcelId"`
	FeaturesVersionID uuid.UUID `db:"features_version_id" json:"featuresVersionId"`
	InFloodplain      bool      `db:"in_floodplain" json:"inFloodplain"`
	InWetlands        bool      `db:"in_wetlands" json:"inWetlands"`
	InProtectedLand   bool      `db:"in_protected_land" json:"inProtectedLand"`
	DistToHighwayM    float64   `db:"dist_to_highway_m" json:"distToHighwayM"`
	DistToRailM       float64   `db:"dist_to_rail_m" json:"distToRailM"`
	DistToAirportM    float64   `db:"dist_to_airport_m" json:"distToAirportM"`
	DistToPowerLineM  float64   `db:"dist_to_power_line_m" json:"distToPowerLineM"`
	DistToSubstationM float64   `db:"dist_to_substation_m" json:"distToSubstationM"`
	ComputedAt        time.Time `db:"computed_at" json:"computedAt"`
}

// ScoringProfile defines how parcels are scored
type ScoringProfile struct {
	ScoringProfileID    uuid.UUID `db:"scoring_profile_id" json:"scoringProfileId"`
	TenantID            uuid.UUID `db:"tenant_id" json:"tenantId"`
	Name                string    `db:"name" json:"name"`
	Version             int       `db:"version" json:"version"`
	WeightsJSON         string    `db:"weights_json" json:"-"` // Store as JSON, parse to map
	ThresholdsJSON      string    `db:"thresholds_json" json:"-"`
	HardConstraintsJSON string    `db:"hard_constraints_json" json:"-"`
	CreatedBy           uuid.UUID `db:"created_by" json:"createdBy"`
	CreatedAt           time.Time `db:"created_at" json:"createdAt"`
}

// Project groups saved searches and work
type Project struct {
	ProjectID uuid.UUID `db:"project_id" json:"projectId"`
	TenantID  uuid.UUID `db:"tenant_id" json:"tenantId"`
	Name      string    `db:"name" json:"name"`
	CreatedBy uuid.UUID `db:"created_by" json:"createdBy"`
	CreatedAt time.Time `db:"created_at" json:"createdAt"`
}

// SavedSearch stores a search query for reuse
type SavedSearch struct {
	SavedSearchID    uuid.UUID  `db:"saved_search_id" json:"savedSearchId"`
	TenantID         uuid.UUID  `db:"tenant_id" json:"tenantId"`
	ProjectID        *uuid.UUID `db:"project_id" json:"projectId,omitempty"`
	Name             string     `db:"name" json:"name"`
	QueryJSON        string     `db:"query_json" json:"-"`
	ScoringProfileID *uuid.UUID `db:"scoring_profile_id" json:"scoringProfileId,omitempty"`
	DatasetVersionID *uuid.UUID `db:"dataset_version_id" json:"datasetVersionId,omitempty"`
	CreatedBy        uuid.UUID  `db:"created_by" json:"createdBy"`
	CreatedAt        time.Time  `db:"created_at" json:"createdAt"`
}

// Job represents an async job (export, tiling, etc.)
type Job struct {
	JobID      uuid.UUID `db:"job_id" json:"jobId"`
	TenantID   uuid.UUID `db:"tenant_id" json:"tenantId"`
	Type       string    `db:"type" json:"type"`     // export, tile_build, feature_recompute
	Status     string    `db:"status" json:"status"` // pending, processing, completed, failed
	InputJSON  string    `db:"input_json" json:"-"`
	OutputJSON string    `db:"output_json" json:"-"`
	ErrorJSON  string    `db:"error_json" json:"-"`
	Attempts   int       `db:"attempts" json:"attempts"`
	CreatedAt  time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt  time.Time `db:"updated_at" json:"updatedAt"`
}

// Export represents a generated export file
type Export struct {
	ExportID    uuid.UUID `db:"export_id" json:"exportId"`
	TenantID    uuid.UUID `db:"tenant_id" json:"tenantId"`
	JobID       uuid.UUID `db:"job_id" json:"jobId"`
	Kind        string    `db:"kind" json:"kind"` // pdf, csv, geojson
	ObjectKey   string    `db:"object_key" json:"objectKey"`
	ContentType string    `db:"content_type" json:"contentType"`
	CreatedAt   time.Time `db:"created_at" json:"createdAt"`
}

// PrivateLayer represents a tenant-scoped private layer upload
type PrivateLayer struct {
	LayerID      uuid.UUID  `db:"layer_id" json:"layerId"`
	TenantID     uuid.UUID  `db:"tenant_id" json:"tenantId"`
	Name         string     `db:"name" json:"name"`
	LayerType    string     `db:"layer_type" json:"layerType"` // point, line, polygon, mixed
	FileName     string     `db:"file_name" json:"fileName"`
	FileSize     int64      `db:"file_size" json:"fileSize"`
	Status       string     `db:"status" json:"status"` // uploading, processing, completed, failed
	DataRunID    *uuid.UUID `db:"data_run_id" json:"dataRunId,omitempty"`
	ObjectKey    string     `db:"object_key" json:"objectKey,omitempty"`
	ErrorMessage string     `db:"error_message" json:"errorMessage,omitempty"`
	CreatedBy    uuid.UUID  `db:"created_by" json:"createdBy"`
	CreatedAt    time.Time  `db:"created_at" json:"createdAt"`
	UpdatedAt    time.Time  `db:"updated_at" json:"updatedAt"`
}

// Favorite represents a user's favorite/bookmarked parcel
type Favorite struct {
	FavoriteID uuid.UUID `db:"favorite_id" json:"favoriteId"`
	TenantID   uuid.UUID `db:"tenant_id" json:"tenantId"`
	UserID     uuid.UUID `db:"user_id" json:"userId"`
	ParcelID   uuid.UUID `db:"parcel_id" json:"parcelId"`
	CreatedAt  time.Time `db:"created_at" json:"createdAt"`
}
