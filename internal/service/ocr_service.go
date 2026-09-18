package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/scannerapp/backend/internal/model"
	"github.com/scannerapp/backend/internal/repository/redis"
)

var (
	ErrOCRLimitExceeded = errors.New("free OCR limit reached (max 5 requests). Please upgrade to premium")
)

type OCRService interface {
	ProcessOCR(ctx context.Context, userID uuid.UUID, tier model.UserTier, req *model.OCRRequest) (*model.OCRResponse, error)
	GetQuota(ctx context.Context, userID uuid.UUID, tier model.UserTier) (int64, error)
}

type ocrService struct {
	limiterRepo redis.LimiterRepository
	maxFreeOCR  int64
}

func NewOCRService(limiterRepo redis.LimiterRepository, maxFreeOCR int64) OCRService {
	return &ocrService{
		limiterRepo: limiterRepo,
		maxFreeOCR:  maxFreeOCR,
	}
}

func (s *ocrService) ProcessOCR(ctx context.Context, userID uuid.UUID, tier model.UserTier, req *model.OCRRequest) (*model.OCRResponse, error) {
	if tier == model.TierFree {
		usage, err := s.limiterRepo.GetOCRUsage(ctx, userID)
		if err != nil {
			return nil, err
		}
		if usage >= s.maxFreeOCR {
			return nil, ErrOCRLimitExceeded
		}

		// Increment usage
		_, err = s.limiterRepo.IncrementOCRUsage(ctx, userID)
		if err != nil {
			return nil, err
		}
	}

	remaining, _ := s.limiterRepo.GetRemainingOCR(ctx, userID, s.maxFreeOCR)

	// Simulated cloud OCR engine recognition (can integrate with Google Cloud Vision or Tesseract)
	extractedText := "Recognized document text extracted via cloud OCR service."

	return &model.OCRResponse{
		ExtractedText: extractedText,
		Confidence:    0.98,
		RemainingUses: int(remaining),
	}, nil
}

func (s *ocrService) GetQuota(ctx context.Context, userID uuid.UUID, tier model.UserTier) (int64, error) {
	if tier == model.TierPremium {
		return 999999, nil
	}
	return s.limiterRepo.GetRemainingOCR(ctx, userID, s.maxFreeOCR)
}
