package handler

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/reangeline/backend_applywise/internal/adapters/inbound/http/middleware"
	"github.com/reangeline/backend_applywise/internal/core/domain"
	"github.com/reangeline/backend_applywise/internal/core/ports/inbound"
	"github.com/reangeline/backend_applywise/pkg/security"
)

type ApplyAssistHandler struct {
	applyAssistService inbound.ApplyAssistService
}

func NewApplyAssistHandler(applyAssistService inbound.ApplyAssistService) *ApplyAssistHandler {
	return &ApplyAssistHandler{applyAssistService: applyAssistService}
}

type suggestAnswerRequestDTO struct {
	ResumeID       string `json:"resume_id"`
	JobTitle       string `json:"job_title"`
	CompanyName    string `json:"company_name"`
	JobDescription string `json:"job_description"`
	Question       string `json:"question"`
}

func (h *ApplyAssistHandler) SuggestAnswer(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDContextKey).(string)
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var body suggestAnswerRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.ResumeID == "" || body.Question == "" {
		respondError(w, http.StatusBadRequest, "resume_id and question are required")
		return
	}

	result, err := h.applyAssistService.SuggestAnswer(r.Context(), inbound.SuggestAnswerRequest{
		UserID:         userID,
		ResumeID:       body.ResumeID,
		JobTitle:       body.JobTitle,
		CompanyName:    body.CompanyName,
		JobDescription: body.JobDescription,
		Question:       body.Question,
	})
	if err != nil {
		respondApplyAssistError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, result)
}

func respondApplyAssistError(w http.ResponseWriter, err error) {
	log.Printf("[apply-assist] error=%T: %v", err, err)
	switch {
	case errors.Is(err, domain.ErrResumeNotFound):
		respondError(w, http.StatusNotFound, "resume not found")
	case errors.Is(err, domain.ErrPremiumRequired):
		respondError(w, http.StatusForbidden, "recurso exclusivo para assinantes Premium")
	case errors.Is(err, domain.ErrSubscriptionNotFound):
		respondError(w, http.StatusForbidden, "subscription not found")
	case errors.Is(err, security.ErrInjectionDetected), errors.Is(err, security.ErrInputTooLong):
		respondError(w, http.StatusBadRequest, err.Error())
	default:
		respondError(w, http.StatusInternalServerError, err.Error())
	}
}
