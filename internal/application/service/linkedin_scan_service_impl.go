package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/reangeline/backend_applywise/internal/core/domain"
	"github.com/reangeline/backend_applywise/internal/core/ports/inbound"
	"github.com/reangeline/backend_applywise/internal/core/ports/outbound"
	"github.com/reangeline/backend_applywise/pkg/security"
)

type linkedInScanServiceImpl struct {
	linkedInScanRepo outbound.LinkedInScanRepository
	aiService        outbound.AIService
}

func NewLinkedInScanService(
	linkedInScanRepo outbound.LinkedInScanRepository,
	aiService outbound.AIService,
) inbound.LinkedInScanService {
	return &linkedInScanServiceImpl{
		linkedInScanRepo: linkedInScanRepo,
		aiService:        aiService,
	}
}

func (s *linkedInScanServiceImpl) ScanProfile(ctx context.Context, req inbound.ScanLinkedInProfileRequest) (*domain.LinkedInScan, error) {
	text, err := extractTextFromPDF(req.PDFBytes)
	if err != nil {
		return nil, fmt.Errorf("PDF text extraction failed: %w", err)
	}

	profileText := security.SanitizeForPrompt(text)
	if err := security.ValidateResumeContent(profileText); err != nil {
		return nil, err
	}

	targetRole := security.SanitizeForPrompt(req.TargetRole)
	if targetRole != "" {
		if err := security.ValidateShortField(targetRole, "target role"); err != nil {
			return nil, err
		}
	}

	result, err := s.aiService.ScanLinkedInProfile(ctx, &outbound.LinkedInScanInput{
		ProfileText: profileText,
		TargetRole:  targetRole,
	})
	if err != nil {
		return nil, fmt.Errorf("AI LinkedIn scan failed: %w", err)
	}

	now := time.Now().UTC()
	scan := &domain.LinkedInScan{
		ID:              uuid.New().String(),
		UserID:          req.UserID,
		Score:           result.Score,
		Sections:        result.Sections,
		PredictedSkills: result.PredictedSkills,
		Tips:            result.Tips,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := s.linkedInScanRepo.Upsert(ctx, scan); err != nil {
		return nil, fmt.Errorf("failed to save LinkedIn scan: %w", err)
	}

	return scan, nil
}

func (s *linkedInScanServiceImpl) GetLatestScan(ctx context.Context, userID string) (*domain.LinkedInScan, error) {
	return s.linkedInScanRepo.GetByUserID(ctx, userID)
}
