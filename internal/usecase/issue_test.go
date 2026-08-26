package usecase_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
	"github.com/MohdFarhanS/sentinelix-backend/internal/usecase"
)

func TestIssueUsecase_GetByID_Success(t *testing.T) {
	issueRepo := new(mockIssueRepo)
	issueRepo.On("GetByID", mock.Anything, "issue-1").Return(&domain.Issue{
		ID: "issue-1", ProjectID: "proj-1", Title: "TypeError",
	}, nil)

	projectRepo := new(mockProjectRepo)
	projectRepo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{
		ID: "proj-1", UserID: "user-1",
	}, nil)

	uc := usecase.NewIssueUsecase(issueRepo, projectRepo, nil)
	issue, err := uc.GetByID(context.Background(), "user-1", "issue-1")

	assert.NoError(t, err)
	assert.Equal(t, "TypeError", issue.Title)
}

func TestIssueUsecase_GetByID_NotFound(t *testing.T) {
	issueRepo := new(mockIssueRepo)
	issueRepo.On("GetByID", mock.Anything, "issue-x").Return(nil, domain.ErrIssueNotFound)

	projectRepo := new(mockProjectRepo) // GetByID tidak akan dipanggil — issue sudah gagal duluan

	uc := usecase.NewIssueUsecase(issueRepo, projectRepo, nil)
	_, err := uc.GetByID(context.Background(), "user-1", "issue-x")

	assert.ErrorIs(t, err, domain.ErrIssueNotFound)
	projectRepo.AssertNotCalled(t, "GetByID", mock.Anything, mock.Anything)
}

func TestIssueUsecase_ListEvents_Success(t *testing.T) {
	issueRepo := new(mockIssueRepo)
	issueRepo.On("GetByID", mock.Anything, "issue-1").Return(&domain.Issue{
		ID: "issue-1", ProjectID: "proj-1",
	}, nil)

	projectRepo := new(mockProjectRepo)
	projectRepo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{
		ID: "proj-1", UserID: "user-1",
	}, nil)

	eventRepo := new(mockEventRepo)
	eventRepo.On("ListByIssueID", mock.Anything, "issue-1", 50).Return([]*domain.Event{
		{ID: "event-1", IssueID: "issue-1"},
	}, nil)

	uc := usecase.NewIssueUsecase(issueRepo, projectRepo, eventRepo)
	events, err := uc.ListEvents(context.Background(), usecase.ListEventsInput{
		UserID: "user-1", IssueID: "issue-1", Limit: 50,
	})

	assert.NoError(t, err)
	assert.Len(t, events, 1)
}

func TestIssueUsecase_ListEvents_Forbidden(t *testing.T) {
	issueRepo := new(mockIssueRepo)
	issueRepo.On("GetByID", mock.Anything, "issue-1").Return(&domain.Issue{
		ID: "issue-1", ProjectID: "proj-1",
	}, nil)

	projectRepo := new(mockProjectRepo)
	projectRepo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{
		ID: "proj-1", UserID: "user-OTHER",
	}, nil)

	eventRepo := new(mockEventRepo) // tidak di-.On() sama sekali — kalau kepanggil, testify panic (assertion ketat)

	uc := usecase.NewIssueUsecase(issueRepo, projectRepo, eventRepo)
	_, err := uc.ListEvents(context.Background(), usecase.ListEventsInput{
		UserID: "user-1", IssueID: "issue-1", Limit: 50,
	})

	assert.ErrorIs(t, err, usecase.ErrForbidden)
	eventRepo.AssertNotCalled(t, "ListByIssueID", mock.Anything, mock.Anything, mock.Anything)
}

func TestIssueUsecase_GetByID_Forbidden(t *testing.T) {
	issueRepo := new(mockIssueRepo)
	issueRepo.On("GetByID", mock.Anything, "issue-1").Return(&domain.Issue{
		ID: "issue-1", ProjectID: "proj-1",
	}, nil)

	projectRepo := new(mockProjectRepo)
	projectRepo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{
		ID: "proj-1", UserID: "user-OTHER",
	}, nil)

	uc := usecase.NewIssueUsecase(issueRepo, projectRepo, nil)
	_, err := uc.GetByID(context.Background(), "user-1", "issue-1")

	assert.ErrorIs(t, err, usecase.ErrForbidden)
}

func TestIssueUsecase_List_ProjectNotFound(t *testing.T) {
	issueRepo := new(mockIssueRepo) // List tidak di-.On() — tidak akan dipanggil, ownership gagal duluan
	projectRepo := new(mockProjectRepo)
	projectRepo.On("GetByID", mock.Anything, "proj-x").Return(nil, domain.ErrProjectNotFound)

	uc := usecase.NewIssueUsecase(issueRepo, projectRepo, nil)
	_, err := uc.List(context.Background(), usecase.ListIssuesInput{
		UserID: "user-1", ProjectID: "proj-x", Page: 1, Limit: 20,
	})

	assert.ErrorIs(t, err, domain.ErrProjectNotFound)
	issueRepo.AssertNotCalled(t, "List", mock.Anything, mock.Anything)
}

func TestIssueUsecase_List_Forbidden(t *testing.T) {
	issueRepo := new(mockIssueRepo)
	projectRepo := new(mockProjectRepo)
	projectRepo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{
		ID: "proj-1", UserID: "user-OTHER",
	}, nil)

	uc := usecase.NewIssueUsecase(issueRepo, projectRepo, nil)
	_, err := uc.List(context.Background(), usecase.ListIssuesInput{
		UserID: "user-1", ProjectID: "proj-1", Page: 1, Limit: 20,
	})

	assert.ErrorIs(t, err, usecase.ErrForbidden)
	issueRepo.AssertNotCalled(t, "List", mock.Anything, mock.Anything)
}
