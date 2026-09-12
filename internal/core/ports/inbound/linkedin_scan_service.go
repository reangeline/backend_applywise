package inbound

import (
	"context"

	"github.com/reangeline/backend_applywise/internal/core/domain"
)

// ScanLinkedInProfileRequest carrega o PDF exportado do perfil do LinkedIn do usuário
// ("Salvar em PDF") pra ser auditado (spec 015).
type ScanLinkedInProfileRequest struct {
	UserID     string
	PDFBytes   []byte
	FileName   string
	TargetRole string // opcional
}

// LinkedInScanService audita o perfil do LinkedIn do usuário (via upload de PDF) contra um
// checklist fixo de boas práticas, e guarda só o scan mais recente por usuário — sem
// histórico (spec 015). Sem crédito envolvido, só autenticado.
type LinkedInScanService interface {
	ScanProfile(ctx context.Context, req ScanLinkedInProfileRequest) (*domain.LinkedInScan, error)
	GetLatestScan(ctx context.Context, userID string) (*domain.LinkedInScan, error)
}
