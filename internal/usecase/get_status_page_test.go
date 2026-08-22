package usecase_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
	"github.com/MohdFarhanS/sentinelix-backend/internal/usecase"
)

type mockStatusRepo struct {
	mock.Mock
}

func (m *mockStatusRepo) GetProjectStatusBySlug(ctx context.Context, slug string) (*domain.ProjectStatusData, error) {
	args := m.Called(ctx, slug)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.ProjectStatusData), args.Error(1)
}

func TestGetStatusPageUsecase_Execute_SlugNotFound(t *testing.T) {
	statusRepo := new(mockStatusRepo)

	// Repo mengembalikan (nil, nil) — konvensi status_repo.go untuk
	// "slug tidak ketemu" (bukan pgx.ErrNoRows yang di-propagate mentah).
	statusRepo.On("GetProjectStatusBySlug", mock.Anything, "not-exist").Return(nil, nil)

	uc := usecase.NewGetStatusPageUsecase(statusRepo)
	_, err := uc.Execute(context.Background(), "not-exist")

	assert.ErrorIs(t, err, domain.ErrProjectSlugNotFound)
}

func TestGetStatusPageUsecase_Execute_RepoError(t *testing.T) {
	statusRepo := new(mockStatusRepo)
	repoErr := assert.AnError

	statusRepo.On("GetProjectStatusBySlug", mock.Anything, "newsportal-prod").Return(nil, repoErr)

	uc := usecase.NewGetStatusPageUsecase(statusRepo)
	_, err := uc.Execute(context.Background(), "newsportal-prod")

	assert.ErrorIs(t, err, repoErr)
}

func TestGetStatusPageUsecase_Execute_AllMonitorsUp_Operational(t *testing.T) {
	statusRepo := new(mockStatusRepo)

	statusRepo.On("GetProjectStatusBySlug", mock.Anything, "newsportal-prod").Return(&domain.ProjectStatusData{
		ProjectName: "NewsPortal",
		Monitors: []domain.MonitorStatusData{
			{Name: "API Health", RawStatus: domain.MonitorStatusUp, Uptime30d: 99.94},
			{Name: "Website", RawStatus: domain.MonitorStatusUp, Uptime30d: 100},
		},
	}, nil)

	uc := usecase.NewGetStatusPageUsecase(statusRepo)
	result, err := uc.Execute(context.Background(), "newsportal-prod")

	assert.NoError(t, err)
	assert.Equal(t, "NewsPortal", result.ProjectName)
	assert.Equal(t, domain.OverallStatusOperational, result.OverallStatus)
	assert.Len(t, result.Monitors, 2)
	assert.True(t, result.Monitors[0].IsUp)
	assert.Equal(t, "API Health", result.Monitors[0].Name)
	assert.Equal(t, 99.94, result.Monitors[0].Uptime30d)
}

func TestGetStatusPageUsecase_Execute_AllMonitorsDown_MajorOutage(t *testing.T) {
	statusRepo := new(mockStatusRepo)

	statusRepo.On("GetProjectStatusBySlug", mock.Anything, "newsportal-prod").Return(&domain.ProjectStatusData{
		ProjectName: "NewsPortal",
		Monitors: []domain.MonitorStatusData{
			{Name: "API Health", RawStatus: domain.MonitorStatusDown, Uptime30d: 20},
			{Name: "Website", RawStatus: domain.MonitorStatusDown, Uptime30d: 15},
		},
	}, nil)

	uc := usecase.NewGetStatusPageUsecase(statusRepo)
	result, err := uc.Execute(context.Background(), "newsportal-prod")

	assert.NoError(t, err)
	assert.Equal(t, domain.OverallStatusMajorOutage, result.OverallStatus)
	assert.False(t, result.Monitors[0].IsUp)
	assert.False(t, result.Monitors[1].IsUp)
}

