package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/reangeline/backend_applywise/internal/adapters/inbound/http/middleware"
	"github.com/reangeline/backend_applywise/internal/core/ports/inbound"
)

type ResumeHandler struct {
	resumeService inbound.ResumeOptimizerService
}

func NewResumeHandler(resumeService inbound.ResumeOptimizerService) *ResumeHandler {
	return &ResumeHandler{resumeService: resumeService}
}

type UploadResumeRequestDTO struct {
	Content string `json:"content" validate:"required"`
}

type OptimizeResumeRequestDTO struct {
	ResumeID       string `json:"resume_id" validate:"required"`
	JobDescription string `json:"job_description" validate:"required"`
	TargetCompany  string `json:"target_company,omitempty"`
	TargetRole     string `json:"target_role,omitempty"`
}

type SuggestAdditionRequestDTO struct {
	Gap            string `json:"gap" validate:"required"`
	JobTitle       string `json:"job_title,omitempty"`
	CompanyName    string `json:"company_name,omitempty"`
	JobDescription string `json:"job_description,omitempty"`
}

type ManualResumeRequestDTO struct {
	ResumeID        string                   `json:"resume_id,omitempty"`
	Type            string                   `json:"type,omitempty"`
	Nickname        string                   `json:"nickname,omitempty"`
	Personal        map[string]interface{}   `json:"personal,omitempty"`
	Experiences     []map[string]interface{} `json:"experiences,omitempty"`
	Education       []map[string]interface{} `json:"education,omitempty"`
	Projects        []map[string]interface{} `json:"projects,omitempty"`
	Languages       []map[string]interface{} `json:"languages,omitempty"`
	ATSScore        *float64                 `json:"ats_score,omitempty"`
	ATSImprovements []string                 `json:"ats_improvements,omitempty"`
	CreatedAt       string                   `json:"created_at,omitempty"`
	UpdatedAt       string                   `json:"updated_at,omitempty"`
}

func (h *ResumeHandler) UploadResume(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDContextKey).(string)

	var req UploadResumeRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	resume, err := h.resumeService.UploadResume(r.Context(), inbound.UploadResumeRequest{
		UserID:  userID,
		Content: req.Content,
	})
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, resume)
}

func (h *ResumeHandler) ListResumes(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDContextKey).(string)

	resumes, err := h.resumeService.ListResumes(r.Context(), userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, resumes)
}

func (h *ResumeHandler) GetResume(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDContextKey).(string)
	resumeID := chi.URLParam(r, "resumeID")

	resume, err := h.resumeService.GetResume(r.Context(), userID, resumeID)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, resume)
}

func (h *ResumeHandler) OptimizeResume(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDContextKey).(string)

	var req OptimizeResumeRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	job, err := h.resumeService.StartOptimization(r.Context(), inbound.OptimizeResumeRequest{
		UserID:         userID,
		ResumeID:       req.ResumeID,
		JobDescription: req.JobDescription,
		TargetCompany:  req.TargetCompany,
		TargetRole:     req.TargetRole,
	})
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusAccepted, job)
}

func (h *ResumeHandler) SuggestAddition(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDContextKey).(string)
	resumeID := chi.URLParam(r, "resumeID")

	var req SuggestAdditionRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := h.resumeService.SuggestAddition(r.Context(), inbound.SuggestAdditionRequest{
		UserID:         userID,
		ResumeID:       resumeID,
		Gap:            req.Gap,
		JobTitle:       req.JobTitle,
		CompanyName:    req.CompanyName,
		JobDescription: req.JobDescription,
	})
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, result)
}

func (h *ResumeHandler) GetOptimizationJobStatus(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDContextKey).(string)
	jobID := chi.URLParam(r, "jobID")

	job, err := h.resumeService.GetOptimizationJob(r.Context(), userID, jobID)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, job)
}

func (h *ResumeHandler) ListOptimizedResumes(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDContextKey).(string)

	optimized, err := h.resumeService.ListOptimizedResumes(r.Context(), userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, optimized)
}

func (h *ResumeHandler) GetOptimizedResume(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDContextKey).(string)
	optimizedID := chi.URLParam(r, "optimizedID")

	optimized, err := h.resumeService.GetOptimizedResume(r.Context(), userID, optimizedID)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, optimized)
}

