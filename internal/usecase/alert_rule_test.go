package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
	"github.com/MohdFarhanS/sentinelix-backend/internal/usecase"
)

type mockAlertRuleRepo struct {
	mock.Mock
}

func (m *mockAlertRuleRepo) Create(ctx context.Context, r *domain.AlertRule) error {
	args := m.Called(ctx, r)
	r.ID = "rule-generated-id"
	r.CreatedAt = time.Now()
	return args.Error(0)
}

func (m *mockAlertRuleRepo) GetByID(ctx context.Context, id string) (*domain.AlertRule, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.AlertRule), args.Error(1)
}

func (m *mockAlertRuleRepo) ListByProjectID(ctx context.Context, projectID string) ([]*domain.AlertRule, error) {
	args := m.Called(ctx, projectID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.AlertRule), args.Error(1)
}

func (m *mockAlertRuleRepo) ListActiveNewIssueRules(ctx context.Context, projectID string) ([]*domain.AlertRule, error) {
	args := m.Called(ctx, projectID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.AlertRule), args.Error(1)
}

func (m *mockAlertRuleRepo) ListActiveThresholdRules(ctx context.Context) ([]*domain.AlertRule, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.AlertRule), args.Error(1)
}

func (m *mockAlertRuleRepo) Update(ctx context.Context, r *domain.AlertRule) error {
	args := m.Called(ctx, r)
	return args.Error(0)
}

func (m *mockAlertRuleRepo) Delete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// mockAuditLogRepo — dipakai bareng oleh alert_rule_test.go DAN
// project_test.go (satu package usecase_test) — TIDAK didefinisikan
// ulang di project_test.go, sama pola seperti mockProjectRepo.
type mockAuditLogRepo struct{ mock.Mock }

var errAuditLogWrite = errors.New("simulated audit log write failure")

func (m *mockAuditLogRepo) Create(ctx context.Context, log *domain.AuditLog) error {
	args := m.Called(ctx, log)
	return args.Error(0)
}

func TestAlertRuleUsecase_Create_Success(t *testing.T) {
	alertRuleRepo := new(mockAlertRuleRepo)
	projectRepo := new(mockProjectRepo)
	auditLogRepo := new(mockAuditLogRepo)

	projectRepo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{
		ID: "proj-1", UserID: "user-1",
	}, nil)
	alertRuleRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.AlertRule")).Return(nil)
	auditLogRepo.On("Create", mock.Anything, mock.Anything).Return(nil)

	uc := usecase.NewAlertRuleUsecase(alertRuleRepo, projectRepo, auditLogRepo, zerolog.Nop())
	rule, err := uc.Create(context.Background(), usecase.CreateAlertRuleInput{
		UserID:          "user-1",
		ProjectID:       "proj-1",
		ConditionType:   domain.ConditionTypeNewIssue,
		CooldownMinutes: 60,
		Channel:         domain.ChannelSlack,
		ChannelTarget:   "https://hooks.slack.com/services/xxx",
	})

	assert.NoError(t, err)
	assert.Equal(t, "rule-generated-id", rule.ID)
}

// NEW (Sprint 9): mastiin audit log BENERAN ditulis dengan action &
// resource_id yang benar setelah Create sukses.
func TestAlertRuleUsecase_Create_WritesAuditLog(t *testing.T) {
	alertRuleRepo := new(mockAlertRuleRepo)
	projectRepo := new(mockProjectRepo)
	auditLogRepo := new(mockAuditLogRepo)

	projectRepo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{
		ID: "proj-1", UserID: "user-1",
	}, nil)
	alertRuleRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.AlertRule")).Return(nil)
	auditLogRepo.On("Create", mock.Anything, mock.MatchedBy(func(log *domain.AuditLog) bool {
		return log.Action == domain.ActionAlertRuleCreated &&
			log.ResourceType == domain.ResourceTypeAlertRule &&
			log.ResourceID == "rule-generated-id" &&
			*log.ActorUserID == "user-1"
	})).Return(nil)

	uc := usecase.NewAlertRuleUsecase(alertRuleRepo, projectRepo, auditLogRepo, zerolog.Nop())
	_, err := uc.Create(context.Background(), usecase.CreateAlertRuleInput{
		UserID:          "user-1",
		ProjectID:       "proj-1",
		ConditionType:   domain.ConditionTypeNewIssue,
		CooldownMinutes: 60,
		Channel:         domain.ChannelSlack,
		ChannelTarget:   "https://hooks.slack.com/services/xxx",
	})

	require.NoError(t, err)
	auditLogRepo.AssertExpectations(t)
}

