package usecase

import (
	"context"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
	"github.com/MohdFarhanS/sentinelix-backend/pkg/fingerprint"
)

// GroupIssueUsecase mengimplementasikan langkah 4-6 di "Alur Ingestion"
// (05-ARCHITECTURE.md): hitung fingerprint dari event yang masuk, lalu
// upsert ke issue (baru atau tambah count), lalu simpan event mentahnya.
type GroupIssueUsecase struct {
	issueRepo domain.IssueRepository
	eventRepo domain.EventRepository
}

func NewGroupIssueUsecase(issueRepo domain.IssueRepository, eventRepo domain.EventRepository) * GroupIssueUsecase {
	return &GroupIssueUsecase{issueRepo: issueRepo, eventRepo: eventRepo}
}

// Execute menerima satu event (hasil consume dari Redis Stream), lalu:
//  1. Hitung fingerprint dari message + stack_trace.
//  2. Upsert issue (insert baru kalau fingerprint belum ada, atau
//     tambah count + update last_seen kalau sudah ada).
//  3. Simpan event mentah, terhubung ke issue.ID hasil langkah 2.
//
// event.Title issue diambil dari event.Message — sesuai contoh response
// di 04-API-DESIGN.md (§5 GET /issues/:id, "title": "TypeError: ...").
func (uc *GroupIssueUsecase) Execute(ctx context.Context, event *domain.Event) (*domain.Issue, bool, error) {
	fp := fingerprint.Compute(event.Message, event.StackTrace)

	issue, wasCreated, err := uc.issueRepo.Upsert(ctx, event.ProjectID, fp, event.Message, event.Level, event.OccurredAt)
	if err != nil {
		return nil, false, err
	}

	event.IssueID = issue.ID
	if err := uc.eventRepo.Insert(ctx, event); err != nil {
		return nil, false, err
	}

	return issue, wasCreated, nil
}