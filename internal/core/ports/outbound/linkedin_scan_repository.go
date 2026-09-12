package outbound

import (
	"context"

	"github.com/reangeline/backend_applywise/internal/core/domain"
)

// LinkedInScanRepository persiste o scan de LinkedIn mais recente por usuário — sem
// histórico, um registro é sobrescrito pelo próximo (spec 015).
type LinkedInScanRepository interface {
	Upsert(ctx context.Context, scan *domain.LinkedInScan) error
	GetByUserID(ctx context.Context, userID string) (*domain.LinkedInScan, error)
}