// NEW (Sprint 9): audit log gagal tulis TIDAK BOLEH menggagalkan Create
// yang sudah sukses — best-effort, sesuai keputusan Sprint 9.
func TestAlertRuleUsecase_Create_AuditLogFailure_StillSucceeds(t *testing.T) {
	alertRuleRepo := new(mockAlertRuleRepo)
	projectRepo := new(mockProjectRepo)
	auditLogRepo := new(mockAuditLogRepo)

	projectRepo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{
		ID: "proj-1", UserID: "user-1",
	}, nil)
	alertRuleRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.AlertRule")).Return(nil)
	auditLogRepo.On("Create", mock.Anything, mock.Anything).
		Return(errAuditLogWrite)

	uc := usecase.NewAlertRuleUsecase(alertRuleRepo, projectRepo, auditLogRepo, zerolog.Nop())
	rule, err := uc.Create(context.Background(), usecase.CreateAlertRuleInput{
		UserID:          "user-1",
		ProjectID:       "proj-1",
		ConditionType:   domain.ConditionTypeNewIssue,
		CooldownMinutes: 60,
		Channel:         domain.ChannelSlack,
		ChannelTarget:   "https://hooks.slack.com/services/xxx",
	})

	assert.NoError(t, err)
	assert.Equal(t, "rule-generated-id", rule.ID)
}

func TestAlertRuleUsecase_Create_ProjectNotFound(t *testing.T) {
	alertRuleRepo := new(mockAlertRuleRepo)
	projectRepo := new(mockProjectRepo)
	auditLogRepo := new(mockAuditLogRepo)

	projectRepo.On("GetByID", mock.Anything, "proj-x").Return(nil, domain.ErrProjectNotFound)

	uc := usecase.NewAlertRuleUsecase(alertRuleRepo, projectRepo, auditLogRepo, zerolog.Nop())
	_, err := uc.Create(context.Background(), usecase.CreateAlertRuleInput{
		UserID:    "user-1",
		ProjectID: "proj-x",
	})

	assert.ErrorIs(t, err, domain.ErrProjectNotFound)
	alertRuleRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	auditLogRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestAlertRuleUsecase_Create_Forbidden(t *testing.T) {
	alertRuleRepo := new(mockAlertRuleRepo)
	projectRepo := new(mockProjectRepo)
	auditLogRepo := new(mockAuditLogRepo)

	projectRepo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{
		ID: "proj-1", UserID: "user-OTHER",
	}, nil)

	uc := usecase.NewAlertRuleUsecase(alertRuleRepo, projectRepo, auditLogRepo, zerolog.Nop())
	_, err := uc.Create(context.Background(), usecase.CreateAlertRuleInput{
		UserID:    "user-1",
		ProjectID: "proj-1",
	})

	assert.ErrorIs(t, err, usecase.ErrForbidden)
	alertRuleRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestAlertRuleUsecase_Create_ValidationError(t *testing.T) {
	alertRuleRepo := new(mockAlertRuleRepo)
	projectRepo := new(mockProjectRepo)
	auditLogRepo := new(mockAuditLogRepo)

	projectRepo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{
		ID: "proj-1", UserID: "user-1",
	}, nil)

	uc := usecase.NewAlertRuleUsecase(alertRuleRepo, projectRepo, auditLogRepo, zerolog.Nop())
	_, err := uc.Create(context.Background(), usecase.CreateAlertRuleInput{
		UserID:          "user-1",
		ProjectID:       "proj-1",
		ConditionType:   domain.ConditionTypeNewIssue,
		CooldownMinutes: 60,
		Channel:         "invalid-channel",
		ChannelTarget:   "target",
	})

	assert.ErrorIs(t, err, domain.ErrAlertChannelInvalid)
	alertRuleRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	auditLogRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestAlertRuleUsecase_List_Success(t *testing.T) {
	alertRuleRepo := new(mockAlertRuleRepo)
	projectRepo := new(mockProjectRepo)
	auditLogRepo := new(mockAuditLogRepo)

	projectRepo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{
		ID: "proj-1", UserID: "user-1",
	}, nil)
	alertRuleRepo.On("ListByProjectID", mock.Anything, "proj-1").Return([]*domain.AlertRule{
		{ID: "rule-1", ProjectID: "proj-1"},
	}, nil)

	uc := usecase.NewAlertRuleUsecase(alertRuleRepo, projectRepo, auditLogRepo, zerolog.Nop())
	rules, err := uc.List(context.Background(), usecase.ListAlertRulesInput{
		UserID:    "user-1",
		ProjectID: "proj-1",
	})

	assert.NoError(t, err)
	assert.Len(t, rules, 1)
}

