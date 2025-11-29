package api

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// ParcelResponse represents a parcel response
type ParcelResponse struct {
	ParcelID     string                 `json:"parcelId"`
	APN          string                 `json:"apn"`
	Acres        float64                `json:"acres"`
	ZoningRaw    string                 `json:"zoningRaw,omitempty"`
	ZoningTags   []string               `json:"zoningTags,omitempty"`
	Jurisdiction string                 `json:"jurisdiction,omitempty"`
	StateFips    string                 `json:"stateFips"`
	Features     *ParcelFeatureResponse `json:"features,omitempty"`
}

// ParcelFeatureResponse represents parcel features
type ParcelFeatureResponse struct {
	InFloodplain      bool     `json:"inFloodplain"`
	InWetlands        bool     `json:"inWetlands"`
	InProtectedLand   bool     `json:"inProtectedLand"`
	DistToHighwayM    *float64 `json:"distToHighwayM,omitempty"`
	DistToRailM       *float64 `json:"distToRailM,omitempty"`
	DistToAirportM    *float64 `json:"distToAirportM,omitempty"`
	DistToPowerLineM  *float64 `json:"distToPowerLineM,omitempty"`
	DistToSubstationM *float64 `json:"distToSubstationM,omitempty"`
}

// NearbyFeatureResponse represents a nearby feature
type NearbyFeatureResponse struct {
	FeatureID string      `json:"featureId"`
	Name      string      `json:"name,omitempty"`
	DistanceM float64     `json:"distanceM"`
	Geometry  interface{} `json:"geometry,omitempty"`
}

// NearbyResponse represents nearby features response
type NearbyResponse struct {
	ParcelID string                             `json:"parcelId"`
	Features map[string][]NearbyFeatureResponse `json:"features"`
}

// JobResponse represents a job response
type JobResponse struct {
	JobID     string      `json:"jobId"`
	Type      string      `json:"type"`
	Status    string      `json:"status"`
	Input     interface{} `json:"input,omitempty"`
	Output    interface{} `json:"output,omitempty"`
	Error     interface{} `json:"error,omitempty"`
	Attempts  int         `json:"attempts"`
	CreatedAt string      `json:"createdAt"`
	UpdatedAt string      `json:"updatedAt"`
}

// ExportResponse represents an export response
type ExportResponse struct {
	ExportID    string `json:"exportId"`
	JobID       string `json:"jobId"`
	Kind        string `json:"kind"`
	ObjectKey   string `json:"objectKey"`
	ContentType string `json:"contentType"`
	URL         string `json:"url,omitempty"` // Presigned URL
	CreatedAt   string `json:"createdAt"`
}

// ScoringProfileResponse represents a scoring profile response
type ScoringProfileResponse struct {
	ScoringProfileID string                 `json:"scoringProfileId"`
	TenantID         string                 `json:"tenantId"`
	Name             string                 `json:"name"`
	Version          int                    `json:"version"`
	Weights          map[string]float64     `json:"weights"`
	Thresholds       map[string]interface{} `json:"thresholds,omitempty"`
	HardConstraints  map[string]interface{} `json:"hardConstraints,omitempty"`
	CreatedBy        string                 `json:"createdBy"`
	CreatedAt        string                 `json:"createdAt"`
}