func (h *ResumeHandler) CreateManualResume(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDContextKey).(string)

	var req ManualResumeRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	parsed := map[string]interface{}{}
	if req.Personal != nil {
		parsed["personal"] = req.Personal
	}
	if req.Experiences != nil {
		parsed["experiences"] = req.Experiences
	}
	if req.Education != nil {
		parsed["education"] = req.Education
	}
	if req.Projects != nil {
		parsed["projects"] = req.Projects
	}
	if req.Languages != nil {
		parsed["languages"] = req.Languages
	}
	if req.ATSScore != nil {
		parsed["ats_score"] = *req.ATSScore
	}
	if req.ATSImprovements != nil {
		parsed["ats_improvements"] = req.ATSImprovements
	}
	if req.Nickname != "" {
		parsed["nickname"] = req.Nickname
	}

	created, err := h.resumeService.CreateManualResume(r.Context(), inbound.CreateManualResumeRequest{
		UserID:     userID,
		ResumeID:   req.ResumeID,
		Nickname:   req.Nickname,
		ParsedData: parsed,
	})

	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, created)
}

func (h *ResumeHandler) UpdateManualResume(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDContextKey).(string)
	resumeID := chi.URLParam(r, "resumeID")

	var req ManualResumeRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	parsed := map[string]interface{}{}
	if req.Personal != nil {
		parsed["personal"] = req.Personal
	}
	if req.Experiences != nil {
		parsed["experiences"] = req.Experiences
	}
	if req.Education != nil {
		parsed["education"] = req.Education
	}
	if req.Projects != nil {
		parsed["projects"] = req.Projects
	}
	if req.Languages != nil {
		parsed["languages"] = req.Languages
	}
	if req.ATSScore != nil {
		parsed["ats_score"] = *req.ATSScore
	}
	if req.ATSImprovements != nil {
		parsed["ats_improvements"] = req.ATSImprovements
	}

	updated, err := h.resumeService.UpdateManualResume(r.Context(), inbound.UpdateManualResumeRequest{
		UserID:     userID,
		ResumeID:   resumeID,
		Nickname:   req.Nickname,
		ParsedData: parsed,
	})

	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, updated)
}

func (h *ResumeHandler) UpdateOptimizedResume(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDContextKey).(string)
	optimizedID := chi.URLParam(r, "optimizedID")

	var req ManualResumeRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	parsed := map[string]interface{}{}
	if req.Personal != nil {
		parsed["personal"] = req.Personal
	}
	if req.Experiences != nil {
		parsed["experiences"] = req.Experiences
	}
	if req.Education != nil {
		parsed["education"] = req.Education
	}
	if req.Projects != nil {
		parsed["projects"] = req.Projects
	}
	if req.Languages != nil {
		parsed["languages"] = req.Languages
	}

	updated, err := h.resumeService.UpdateOptimizedResume(r.Context(), inbound.UpdateOptimizedResumeRequest{
		UserID:            userID,
		OptimizedResumeID: optimizedID,
		Nickname:          req.Nickname,
		ParsedData:        parsed,
	})
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, updated)
}

func (h *ResumeHandler) DeleteResume(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDContextKey).(string)
	resumeID := chi.URLParam(r, "resumeID")

	if err := h.resumeService.DeleteResume(r.Context(), userID, resumeID); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// LinkedInOptimizeRequestDTO is the request body for LinkedIn optimization.
type LinkedInOptimizeRequestDTO struct {
	ResumeID string `json:"resume_id" validate:"required"`
}

// OptimizeForLinkedIn receives a manual resume ID and queues an async LinkedIn optimization job.
func (h *ResumeHandler) OptimizeForLinkedIn(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDContextKey).(string)

	var req LinkedInOptimizeRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ResumeID == "" {
		respondError(w, http.StatusBadRequest, "resume_id is required")
		return
	}

	job, err := h.resumeService.StartLinkedInOptimization(r.Context(), inbound.StartLinkedInOptimizationRequest{
		UserID:   userID,
		ResumeID: req.ResumeID,
	})
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusAccepted, job)
}

// ParsePDFResume accepts a multipart/form-data upload with a "file" field (PDF),
// extracts text, passes it to the AI for structured parsing, and returns the
// parsed data shaped as a manual resume. Nothing is saved to the DB.
func (h *ResumeHandler) ParsePDFResume(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.UserIDContextKey).(string)

	// Limit upload size to 10 MB
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		respondError(w, http.StatusBadRequest, "file too large or invalid multipart form")
		return
	}

	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		respondError(w, http.StatusBadRequest, "missing 'file' field in form")
		return
	}
	defer file.Close()

	pdfBytes, err := io.ReadAll(file)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to read uploaded file")
		return
	}

	result, err := h.resumeService.ParsePDFResume(r.Context(), inbound.ParsePDFResumeRequest{
		UserID:   userID,
		PDFBytes: pdfBytes,
		FileName: fileHeader.Filename,
	})
	if err != nil {
		respondError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, result)
}
