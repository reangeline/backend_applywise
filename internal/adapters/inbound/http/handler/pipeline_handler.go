package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/reangeline/backend_applywise/internal/adapters/inbound/http/middleware"
	"github.com/reangeline/backend_applywise/internal/analytics"
	"github.com/reangeline/backend_applywise/internal/core/domain"
	"github.com/reangeline/backend_applywise/internal/core/ports/inbound"
	"github.com/reangeline/backend_applywise/internal/core/ports/outbound"
)

type PipelineHandler struct {
	pipelineRepo     outbound.PipelineRepository
	contactRepo      outbound.ContactRepository
	userRepo         outbound.UserRepository
	notifier         outbound.NotificationPublisher
	coachService     inbound.PipelineCoachService
	interviewService inbound.InterviewPracticeService
}

func NewPipelineHandler(
	pipelineRepo outbound.PipelineRepository,
	contactRepo outbound.ContactRepository,
	userRepo outbound.UserRepository,
	notifier outbound.NotificationPublisher,
	coachService inbound.PipelineCoachService,
	interviewService inbound.InterviewPracticeService,
) *PipelineHandler {
	return &PipelineHandler{
		pipelineRepo:     pipelineRepo,
		contactRepo:      contactRepo,
		userRepo:         userRepo,
		notifier:         notifier,
		coachService:     coachService,
		interviewService: interviewService,
	}
}

// ─── DTOs ────────────────────────────────────────────────────────────────────

type TimelineEventResponse struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Label     string `json:"label"`
	Detail    string `json:"detail,omitempty"`
	CreatedAt string `json:"created_at"`
}

type PipelineJobResponse struct {
	ID                string                  `json:"id"`
	UserID            string                  `json:"user_id"`
	CompanyName       string                  `json:"company_name"`
	JobTitle          string                  `json:"job_title"`
	Location          string                  `json:"location,omitempty"`
	Stage             string                  `json:"stage"`
	ResumeID          string                  `json:"resume_id,omitempty"`
	OptimizedResumeID string                  `json:"optimized_resume_id,omitempty"`
	AtsScore          int                     `json:"ats_score,omitempty"`
	MatchedKeywords   []string                `json:"matched_keywords,omitempty"`
	MissingKeywords   []string                `json:"missing_keywords,omitempty"`
	JobDescription    string                  `json:"job_description,omitempty"`
	JobURL            string                  `json:"job_url,omitempty"`
	IsGhosted         bool                    `json:"is_ghosted"`
	IsArchived        bool                    `json:"is_archived"`
	InterviewAt       *string                 `json:"interview_at,omitempty"`
	InterviewType     string                  `json:"interview_type,omitempty"`
	Timeline          []TimelineEventResponse `json:"timeline,omitempty"`
	CreatedAt         string                  `json:"created_at"`
	UpdatedAt         string                  `json:"updated_at"`
}

type createPipelineJobRequest struct {
	CompanyName       string   `json:"company_name"`
	JobTitle          string   `json:"job_title"`
	Location          string   `json:"location,omitempty"`
	Stage             string   `json:"stage,omitempty"`
	ResumeID          string   `json:"resume_id,omitempty"`
	OptimizedResumeID string   `json:"optimized_resume_id,omitempty"`
	AtsScore          int      `json:"ats_score,omitempty"`
	MatchedKeywords   []string `json:"matched_keywords,omitempty"`
	MissingKeywords   []string `json:"missing_keywords,omitempty"`
	JobDescription    string   `json:"job_description,omitempty"`
	JobURL            string   `json:"job_url,omitempty"`
	IsArchived        bool     `json:"is_archived,omitempty"`
}

type updatePipelineJobRequest struct {
	CompanyName       string   `json:"company_name,omitempty"`
	JobTitle          string   `json:"job_title,omitempty"`
	Location          string   `json:"location,omitempty"`
	Stage             string   `json:"stage,omitempty"`
	ResumeID          string   `json:"resume_id,omitempty"`
	OptimizedResumeID string   `json:"optimized_resume_id,omitempty"`
	AtsScore          *int     `json:"ats_score,omitempty"`
	MatchedKeywords   []string `json:"matched_keywords,omitempty"`
	MissingKeywords   []string `json:"missing_keywords,omitempty"`
	JobDescription    string   `json:"job_description,omitempty"`
	JobURL            string   `json:"job_url,omitempty"`
	IsGhosted         *bool    `json:"is_ghosted,omitempty"`
	IsArchived        *bool    `json:"is_archived,omitempty"`
}

