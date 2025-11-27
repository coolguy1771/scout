package scoring

import (
	"math"

	"github.com/coolguy1771/scout/internal/models"
)

// Scorer computes suitability scores for parcels
type Scorer struct {
	profile *Profile
}

// NewScorer creates a new scorer with a profile
func NewScorer(profile *Profile) *Scorer {
	return &Scorer{profile: profile}
}

// NewDefaultScorer creates a scorer with default weights
func NewDefaultScorer() *Scorer {
	defaultProfile := &Profile{
		Weights: map[string]float64{
			"distToHighway":    0.2,
			"distToRail":       0.2,
			"distToAirport":    0.15,
			"distToPowerLine":  0.15,
			"distToSubstation": 0.15,
			"acres":            0.15,
		},
		HardConstraints: map[string]interface{}{
			"excludeFloodplain":    true,
			"excludeWetlands":      true,
			"excludeProtectedLand": true,
		},
	}
	return &Scorer{profile: defaultProfile}
}

// ScoreResult represents the scoring result
type ScoreResult struct {
	Score          float64
	Breakdown      map[string]interface{}
	ConstraintsHit []string
}

// Score computes a suitability score for a parcel
func (s *Scorer) Score(parcel *models.Parcel, features *models.ParcelFeature) (float64, map[string]interface{}, []string) {
	breakdown := make(map[string]interface{})
	constraintsHit := []string{}

	// Apply hard constraints first
	if s.profile.HardConstraints != nil {
		if features != nil {
			if excludeFloodplain, ok := s.profile.HardConstraints["excludeFloodplain"].(bool); ok && excludeFloodplain {
				if features.InFloodplain {
					constraintsHit = append(constraintsHit, "floodplain")
					return 0.0, breakdown, constraintsHit
				}
			}

			if excludeWetlands, ok := s.profile.HardConstraints["excludeWetlands"].(bool); ok && excludeWetlands {
				if features.InWetlands {
					constraintsHit = append(constraintsHit, "wetlands")
					return 0.0, breakdown, constraintsHit
				}
			}

			if excludeProtected, ok := s.profile.HardConstraints["excludeProtectedLand"].(bool); ok && excludeProtected {
				if features.InProtectedLand {
					constraintsHit = append(constraintsHit, "protectedLand")
					return 0.0, breakdown, constraintsHit
				}
			}
		}
	}

	// Compute weighted score
	totalScore := 0.0
	totalWeight := 0.0

	for factor, weight := range s.profile.Weights {
		if weight <= 0 {
			continue
		}

		var normalizedValue float64
		var factorValue interface{}

		switch factor {
		case "distToHighway":
			if features != nil && features.DistToHighwayM > 0 {
				normalizedValue = normalizeDistance(features.DistToHighwayM, 0, 10000) // 0-10km
				factorValue = features.DistToHighwayM
			} else {
				normalizedValue = 0.0
				factorValue = nil
			}
		case "distToRail":
			if features != nil && features.DistToRailM > 0 {
				normalizedValue = normalizeDistance(features.DistToRailM, 0, 10000)
				factorValue = features.DistToRailM
			} else {
				normalizedValue = 0.0
				factorValue = nil
			}
		case "distToAirport":
			if features != nil && features.DistToAirportM > 0 {
				normalizedValue = normalizeDistance(features.DistToAirportM, 0, 50000) // 0-50km
				factorValue = features.DistToAirportM
			} else {
				normalizedValue = 0.0
				factorValue = nil
			}
		case "distToPowerLine":
			if features != nil && features.DistToPowerLineM > 0 {
				normalizedValue = normalizeDistance(features.DistToPowerLineM, 0, 5000) // 0-5km
				factorValue = features.DistToPowerLineM
			} else {
				normalizedValue = 0.0
				factorValue = nil
			}
		case "distToSubstation":
			if features != nil && features.DistToSubstationM > 0 {
				normalizedValue = normalizeDistance(features.DistToSubstationM, 0, 5000)
				factorValue = features.DistToSubstationM
			} else {
				normalizedValue = 0.0
				factorValue = nil
			}
		case "acres":
			// Normalize acres (prefer medium-sized parcels, e.g., 1-100 acres)
			normalizedValue = normalizeAcres(parcel.Acres)
			factorValue = parcel.Acres
		default:
			continue
		}

		contribution := normalizedValue * weight
		totalScore += contribution
		totalWeight += weight

		breakdown[factor] = map[string]interface{}{
			"value":        factorValue,
			"normalized":   normalizedValue,
			"weight":       weight,
			"contribution": contribution,
		}
	}

	// Normalize final score by total weight
	finalScore := 0.0
	if totalWeight > 0 {
		finalScore = totalScore / totalWeight
	}

	// Ensure score is between 0 and 1
	finalScore = math.Max(0.0, math.Min(1.0, finalScore))

	return finalScore, breakdown, constraintsHit
}

// normalizeDistance normalizes distance (closer is better, so we invert)
// Distance is normalized to 0-1 where 0 = max distance, 1 = min distance
func normalizeDistance(distanceM, minM, maxM float64) float64 {
	if distanceM <= minM {
		return 1.0
	}
	if distanceM >= maxM {
		return 0.0
	}
	// Linear interpolation, inverted (closer = better)
	return 1.0 - ((distanceM - minM) / (maxM - minM))
}

// normalizeAcres normalizes acres (prefer medium-sized parcels)
func normalizeAcres(acres float64) float64 {
	// Prefer parcels between 1-100 acres
	// This is a simple bell curve approximation
	if acres < 0.1 {
		return 0.0
	}
	if acres >= 0.1 && acres <= 100 {
		// Peak at ~10 acres
		peak := 10.0
		normalized := 1.0 - math.Abs((acres-peak)/peak)
		return math.Max(0.0, normalized)
	}
	// Larger parcels get lower scores
	if acres > 100 {
		return math.Max(0.0, 1.0-(acres-100)/1000)
	}
	return 0.0
}
