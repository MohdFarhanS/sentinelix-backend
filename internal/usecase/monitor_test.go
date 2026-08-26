package usecase_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
	"github.com/MohdFarhanS/sentinelix-backend/internal/usecase"
)

type mockMonitorSyncPublisher struct {
	mock.Mock
}

func (m *mockMonitorSyncPublisher) Publish(ctx context.Context, projectID, eventType string, data interface{}) error {
	args := m.Called(ctx, projectID, eventType, data)
	return args.Error(0)
}

func TestMonitorUsecase_Create_Success(t *testing.T) {
	monitorRepo := new(mockMonitorRepo)
	checkRepo := new(mockMonitorCheckRepo)
	projectRepo := new(mockProjectRepo)
	publisher := new(mockMonitorSyncPublisher)

	projectRepo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{ID: "proj-1", UserID: "user-1"}, nil)
	monitorRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.Monitor")).Return(nil)
	publisher.On("Publish", mock.Anything, "", domain.MonitorSyncCreated, mock.Anything).Return(nil)

	uc := usecase.NewMonitorUsecase(monitorRepo, checkRepo, projectRepo, publisher)
	monitor, err := uc.Create(context.Background(), usecase.CreateMonitorInput{
		UserID:           "user-1",
		ProjectID:        "proj-1",
		URL:              "https://example.com/health",
		IntervalSec:      60,
		Channel:          domain.ChannelEmail,
		ChannelTarget:    "a@b.com",
		FailureThreshold: 3,
	})

	assert.NoError(t, err)
	assert.Equal(t, "monitor-generated-id", monitor.ID)
	publisher.AssertCalled(t, "Publish", mock.Anything, "", domain.MonitorSyncCreated, mock.Anything)
}

func TestMonitorUsecase_Create_ProjectNotFound(t *testing.T) {
	monitorRepo := new(mockMonitorRepo)
	checkRepo := new(mockMonitorCheckRepo)
	projectRepo := new(mockProjectRepo)
	publisher := new(mockMonitorSyncPublisher)

	projectRepo.On("GetByID", mock.Anything, "proj-x").Return(nil, domain.ErrProjectNotFound)

	uc := usecase.NewMonitorUsecase(monitorRepo, checkRepo, projectRepo, publisher)
	_, err := uc.Create(context.Background(), usecase.CreateMonitorInput{UserID: "user-1", ProjectID: "proj-x"})

	assert.ErrorIs(t, err, domain.ErrProjectNotFound)
	monitorRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	publisher.AssertNotCalled(t, "Publish", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestMonitorUsecase_Create_Forbidden(t *testing.T) {
	monitorRepo := new(mockMonitorRepo)
	checkRepo := new(mockMonitorCheckRepo)
	projectRepo := new(mockProjectRepo)
	publisher := new(mockMonitorSyncPublisher)

	projectRepo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{ID: "proj-1", UserID: "user-OTHER"}, nil)

	uc := usecase.NewMonitorUsecase(monitorRepo, checkRepo, projectRepo, publisher)
	_, err := uc.Create(context.Background(), usecase.CreateMonitorInput{UserID: "user-1", ProjectID: "proj-1"})

	assert.ErrorIs(t, err, usecase.ErrForbidden)
	monitorRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

// TestMonitorUsecase_Create_ValidationError_InvalidURL memverifikasi
// validasi format URL (net/url, scheme http/https) yang kita tambahkan
// belakangan beneran nyambung sampai ke usecase — bukan cuma di level
// domain.Validate() secara terisolasi.
func TestMonitorUsecase_Create_ValidationError_InvalidURL(t *testing.T) {
	monitorRepo := new(mockMonitorRepo)
	checkRepo := new(mockMonitorCheckRepo)
	projectRepo := new(mockProjectRepo)
	publisher := new(mockMonitorSyncPublisher)

	projectRepo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{ID: "proj-1", UserID: "user-1"}, nil)

	uc := usecase.NewMonitorUsecase(monitorRepo, checkRepo, projectRepo, publisher)
	_, err := uc.Create(context.Background(), usecase.CreateMonitorInput{
		UserID:           "user-1",
		ProjectID:        "proj-1",
		URL:              "/health", // relative path, bukan absolute URL
		IntervalSec:      60,
		Channel:          domain.ChannelEmail,
		ChannelTarget:    "a@b.com",
		FailureThreshold: 3,
	})

	assert.ErrorIs(t, err, domain.ErrMonitorURLInvalid)
	monitorRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestMonitorUsecase_List_Forbidden(t *testing.T) {
	monitorRepo := new(mockMonitorRepo)
	checkRepo := new(mockMonitorCheckRepo)
	projectRepo := new(mockProjectRepo)
	publisher := new(mockMonitorSyncPublisher)

	projectRepo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{ID: "proj-1", UserID: "user-OTHER"}, nil)

	uc := usecase.NewMonitorUsecase(monitorRepo, checkRepo, projectRepo, publisher)
	_, err := uc.List(context.Background(), usecase.ListMonitorsInput{UserID: "user-1", ProjectID: "proj-1"})

	assert.ErrorIs(t, err, usecase.ErrForbidden)
}

func TestMonitorUsecase_GetByID_NotFound(t *testing.T) {
	monitorRepo := new(mockMonitorRepo)
	checkRepo := new(mockMonitorCheckRepo)
	projectRepo := new(mockProjectRepo)
	publisher := new(mockMonitorSyncPublisher)

	monitorRepo.On("GetByID", mock.Anything, "mon-x").Return(nil, domain.ErrMonitorNotFound)

	uc := usecase.NewMonitorUsecase(monitorRepo, checkRepo, projectRepo, publisher)
	_, err := uc.GetByID(context.Background(), "user-1", "mon-x")

	assert.ErrorIs(t, err, domain.ErrMonitorNotFound)
}

// TestMonitorUsecase_Update_PartialFields_OnlyChangesGiven memverifikasi
// semantik PATCH: field yang tidak dikirim (nil) TIDAK berubah dari
// state existing — cuma field yang eksplisit dikirim yang di-merge.
func TestMonitorUsecase_Update_PartialFields_OnlyChangesGiven(t *testing.T) {
	monitorRepo := new(mockMonitorRepo)
	checkRepo := new(mockMonitorCheckRepo)
	projectRepo := new(mockProjectRepo)
	publisher := new(mockMonitorSyncPublisher)

	existing := &domain.Monitor{
		ID: "mon-1", ProjectID: "proj-1", URL: "https://old.example.com",
		IntervalSec: 60, Channel: domain.ChannelEmail, ChannelTarget: "old@email.com",
		FailureThreshold: 3, Status: domain.MonitorStatusUp,
	}

	monitorRepo.On("GetByID", mock.Anything, "mon-1").Return(existing, nil)
	projectRepo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{ID: "proj-1", UserID: "user-1"}, nil)
	monitorRepo.On("Update", mock.Anything, mock.AnythingOfType("*domain.Monitor")).Return(nil)
	publisher.On("Publish", mock.Anything, "", domain.MonitorSyncUpdated, mock.Anything).Return(nil)

	newInterval := 120
	uc := usecase.NewMonitorUsecase(monitorRepo, checkRepo, projectRepo, publisher)
	updated, err := uc.Update(context.Background(), usecase.UpdateMonitorInput{
		UserID:      "user-1",
		MonitorID:   "mon-1",
		IntervalSec: &newInterval,
		// URL, Channel, ChannelTarget, FailureThreshold sengaja TIDAK dikirim (nil)
	})

	assert.NoError(t, err)
	assert.Equal(t, 120, updated.IntervalSec)
	assert.Equal(t, "https://old.example.com", updated.URL) // tidak berubah
	assert.Equal(t, "old@email.com", updated.ChannelTarget) // tidak berubah
	publisher.AssertCalled(t, "Publish", mock.Anything, "", domain.MonitorSyncUpdated, mock.Anything)
}