type logInterviewRequest struct {
	InterviewAt   string `json:"interview_at"`
	InterviewType string `json:"interview_type,omitempty"`
	Detail        string `json:"detail,omitempty"`
}

const iso8601MillisLayout = "2006-01-02T15:04:05.000Z"

type logFollowUpRequest struct {
	Detail string `json:"detail,omitempty"`
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func toPipelineJobResponse(job *domain.PipelineJob) PipelineJobResponse {
	resp := PipelineJobResponse{
		ID:                job.ID,
		UserID:            job.UserID,
		CompanyName:       job.CompanyName,
		JobTitle:          job.JobTitle,
		Location:          job.Location,
		Stage:             string(job.Stage),
		ResumeID:          job.ResumeID,
		OptimizedResumeID: job.OptimizedResumeID,
		AtsScore:          job.AtsScore,
		MatchedKeywords:   job.MatchedKeywords,
		MissingKeywords:   job.MissingKeywords,
		JobDescription:    job.JobDescription,
		JobURL:            job.JobURL,
		IsGhosted:         job.IsGhosted,
		IsArchived:        job.IsArchived,
		InterviewType:     job.InterviewType,
		CreatedAt:         job.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         job.UpdatedAt.Format(time.RFC3339),
	}
	if job.InterviewAt != nil {
		s := job.InterviewAt.UTC().Format(iso8601MillisLayout)
		resp.InterviewAt = &s
	}
	if len(job.Timeline) > 0 {
		resp.Timeline = make([]TimelineEventResponse, len(job.Timeline))
		for i, e := range job.Timeline {
			resp.Timeline[i] = TimelineEventResponse{
				ID:        e.ID,
				Type:      string(e.Type),
				Label:     e.Label,
				Detail:    e.Detail,
				CreatedAt: e.CreatedAt.Format(time.RFC3339),
			}
		}
	}
	return resp
}

// ─── Handlers ────────────────────────────────────────────────────────────────

func (h *PipelineHandler) ListJobs(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDContextKey).(string)
	if !ok || userID == "" {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	jobs, err := h.pipelineRepo.List(r.Context(), userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := make([]PipelineJobResponse, 0, len(jobs))
	for i := range jobs {
		resp = append(resp, toPipelineJobResponse(&jobs[i]))
	}
	respondJSON(w, http.StatusOK, resp)
}

func (h *PipelineHandler) CreateJob(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDContextKey).(string)
	if !ok || userID == "" {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req createPipelineJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.CompanyName == "" || req.JobTitle == "" {
		respondError(w, http.StatusBadRequest, "company_name and job_title are required")
		return
	}

	stage := domain.NormalizePipelineJobStage(req.Stage)

	now := time.Now()
	job := &domain.PipelineJob{
		ID:                uuid.New().String(),
		UserID:            userID,
		CompanyName:       req.CompanyName,
		JobTitle:          req.JobTitle,
		Location:          req.Location,
		Stage:             stage,
		ResumeID:          req.ResumeID,
		OptimizedResumeID: req.OptimizedResumeID,
		AtsScore:          req.AtsScore,
		MatchedKeywords:   req.MatchedKeywords,
		MissingKeywords:   req.MissingKeywords,
		JobDescription:    req.JobDescription,
		JobURL:            req.JobURL,
		IsArchived:        req.IsArchived,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	// Add initial timeline event
	job.Timeline = []domain.TimelineEvent{
		{
			ID:        uuid.New().String(),
			Type:      domain.TimelineApplied,
			Label:     "Added to pipeline",
			CreatedAt: now,
		},
	}

	if err := h.pipelineRepo.Create(r.Context(), job); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, toPipelineJobResponse(job))
}

func (h *PipelineHandler) GetJob(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDContextKey).(string)
	if !ok || userID == "" {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	jobID := chi.URLParam(r, "jobId")

	job, err := h.pipelineRepo.Get(r.Context(), userID, jobID)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, toPipelineJobResponse(job))
}

func (h *PipelineHandler) UpdateJob(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDContextKey).(string)
	if !ok || userID == "" {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	jobID := chi.URLParam(r, "jobId")

	existing, err := h.pipelineRepo.Get(r.Context(), userID, jobID)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	var req updatePipelineJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.CompanyName != "" {
		existing.CompanyName = req.CompanyName
	}
	if req.JobTitle != "" {
		existing.JobTitle = req.JobTitle
	}
	if req.Location != "" {
		existing.Location = req.Location
	}
	if req.Stage != "" {
		existing.Stage = domain.NormalizePipelineJobStage(req.Stage)
	}
	if req.ResumeID != "" {
		existing.ResumeID = req.ResumeID
	}
	if req.OptimizedResumeID != "" {
		existing.OptimizedResumeID = req.OptimizedResumeID
	}
	if req.AtsScore != nil {
		existing.AtsScore = *req.AtsScore
	}
	if req.MatchedKeywords != nil {
		existing.MatchedKeywords = req.MatchedKeywords
	}
	if req.MissingKeywords != nil {
		existing.MissingKeywords = req.MissingKeywords
	}
	if req.JobDescription != "" {
		existing.JobDescription = req.JobDescription
	}
	if req.JobURL != "" {
		existing.JobURL = req.JobURL
	}
	if req.IsGhosted != nil {
		existing.IsGhosted = *req.IsGhosted
	}
	if req.IsArchived != nil {
		existing.IsArchived = *req.IsArchived
	}

	if err := h.pipelineRepo.Update(r.Context(), existing); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, toPipelineJobResponse(existing))
}

