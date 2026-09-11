package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/ledongthuc/pdf"
	"github.com/reangeline/backend_applywise/internal/core/domain"
	"github.com/reangeline/backend_applywise/internal/core/ports/inbound"
	"github.com/reangeline/backend_applywise/internal/core/ports/outbound"
	"github.com/reangeline/backend_applywise/pkg/security"
)

type resumeOptimizerServiceImpl struct {
	resumeRepo            outbound.ResumeRepository
	aiService             outbound.AIService
	subscriptionRepo      outbound.SubscriptionRepository
	creditTransactionRepo outbound.CreditTransactionRepository
	queuePublisher        outbound.QueuePublisher
	jobRepo               outbound.OptimizationJobRepository
	notifier              outbound.NotificationPublisher
}

func NewResumeOptimizerService(
	resumeRepo outbound.ResumeRepository,
	aiService outbound.AIService,
	subscriptionRepo outbound.SubscriptionRepository,
	creditTransactionRepo outbound.CreditTransactionRepository,
	queuePublisher outbound.QueuePublisher,
	jobRepo outbound.OptimizationJobRepository,
	notifier outbound.NotificationPublisher,
) inbound.ResumeOptimizerService {
	if queuePublisher == nil {
		queuePublisher = noopQueuePublisher{}
	}
	if notifier == nil {
		notifier = noopNotificationPublisher{}
	}

	return &resumeOptimizerServiceImpl{
		resumeRepo:            resumeRepo,
		aiService:             aiService,
		subscriptionRepo:      subscriptionRepo,
		creditTransactionRepo: creditTransactionRepo,
		queuePublisher:        queuePublisher,
		jobRepo:               jobRepo,
		notifier:              notifier,
	}
}

func (s *resumeOptimizerServiceImpl) UploadResume(
	ctx context.Context,
	req inbound.UploadResumeRequest,
) (*domain.Resume, error) {
	// Validate resume content against prompt injection and size limits
	if err := security.ValidateResumeContent(security.SanitizeForPrompt(req.Content)); err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrInvalidResume, err)
	}
	req.Content = security.SanitizeForPrompt(req.Content)

	// Parse do currículo usando IA
	analysis, err := s.aiService.ParseResume(ctx, req.Content)
	if err != nil {
		return nil, domain.ErrInvalidResume
	}

	resume := &domain.Resume{
		ID:              uuid.New().String(),
		UserID:          req.UserID,
		OriginalContent: req.Content,
		OriginalS3Key:   req.FileKey,
		ParsedData: map[string]interface{}{
			"skills":     analysis.Skills,
			"experience": analysis.Experience,
			"education":  analysis.Education,
			"keywords":   analysis.Keywords,
		},
	}

	if err := s.resumeRepo.CreateResume(ctx, resume); err != nil {
		return nil, err
	}

	return resume, nil
}

