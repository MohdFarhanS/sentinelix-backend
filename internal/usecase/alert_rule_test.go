package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

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

func TestAlertRuleUsecase_Create_Success(t *testing.T) {
	alertRuleRepo := new(mockAlertRuleRepo)
	projectRepo := new(mockProjectRepo)

	projectRepo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{
		ID: "proj-1", UserID: "user-1",
	}, nil)
	alertRuleRepo.On("Create", mock.Anything, mock.AnythingOfType("*domain.AlertRule")).Return(nil)

	uc := usecase.NewAlertRuleUsecase(alertRuleRepo, projectRepo)
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

	projectRepo.On("GetByID", mock.Anything, "proj-x").Return(nil, domain.ErrProjectNotFound)

	uc := usecase.NewAlertRuleUsecase(alertRuleRepo, projectRepo)
	_, err := uc.Create(context.Background(), usecase.CreateAlertRuleInput{
		UserID:    "user-1",
		ProjectID: "proj-x",
	})

	assert.ErrorIs(t, err, domain.ErrProjectNotFound)
	alertRuleRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestAlertRuleUsecase_Create_Forbidden(t *testing.T) {
	alertRuleRepo := new(mockAlertRuleRepo)
	projectRepo := new(mockProjectRepo)

	projectRepo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{
		ID: "proj-1", UserID: "user-OTHER",
	}, nil)

	uc := usecase.NewAlertRuleUsecase(alertRuleRepo, projectRepo)
	_, err := uc.Create(context.Background(), usecase.CreateAlertRuleInput{
		UserID:    "user-1",
		ProjectID: "proj-1",
	})

	assert.ErrorIs(t, err, usecase.ErrForbidden)
	alertRuleRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

// TestAlertRuleUsecase_Create_ValidationError memverifikasi ownership check
// (403/404) SELALU dicek DULUAN sebelum field-level validation (400) —
// keputusan desain yang sudah kita diskusikan di awal Sprint 6.
func TestAlertRuleUsecase_Create_ValidationError(t *testing.T) {
	alertRuleRepo := new(mockAlertRuleRepo)
	projectRepo := new(mockProjectRepo)

	projectRepo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{
		ID: "proj-1", UserID: "user-1",
	}, nil)

	uc := usecase.NewAlertRuleUsecase(alertRuleRepo, projectRepo)
	_, err := uc.Create(context.Background(), usecase.CreateAlertRuleInput{
		UserID:          "user-1",
		ProjectID:       "proj-1",
		ConditionType:   domain.ConditionTypeNewIssue,
		CooldownMinutes: 60,
		Channel:         "invalid-channel", // channel harus 'email' atau 'slack'
		ChannelTarget:   "target",
	})

	assert.ErrorIs(t, err, domain.ErrAlertChannelInvalid)
	alertRuleRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestAlertRuleUsecase_List_Success(t *testing.T) {
	alertRuleRepo := new(mockAlertRuleRepo)
	projectRepo := new(mockProjectRepo)

	projectRepo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{
		ID: "proj-1", UserID: "user-1",
	}, nil)
	alertRuleRepo.On("ListByProjectID", mock.Anything, "proj-1").Return([]*domain.AlertRule{
		{ID: "rule-1", ProjectID: "proj-1"},
	}, nil)

	uc := usecase.NewAlertRuleUsecase(alertRuleRepo, projectRepo)
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

	projectRepo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{
		ID: "proj-1", UserID: "user-OTHER",
	}, nil)

	uc := usecase.NewAlertRuleUsecase(alertRuleRepo, projectRepo)
	_, err := uc.List(context.Background(), usecase.ListAlertRulesInput{
		UserID:    "user-1",
		ProjectID: "proj-1",
	})

	assert.ErrorIs(t, err, usecase.ErrForbidden)
}

func TestAlertRuleUsecase_Update_PartialFields_OnlyChangesGiven(t *testing.T) {
	alertRuleRepo := new(mockAlertRuleRepo)
	projectRepo := new(mockProjectRepo)

	existing := &domain.AlertRule{
		ID: "rule-1", ProjectID: "proj-1", ConditionType: domain.ConditionTypeThreshold,
		Threshold: 50, WindowMinutes: 60, CooldownMinutes: 60, Channel: domain.ChannelSlack, ChannelTarget: "https://hooks.slack.com/xxx",
	}

	alertRuleRepo.On("GetByID", mock.Anything, "rule-1").Return(existing, nil)
	projectRepo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{ID: "proj-1", UserID: "user-1"}, nil)
	alertRuleRepo.On("Update", mock.Anything, mock.AnythingOfType("*domain.AlertRule")).Return(nil)

	newThreshold := 100
	newCooldown := 30
	uc := usecase.NewAlertRuleUsecase(alertRuleRepo, projectRepo)
	updated, err := uc.Update(context.Background(), usecase.UpdateAlertRuleInput{
		UserID: 			"user-1",
		AlertRuleID: 		"rule-1",
		Threshold: 			&newThreshold,
		CooldownMinutes: 	&newCooldown,
	})

	assert.NoError(t, err)
	assert.Equal(t, 100, updated.Threshold)
	assert.Equal(t, 30, updated.CooldownMinutes)
	assert.Equal(t, 60, updated.WindowMinutes)
}

