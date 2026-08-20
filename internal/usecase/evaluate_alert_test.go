package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
	"github.com/MohdFarhanS/sentinelix-backend/internal/usecase"
)

type mockAlertLogRepo struct {
	mock.Mock
}

func (m *mockAlertLogRepo) Create(ctx context.Context, l *domain.AlertLog) error {
	args := m.Called(ctx, l)
	return args.Error(0)
}

func (m *mockAlertLogRepo) GetLastSentAt(ctx context.Context, alertRuleID, issueID string) (*time.Time, error) {
	args := m.Called(ctx, alertRuleID, issueID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*time.Time), args.Error(1)
}

type mockNotifier struct {
	mock.Mock
}

func (m *mockNotifier) Notify(ctx context.Context, rule *domain.AlertRule, issue *domain.Issue) error {
	args := m.Called(ctx, rule, issue)
	return args.Error(0)
}

// --- EvaluateNewIssue ---

func TestEvaluateNewIssue_SendsNotification_WhenNoCooldown(t *testing.T) {
	alertRuleRepo := new(mockAlertRuleRepo)
	alertLogRepo := new(mockAlertLogRepo)
	notifier := new(mockNotifier)
	issueRepo := new(mockIssueRepo)
	eventRepo := new(mockEventRepo)

	rule := &domain.AlertRule{ID: "rule-1", ProjectID: "proj-1", ConditionType: domain.ConditionTypeNewIssue, CooldownMinutes: 60}
	issue := &domain.Issue{ID: "issue-1", ProjectID: "proj-1", Title: "New Error"}

	alertRuleRepo.On("ListActiveNewIssueRules", mock.Anything, "proj-1").Return([]*domain.AlertRule{rule}, nil)
	alertLogRepo.On("GetLastSentAt", mock.Anything, "rule-1", "issue-1").Return(nil, nil)
	notifier.On("Notify", mock.Anything, rule, issue).Return(nil)
	alertLogRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.AlertLog")).Return(nil)

	uc := usecase.NewEvaluateAlertUsecase(alertRuleRepo, alertLogRepo, issueRepo, eventRepo, notifier)
	err := uc.EvaluateNewIssue(context.Background(), issue)

	assert.NoError(t, err)
	notifier.AssertCalled(t, "Notify", mock.Anything, rule, issue)
	alertLogRepo.AssertCalled(t, "Create", mock.Anything, mock.AnythingOfType("*domain.AlertLog"))
}