func (s *resumeOptimizerServiceImpl) StartOptimization(
	ctx context.Context,
	req inbound.OptimizeResumeRequest,
) (*domain.OptimizationJob, error) {
	jobID := uuid.New().String()
	now := time.Now()
	job := &domain.OptimizationJob{
		ID:             jobID,
		UserID:         req.UserID,
		ResumeID:       req.ResumeID,
		JobDescription: req.JobDescription,
		Status:         domain.JobStatusQueued,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.jobRepo.Create(ctx, job); err != nil {
		return nil, err
	}

	msg := outbound.OptimizationJobMessage{
		JobID:          jobID,
		UserID:         req.UserID,
		ResumeID:       req.ResumeID,
		JobDescription: req.JobDescription,
		TargetCompany:  req.TargetCompany,
		TargetRole:     req.TargetRole,
	}

	if err := s.queuePublisher.PublishOptimizationJob(ctx, msg); err != nil {
		_ = s.jobRepo.UpdateStatus(ctx, req.UserID, jobID, domain.JobStatusFailed, err.Error(), "")
		return nil, err
	}

	return job, nil
}

func (s *resumeOptimizerServiceImpl) ProcessOptimizationJob(ctx context.Context, req inbound.ProcessOptimizationJobRequest) (*domain.OptimizedResume, error) {
	if err := s.jobRepo.UpdateStatus(ctx, req.UserID, req.JobID, domain.JobStatusProcessing, "", ""); err != nil {
		return nil, err
	}

	optimized, err := s.runOptimization(ctx, inbound.OptimizeResumeRequest{
		UserID:         req.UserID,
		ResumeID:       req.ResumeID,
		JobDescription: req.JobDescription,
		TargetCompany:  req.TargetCompany,
		TargetRole:     req.TargetRole,
	})
	if err != nil {
		_ = s.jobRepo.UpdateStatus(ctx, req.UserID, req.JobID, domain.JobStatusFailed, err.Error(), "")
		return nil, err
	}

	_ = s.jobRepo.UpdateStatus(ctx, req.UserID, req.JobID, domain.JobStatusCompleted, "", optimized.ID)
	if s.notifier != nil {
		if err := s.notifier.NotifyResumeOptimized(ctx, req.UserID, req.JobID, optimized.ID); err != nil {
			log.Printf("[notify] resume optimization push failed: userID=%s jobID=%s optimizedID=%s err=%v", req.UserID, req.JobID, optimized.ID, err)
		}
	}

	return optimized, nil
}

func (s *resumeOptimizerServiceImpl) runOptimization(
	ctx context.Context,
	req inbound.OptimizeResumeRequest,
) (*domain.OptimizedResume, error) {
	// Busca subscription
	subscription, err := s.subscriptionRepo.GetByUserID(ctx, req.UserID)
	if err != nil {
		return nil, err
	}

	// Verificar se subscription está ativa
	if !subscription.IsActive() {
		return nil, domain.ErrSubscriptionInactive
	}

	// Busca todos os currículos do usuário
	resumes, err := s.resumeRepo.ListResumesByUserID(ctx, req.UserID)
	if err != nil {
		return nil, err
	}

	// Encontra o currículo específico
	var resume *domain.Resume
	for _, r := range resumes {
		if r.ID == req.ResumeID {
			resume = r
			break
		}
	}

	if resume == nil {
		return nil, domain.ErrResumeNotFound
	}

	// Validate job description and short fields before sending to AI
	if err := security.ValidateJobDescription(security.SanitizeForPrompt(req.JobDescription)); err != nil {
		return nil, err
	}
	if req.TargetCompany != "" {
		if err := security.ValidateShortField(req.TargetCompany, "target company"); err != nil {
			return nil, err
		}
	}
	if req.TargetRole != "" {
		if err := security.ValidateShortField(req.TargetRole, "target role"); err != nil {
			return nil, err
		}
	}
	req.JobDescription = security.SanitizeForPrompt(req.JobDescription)

	// Parse da job description
	jobAnalysis, err := s.aiService.ParseJobDescription(ctx, req.JobDescription)
	if err != nil {
		return nil, err
	}

	// Reconstrói análise do currículo - para currículos manuais, usar ParsedData diretamente
	resumeAnalysis := &outbound.ResumeAnalysis{
		Skills:         extractSkillsFromParsedData(resume.ParsedData),
		Experience:     extractExperiencesFromParsedData(resume.ParsedData),
		Education:      extractEducationFromParsedData(resume.ParsedData),
		Keywords:       convertToStringSlice(resume.ParsedData["keywords"]),
		StructuredData: resume.ParsedData,
	}

	// Otimiza usando IA
	result, err := s.aiService.OptimizeResume(ctx, resumeAnalysis, jobAnalysis, resume.OriginalContent)
	if err != nil {
		return nil, err
	}

	// Combine dados estruturados originais com otimizações para preservar campos editáveis no app
	combinedParsedData := mergeParsedData(resume.ParsedData, result.ParsedData)
	if combinedParsedData == nil {
		combinedParsedData = map[string]interface{}{}
	}
	if req.TargetCompany != "" {
		combinedParsedData["target_company"] = req.TargetCompany
	}
	if req.TargetRole != "" {
		combinedParsedData["target_role"] = req.TargetRole
	}

	missingRequirements := result.MissingRequirements
	if len(missingRequirements) == 0 {
		missingRequirements = detectMissingRequirements(jobAnalysis, resumeAnalysis, combinedParsedData)
	}

	coverage := calculateCoverageScore(jobAnalysis, resumeAnalysis, combinedParsedData)
	result.MatchScore = computeMatchScore(result.MatchScore, coverage, len(missingRequirements), len(jobAnalysis.RequiredSkills))

	suggestions := augmentSuggestionsWithGaps(result.Suggestions, missingRequirements)

	// Estimativa salarial — executada apenas quando há cargo/empresa informados.
	// Falha silenciosa: se a IA não encontrar dados, o campo fica nil ou Found=false.
	var domainSalary *domain.SalaryEstimate
	if req.TargetRole != "" || req.TargetCompany != "" {
		salaryCtx, salaryCancel := context.WithTimeout(ctx, 20*time.Second)
		defer salaryCancel()
		if salaryEst, err := s.aiService.EstimateSalary(salaryCtx, req.TargetRole, req.TargetCompany); err == nil && salaryEst != nil {
			domainSalary = &domain.SalaryEstimate{
				Found:      salaryEst.Found,
				Currency:   salaryEst.Currency,
				MinSalary:  salaryEst.MinSalary,
				MaxSalary:  salaryEst.MaxSalary,
				Midpoint:   salaryEst.Midpoint,
				Period:     salaryEst.Period,
				Location:   salaryEst.Location,
				Seniority:  salaryEst.Seniority,
				Notes:      salaryEst.Notes,
				Disclaimer: salaryEst.Disclaimer,
			}
		}
	}

	// Salva resultado
	jobDescriptionID := uuid.New().String()
	optimized := &domain.OptimizedResume{
		ID:                  uuid.New().String(),
		UserID:              req.UserID,
		ResumeID:            req.ResumeID,
		SourceResumeID:      req.ResumeID,
		JobDescriptionID:    jobDescriptionID,
		OptimizedContent:    result.OptimizedContent,
		ParsedData:          combinedParsedData,
		MatchScore:          result.MatchScore,
		Suggestions:         suggestions,
		MissingRequirements: missingRequirements,
		SalaryEstimate:      domainSalary,
		CreatedAt:           time.Now(),
	}

	if err := s.resumeRepo.CreateOptimizedResume(ctx, optimized); err != nil {
		return nil, err
	}

	return optimized, nil
}

func (s *resumeOptimizerServiceImpl) GetResume(ctx context.Context, userID, resumeID string) (*domain.Resume, error) {
	// GetResume agora recebe userID e faz verificação de ownership internamente no repository
	return s.resumeRepo.GetResume(ctx, userID, resumeID)
}

func (s *resumeOptimizerServiceImpl) GetOptimizationJob(ctx context.Context, userID, jobID string) (*domain.OptimizationJob, error) {
	return s.jobRepo.Get(ctx, userID, jobID)
}

func (s *resumeOptimizerServiceImpl) ListResumes(ctx context.Context, userID string) ([]*domain.Resume, error) {
	return s.resumeRepo.ListResumesByUserID(ctx, userID)
}

func (s *resumeOptimizerServiceImpl) CreateManualResume(ctx context.Context, req inbound.CreateManualResumeRequest) (*domain.Resume, error) {
	id := req.ResumeID
	if id == "" {
		id = uuid.New().String()
	}

	now := time.Now()
	resume := &domain.Resume{
		ID:              id,
		UserID:          req.UserID,
		OriginalContent: "",
		ParsedData:      req.ParsedData,
		Type:            "manual",
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	// ensure nickname if provided
	if req.Nickname != "" {
		if resume.ParsedData == nil {
			resume.ParsedData = map[string]interface{}{}
		}
		resume.ParsedData["nickname"] = req.Nickname
	}

	if err := s.resumeRepo.CreateResume(ctx, resume); err != nil {
		return nil, err
	}

	return resume, nil
}

func (s *resumeOptimizerServiceImpl) UpdateManualResume(ctx context.Context, req inbound.UpdateManualResumeRequest) (*domain.Resume, error) {
	resume, err := s.resumeRepo.GetResume(ctx, req.UserID, req.ResumeID)
	if err != nil {
		return nil, err
	}

	if resume.ParsedData == nil {
		resume.ParsedData = map[string]interface{}{}
	}

	// Garantir que currículos manuais mantenham o tipo para não perder campos estruturados na volta
	if resume.Type == "" {
		resume.Type = "manual"
	}

	// merge parsed data (replace keys provided)
	for k, v := range req.ParsedData {
		resume.ParsedData[k] = v
	}

	if req.Nickname != "" {
		resume.ParsedData["nickname"] = req.Nickname
	}

	resume.UpdatedAt = time.Now()

	if err := s.resumeRepo.UpdateResume(ctx, resume); err != nil {
		return nil, err
	}

	return resume, nil
}

func (s *resumeOptimizerServiceImpl) UpdateOptimizedResume(ctx context.Context, req inbound.UpdateOptimizedResumeRequest) (*domain.OptimizedResume, error) {
	// Fetch existing to verify ownership
	list, err := s.resumeRepo.ListOptimizedResumesByUserID(ctx, req.UserID)
	if err != nil {
		return nil, err
	}

	var existing *domain.OptimizedResume
	for _, r := range list {
		if r.ID == req.OptimizedResumeID {
			existing = r
			break
		}
	}
	if existing == nil {
		return nil, domain.ErrResumeNotFound
	}

	if existing.ParsedData == nil {
		existing.ParsedData = map[string]interface{}{}
	}

	// Merge incoming ParsedData
	for k, v := range req.ParsedData {
		existing.ParsedData[k] = v
	}

	// Update nickname inside ParsedData if provided
	if req.Nickname != "" {
		existing.ParsedData["nickname"] = req.Nickname
	}

	if err := s.resumeRepo.UpdateOptimizedResume(ctx, existing); err != nil {
		return nil, err
	}

	return existing, nil
}

func (s *resumeOptimizerServiceImpl) DeleteResume(ctx context.Context, userID, resumeID string) error {
	// Tenta remover currículo original; se não existir, tenta remover currículo otimizado
	err := s.resumeRepo.DeleteResume(ctx, userID, resumeID)
	if err == nil {
		return nil
	}

	if errors.Is(err, domain.ErrResumeNotFound) {
		return s.resumeRepo.DeleteOptimizedResume(ctx, userID, resumeID)
	}

	return err
}

func (s *resumeOptimizerServiceImpl) GetOptimizedResume(ctx context.Context, userID, optimizedResumeID string) (*domain.OptimizedResume, error) {
	optimized, err := s.resumeRepo.GetOptimizedResume(ctx, userID, optimizedResumeID)
	if err != nil {
		return nil, err
	}

	return optimized, nil
}

func (s *resumeOptimizerServiceImpl) ListOptimizedResumes(ctx context.Context, userID string) ([]*domain.OptimizedResume, error) {
	return s.resumeRepo.ListOptimizedResumesByUserID(ctx, userID)
}

// StartLinkedInOptimization queues an async LinkedIn optimization job for a manual resume.
func (s *resumeOptimizerServiceImpl) StartLinkedInOptimization(
	ctx context.Context,
	req inbound.StartLinkedInOptimizationRequest,
) (*domain.OptimizationJob, error) {
	jobID := uuid.New().String()
	now := time.Now()
	job := &domain.OptimizationJob{
		ID:        jobID,
		UserID:    req.UserID,
		ResumeID:  req.ResumeID,
		Status:    domain.JobStatusQueued,
		Metadata:  map[string]interface{}{"job_type": outbound.JobTypeLinkedIn},
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.jobRepo.Create(ctx, job); err != nil {
		return nil, err
	}

	msg := outbound.OptimizationJobMessage{
		JobID:    jobID,
		UserID:   req.UserID,
		ResumeID: req.ResumeID,
		JobType:  outbound.JobTypeLinkedIn,
	}

	if err := s.queuePublisher.PublishOptimizationJob(ctx, msg); err != nil {
		_ = s.jobRepo.UpdateStatus(ctx, req.UserID, jobID, domain.JobStatusFailed, err.Error(), "")
		return nil, err
	}

	return job, nil
}

// ProcessLinkedInOptimizationJob is called by the async worker to execute the LinkedIn optimization.
func (s *resumeOptimizerServiceImpl) ProcessLinkedInOptimizationJob(
	ctx context.Context,
	req inbound.ProcessLinkedInOptimizationJobRequest,
) (*domain.OptimizedResume, error) {
	if err := s.jobRepo.UpdateStatus(ctx, req.UserID, req.JobID, domain.JobStatusProcessing, "", ""); err != nil {
		return nil, err
	}

	// Verify subscription
	subscription, err := s.subscriptionRepo.GetByUserID(ctx, req.UserID)
	if err != nil {
		_ = s.jobRepo.UpdateStatus(ctx, req.UserID, req.JobID, domain.JobStatusFailed, err.Error(), "")
		return nil, err
	}

	if !subscription.IsActive() {
		_ = s.jobRepo.UpdateStatus(ctx, req.UserID, req.JobID, domain.JobStatusFailed, domain.ErrSubscriptionInactive.Error(), "")
		return nil, domain.ErrSubscriptionInactive
	}

	// Fetch resume
	resume, err := s.resumeRepo.GetResume(ctx, req.UserID, req.ResumeID)
	if err != nil {
		_ = s.jobRepo.UpdateStatus(ctx, req.UserID, req.JobID, domain.JobStatusFailed, err.Error(), "")
		return nil, err
	}

	resumeAnalysis := &outbound.ResumeAnalysis{
		Skills:         extractSkillsFromParsedData(resume.ParsedData),
		Experience:     extractExperiencesFromParsedData(resume.ParsedData),
		Education:      extractEducationFromParsedData(resume.ParsedData),
		Keywords:       convertToStringSlice(resume.ParsedData["keywords"]),
		StructuredData: resume.ParsedData,
	}

	// Call AI
	linkedInResult, err := s.aiService.OptimizeForLinkedIn(ctx, resumeAnalysis)
	if err != nil {
		_ = s.jobRepo.UpdateStatus(ctx, req.UserID, req.JobID, domain.JobStatusFailed, err.Error(), "")
		return nil, err
	}

	// Persist result as an OptimizedResume with type=linkedin
	parsedData := map[string]interface{}{
		"type":                   "linkedin",
		"headline":               linkedInResult.Headline,
		"about":                  linkedInResult.About,
		"experiences":            linkedInResult.Experiences,
		"skills":                 linkedInResult.Skills,
		"languages":              linkedInResult.Languages,
		"profile_strength_score": linkedInResult.ProfileStrengthScore,
	}

	optimized := &domain.OptimizedResume{
		ID:               uuid.New().String(),
		UserID:           req.UserID,
		ResumeID:         req.ResumeID,
		SourceResumeID:   req.ResumeID,
		OptimizedContent: linkedInResult.About,
		ParsedData:       parsedData,
		MatchScore:       linkedInResult.ProfileStrengthScore,
		Suggestions:      linkedInResult.Suggestions,
		CreatedAt:        time.Now(),
	}

	if err := s.resumeRepo.CreateOptimizedResume(ctx, optimized); err != nil {
		_ = s.jobRepo.UpdateStatus(ctx, req.UserID, req.JobID, domain.JobStatusFailed, err.Error(), "")
		return nil, err
	}

	_ = s.jobRepo.UpdateStatus(ctx, req.UserID, req.JobID, domain.JobStatusCompleted, "", optimized.ID)

	if s.notifier != nil {
		if err := s.notifier.NotifyLinkedInOptimized(ctx, req.UserID, req.JobID, optimized.ID); err != nil {
			log.Printf("[notify] linkedin optimization push failed: userID=%s jobID=%s optimizedID=%s err=%v", req.UserID, req.JobID, optimized.ID, err)
		}
	}

	return optimized, nil
}

type noopNotificationPublisher struct{}

func (noopNotificationPublisher) NotifyResumeOptimized(ctx context.Context, userID, jobID, optimizedResumeID string) error {
	return nil
}

func (noopNotificationPublisher) NotifyLinkedInOptimized(ctx context.Context, userID, jobID, optimizedResumeID string) error {
	return nil
}

func (noopNotificationPublisher) NotifyInterviewScheduled(ctx context.Context, userID, jobID, companyName, interviewType, interviewAt string) error {
	return nil
}

type noopQueuePublisher struct{}

func (noopQueuePublisher) PublishOptimizationJob(ctx context.Context, msg outbound.OptimizationJobMessage) error {
	return nil
}

func convertToStringSlice(data interface{}) []string {
	if data == nil {
		return []string{}
	}

	// Se já é []string, retorna direto
	if strSlice, ok := data.([]string); ok {
		return strSlice
	}

	// Se é []interface{}, converte cada item
	if interfaceSlice, ok := data.([]interface{}); ok {
		result := make([]string, len(interfaceSlice))
		for i, v := range interfaceSlice {
			if str, ok := v.(string); ok {
				result[i] = str
			}
		}
		return result
	}

	return []string{}
}

// extractSkillsFromParsedData extrai skills do ParsedData de currículo manual
func extractSkillsFromParsedData(data map[string]interface{}) []string {
	skills := []string{}

	// Extrai skills de experiences
	if exps, ok := data["experiences"].([]interface{}); ok {
		for _, exp := range exps {
			if expMap, ok := exp.(map[string]interface{}); ok {
				if desc, ok := expMap["description"].(string); ok && desc != "" {
					skills = append(skills, desc)
				}
			}
		}
	}

	return skills
}

// extractExperiencesFromParsedData extrai descrições de experiências
func extractExperiencesFromParsedData(data map[string]interface{}) []string {
	experiences := []string{}

	if exps, ok := data["experiences"].([]interface{}); ok {
		for _, exp := range exps {
			if expMap, ok := exp.(map[string]interface{}); ok {
				role, _ := expMap["role"].(string)
				company, _ := expMap["company"].(string)
				description, _ := expMap["description"].(string)

				if role != "" && company != "" {
					experiences = append(experiences, fmt.Sprintf("%s at %s: %s", role, company, description))
				}
			}
		}
	}

	return experiences
}

// extractEducationFromParsedData extrai informações de educação
func extractEducationFromParsedData(data map[string]interface{}) []string {
	education := []string{}

	if edus, ok := data["education"].([]interface{}); ok {
		for _, edu := range edus {
			if eduMap, ok := edu.(map[string]interface{}); ok {
				degree, _ := eduMap["degree"].(string)
				institution, _ := eduMap["institution"].(string)

				if degree != "" && institution != "" {
					education = append(education, fmt.Sprintf("%s from %s", degree, institution))
				}
			}
		}
	}

	return education
}

// mergeParsedData preserva os campos estruturados originais e aplica otimizações quando disponíveis
func mergeParsedData(original, optimized map[string]interface{}) map[string]interface{} {
	merged := cloneMap(original)
	if merged == nil {
		merged = map[string]interface{}{}
	}

	if len(optimized) == 0 {
		return merged
	}

	for key, incoming := range optimized {
		merged[key] = mergeValue(merged[key], incoming)
	}

	return merged
}

func mergeValue(existing, incoming interface{}) interface{} {
	switch val := incoming.(type) {
	case map[string]interface{}:
		var base map[string]interface{}
		if existingMap, ok := existing.(map[string]interface{}); ok {
			base = existingMap
		}
		return mergeParsedData(base, val)
	case []interface{}:
		if len(val) == 0 && existing != nil {
			return existing
		}
		return cloneValue(val)
	case nil:
		if existing != nil {
			return existing
		}
		return nil
	default:
		return val
	}
}

func cloneMap(data map[string]interface{}) map[string]interface{} {
	if data == nil {
		return nil
	}

	cloned := make(map[string]interface{}, len(data))
	for key, value := range data {
		cloned[key] = cloneValue(value)
	}

	return cloned
}

func cloneValue(value interface{}) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		return cloneMap(v)
	case []interface{}:
		copied := make([]interface{}, len(v))
		for i, item := range v {
			copied[i] = cloneValue(item)
		}
		return copied
	default:
		return v
	}
}

// detectMissingRequirements garante um campo destacado com lacunas entre currículo e vaga
func detectMissingRequirements(job *outbound.JobAnalysis, resumeAnalysis *outbound.ResumeAnalysis, parsedData map[string]interface{}) []string {
	if job == nil || resumeAnalysis == nil {
		return []string{}
	}

	seen := map[string]struct{}{}
	result := []string{}

	textBuilder := strings.Builder{}
	appendParts := func(parts []string) {
		for _, part := range parts {
			if part == "" {
				continue
			}
			textBuilder.WriteString(" ")
			textBuilder.WriteString(strings.ToLower(part))
		}
	}

	appendParts(resumeAnalysis.Skills)
	appendParts(resumeAnalysis.Experience)
	appendParts(resumeAnalysis.Education)
	appendParts(resumeAnalysis.Keywords)
	textBuilder.WriteString(strings.ToLower(flattenParsedText(parsedData)))

	combinedText := textBuilder.String()

	for _, required := range job.RequiredSkills {
		required = strings.TrimSpace(required)
		if required == "" {
			continue
		}

		key := strings.ToLower(required)
		if _, already := seen[key]; already {
			continue
		}

		if !strings.Contains(combinedText, key) {
			seen[key] = struct{}{}
			result = append(result, required)
		}
	}

	return result
}

func flattenParsedText(parsed map[string]interface{}) string {
	if parsed == nil {
		return ""
	}

	b := strings.Builder{}

	if personal, ok := parsed["personal"].(map[string]interface{}); ok {
		for _, key := range []string{"summary", "current_role", "full_name", "headline"} {
			if val, ok := personal[key].(string); ok {
				b.WriteString(" ")
				b.WriteString(val)
			}
		}
	}

	if experiences, ok := parsed["experiences"].([]interface{}); ok {
		for _, exp := range experiences {
			if expMap, ok := exp.(map[string]interface{}); ok {
				for _, key := range []string{"role", "company", "description"} {
					if val, ok := expMap[key].(string); ok {
						b.WriteString(" ")
						b.WriteString(val)
					}
				}
			}
		}
	}

	if education, ok := parsed["education"].([]interface{}); ok {
		for _, edu := range education {
			if eduMap, ok := edu.(map[string]interface{}); ok {
				for _, key := range []string{"degree", "institution"} {
					if val, ok := eduMap[key].(string); ok {
						b.WriteString(" ")
						b.WriteString(val)
					}
				}
			}
		}
	}

	if projects, ok := parsed["projects"].([]interface{}); ok {
		for _, proj := range projects {
			if projMap, ok := proj.(map[string]interface{}); ok {
				for _, key := range []string{"name", "description"} {
					if val, ok := projMap[key].(string); ok {
						b.WriteString(" ")
						b.WriteString(val)
					}
				}
			}
		}
	}

	if skills, ok := parsed["skills"].([]interface{}); ok {
		for _, skill := range skills {
			if val, ok := skill.(string); ok {
				b.WriteString(" ")
				b.WriteString(val)
			}
		}
	}

	if skillStrings, ok := parsed["skills"].([]string); ok {
		for _, skill := range skillStrings {
			if skill == "" {
				continue
			}
			b.WriteString(" ")
			b.WriteString(skill)
		}
	}

	if languages, ok := parsed["languages"].([]interface{}); ok {
		for _, lang := range languages {
			if langMap, ok := lang.(map[string]interface{}); ok {
				if name, ok := langMap["language"].(string); ok && name != "" {
					b.WriteString(" ")
					b.WriteString(name)
				}

				if prof, ok := langMap["proficiency"].(string); ok && prof != "" {
					b.WriteString(" ")
					b.WriteString(prof)
				}
			}
		}
	}

	return b.String()
}

func calculateCoverageScore(job *outbound.JobAnalysis, resumeAnalysis *outbound.ResumeAnalysis, parsedData map[string]interface{}) float64 {
	if job == nil || len(job.RequiredSkills) == 0 {
		return 100
	}

	combinedText := strings.ToLower(flattenParsedText(parsedData))

	matches := 0
	for _, required := range job.RequiredSkills {
		req := strings.TrimSpace(strings.ToLower(required))
		if req == "" {
			continue
		}

		if strings.Contains(combinedText, req) {
			matches++
		}
	}

	coverage := (float64(matches) / float64(len(job.RequiredSkills))) * 100
	if coverage < 0 {
		coverage = 0
	}
	if coverage > 100 {
		coverage = 100
	}

	return coverage
}

func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

// computeMatchScore blends AI score and textual coverage, with a penalty for missing requirements.
// This avoids artificially high scores when required skills are absent.
func computeMatchScore(aiScore float64, coverageScore float64, missing int, totalRequired int) float64 {
	ai := clamp01(aiScore / 100)
	cov := clamp01(coverageScore / 100)

	missingPenalty := 0.0
	if totalRequired > 0 && missing > 0 {
		missingRatio := float64(missing) / float64(totalRequired)
		missingPenalty = clamp01(missingRatio) * 0.6 // up to -60% for missing all
	}

	// Weight coverage higher than AI score, because coverage is deterministic.
	weighted := (0.65 * cov) + (0.35 * ai)
	weighted = weighted * (1 - missingPenalty)

	final := weighted * 100
	return math.Max(0, math.Min(100, final))
}

// augmentSuggestionsWithGaps garante que as sugestões tragam gaps explícitos (ex.: aprender Rust)
func augmentSuggestionsWithGaps(existing []string, missing []string) []string {
	result := make([]string, 0, len(existing)+len(missing))
	result = append(result, existing...)

	seen := map[string]struct{}{}
	for _, s := range existing {
		seen[strings.ToLower(strings.TrimSpace(s))] = struct{}{}
	}

	for _, gap := range missing {
		gap = strings.TrimSpace(gap)
		if gap == "" {
			continue
		}

		norm := strings.ToLower(gap)
		if _, ok := seen[norm]; ok {
			continue
		}

		suggestion := fmt.Sprintf("Missing requirement: %s. The job asks for %s. If you have it, add clear examples; otherwise, highlight an equivalent tool or experience that covers this need.", gap, gap)
		result = append(result, suggestion)
		seen[norm] = struct{}{}
	}

	return result
}

// extractTextFromPDF reads plain text from PDF bytes using ledongthuc/pdf.
// Returns the extracted text, or an error if extraction fails.
func extractTextFromPDF(data []byte) (string, error) {
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("failed to read PDF: %w", err)
	}

	var sb strings.Builder
	for i := 1; i <= r.NumPage(); i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			continue // skip pages we cannot read
		}
		sb.WriteString(text)
		sb.WriteString("\n")
	}

	result := strings.TrimSpace(sb.String())
	if result == "" {
		return "", fmt.Errorf("no readable text found in PDF — the file may be scanned or image-based")
	}
	return result, nil
}