func TestMonitorUsecase_Delete_Success_PublishesSyncEvent(t *testing.T) {
	monitorRepo := new(mockMonitorRepo)
	checkRepo := new(mockMonitorCheckRepo)
	projectRepo := new(mockProjectRepo)
	publisher := new(mockMonitorSyncPublisher)

	existing := &domain.Monitor{ID: "mon-1", ProjectID: "proj-1"}
	monitorRepo.On("GetByID", mock.Anything, "mon-1").Return(existing, nil)
	projectRepo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{ID: "proj-1", UserID: "user-1"}, nil)
	monitorRepo.On("Delete", mock.Anything, "mon-1").Return(nil)
	publisher.On("Publish", mock.Anything, "", domain.MonitorSyncDeleted, mock.Anything).Return(nil)

	uc := usecase.NewMonitorUsecase(monitorRepo, checkRepo, projectRepo, publisher)
	err := uc.Delete(context.Background(), usecase.DeleteMonitorInput{UserID: "user-1", MonitorID: "mon-1"})

	assert.NoError(t, err)
	publisher.AssertCalled(t, "Publish", mock.Anything, "", domain.MonitorSyncDeleted, mock.Anything)
}

func TestMonitorUsecase_Delete_Forbidden_DoesNotPublish(t *testing.T) {
	monitorRepo := new(mockMonitorRepo)
	checkRepo := new(mockMonitorCheckRepo)
	projectRepo := new(mockProjectRepo)
	publisher := new(mockMonitorSyncPublisher)

	existing := &domain.Monitor{ID: "mon-1", ProjectID: "proj-1"}
	monitorRepo.On("GetByID", mock.Anything, "mon-1").Return(existing, nil)
	projectRepo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{ID: "proj-1", UserID: "user-OTHER"}, nil)

	uc := usecase.NewMonitorUsecase(monitorRepo, checkRepo, projectRepo, publisher)
	err := uc.Delete(context.Background(), usecase.DeleteMonitorInput{UserID: "user-1", MonitorID: "mon-1"})

	assert.ErrorIs(t, err, usecase.ErrForbidden)
	monitorRepo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
	publisher.AssertNotCalled(t, "Publish", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}