func (h *PipelineHandler) DeleteJob(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDContextKey).(string)
	if !ok || userID == "" {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	jobID := chi.URLParam(r, "jobId")

	if err := h.pipelineRepo.Delete(r.Context(), userID, jobID); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *PipelineHandler) GhostJob(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDContextKey).(string)
	if !ok || userID == "" {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	jobID := chi.URLParam(r, "jobId")

	job, err := h.pipelineRepo.Get(r.Context(), userID, jobID)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	job.IsGhosted = true
	if err := h.pipelineRepo.Update(r.Context(), job); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	event := domain.TimelineEvent{
		ID:        uuid.New().String(),
		Type:      domain.TimelineGhosted,
		Label:     "Marked as ghosted",
		CreatedAt: time.Now(),
	}
	_ = h.pipelineRepo.AppendTimelineEvent(r.Context(), userID, jobID, event)

	respondJSON(w, http.StatusOK, toPipelineJobResponse(job))
}

// ─── Interview helpers ────────────────────────────────────────────────────────

var interviewAtFormats = []string{
	time.RFC3339,              // "2026-04-15T10:00:00Z"
	time.RFC3339Nano,          // "2026-04-15T10:00:00.000Z"
	"2006-01-02T15:04:05.000", // "2026-04-15T10:00:00.000"  (no tz — treat as UTC)
	"2006-01-02T15:04:05",     // "2026-04-15T10:00:00"       (no tz — treat as UTC)
}

func parseInterviewAt(s string) (time.Time, error) {
	for _, layout := range interviewAtFormats {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised date format: %s", s)
}

var interviewTypeLabels = map[string]string{
	"phone_screen": "Phone screen",
	"technical":    "Technical",
	"hr":           "HR",
	"final_round":  "Final round",
}

func isValidInterviewType(t string) bool {
	_, ok := interviewTypeLabels[t]
	return ok
}

func interviewTypeLabel(t string) string {
	if l, ok := interviewTypeLabels[t]; ok {
		return l
	}
	if t != "" {
		return t
	}
	return "Interview"
}

func (h *PipelineHandler) LogInterview(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDContextKey).(string)
	if !ok || userID == "" {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	jobID := chi.URLParam(r, "jobId")

	var req logInterviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.InterviewAt == "" {
		respondError(w, http.StatusBadRequest, "interview_at is required")
		return
	}
	if !isValidInterviewType(req.InterviewType) {
		respondError(w, http.StatusBadRequest, "interview_type must be one of: phone_screen, technical, hr, final_round")
		return
	}

	interviewAt, err := parseInterviewAt(req.InterviewAt)
	if err != nil {
		respondError(w, http.StatusBadRequest, "interview_at must be ISO 8601 or RFC3339 format")
		return
	}

	job, err := h.pipelineRepo.Get(r.Context(), userID, jobID)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	job.Stage = domain.StageInterview
	job.InterviewAt = &interviewAt
	job.InterviewType = req.InterviewType
	if err := h.pipelineRepo.Update(r.Context(), job); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	event := domain.TimelineEvent{
		ID:        uuid.New().String(),
		Type:      domain.TimelineInterviewScheduled,
		Label:     interviewTypeLabel(req.InterviewType) + " interview scheduled",
		Detail:    req.Detail,
		CreatedAt: time.Now(),
	}
	_ = h.pipelineRepo.AppendTimelineEvent(r.Context(), userID, jobID, event)

	// Send push notification (best-effort)
	user, err := h.userRepo.GetByID(r.Context(), userID)
	if err == nil && user != nil {
		_ = h.notifier.NotifyInterviewScheduled(r.Context(), userID, jobID, job.CompanyName, job.InterviewType, req.InterviewAt)
	}

	respondJSON(w, http.StatusOK, toPipelineJobResponse(job))
}

func (h *PipelineHandler) LogFollowUp(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDContextKey).(string)
	if !ok || userID == "" {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	jobID := chi.URLParam(r, "jobId")

	var req logFollowUpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	_, err := h.pipelineRepo.Get(r.Context(), userID, jobID)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	event := domain.TimelineEvent{
		ID:        uuid.New().String(),
		Type:      domain.TimelineFollowUpSent,
		Label:     "Follow-up sent",
		Detail:    req.Detail,
		CreatedAt: time.Now(),
	}
	if err := h.pipelineRepo.AppendTimelineEvent(r.Context(), userID, jobID, event); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusNoContent, nil)
}

