package outbound

import (
	"context"

	"github.com/reangeline/backend_applywise/internal/core/domain"
)

type ResumeAnalysis struct {
	Skills         []string               `json:"skills"`
	Experience     []string               `json:"experience"`
	Education      []string               `json:"education"`
	Keywords       []string               `json:"keywords"`
	StructuredData map[string]interface{} `json:"structured_data"`
}

type JobAnalysis struct {
	RequiredSkills []string `json:"required_skills"`
	Keywords       []string `json:"keywords"`
	Experience     string   `json:"experience"`
}

type OptimizationResult struct {
	OptimizedContent    string                 `json:"optimized_content"`
	ParsedData          map[string]interface{} `json:"parsed_data"`
	MatchScore          float64                `json:"match_score"`
	Suggestions         []string               `json:"suggestions"`
	MissingRequirements []string               `json:"missing_requirements"`
}

// SalaryEstimate representa a estimativa salarial para um cargo/empresa.
// Quando não há dados confiáveis, Found é false e todos os valores são zero/vazio.
type SalaryEstimate struct {
	Found      bool    `json:"found"`
	Currency   string  `json:"currency,omitempty"`
	MinSalary  float64 `json:"min_salary,omitempty"`
	MaxSalary  float64 `json:"max_salary,omitempty"`
	Midpoint   float64 `json:"midpoint,omitempty"`
	Period     string  `json:"period,omitempty"` // "monthly" or "yearly"
	Location   string  `json:"location,omitempty"`
	Seniority  string  `json:"seniority,omitempty"`
	Notes      string  `json:"notes,omitempty"`
	Disclaimer string  `json:"disclaimer,omitempty"`
}

// LinkedInExperience represents one work experience item in the LinkedIn profile.
type LinkedInExperience struct {
	Role        string   `json:"role"`
	Company     string   `json:"company"`
	StartDate   string   `json:"start_date"`
	EndDate     string   `json:"end_date,omitempty"`
	IsCurrent   bool     `json:"is_current"`
	Description []string `json:"description"`
}

// LinkedInLanguage represents a language entry in the LinkedIn profile.
type LinkedInLanguage struct {
	Name  string `json:"name"`
	Level string `json:"level"`
}

// LinkedInOptimizationResult is the structured output of a LinkedIn profile optimization.
type LinkedInOptimizationResult struct {
	Headline             string               `json:"headline"`
	About                string               `json:"about"`
	Experiences          []LinkedInExperience `json:"experiences"`
	Skills               []string             `json:"skills"`
	Languages            []LinkedInLanguage   `json:"languages"`
	Suggestions          []string             `json:"suggestions"`
	ProfileStrengthScore float64              `json:"profile_strength_score"`
}

// PDFResumeData represents the structured resume data extracted from a PDF.
type PDFResumeData struct {
	Personal        map[string]interface{}   `json:"personal"`
	Experiences     []map[string]interface{} `json:"experiences"`
	Education       []map[string]interface{} `json:"education"`
	Projects        []map[string]interface{} `json:"projects"`
	Languages       []map[string]interface{} `json:"languages"`
	ATSScore        float64                  `json:"ats_score"`
	ATSImprovements []string                 `json:"ats_improvements"`
}

// CoachJobInput holds all context needed to generate stage-specific coaching content.
type CoachJobInput struct {
	Stage            string
	JobTitle         string
	CompanyName      string
	Location         string
	AtsScore         int
	JobDescription   string
	JobURL           string
	MatchedKeywords  []string
	MissingKeywords  []string
	DaysSinceApplied int
	Tone             string // "default" | "formal" | "shorter"
}

// CoachResult holds the AI-generated coaching content.
type CoachResult struct {
	Content string
	Type    string // "followup" | "interview_prep" | "offer_insights" | "feedback_request"
}

// InterviewQuestionInput holds all context needed to generate a practice interview question.
type InterviewQuestionInput struct {
	Kind            string // "behavioral" | "technical" | "situational" | "screening"
	JobTitle        string
	CompanyName     string
	JobDescription  string
	MatchedKeywords []string
	MissingKeywords []string
	// TargetGaps combina MissingKeywords (do PipelineJob) + MissingRequirements (do
	// OptimizedResume, em frase) — gaps reais do currículo pra essa vaga específica, pra
	// priorizar na geração da pergunta (spec 012), não é só contexto passivo.
	TargetGaps        []string
	ResumeData        map[string]interface{}
	PreviousQuestions []string // pra não repetir tema
	PastGaps          []string // gaps de respostas anteriores, pra mirar nos pontos fracos
}