func TestAlertRuleUsecase_List_Forbidden(t *testing.T) {
	alertRuleRepo := new(mockAlertRuleRepo)
	projectRepo := new(mockProjectRepo)
	auditLogRepo := new(mockAuditLogRepo)

	projectRepo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{
		ID: "proj-1", UserID: "user-OTHER",
	}, nil)

	uc := usecase.NewAlertRuleUsecase(alertRuleRepo, projectRepo, auditLogRepo, zerolog.Nop())
	_, err := uc.List(context.Background(), usecase.ListAlertRulesInput{
		UserID:    "user-1",
		ProjectID: "proj-1",
	})

	assert.ErrorIs(t, err, usecase.ErrForbidden)
}

func TestAlertRuleUsecase_Update_PartialFields_OnlyChangesGiven(t *testing.T) {
	alertRuleRepo := new(mockAlertRuleRepo)
	projectRepo := new(mockProjectRepo)
	auditLogRepo := new(mockAuditLogRepo)

	existing := &domain.AlertRule{
		ID: "rule-1", ProjectID: "proj-1", ConditionType: domain.ConditionTypeThreshold,
		Threshold: 50, WindowMinutes: 60, CooldownMinutes: 60, Channel: domain.ChannelSlack, ChannelTarget: "https://hooks.slack.com/xxx",
	}

	alertRuleRepo.On("GetByID", mock.Anything, "rule-1").Return(existing, nil)
	projectRepo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{ID: "proj-1", UserID: "user-1"}, nil)
	alertRuleRepo.On("Update", mock.Anything, mock.AnythingOfType("*domain.AlertRule")).Return(nil)
	auditLogRepo.On("Create", mock.Anything, mock.Anything).Return(nil)

	newThreshold := 100
	newCooldown := 30
	uc := usecase.NewAlertRuleUsecase(alertRuleRepo, projectRepo, auditLogRepo, zerolog.Nop())
	updated, err := uc.Update(context.Background(), usecase.UpdateAlertRuleInput{
		UserID:          "user-1",
		AlertRuleID:     "rule-1",
		Threshold:       &newThreshold,
		CooldownMinutes: &newCooldown,
	})

	assert.NoError(t, err)
	assert.Equal(t, 100, updated.Threshold)
	assert.Equal(t, 30, updated.CooldownMinutes)
	assert.Equal(t, 60, updated.WindowMinutes)
}

// NEW (Sprint 9): metadata audit log Update cuma isi field yang BENERAN
// dikirim (Threshold & CooldownMinutes), TIDAK ikut nyimpen WindowMinutes
// yang tidak diubah — sesuai desain "changedFields" di AlertRuleUsecase.Update.
func TestAlertRuleUsecase_Update_AuditLogOnlyIncludesChangedFields(t *testing.T) {
	alertRuleRepo := new(mockAlertRuleRepo)
	projectRepo := new(mockProjectRepo)
	auditLogRepo := new(mockAuditLogRepo)

	existing := &domain.AlertRule{
		ID: "rule-1", ProjectID: "proj-1", ConditionType: domain.ConditionTypeThreshold,
		Threshold: 50, WindowMinutes: 60, CooldownMinutes: 60, Channel: domain.ChannelSlack, ChannelTarget: "https://hooks.slack.com/xxx",
	}

	alertRuleRepo.On("GetByID", mock.Anything, "rule-1").Return(existing, nil)
	projectRepo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{ID: "proj-1", UserID: "user-1"}, nil)
	alertRuleRepo.On("Update", mock.Anything, mock.AnythingOfType("*domain.AlertRule")).Return(nil)
	auditLogRepo.On("Create", mock.Anything, mock.MatchedBy(func(log *domain.AuditLog) bool {
		_, hasThreshold := log.Metadata["threshold"]
		_, hasWindowMinutes := log.Metadata["window_minutes"]
		return log.Action == domain.ActionAlertRuleUpdated && hasThreshold && !hasWindowMinutes
	})).Return(nil)

	newThreshold := 100
	uc := usecase.NewAlertRuleUsecase(alertRuleRepo, projectRepo, auditLogRepo, zerolog.Nop())
	_, err := uc.Update(context.Background(), usecase.UpdateAlertRuleInput{
		UserID:      "user-1",
		AlertRuleID: "rule-1",
		Threshold:   &newThreshold,
	})

	assert.NoError(t, err)
	auditLogRepo.AssertExpectations(t)
}

