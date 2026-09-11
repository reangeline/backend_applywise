package inbound

import (
	"context"

	"github.com/reangeline/backend_applywise/internal/core/domain"
)

type UploadResumeRequest struct {
	UserID  string
	Content string
	FileKey string
}

type OptimizeResumeRequest struct {
	UserID         string
	ResumeID       string
	JobDescription string
	TargetCompany  string
	TargetRole     string
}

// ProcessOptimizationJobRequest is consumed by async workers.
type ProcessOptimizationJobRequest struct {
	JobID          string
	UserID         string
	ResumeID       string
	JobDescription string
	TargetCompany  string
	TargetRole     string
}

type CreateManualResumeRequest struct {
	UserID     string
	ResumeID   string // optional, if empty a new one will be generated
	Nickname   string
	ParsedData map[string]interface{}
}

type UpdateManualResumeRequest struct {
	UserID     string
	ResumeID   string
	Nickname   string
	ParsedData map[string]interface{}
}

type UpdateOptimizedResumeRequest struct {
	UserID            string
	OptimizedResumeID string
	Nickname          string
	ParsedData        map[string]interface{}
}

// StartLinkedInOptimizationRequest triggers an async LinkedIn optimization job.
type StartLinkedInOptimizationRequest struct {
	UserID   string
	ResumeID string
}

// ProcessLinkedInOptimizationJobRequest is consumed by async workers.
type ProcessLinkedInOptimizationJobRequest struct {
	JobID    string
	UserID   string
	ResumeID string
}

// ParsePDFResumeRequest carries the raw PDF bytes to be parsed by the AI.
type ParsePDFResumeRequest struct {
	UserID   string
	PDFBytes []byte
	FileName string
}

// SuggestAdditionRequest carries the context needed to suggest a sentence incorporating a
// missing skill/requirement into the candidate's resume (spec 014).
type SuggestAdditionRequest struct {
	UserID         string
	ResumeID       string
	Gap            string
	JobTitle       string
	CompanyName    string
	JobDescription string
}

// SuggestAdditionResult holds the AI-suggested sentence.
type SuggestAdditionResult struct {
	SuggestedText string
}

// ResumeOptimizerService define os casos de uso de otimização de currículo
type ResumeOptimizerService interface {
	UploadResume(ctx context.Context, req UploadResumeRequest) (*domain.Resume, error)
	GetResume(ctx context.Context, userID, resumeID string) (*domain.Resume, error)
	ListResumes(ctx context.Context, userID string) ([]*domain.Resume, error)

	CreateManualResume(ctx context.Context, req CreateManualResumeRequest) (*domain.Resume, error)
	UpdateManualResume(ctx context.Context, req UpdateManualResumeRequest) (*domain.Resume, error)
	UpdateOptimizedResume(ctx context.Context, req UpdateOptimizedResumeRequest) (*domain.OptimizedResume, error)
	DeleteResume(ctx context.Context, userID, resumeID string) error

	StartOptimization(ctx context.Context, req OptimizeResumeRequest) (*domain.OptimizationJob, error)
	ProcessOptimizationJob(ctx context.Context, req ProcessOptimizationJobRequest) (*domain.OptimizedResume, error)
	GetOptimizationJob(ctx context.Context, userID, jobID string) (*domain.OptimizationJob, error)
	GetOptimizedResume(ctx context.Context, userID, optimizedResumeID string) (*domain.OptimizedResume, error)
	ListOptimizedResumes(ctx context.Context, userID string) ([]*domain.OptimizedResume, error)

	StartLinkedInOptimization(ctx context.Context, req StartLinkedInOptimizationRequest) (*domain.OptimizationJob, error)
	ProcessLinkedInOptimizationJob(ctx context.Context, req ProcessLinkedInOptimizationJobRequest) (*domain.OptimizedResume, error)

	// ParsePDFResume extracts structured resume data from a PDF file using AI.
	// It does NOT persist anything — the result is intended to pre-fill the manual resume form on the client.
	ParsePDFResume(ctx context.Context, req ParsePDFResumeRequest) (map[string]interface{}, error)

	// SuggestAddition sugere uma frase pra incorporar uma skill/requisito faltando (spec 014).
	// Não persiste nada — o resultado é um rascunho pro usuário revisar antes de salvar.
	SuggestAddition(ctx context.Context, req SuggestAdditionRequest) (*SuggestAdditionResult, error)
}