// InterviewQuestionResult holds the AI-generated interview question.
type InterviewQuestionResult struct {
	Question     string
	WhatTheyWant string
	MethodHint   string
}

// InterviewAnswerInput holds the question and the candidate's answer to be evaluated.
type InterviewAnswerInput struct {
	Kind            string
	Question        string
	JobTitle        string
	CompanyName     string
	JobDescription  string
	ResumeData      map[string]interface{}
	CandidateAnswer string
}

// InterviewStarScores holds the STAR breakdown — only populated for behavioral questions.
type InterviewStarScores struct {
	Situation int
	Task      int
	Action    int
	Result    int
}

// InterviewAnswerResult holds the AI evaluation of a candidate's answer.
type InterviewAnswerResult struct {
	ContentScore int
	Star         *InterviewStarScores // nil se não for behavioral
	Strengths    []string
	Gaps         []string
	ModelAnswer  string
	FollowUp     string
}

// ApplyAssistAnswerInput holds the context needed to suggest an answer to an application
// screening question (Easy Apply custom questions, e.g. "years of experience with X").
type ApplyAssistAnswerInput struct {
	Question       string
	JobTitle       string
	CompanyName    string
	JobDescription string
	ResumeData     map[string]interface{}
}

// ApplyAssistAnswerResult holds the AI-suggested answer.
type ApplyAssistAnswerResult struct {
	SuggestedAnswer string
}

// ResumeAdditionInput holds the context needed to suggest a sentence incorporating a
// missing skill/requirement into the candidate's resume (spec 014).
type ResumeAdditionInput struct {
	Gap            string // a palavra-chave ou requisito faltando que a vaga pede
	JobTitle       string
	CompanyName    string
	JobDescription string
	ResumeData     map[string]interface{}
}

// ResumeAdditionResult holds the AI-suggested sentence.
type ResumeAdditionResult struct {
	SuggestedText string
}

// LinkedInScanInput holds the extracted text of a user's LinkedIn profile PDF export, to be
// audited against a fixed checklist (spec 015).
type LinkedInScanInput struct {
	ProfileText string
	TargetRole  string // opcional — se vazio, a IA infere da headline/experiência
}

// LinkedInScanResult is the structured audit report — mirrors domain.LinkedInScan minus the
// persistence fields (ID/UserID/timestamps), which the service layer fills in.
type LinkedInScanResult struct {
	Score           float64
	Sections        []domain.LinkedInScanSection
	PredictedSkills []string
	Tips            []string
}

// AIService define integração com serviço de IA
type AIService interface {
	ParseResume(ctx context.Context, content string) (*ResumeAnalysis, error)
	ParseJobDescription(ctx context.Context, content string) (*JobAnalysis, error)
	OptimizeResume(ctx context.Context, resume *ResumeAnalysis, job *JobAnalysis, originalResume string) (*OptimizationResult, error)
	EstimateSalary(ctx context.Context, targetRole, targetCompany string) (*SalaryEstimate, error)
	OptimizeForLinkedIn(ctx context.Context, resume *ResumeAnalysis) (*LinkedInOptimizationResult, error)
	ParseResumeFromText(ctx context.Context, text string) (*PDFResumeData, error)
	GenerateCoachContent(ctx context.Context, input *CoachJobInput) (*CoachResult, error)
	GenerateInterviewQuestion(ctx context.Context, input *InterviewQuestionInput) (*InterviewQuestionResult, error)
	EvaluateInterviewAnswer(ctx context.Context, input *InterviewAnswerInput) (*InterviewAnswerResult, error)
	SuggestApplyAnswer(ctx context.Context, input *ApplyAssistAnswerInput) (*ApplyAssistAnswerResult, error)
	SuggestResumeAddition(ctx context.Context, input *ResumeAdditionInput) (*ResumeAdditionResult, error)
	ScanLinkedInProfile(ctx context.Context, input *LinkedInScanInput) (*LinkedInScanResult, error)
}
