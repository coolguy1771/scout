package scoring

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/coolguy1771/scout/internal/repository"
)

// Service provides scoring functionality
type Service struct {
	profileRepo *repository.ScoringProfileRepository
	logger      *zap.Logger
}

// NewService creates a new scoring service
func NewService(profileRepo *repository.ScoringProfileRepository, logger *zap.Logger) *Service {
	return &Service{
		profileRepo: profileRepo,
		logger:      logger,
	}
}

// GetScorer retrieves a scorer for a profile ID
func (s *Service) GetScorer(ctx context.Context, profileID uuid.UUID, tenantID uuid.UUID) (*Scorer, error) {
	profileModel, err := s.profileRepo.GetByID(ctx, profileID, tenantID)
	if err != nil {
		return nil, err
	}

	profile, err := ParseProfile(profileModel.WeightsJSON)
	if err != nil {
		return nil, err
	}

	// Parse thresholds and hard constraints if present
	if profileModel.ThresholdsJSON != "" {
		if err := json.Unmarshal([]byte(profileModel.ThresholdsJSON), &profile.Thresholds); err != nil {
			s.logger.Warn("Failed to parse thresholds", zap.Error(err))
		}
	}

	if profileModel.HardConstraintsJSON != "" {
		if err := json.Unmarshal([]byte(profileModel.HardConstraintsJSON), &profile.HardConstraints); err != nil {
			s.logger.Warn("Failed to parse hard constraints", zap.Error(err))
		}
	}

	return NewScorer(profile), nil
}
