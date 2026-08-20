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

// --- mock repositories (testify style — Sprint 6 standarisasi, lihat
// diskusi opsi B: mock BARU dan REFACTOR mock lama semua pakai testify) ---

type mockIssueRepo struct {
	mock.Mock
}

func (m *mockIssueRepo) Upsert(ctx context.Context, projectID, fingerprint, title, level string, occurredAt time.Time) (*domain.Issue, bool, error) {
	args := m.Called(ctx, projectID, fingerprint, title, level, occurredAt)
	if args.Get(0) == nil {
		return nil, args.Bool(1), args.Error(2)
	}
	return args.Get(0).(*domain.Issue), args.Bool(1), args.Error(2)
}

func (m *mockIssueRepo) List(ctx context.Context, filter domain.IssueFilter) (*domain.IssueListResult, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.IssueListResult), args.Error(1)
}

func (m *mockIssueRepo) GetByID(ctx context.Context, id string) (*domain.Issue, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Issue), args.Error(1)
}

type mockEventRepo struct {
	mock.Mock
}

func (m *mockEventRepo) Insert(ctx context.Context, event *domain.Event) error {
	args := m.Called(ctx, event)
	return args.Error(0)
}

func (m *mockEventRepo) ListByIssueID(ctx context.Context, issueID string, limit int) ([]*domain.Event, error) {
	args := m.Called(ctx, issueID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.Event), args.Error(1)
}

func (m *mockEventRepo) CountGroupedByIssueSince(ctx context.Context, projectID string, since time.Time) (map[string]int, error) {
	args := m.Called(ctx, projectID, since)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]int), args.Error(1)
}

// --- tests ---

func TestGroupIssue_NewFingerprint_CreatesIssue(t *testing.T) {
	issueRepo := new(mockIssueRepo)
	eventRepo := new(mockEventRepo)

	issueRepo.On("Upsert", mock.Anything, "project-1", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&domain.Issue{ID: "issue-1", ProjectID: "project-1", Count: 1}, true, nil)
	eventRepo.On("Insert", mock.Anything, mock.AnythingOfType("*domain.Event")).Return(nil)

	uc := usecase.NewGroupIssueUsecase(issueRepo, eventRepo)
	event := &domain.Event{
		ProjectID:  "project-1",
		Message:    "TypeError: Cannot read property 'id' of undefined",
		StackTrace: "at checkout.go:88",
		OccurredAt: time.Now(),
	}

	issue, wasCreated, err := uc.Execute(context.Background(), event)

	assert.NoError(t, err)
	assert.True(t, wasCreated, "expected wasCreated=true for new fingerprints")
	assert.Equal(t, "issue-1", issue.ID)
}

func TestGroupIssue_ExistingFingerprint_UpdatesCount(t *testing.T) {
	issueRepo := new(mockIssueRepo)
	eventRepo := new(mockEventRepo)

	issueRepo.On("Upsert", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&domain.Issue{ID: "issue-1", Count: 5}, false, nil)
	eventRepo.On("Insert", mock.Anything, mock.AnythingOfType("*domain.Event")).Return(nil)

	uc := usecase.NewGroupIssueUsecase(issueRepo, eventRepo)
	event := &domain.Event{ProjectID: "project-1", Message: "same error", StackTrace: "at foo.go:1", OccurredAt: time.Now()}

	issue, wasCreated, err := uc.Execute(context.Background(), event)

	assert.NoError(t, err)
	assert.False(t, wasCreated, "expected wasCreated=false for existing fingerprints")
	assert.Equal(t, 5, issue.Count)
}

func TestGroupIssue_EventGetsLinkedToIssueID(t *testing.T) {
	issueRepo := new(mockIssueRepo)
	eventRepo := new(mockEventRepo)

	issueRepo.On("Upsert", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&domain.Issue{ID: "issue-xyz"}, true, nil)
	eventRepo.On("Insert", mock.Anything, mock.AnythingOfType("*domain.Event")).Return(nil)

	uc := usecase.NewGroupIssueUsecase(issueRepo, eventRepo)
	event := &domain.Event{ProjectID: "project-1", Message: "msg", StackTrace: "at foo.go:1", OccurredAt: time.Now()}

	_, _, err := uc.Execute(context.Background(), event)
	assert.NoError(t, err)

	// AssertCalled + cek argumen persis — pengganti pola `eventRepo.inserted`
	// manual di versi func-field sebelumnya. testify simpan history call,
	// jadi kita bisa inspect argumen event yang benar-benar dikirim.
	eventRepo.AssertCalled(t, "Insert", mock.Anything, mock.MatchedBy(func(e *domain.Event) bool {
		return e.IssueID == "issue-xyz"
	}))
}

func TestGroupIssue_UpsertFails_ReturnsError(t *testing.T) {
	issueRepo := new(mockIssueRepo)
	eventRepo := new(mockEventRepo)

	issueRepo.On("Upsert", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, false, errors.New("db connection error"))

	uc := usecase.NewGroupIssueUsecase(issueRepo, eventRepo)
	event := &domain.Event{ProjectID: "project-1", Message: "msg", StackTrace: "at foo.go:1", OccurredAt: time.Now()}

	_, _, err := uc.Execute(context.Background(), event)

	assert.Error(t, err, "expected error from the Upsert is passed to the caller")
	eventRepo.AssertNotCalled(t, "Insert", mock.Anything, mock.Anything)
}

func TestGroupIssue_EventInsertFails_ReturnsError(t *testing.T) {
	issueRepo := new(mockIssueRepo)
	eventRepo := new(mockEventRepo)

	issueRepo.On("Upsert", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&domain.Issue{ID: "issue-1"}, true, nil)
	eventRepo.On("Insert", mock.Anything, mock.AnythingOfType("*domain.Event")).Return(errors.New("insert failed"))

	uc := usecase.NewGroupIssueUsecase(issueRepo, eventRepo)
	event := &domain.Event{ProjectID: "project-1", Message: "msg", StackTrace: "at foo.go:1", OccurredAt: time.Now()}

	_, _, err := uc.Execute(context.Background(), event)

	assert.Error(t, err, "expected error from eventRepo.Insert is passed to the caller")
}