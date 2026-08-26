package usecase

import (
	"context"
	"errors"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
)

// ErrForbidden dipakai kalau project exist tapi bukan milik user yang login.
// Beda dari domain.ErrProjectNotFound (project memang tidak ada) — biar
// handler bisa balikin 403 vs 404 secara akurat.
var ErrForbidden = errors.New("access forbidden: This project does not belong to this user")

type IssueUsecase struct {
	issueRepo   domain.IssueRepository
	projectRepo domain.ProjectRepository
	eventRepo   domain.EventRepository
}

func NewIssueUsecase(issueRepo domain.IssueRepository, projectRepo domain.ProjectRepository, eventRepo domain.EventRepository) *IssueUsecase {
	return &IssueUsecase{issueRepo: issueRepo, projectRepo: projectRepo, eventRepo: eventRepo}
}

type ListIssuesInput struct {
	UserID    string
	ProjectID string
	Status    string
	Page      int
	Limit     int
}

func (uc *IssueUsecase) List(ctx context.Context, in ListIssuesInput) (*domain.IssueListResult, error) {
	project, err := uc.projectRepo.GetByID(ctx, in.ProjectID)
	if err != nil {
		return nil, err // termasuk domain.ErrProjectNotFound
	}
	if project.UserID != in.UserID {
		return nil, ErrForbidden
	}

	return uc.issueRepo.List(ctx, domain.IssueFilter{
		ProjectID: in.ProjectID,
		Status:    in.Status,
		Page:      in.Page,
		Limit:     in.Limit,
	})
}

// getOwnedIssue ambil issue by ID lalu verifikasi project-nya milik user
// yang login. Dipakai bareng oleh GetByI & ListEvents biar ownership-check
// logic tidak duplikat (issue itu anak dari project, event itu anak dari
// issue, jadi cek ownership selalu lewat issue -> project -> user).
func (uc *IssueUsecase) getOwnedIssue(ctx context.Context, userID, issueID string) (*domain.Issue, error) {
	issue, err := uc.issueRepo.GetByID(ctx, issueID)
	if err != nil {
		return nil, err // termasuk domain.ErrIssueNotFound
	}

	project, err := uc.projectRepo.GetByID(ctx, issue.ProjectID)
	if err != nil {
		return nil, err
	}
	if project.UserID != userID {
		return nil, ErrForbidden
	}

	return issue, nil
}

func (uc *IssueUsecase) GetByID(ctx context.Context, userID, IssueID string) (*domain.Issue, error) {
	return uc.getOwnedIssue(ctx, userID, IssueID)
}

type ListEventsInput struct {
	UserID  string
	IssueID string
	Limit   int
}

func (uc *IssueUsecase) ListEvents(ctx context.Context, in ListEventsInput) ([]*domain.Event, error) {
	if _, err := uc.getOwnedIssue(ctx, in.UserID, in.IssueID); err != nil {
		return nil, err
	}

	limit := in.Limit
	if limit <= 0 || limit > 50 {
		limit = 50
	}

	return uc.eventRepo.ListByIssueID(ctx, in.IssueID, limit)
}
