package usecase_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
	"github.com/MohdFarhanS/sentinelix-backend/internal/usecase"
)

// --- Mocks khusus monitor (dipakai juga di monitor_test.go, satu
// package usecase_test) ---

type mockMonitorRepo struct {
	mock.Mock
}

func (m *mockMonitorRepo) Create(ctx context.Context, mo *domain.Monitor) error {
	args := m.Called(ctx, mo)
	mo.ID = "monitor-generated-id"
	mo.Status = domain.MonitorStatusUnknown
	mo.CreatedAt = time.Now()
	return args.Error(0)
}

func (m *mockMonitorRepo) GetByID(ctx context.Context, id string) (*domain.Monitor, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Monitor), args.Error(1)
}

func (m *mockMonitorRepo) ListByProjectID(ctx context.Context, projectID string) ([]*domain.Monitor, error) {
	args := m.Called(ctx, projectID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.Monitor), args.Error(1)
}

func (m *mockMonitorRepo) ListAll(ctx context.Context) ([]*domain.Monitor, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.Monitor), args.Error(1)
}

func (m *mockMonitorRepo) Update(ctx context.Context, mo *domain.Monitor) error {
	args := m.Called(ctx, mo)
	return args.Error(0)
}

func (m *mockMonitorRepo) UpdateStatus(ctx context.Context, id, status string) error {
	args := m.Called(ctx, id, status)
	return args.Error(0)
}

func (m *mockMonitorRepo) Delete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

type mockMonitorCheckRepo struct {
	mock.Mock
}

func (m *mockMonitorCheckRepo) Create(ctx context.Context, c *domain.MonitorCheck) error {
	args := m.Called(ctx, c)
	return args.Error(0)
}

func (m *mockMonitorCheckRepo) ListRecentByMonitorID(ctx context.Context, monitorID string, limit int) ([]*domain.MonitorCheck, error) {
	args := m.Called(ctx, monitorID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.MonitorCheck), args.Error(1)
}

func (m *mockMonitorCheckRepo) ListByMonitorID(ctx context.Context, monitorID string, from, to *time.Time) ([]*domain.MonitorCheck, error) {
	args := m.Called(ctx, monitorID, from, to)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.MonitorCheck), args.Error(1)
}

type mockMonitorNotifier struct {
	mock.Mock
}

func (m *mockMonitorNotifier) NotifyMonitorDown(ctx context.Context, mo *domain.Monitor, consecutiveFailures int) error {
	args := m.Called(ctx, mo, consecutiveFailures)
	return args.Error(0)
}

// --- Check ---