func TestAlertRuleUsecase_Update_RevalidatesFinalState(t *testing.T) {
	alertRuleRepo := new(mockAlertRuleRepo)
	projectRepo := new(mockProjectRepo)

	existing := &domain.AlertRule{
		ID: "rule-1", ProjectID: "proj-1", ConditionType: domain.ConditionTypeNewIssue, 
		WindowMinutes: 0, CooldownMinutes: 60, Channel: domain.ChannelEmail, ChannelTarget: "a@b.com",
	}

	alertRuleRepo.On("GetByID", mock.Anything, "rule-1").Return(existing, nil)
	projectRepo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{ID: "proj-1", UserID: "user-1"}, nil)

	newConditionType := domain.ConditionTypeThreshold
	uc := usecase.NewAlertRuleUsecase(alertRuleRepo, projectRepo)
	_, err := uc.Update(context.Background(), usecase.UpdateAlertRuleInput{
		UserID: 		"user-1",
		AlertRuleID: 	"rule-1",
		ConditionType: 	&newConditionType,
	})
	
	assert.ErrorIs(t, err, domain.ErrAlertWindowMinutesInvalid)
	alertRuleRepo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
}

func TestAlertRuleUsecase_Update_NotFound(t *testing.T) {
	alertRuleRepo := new(mockAlertRuleRepo)
	projectRepo := new(mockProjectRepo)

	alertRuleRepo.On("GetByID", mock.Anything, "rule-x").Return(nil, domain.ErrAlertRuleNotFound)

	uc := usecase.NewAlertRuleUsecase(alertRuleRepo, projectRepo)
	_, err := uc.Update(context.Background(), usecase.UpdateAlertRuleInput{UserID: "user-1", AlertRuleID: "rule-x"})

	assert.ErrorIs(t, err, domain.ErrAlertRuleNotFound)
	projectRepo.AssertNotCalled(t, "GetByID", mock.Anything, mock.Anything)
}

func TestAlertRuleUsecase_Update_Forbidden(t *testing.T) {
	alertRuleRepo := new(mockAlertRuleRepo)
	projectRepo := new(mockProjectRepo)

	existing := &domain.AlertRule{ID: "rule-1", ProjectID: "proj-1"}
	alertRuleRepo.On("GetByID", mock.Anything, "rule-1").Return(existing, nil)
	projectRepo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{ID: "proj-1", UserID: "user-OTHER"}, nil)

	uc := usecase.NewAlertRuleUsecase(alertRuleRepo, projectRepo)
	_, err := uc.Update(context.Background(), usecase.UpdateAlertRuleInput{UserID: "user-1", AlertRuleID: "rule-1"})

	assert.ErrorIs(t, err, usecase.ErrForbidden)
	alertRuleRepo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
}

func TestAlertRuleUsecase_Delete_Success(t *testing.T) {
	alertRuleRepo := new(mockAlertRuleRepo)
	projectRepo := new(mockProjectRepo)

	existing := &domain.AlertRule{ID: "rule-1", ProjectID: "proj-1"}
	alertRuleRepo.On("GetByID", mock.Anything, "rule-1").Return(existing, nil)
	projectRepo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{ID: "proj-1", UserID: "user-1"}, nil)
	alertRuleRepo.On("Delete", mock.Anything, "rule-1").Return(nil)

	uc := usecase.NewAlertRuleUsecase(alertRuleRepo, projectRepo)
	err := uc.Delete(context.Background(), usecase.DeleteAlertRuleInput{UserID: "user-1", AlertRuleID: "rule-1"})

	assert.NoError(t, err)
	alertRuleRepo.AssertCalled(t, "Delete", mock.Anything, "rule-1")
}

func TestAlertRuleUsecase_Delete_Forbidden_DoesNotDelete(t *testing.T) {
	alertRuleRepo := new(mockAlertRuleRepo)
	projectRepo := new(mockProjectRepo)

	existing := &domain.AlertRule{ID: "rule-1", ProjectID: "proj-1"}
	alertRuleRepo.On("GetByID", mock.Anything, "rule-1").Return(existing, nil)
	projectRepo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{ID: "proj-1", UserID: "user-OTHER"}, nil)

	uc := usecase.NewAlertRuleUsecase(alertRuleRepo, projectRepo)
	err := uc.Delete(context.Background(), usecase.DeleteAlertRuleInput{UserID: "user-1", AlertRuleID: "rule-1"})

	assert.ErrorIs(t, err, usecase.ErrForbidden)
	alertRuleRepo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
}