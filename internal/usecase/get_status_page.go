package usecase

import (
	"context"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
)

type GetStatusPageUsecase struct {
	statusRepo domain.StatusRepository
}

func NewGetStatusPageUsecase(statusRepo domain.StatusRepository) *GetStatusPageUsecase {
	return &GetStatusPageUsecase{statusRepo: statusRepo}
}

// Execute mengubah data RAW dari repository jadi StatusPageResponse
// publik. Konversi RawStatus -> IsUp (bool) dan perhitungan
// OverallStatus SENGAJA dilakukan di sini (bukan di repo) — supaya
// ComputeOverallStatus bisa exclude status "unknown" dengan benar
// SEBELUM di-collapse jadi bool (lihat domain/status.go).
func (uc *GetStatusPageUsecase) Execute(ctx context.Context, slug string) (*domain.StatusPageResponse, error) {
	data, err := uc.statusRepo.GetProjectStatusBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, domain.ErrProjectSlugNotFound
	}

	rawStatuses := make([]string, 0, len(data.Monitors))
	monitors := make([]domain.StatusMonitorInfo, 0, len(data.Monitors))
	for _, m := range data.Monitors {
		rawStatuses = append(rawStatuses, m.RawStatus)
		monitors = append(monitors, domain.StatusMonitorInfo{
			Name:      m.Name,
			IsUp:      domain.MonitorIsUp(m.RawStatus),
			Uptime30d: m.Uptime30d,
		})
	}

	return &domain.StatusPageResponse{
		ProjectName:   data.ProjectName,
		OverallStatus: domain.ComputeOverallStatus(rawStatuses),
		Monitors:      monitors,
	}, nil
}