// ParsePDFResume extracts structured resume data from a PDF and returns it as
// a map ready for the client to pre-fill the manual resume form.
// Nothing is saved to the database.
func (s *resumeOptimizerServiceImpl) ParsePDFResume(ctx context.Context, req inbound.ParsePDFResumeRequest) (map[string]interface{}, error) {
	text, err := extractTextFromPDF(req.PDFBytes)
	if err != nil {
		return nil, fmt.Errorf("PDF text extraction failed: %w", err)
	}

	parsed, err := s.aiService.ParseResumeFromText(ctx, text)
	if err != nil {
		return nil, fmt.Errorf("AI parsing failed: %w", err)
	}

	// Build a response shaped like a manual resume so the Flutter client can
	// deserialise it with the existing Resume.fromJson() without changes.
	now := time.Now().UTC().Format(time.RFC3339)
	return map[string]interface{}{
		"id":         uuid.New().String(), // temporary — not persisted
		"type":       "manual",
		"created_at": now,
		"parsed_data": map[string]interface{}{
			"personal":         parsed.Personal,
			"experiences":      parsed.Experiences,
			"education":        parsed.Education,
			"projects":         parsed.Projects,
			"languages":        parsed.Languages,
			"ats_score":        parsed.ATSScore,
			"ats_improvements": parsed.ATSImprovements,
		},
	}, nil
}

// SuggestAddition sugere uma frase pra incorporar uma skill/requisito que a vaga pede e o
// currículo não mostra (spec 014). Não persiste nada — o usuário revisa/edita antes de
// salvar. Sem checagem de crédito/plano: resume stuff é grátis pra qualquer cadastrado
// desde a spec 013.
func (s *resumeOptimizerServiceImpl) SuggestAddition(ctx context.Context, req inbound.SuggestAdditionRequest) (*inbound.SuggestAdditionResult, error) {
	resume, err := s.resumeRepo.GetResume(ctx, req.UserID, req.ResumeID)
	if err != nil {
		return nil, err
	}

	gap := security.SanitizeForPrompt(req.Gap)
	if err := security.ValidateShortField(gap, "gap"); err != nil {
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

	result, err := s.aiService.SuggestResumeAddition(ctx, &outbound.ResumeAdditionInput{
		Gap:            gap,
		JobTitle:       jobTitle,
		CompanyName:    companyName,
		JobDescription: jobDescription,
		ResumeData:     resume.ParsedData,
	})
	if err != nil {
		return nil, err
	}

	return &inbound.SuggestAdditionResult{SuggestedText: result.SuggestedText}, nil
}