// ─── Coach ───────────────────────────────────────────────────────────────────

type coachRequest struct {
	Stage            string   `json:"stage"`
	JobTitle         string   `json:"job_title"`
	CompanyName      string   `json:"company_name"`
	Location         string   `json:"location"`
	AtsScore         int      `json:"ats_score"`
	ResumeVersion    string   `json:"resume_version"`
	JobDescription   string   `json:"job_description"`
	JobURL           string   `json:"job_url"`
	MatchedKeywords  []string `json:"matched_keywords"`
	MissingKeywords  []string `json:"missing_keywords"`
	DaysSinceApplied int      `json:"days_since_applied"`
	Tone             string   `json:"tone"`
}

func (h *PipelineHandler) Coach(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDContextKey).(string)
	if !ok || userID == "" {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	jobID := chi.URLParam(r, "jobId")

	var req coachRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Stage == "" {
		respondError(w, http.StatusBadRequest, "stage is required")
		return
	}

	if req.Tone == "" {
		req.Tone = "default"
	}

	resp, err := h.coachService.Coach(r.Context(), inbound.CoachJobRequest{
		UserID:           userID,
		JobID:            jobID,
		Stage:            req.Stage,
		JobTitle:         req.JobTitle,
		CompanyName:      req.CompanyName,
		Location:         req.Location,
		AtsScore:         req.AtsScore,
		ResumeVersion:    req.ResumeVersion,
		JobDescription:   req.JobDescription,
		JobURL:           req.JobURL,
		MatchedKeywords:  req.MatchedKeywords,
		MissingKeywords:  req.MissingKeywords,
		DaysSinceApplied: req.DaysSinceApplied,
		Tone:             req.Tone,
	})
	if err != nil {
		log.Printf("[coach] jobID=%s stage=%s userID=%s error=%T: %v", jobID, req.Stage, userID, err, err)
		switch err {
		case domain.ErrPipelineJobNotFound:
			respondError(w, http.StatusNotFound, "pipeline job not found")
		case domain.ErrSubscriptionNotFound:
			respondError(w, http.StatusForbidden, "subscription not found")
		case domain.ErrSubscriptionInactive:
			respondError(w, http.StatusForbidden, "subscription inactive")
		case domain.ErrInsufficientCredits:
			respondError(w, http.StatusPaymentRequired, "insufficient credits")
		case domain.ErrForbidden:
			respondError(w, http.StatusUnprocessableEntity, "no coach action available for this stage")
		default:
			respondError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	respondJSON(w, http.StatusOK, resp)
}

// ─── Contacts ────────────────────────────────────────────────────────────────

type contactRequest struct {
	Name        string `json:"name"`
	Role        string `json:"role,omitempty"`
	LinkedinURL string `json:"linkedinUrl,omitempty"`
	Email       string `json:"email,omitempty"`
	Notes       string `json:"notes,omitempty"`
}

type contactResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Role        string `json:"role"`
	LinkedinURL string `json:"linkedinUrl"`
	Email       string `json:"email"`
	Notes       string `json:"notes"`
}

func (h *PipelineHandler) ListContacts(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDContextKey).(string)
	if !ok || userID == "" {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	jobID := chi.URLParam(r, "jobId")

	contacts, err := h.contactRepo.List(r.Context(), userID, jobID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := make([]contactResponse, len(contacts))
	for i, c := range contacts {
		resp[i] = contactResponse{
			ID:          c.ID,
			Name:        c.Name,
			Role:        c.Role,
			LinkedinURL: c.LinkedinURL,
			Email:       c.Email,
			Notes:       c.Notes,
		}
	}
	respondJSON(w, http.StatusOK, resp)
}

func (h *PipelineHandler) AddContact(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDContextKey).(string)
	if !ok || userID == "" {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	jobID := chi.URLParam(r, "jobId")

	var req contactRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "name is required")
		return
	}

	contact := &domain.PipelineContact{
		JobID:       jobID,
		UserID:      userID,
		Name:        req.Name,
		Role:        req.Role,
		LinkedinURL: req.LinkedinURL,
		Email:       req.Email,
		Notes:       req.Notes,
	}
	if err := h.contactRepo.Add(r.Context(), contact); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, contactResponse{
		ID:          contact.ID,
		Name:        contact.Name,
		Role:        contact.Role,
		LinkedinURL: contact.LinkedinURL,
		Email:       contact.Email,
		Notes:       contact.Notes,
	})
}