func TestEvaluateNewIssue_SkipsNotification_WhenInCooldown(t *testing.T) {
	alertRuleRepo := new(mockAlertRuleRepo)
	alertLogRepo := new(mockAlertLogRepo)
	notifier := new(mockNotifier)
	issueRepo := new(mockIssueRepo)
	eventRepo := new(mockEventRepo)

	rule := &domain.AlertRule{ID: "rule-1", ProjectID: "proj-1", ConditionType: domain.ConditionTypeNewIssue, CooldownMinutes: 60}
	issue := &domain.Issue{ID: "issue-1", ProjectID: "proj-1"}

	// Notifikasi terakhir 10 menit lalu, cooldown 60 menit — masih dalam cooldown.
	recentSend := time.Now().Add(-10 * time.Minute)

	alertRuleRepo.On("ListActiveNewIssueRules", mock.Anything, "proj-1").Return([]*domain.AlertRule{rule}, nil)
	alertLogRepo.On("GetLastSentAt", mock.Anything, "rule-1", "issue-1").Return(&recentSend, nil)

	uc := usecase.NewEvaluateAlertUsecase(alertRuleRepo, alertLogRepo, issueRepo, eventRepo, notifier)
	err := uc.EvaluateNewIssue(context.Background(), issue)

	assert.NoError(t, err)
	notifier.AssertNotCalled(t, "Notify", mock.Anything, mock.Anything, mock.Anything)
	alertLogRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestEvaluateNewIssue_NoRules_NoNotification(t *testing.T) {
	alertRuleRepo := new(mockAlertRuleRepo)
	alertLogRepo := new(mockAlertLogRepo)
	notifier := new(mockNotifier)
	issueRepo := new(mockIssueRepo)
	eventRepo := new(mockEventRepo)

	issue := &domain.Issue{ID: "issue-1", ProjectID: "proj-1"}
	alertRuleRepo.On("ListActiveNewIssueRules", mock.Anything, "proj-1").Return([]*domain.AlertRule{}, nil)

	uc := usecase.NewEvaluateAlertUsecase(alertRuleRepo, alertLogRepo, issueRepo, eventRepo, notifier)
	err := uc.EvaluateNewIssue(context.Background(), issue)

	assert.NoError(t, err)
	notifier.AssertNotCalled(t, "Notify", mock.Anything, mock.Anything, mock.Anything)
}

// TestEvaluateNewIssue_NotifierFails_DoesNotCreateAlertLog memverifikasi
// urutan penting di notifyIfNotCooldown: Create alert_logs HANYA terjadi
// SETELAH Notify sukses — kalau Notify gagal, TIDAK ADA log tercatat
// (biar rule ini tidak kena cooldown palsu).
func TestEvaluateNewIssue_NotifierFails_DoesNotCreateAlertLog(t *testing.T) {
	alertRuleRepo := new(mockAlertRuleRepo)
	alertLogRepo := new(mockAlertLogRepo)
	notifier := new(mockNotifier)
	issueRepo := new(mockIssueRepo)
	eventRepo := new(mockEventRepo)

	rule := &domain.AlertRule{ID: "rule-1", ProjectID: "proj-1", ConditionType: domain.ConditionTypeNewIssue, CooldownMinutes: 60}
	issue := &domain.Issue{ID: "issue-1", ProjectID: "proj-1"}

	alertRuleRepo.On("ListActiveNewIssueRules", mock.Anything, "proj-1").Return([]*domain.AlertRule{rule}, nil)
	alertLogRepo.On("GetLastSentAt", mock.Anything, "rule-1", "issue-1").Return(nil, nil)
	notifier.On("Notify", mock.Anything, rule, issue).Return(errors.New("resend API down"))

	uc := usecase.NewEvaluateAlertUsecase(alertRuleRepo, alertLogRepo, issueRepo, eventRepo, notifier)
	// EvaluateNewIssue SENGAJA tidak propagate error dari 1 rule yang gagal
	// (lihat komentar di usecase asli) — makanya overall error tetap nil.
	err := uc.EvaluateNewIssue(context.Background(), issue)

	assert.NoError(t, err)
	alertLogRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

// --- EvaluateThresholds ---

func TestEvaluateThresholds_SendsNotification_WhenCountExceedsThreshold(t *testing.T) {
	alertRuleRepo := new(mockAlertRuleRepo)
	alertLogRepo := new(mockAlertLogRepo)
	notifier := new(mockNotifier)
	eventRepo := new(mockEventRepo)
	issueRepo := new(mockIssueRepo)

	rule := &domain.AlertRule{
		ID: "rule-1", ProjectID: "proj-1", ConditionType: domain.ConditionTypeThreshold,
		Threshold: 3, WindowMinutes: 5, CooldownMinutes: 60,
	}
	issue := &domain.Issue{ID: "issue-1", ProjectID: "proj-1", Title: "Spiking Error"}

	alertRuleRepo.On("ListActiveThresholdRules", mock.Anything).Return([]*domain.AlertRule{rule}, nil)
	eventRepo.On("CountGroupedByIssueSince", mock.Anything, "proj-1", mock.AnythingOfType("time.Time")).
		Return(map[string]int{"issue-1": 5}, nil)
	issueRepo.On("GetByID", mock.Anything, "issue-1").Return(issue, nil)
	alertLogRepo.On("GetLastSentAt", mock.Anything, "rule-1", "issue-1").Return(nil, nil)
	notifier.On("Notify", mock.Anything, rule, issue).Return(nil)
	alertLogRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.AlertLog")).Return(nil)

	uc := usecase.NewEvaluateAlertUsecase(alertRuleRepo, alertLogRepo, issueRepo, eventRepo, notifier)
	err := uc.EvaluateThresholds(context.Background())

	assert.NoError(t, err)
	notifier.AssertCalled(t, "Notify", mock.Anything, rule, issue)
}

func TestEvaluateThresholds_SkipsNotification_WhenCountBelowThreshold(t *testing.T) {
	alertRuleRepo := new(mockAlertRuleRepo)
	alertLogRepo := new(mockAlertLogRepo)
	notifier := new(mockNotifier)
	eventRepo := new(mockEventRepo)
	issueRepo := new(mockIssueRepo)

	rule := &domain.AlertRule{
		ID: "rule-1", ProjectID: "proj-1", ConditionType: domain.ConditionTypeThreshold,
		Threshold: 3, WindowMinutes: 5, CooldownMinutes: 60,
	}

	alertRuleRepo.On("ListActiveThresholdRules", mock.Anything).Return([]*domain.AlertRule{rule}, nil)
	eventRepo.On("CountGroupedByIssueSince", mock.Anything, "proj-1", mock.AnythingOfType("time.Time")).
		Return(map[string]int{"issue-1": 2}, nil) // 2 <= threshold 3

	uc := usecase.NewEvaluateAlertUsecase(alertRuleRepo, alertLogRepo, issueRepo, eventRepo, notifier)
	err := uc.EvaluateThresholds(context.Background())

	assert.NoError(t, err)
	notifier.AssertNotCalled(t, "Notify", mock.Anything, mock.Anything, mock.Anything)
	issueRepo.AssertNotCalled(t, "GetByID", mock.Anything, mock.Anything) // count <= threshold → skip sebelum sempat GetByID
}

func TestEvaluateThresholds_SkipsNotification_WhenInCooldown(t *testing.T) {
	alertRuleRepo := new(mockAlertRuleRepo)
	alertLogRepo := new(mockAlertLogRepo)
	notifier := new(mockNotifier)
	eventRepo := new(mockEventRepo)
	issueRepo := new(mockIssueRepo)

	rule := &domain.AlertRule{
		ID: "rule-1", ProjectID: "proj-1", ConditionType: domain.ConditionTypeThreshold,
		Threshold: 3, WindowMinutes: 5, CooldownMinutes: 60,
	}
	issue := &domain.Issue{ID: "issue-1", ProjectID: "proj-1"}
	recentSend := time.Now().Add(-5 * time.Minute)

	alertRuleRepo.On("ListActiveThresholdRules", mock.Anything).Return([]*domain.AlertRule{rule}, nil)
	eventRepo.On("CountGroupedByIssueSince", mock.Anything, "proj-1", mock.AnythingOfType("time.Time")).
		Return(map[string]int{"issue-1": 10}, nil)
	issueRepo.On("GetByID", mock.Anything, "issue-1").Return(issue, nil)
	alertLogRepo.On("GetLastSentAt", mock.Anything, "rule-1", "issue-1").Return(&recentSend, nil)

	uc := usecase.NewEvaluateAlertUsecase(alertRuleRepo, alertLogRepo, issueRepo, eventRepo, notifier)
	err := uc.EvaluateThresholds(context.Background())

	assert.NoError(t, err)
	notifier.AssertNotCalled(t, "Notify", mock.Anything, mock.Anything, mock.Anything)
}