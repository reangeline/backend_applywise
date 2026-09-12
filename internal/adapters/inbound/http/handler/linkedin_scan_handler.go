package handler

import (
	"errors"
	"io"
	"net/http"

	"github.com/reangeline/backend_applywise/internal/adapters/inbound/http/middleware"
	"github.com/reangeline/backend_applywise/internal/core/domain"
	"github.com/reangeline/backend_applywise/internal/core/ports/inbound"
	"github.com/reangeline/backend_applywise/pkg/security"
)

type LinkedInScanHandler struct {
	linkedInScanService inbound.LinkedInScanService
}

func NewLinkedInScanHandler(linkedInScanService inbound.LinkedInScanService) *LinkedInScanHandler {
	return &LinkedInScanHandler{linkedInScanService: linkedInScanService}
}

// ScanProfile accepts a multipart/form-data upload with a "file" field (the user's LinkedIn
// profile exported as PDF via LinkedIn's own "Save to PDF"), audits it, and persists the
// result (replacing any previous scan for this user) — spec 015.
func (h *LinkedInScanHandler) ScanProfile(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.UserIDContextKey).(string)

	// Limit upload size to 10 MB — same limit as ParsePDFResume.
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		respondError(w, http.StatusBadRequest, "file too large or invalid multipart form")
		return
	}

	file, _, err := r.FormFile("file")
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

	targetRole := r.FormValue("target_role")

	scan, err := h.linkedInScanService.ScanProfile(r.Context(), inbound.ScanLinkedInProfileRequest{
		UserID:     userID,
		PDFBytes:   pdfBytes,
		TargetRole: targetRole,
	})
	if err != nil {
		respondLinkedInScanError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, scan)
}

// GetLatestScan returns the most recent saved scan for the user, or 404 if they've never
// scanned before.
func (h *LinkedInScanHandler) GetLatestScan(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.UserIDContextKey).(string)

	scan, err := h.linkedInScanService.GetLatestScan(r.Context(), userID)
	if err != nil {
		respondLinkedInScanError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, scan)
}

func respondLinkedInScanError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrLinkedInScanNotFound):
		respondError(w, http.StatusNotFound, "nenhum scan salvo ainda")
	case errors.Is(err, security.ErrInjectionDetected), errors.Is(err, security.ErrInputTooLong):
		respondError(w, http.StatusBadRequest, err.Error())
	default:
		respondError(w, http.StatusUnprocessableEntity, err.Error())
	}
}
