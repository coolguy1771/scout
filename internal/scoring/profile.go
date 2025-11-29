package scoring

import (
	"encoding/json"
	"fmt"
)

// Profile represents a scoring profile configuration
type Profile struct {
	Weights         map[string]float64     `json:"weights"`
	Thresholds      map[string]interface{} `json:"thresholds,omitempty"`
	HardConstraints map[string]interface{} `json:"hardConstraints,omitempty"`
}

// ParseProfile parses JSON into a Profile
func ParseProfile(data string) (*Profile, error) {
	var profile Profile
	if err := json.Unmarshal([]byte(data), &profile); err != nil {
		return nil, fmt.Errorf("failed to parse scoring profile: %w", err)
	}

	// Validate weights sum to reasonable range (not required to be 1.0, but should be > 0)
	totalWeight := 0.0
	for _, weight := range profile.Weights {
		if weight < 0 {
			return nil, fmt.Errorf("weights must be non-negative")
		}
		totalWeight += weight
	}

	if totalWeight == 0 {
		return nil, fmt.Errorf("total weight must be greater than 0")
	}

	return &profile, nil
}

// Validate validates the profile
func (p *Profile) Validate() error {
	if len(p.Weights) == 0 {
		return fmt.Errorf("weights cannot be empty")
	}

	for key, weight := range p.Weights {
		if weight < 0 {
			return fmt.Errorf("weight for %s must be non-negative", key)
		}
	}

	return nil
}