func TestAlertRuleUsecase_Update_RevalidatesFinalState(t *testing.T) {
	alertRuleRepo := new(mockAlertRuleRepo)
	projectRepo := new(mockProjectRepo)
	auditLogRepo := new(mockAuditLogRepo)

	existing := &domain.AlertRule{
		ID: "rule-1", ProjectID: "proj-1", ConditionType: domain.ConditionTypeNewIssue,
		WindowMinutes: 0, CooldownMinutes: 60, Channel: domain.ChannelEmail, ChannelTarget: "a@b.com",
	}

	alertRuleRepo.On("GetByID", mock.Anything, "rule-1").Return(existing, nil)
	projectRepo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{ID: "proj-1", UserID: "user-1"}, nil)

	newConditionType := domain.ConditionTypeThreshold
	uc := usecase.NewAlertRuleUsecase(alertRuleRepo, projectRepo, auditLogRepo, zerolog.Nop())
	_, err := uc.Update(context.Background(), usecase.UpdateAlertRuleInput{
		UserID:        "user-1",
		AlertRuleID:   "rule-1",
		ConditionType: &newConditionType,
	})

	assert.ErrorIs(t, err, domain.ErrAlertWindowMinutesInvalid)
	alertRuleRepo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
	auditLogRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestAlertRuleUsecase_Update_NotFound(t *testing.T) {
	alertRuleRepo := new(mockAlertRuleRepo)
	projectRepo := new(mockProjectRepo)
	auditLogRepo := new(mockAuditLogRepo)

	alertRuleRepo.On("GetByID", mock.Anything, "rule-x").Return(nil, domain.ErrAlertRuleNotFound)

	uc := usecase.NewAlertRuleUsecase(alertRuleRepo, projectRepo, auditLogRepo, zerolog.Nop())
	_, err := uc.Update(context.Background(), usecase.UpdateAlertRuleInput{UserID: "user-1", AlertRuleID: "rule-x"})

	assert.ErrorIs(t, err, domain.ErrAlertRuleNotFound)
	projectRepo.AssertNotCalled(t, "GetByID", mock.Anything, mock.Anything)
}

func TestAlertRuleUsecase_Update_Forbidden(t *testing.T) {
	alertRuleRepo := new(mockAlertRuleRepo)
	projectRepo := new(mockProjectRepo)
	auditLogRepo := new(mockAuditLogRepo)

	existing := &domain.AlertRule{ID: "rule-1", ProjectID: "proj-1"}
	alertRuleRepo.On("GetByID", mock.Anything, "rule-1").Return(existing, nil)
	projectRepo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{ID: "proj-1", UserID: "user-OTHER"}, nil)

	uc := usecase.NewAlertRuleUsecase(alertRuleRepo, projectRepo, auditLogRepo, zerolog.Nop())
	_, err := uc.Update(context.Background(), usecase.UpdateAlertRuleInput{UserID: "user-1", AlertRuleID: "rule-1"})

	assert.ErrorIs(t, err, usecase.ErrForbidden)
	alertRuleRepo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
}

func TestAlertRuleUsecase_Delete_Success(t *testing.T) {
	alertRuleRepo := new(mockAlertRuleRepo)
	projectRepo := new(mockProjectRepo)
	auditLogRepo := new(mockAuditLogRepo)

	existing := &domain.AlertRule{ID: "rule-1", ProjectID: "proj-1"}
	alertRuleRepo.On("GetByID", mock.Anything, "rule-1").Return(existing, nil)
	projectRepo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{ID: "proj-1", UserID: "user-1"}, nil)
	alertRuleRepo.On("Delete", mock.Anything, "rule-1").Return(nil)
	auditLogRepo.On("Create", mock.Anything, mock.MatchedBy(func(log *domain.AuditLog) bool {
		return log.Action == domain.ActionAlertRuleDeleted && log.ResourceID == "rule-1"
	})).Return(nil)

	uc := usecase.NewAlertRuleUsecase(alertRuleRepo, projectRepo, auditLogRepo, zerolog.Nop())
	err := uc.Delete(context.Background(), usecase.DeleteAlertRuleInput{UserID: "user-1", AlertRuleID: "rule-1"})

	assert.NoError(t, err)
	alertRuleRepo.AssertCalled(t, "Delete", mock.Anything, "rule-1")
	auditLogRepo.AssertExpectations(t)
}

func TestAlertRuleUsecase_Delete_Forbidden_DoesNotDelete(t *testing.T) {
	alertRuleRepo := new(mockAlertRuleRepo)
	projectRepo := new(mockProjectRepo)
	auditLogRepo := new(mockAuditLogRepo)

	existing := &domain.AlertRule{ID: "rule-1", ProjectID: "proj-1"}
	alertRuleRepo.On("GetByID", mock.Anything, "rule-1").Return(existing, nil)
	projectRepo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{ID: "proj-1", UserID: "user-OTHER"}, nil)

	uc := usecase.NewAlertRuleUsecase(alertRuleRepo, projectRepo, auditLogRepo, zerolog.Nop())
	err := uc.Delete(context.Background(), usecase.DeleteAlertRuleInput{UserID: "user-1", AlertRuleID: "rule-1"})

	assert.ErrorIs(t, err, usecase.ErrForbidden)
	alertRuleRepo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
	auditLogRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}