func TestMonitorCheckerUsecase_Check_StaysUp_NoStatusChange(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	monitorRepo := new(mockMonitorRepo)
	checkRepo := new(mockMonitorCheckRepo)
	notifier := new(mockMonitorNotifier)
	publisher := new(mockMonitorSyncPublisher)

	monitor := &domain.Monitor{ID: "mon-1", ProjectID: "proj-1", URL: server.URL, Status: domain.MonitorStatusUp, FailureThreshold: 3}

	checkRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.MonitorCheck")).Return(nil)

	uc := usecase.NewMonitorCheckerUsecase(monitorRepo, checkRepo, notifier, publisher)
	check, err := uc.Check(context.Background(), monitor)

	assert.NoError(t, err)
	assert.True(t, check.IsUp)
	assert.Equal(t, http.StatusOK, check.StatusCode)
	monitorRepo.AssertNotCalled(t, "UpdateStatus", mock.Anything, mock.Anything, mock.Anything)
	notifier.AssertNotCalled(t, "NotifyMonitorDown", mock.Anything, mock.Anything, mock.Anything)
	// Status TIDAK berubah — broadcaster tidak boleh kepanggil sama sekali.
	publisher.AssertNotCalled(t, "Publish", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestMonitorCheckerUsecase_Check_RecoversImmediately(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	monitorRepo := new(mockMonitorRepo)
	checkRepo := new(mockMonitorCheckRepo)
	notifier := new(mockMonitorNotifier)
	publisher := new(mockMonitorSyncPublisher)

	monitor := &domain.Monitor{ID: "mon-1", ProjectID: "proj-1", URL: server.URL, Status: domain.MonitorStatusDown, FailureThreshold: 3}

	checkRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.MonitorCheck")).Return(nil)
	monitorRepo.On("UpdateStatus", mock.Anything, "mon-1", domain.MonitorStatusUp).Return(nil)
	publisher.On("Publish", mock.Anything, "proj-1", "monitor.status_changed", mock.Anything).Return(nil)

	uc := usecase.NewMonitorCheckerUsecase(monitorRepo, checkRepo, notifier, publisher)
	_, err := uc.Check(context.Background(), monitor)

	assert.NoError(t, err)
	// Recovery IMMEDIATE — 1 check sukses cukup, tidak perlu N kali.
	assert.Equal(t, domain.MonitorStatusUp, monitor.Status)
	monitorRepo.AssertCalled(t, "UpdateStatus", mock.Anything, "mon-1", domain.MonitorStatusUp)
	notifier.AssertNotCalled(t, "NotifyMonitorDown", mock.Anything, mock.Anything, mock.Anything)
	// Recovery JUGA harus broadcast — bukan cuma pas down.
	publisher.AssertCalled(t, "Publish", mock.Anything, "proj-1", "monitor.status_changed", mock.Anything)
}

func TestMonitorCheckerUsecase_Check_NotEnoughConsecutiveFailures_NoTransition(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	monitorRepo := new(mockMonitorRepo)
	checkRepo := new(mockMonitorCheckRepo)
	notifier := new(mockMonitorNotifier)
	publisher := new(mockMonitorSyncPublisher)

	monitor := &domain.Monitor{ID: "mon-1", ProjectID: "proj-1", URL: server.URL, Status: domain.MonitorStatusUp, FailureThreshold: 3}

	checkRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.MonitorCheck")).Return(nil)
	checkRepo.On("ListRecentByMonitorID", mock.Anything, "mon-1", 3).Return([]*domain.MonitorCheck{
		{IsUp: false}, {IsUp: false},
	}, nil)

	uc := usecase.NewMonitorCheckerUsecase(monitorRepo, checkRepo, notifier, publisher)
	_, err := uc.Check(context.Background(), monitor)

	assert.NoError(t, err)
	assert.Equal(t, domain.MonitorStatusUp, monitor.Status)
	monitorRepo.AssertNotCalled(t, "UpdateStatus", mock.Anything, mock.Anything, mock.Anything)
	notifier.AssertNotCalled(t, "NotifyMonitorDown", mock.Anything, mock.Anything, mock.Anything)
	publisher.AssertNotCalled(t, "Publish", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestMonitorCheckerUsecase_Check_ThresholdReached_TransitionsAndNotifies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	monitorRepo := new(mockMonitorRepo)
	checkRepo := new(mockMonitorCheckRepo)
	notifier := new(mockMonitorNotifier)
	publisher := new(mockMonitorSyncPublisher)

	monitor := &domain.Monitor{ID: "mon-1", ProjectID: "proj-1", URL: server.URL, Status: domain.MonitorStatusUp, FailureThreshold: 3}

	checkRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.MonitorCheck")).Return(nil)
	checkRepo.On("ListRecentByMonitorID", mock.Anything, "mon-1", 3).Return([]*domain.MonitorCheck{
		{IsUp: false}, {IsUp: false}, {IsUp: false},
	}, nil)
	monitorRepo.On("UpdateStatus", mock.Anything, "mon-1", domain.MonitorStatusDown).Return(nil)
	notifier.On("NotifyMonitorDown", mock.Anything, monitor, 3).Return(nil)
	publisher.On("Publish", mock.Anything, "proj-1", "monitor.status_changed", mock.Anything).Return(nil)

	uc := usecase.NewMonitorCheckerUsecase(monitorRepo, checkRepo, notifier, publisher)
	_, err := uc.Check(context.Background(), monitor)

	assert.NoError(t, err)
	assert.Equal(t, domain.MonitorStatusDown, monitor.Status)
	notifier.AssertCalled(t, "NotifyMonitorDown", mock.Anything, monitor, 3)
	publisher.AssertCalled(t, "Publish", mock.Anything, "proj-1", "monitor.status_changed", mock.Anything)
}

func TestMonitorCheckerUsecase_Check_FailureStreakBroken_NoTransition(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	monitorRepo := new(mockMonitorRepo)
	checkRepo := new(mockMonitorCheckRepo)
	notifier := new(mockMonitorNotifier)
	publisher := new(mockMonitorSyncPublisher)

	monitor := &domain.Monitor{ID: "mon-1", ProjectID: "proj-1", URL: server.URL, Status: domain.MonitorStatusUp, FailureThreshold: 3}

	checkRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.MonitorCheck")).Return(nil)
	checkRepo.On("ListRecentByMonitorID", mock.Anything, "mon-1", 3).Return([]*domain.MonitorCheck{
		{IsUp: false}, {IsUp: true}, {IsUp: false},
	}, nil)

	uc := usecase.NewMonitorCheckerUsecase(monitorRepo, checkRepo, notifier, publisher)
	_, err := uc.Check(context.Background(), monitor)

	assert.NoError(t, err)
	assert.Equal(t, domain.MonitorStatusUp, monitor.Status)
	notifier.AssertNotCalled(t, "NotifyMonitorDown", mock.Anything, mock.Anything, mock.Anything)
	publisher.AssertNotCalled(t, "Publish", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// TestMonitorCheckerUsecase_Check_CalledTwiceWhileDown_NotifiesOnlyOnce
// adalah REGRESSION TEST buat bug yang ketemu pas manual testing Postman
// (section C6) — lihat komentar lengkap di versi sebelumnya. Sekarang
// juga sekalian verifikasi publisher CUMA kepanggil sekali (bukan cuma
// notifier), karena keduanya di-gate oleh kondisi transisi yang sama.
func TestMonitorCheckerUsecase_Check_CalledTwiceWhileDown_NotifiesOnlyOnce(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	monitorRepo := new(mockMonitorRepo)
	checkRepo := new(mockMonitorCheckRepo)
	notifier := new(mockMonitorNotifier)
	publisher := new(mockMonitorSyncPublisher)

	monitor := &domain.Monitor{ID: "mon-1", ProjectID: "proj-1", URL: server.URL, Status: domain.MonitorStatusUp, FailureThreshold: 1}

	checkRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.MonitorCheck")).Return(nil)
	checkRepo.On("ListRecentByMonitorID", mock.Anything, "mon-1", 1).Return([]*domain.MonitorCheck{
		{IsUp: false},
	}, nil)
	monitorRepo.On("UpdateStatus", mock.Anything, "mon-1", domain.MonitorStatusDown).Return(nil).Once()
	notifier.On("NotifyMonitorDown", mock.Anything, monitor, 1).Return(nil).Once()
	publisher.On("Publish", mock.Anything, "proj-1", "monitor.status_changed", mock.Anything).Return(nil).Once()

	uc := usecase.NewMonitorCheckerUsecase(monitorRepo, checkRepo, notifier, publisher)

	_, err1 := uc.Check(context.Background(), monitor)
	_, err2 := uc.Check(context.Background(), monitor)

	assert.NoError(t, err1)
	assert.NoError(t, err2)
	monitorRepo.AssertNumberOfCalls(t, "UpdateStatus", 1)
	notifier.AssertNumberOfCalls(t, "NotifyMonitorDown", 1)
	publisher.AssertNumberOfCalls(t, "Publish", 1)
}