func (h *PipelineHandler) DeleteContact(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDContextKey).(string)
	if !ok || userID == "" {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	jobID := chi.URLParam(r, "jobId")
	contactID := chi.URLParam(r, "contactId")

	if err := h.contactRepo.Delete(r.Context(), userID, jobID, contactID); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// ─── Analytics ───────────────────────────────────────────────────────────────

func (h *PipelineHandler) GetPipelineAnalytics(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDContextKey).(string)
	if !ok || userID == "" {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	jobs, err := h.pipelineRepo.List(r.Context(), userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	result := analytics.Compute(jobs)
	respondJSON(w, http.StatusOK, result)
}

// ─── Interview Practice ──────────────────────────────────────────────────────

type nextInterviewQuestionRequest struct {
	Kind string `json:"kind"`
}

type submitInterviewAnswerRequest struct {
	Answer string `json:"answer"`
}

func respondInterviewError(w http.ResponseWriter, jobID string, err error) {
	log.Printf("[interview] jobID=%s error=%T: %v", jobID, err, err)
	switch err {
	case domain.ErrPipelineJobNotFound:
		respondError(w, http.StatusNotFound, "pipeline job not found")
	case domain.ErrInterviewQuestionNotFound:
		respondError(w, http.StatusNotFound, "interview question not found")
	case domain.ErrSubscriptionNotFound:
		respondError(w, http.StatusForbidden, "subscription not found")
	case domain.ErrSubscriptionInactive:
		respondError(w, http.StatusForbidden, "subscription inactive")
	case domain.ErrInsufficientCredits:
		respondError(w, http.StatusPaymentRequired, "insufficient credits")
	case domain.ErrForbidden:
		respondError(w, http.StatusUnprocessableEntity, "interview practice not available for this stage")
	default:
		respondError(w, http.StatusInternalServerError, err.Error())
	}
}

func (h *PipelineHandler) ListInterviewQuestions(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDContextKey).(string)
	if !ok || userID == "" {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	jobID := chi.URLParam(r, "jobId")

	history, err := h.interviewService.ListHistory(r.Context(), userID, jobID)
	if err != nil {
		respondInterviewError(w, jobID, err)
		return
	}
	respondJSON(w, http.StatusOK, history)
}

func (h *PipelineHandler) NextInterviewQuestion(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDContextKey).(string)
	if !ok || userID == "" {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	jobID := chi.URLParam(r, "jobId")

	var req nextInterviewQuestionRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	question, err := h.interviewService.NextQuestion(r.Context(), inbound.NextQuestionRequest{
		UserID: userID,
		JobID:  jobID,
		Kind:   req.Kind,
	})
	if err != nil {
		respondInterviewError(w, jobID, err)
		return
	}
	respondJSON(w, http.StatusCreated, question)
}

func (h *PipelineHandler) SubmitInterviewAnswer(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDContextKey).(string)
	if !ok || userID == "" {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	jobID := chi.URLParam(r, "jobId")
	questionID := chi.URLParam(r, "questionId")

	var req submitInterviewAnswerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Answer == "" {
		respondError(w, http.StatusBadRequest, "answer is required")
		return
	}

	question, err := h.interviewService.SubmitAnswer(r.Context(), inbound.SubmitAnswerRequest{
		UserID:     userID,
		JobID:      jobID,
		QuestionID: questionID,
		Answer:     req.Answer,
	})
	if err != nil {
		respondInterviewError(w, jobID, err)
		return
	}
	respondJSON(w, http.StatusOK, question)
}