func TestGetStatusPageUsecase_Execute_MixedUpDown_DegradedPerformance(t *testing.T) {
	statusRepo := new(mockStatusRepo)

	statusRepo.On("GetProjectStatusBySlug", mock.Anything, "newsportal-prod").Return(&domain.ProjectStatusData{
		ProjectName: "NewsPortal",
		Monitors: []domain.MonitorStatusData{
			{Name: "API Health", RawStatus: domain.MonitorStatusUp, Uptime30d: 99},
			{Name: "Website", RawStatus: domain.MonitorStatusDown, Uptime30d: 40},
		},
	}, nil)

	uc := usecase.NewGetStatusPageUsecase(statusRepo)
	result, err := uc.Execute(context.Background(), "newsportal-prod")

	assert.NoError(t, err)
	assert.Equal(t, domain.OverallStatusDegradedPerformance, result.OverallStatus)
}

// TestGetStatusPageUsecase_Execute_UnknownMonitors_ExcludedFromAggregate
// memverifikasi keputusan desain: monitor "unknown" (belum sempat
// di-checker) TIDAK ikut menyeret overall_status jadi degraded/outage,
// meskipun IsUp per-monitor-nya tetap true (netral) di response publik.
func TestGetStatusPageUsecase_Execute_UnknownMonitors_ExcludedFromAggregate(t *testing.T) {
	statusRepo := new(mockStatusRepo)

	statusRepo.On("GetProjectStatusBySlug", mock.Anything, "newsportal-prod").Return(&domain.ProjectStatusData{
		ProjectName: "NewsPortal",
		Monitors: []domain.MonitorStatusData{
			{Name: "Brand New Monitor", RawStatus: domain.MonitorStatusUnknown, Uptime30d: 100},
		},
	}, nil)

	uc := usecase.NewGetStatusPageUsecase(statusRepo)
	result, err := uc.Execute(context.Background(), "newsportal-prod")

	assert.NoError(t, err)
	assert.Equal(t, domain.OverallStatusOperational, result.OverallStatus)
	assert.True(t, result.Monitors[0].IsUp) // unknown ditampilkan netral (up), bukan down
}

// TestGetStatusPageUsecase_Execute_MixedUnknownAndDown_StillDegraded
// memastikan "unknown" cuma di-EXCLUDE dari hitungan, bukan bikin
// keseluruhan hasil jadi operational secara keliru kalau ada monitor
// down definitif lain di project yang sama. Butuh MINIMAL 1 monitor up
// definitif juga di sini — kalau cuma unknown+down, hasil yang benar
// justru major_outage (1 dari 1 monitor definitif down = 100% down),
// bukan degraded.
func TestGetStatusPageUsecase_Execute_MixedUnknownAndDown_StillDegraded(t *testing.T) {
	statusRepo := new(mockStatusRepo)

	statusRepo.On("GetProjectStatusBySlug", mock.Anything, "newsportal-prod").Return(&domain.ProjectStatusData{
		ProjectName: "NewsPortal",
		Monitors: []domain.MonitorStatusData{
			{Name: "Brand New Monitor", RawStatus: domain.MonitorStatusUnknown, Uptime30d: 100},
			{Name: "API Health", RawStatus: domain.MonitorStatusUp, Uptime30d: 99},
			{Name: "Website", RawStatus: domain.MonitorStatusDown, Uptime30d: 10},
		},
	}, nil)

	uc := usecase.NewGetStatusPageUsecase(statusRepo)
	result, err := uc.Execute(context.Background(), "newsportal-prod")

	assert.NoError(t, err)
	assert.Equal(t, domain.OverallStatusDegradedPerformance, result.OverallStatus)
}

func TestGetStatusPageUsecase_Execute_NoMonitors_Operational(t *testing.T) {
	statusRepo := new(mockStatusRepo)

	statusRepo.On("GetProjectStatusBySlug", mock.Anything, "newsportal-prod").Return(&domain.ProjectStatusData{
		ProjectName: "NewsPortal",
		Monitors:    []domain.MonitorStatusData{},
	}, nil)

	uc := usecase.NewGetStatusPageUsecase(statusRepo)
	result, err := uc.Execute(context.Background(), "newsportal-prod")

	assert.NoError(t, err)
	assert.Equal(t, domain.OverallStatusOperational, result.OverallStatus)
	assert.Empty(t, result.Monitors)
}