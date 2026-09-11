package service

import (
	"context"

	"github.com/reangeline/backend_applywise/internal/core/domain"
	"github.com/reangeline/backend_applywise/internal/core/ports/inbound"
	"github.com/reangeline/backend_applywise/internal/core/ports/outbound"
	"github.com/reangeline/backend_applywise/pkg/security"
)

type applyAssistServiceImpl struct {
	resumeRepo outbound.ResumeRepository
	subRepo    outbound.SubscriptionRepository
	aiService  outbound.AIService
}

func NewApplyAssistService(
	resumeRepo outbound.ResumeRepository,
	subRepo outbound.SubscriptionRepository,
	aiService outbound.AIService,
) inbound.ApplyAssistService {
	return &applyAssistServiceImpl{
		resumeRepo: resumeRepo,
		subRepo:    subRepo,
		aiService:  aiService,
	}
}

func (s *applyAssistServiceImpl) SuggestAnswer(ctx context.Context, req inbound.SuggestAnswerRequest) (*inbound.SuggestAnswerResult, error) {
	sub, err := s.subRepo.GetByUserID(ctx, req.UserID)
	if err != nil {
		return nil, err
	}
	if sub.Plan != domain.PlanPremium || !sub.IsActive() {
		return nil, domain.ErrPremiumRequired
	}

	resume, err := s.resumeRepo.GetResume(ctx, req.UserID, req.ResumeID)
	if err != nil {
		return nil, err
	}

	question := security.SanitizeForPrompt(req.Question)
	if err := security.ValidateShortField(question, "question"); err != nil {
		return nil, err
	}

	jobTitle := security.SanitizeForPrompt(req.JobTitle)
	if err := security.ValidateShortField(jobTitle, "job title"); err != nil {
		return nil, err
	}

	companyName := security.SanitizeForPrompt(req.CompanyName)
	if err := security.ValidateShortField(companyName, "company name"); err != nil {
		return nil, err
	}

	jobDescription := security.SanitizeForPrompt(req.JobDescription)
	if jobDescription != "" {
		if err := security.ValidateJobDescription(jobDescription); err != nil {
			return nil, err
		}
	}

	result, err := s.aiService.SuggestApplyAnswer(ctx, &outbound.ApplyAssistAnswerInput{
		Question:       question,
		JobTitle:       jobTitle,
		CompanyName:    companyName,
		JobDescription: jobDescription,
		ResumeData:     resume.ParsedData,
	})
	if err != nil {
		return nil, err
	}

	return &inbound.SuggestAnswerResult{SuggestedAnswer: result.SuggestedAnswer}, nil
}
