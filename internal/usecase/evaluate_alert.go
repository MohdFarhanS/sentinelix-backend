package usecase

import (
	"context"
	"time"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
)

// Notifier didefinisikan di sini (bukan di domain) karena ini kontrak
// antara usecase dan infrastructure notifikasi (Resend/Slack) — usecase
// cuma butuh tahu "kirim pesan ini", tidak peduli implementasinya HTTP
// API Resend atau Slack webhook. Pola ini sama seperti Repository
// interface, tapi untuk side-effect keluar (bukan penyimpanan data).
type Notifier interface {
	Notify(ctx context.Context, rule *domain.AlertRule, issue *domain.Issue) error
}

type EvaluateAlertUsecase struct {
	alertRuleRepo 	domain.AlertRuleRepository
	alertLogRepo 	domain.AlertLogRepository
	issueRepo 		domain.IssueRepository
	eventRepo 		domain.EventRepository
	notifier 		Notifier
}

func NewEvaluateAlertUsecase(
	alertRuleRepo 	domain.AlertRuleRepository,
	alertLogRepo 	domain.AlertLogRepository,
	issueRepo 		domain.IssueRepository,
	eventRepo 		domain.EventRepository,
	notifier 		Notifier,
) *EvaluateAlertUsecase {
	return &EvaluateAlertUsecase{
		alertRuleRepo: 	alertRuleRepo,
		alertLogRepo: 	alertLogRepo,
		issueRepo: 		issueRepo,
		eventRepo: 		eventRepo,
		notifier: 		notifier,
	}
}

// EvaluateNewIssue dipanggil event-driven dari ingest_consumer.go, TEPAT
// setelah GroupIssueUsecase.Execute mengembalikan wasCreated=true. issue
// yang dioper sudah pasti baru (issue itu sendiri jadi bukti kondisi
// "new_issue" terpenuhi) — tidak ada threshold check di sini.
func (uc *EvaluateAlertUsecase) EvaluateNewIssue(ctx context.Context, issue *domain.Issue) error {
	rules, err := uc.alertRuleRepo.ListActiveNewIssueRules(ctx, issue.ProjectID)
	if err != nil {
		return err
	}

	for _, rule := range rules {
		if err := uc.notifyIfNotCooldown(ctx, rule, issue); err != nil {
			// Sengaja tidak return langsung — 1 rule gagal kirim
			// (misal Resend down) tidak boleh menggagalkan rule lain
			// dalam project yang sama. Error di-collect via logging
			// di caller (worker), bukan di sini (usecase tidak boleh
			// import logger langsung, lihat 05-ARCHITECTURE.md aturan
			// dependency).
			continue
		}
	}
	return nil
}

// EvaluateThresholds dipanggil periodic ticker dari worker. Cakupannya
// LINTAS SEMUA PROJECT sekaligus (beda dari EvaluateNewIssue yang scoped
// ke 1 project) — makanya query count di-group per project supaya tidak
// N+1 antar rule yang kebetulan project_id-nya sama.
func (uc *EvaluateAlertUsecase) EvaluateThresholds(ctx context.Context) error {
	rules, err := uc.alertRuleRepo.ListActiveThresholdRules(ctx)
	if err != nil {
		return err
	}

	// Cache count per (projectID, windowMinutes) supaya rule lain dengan
	// window_minutes SAMA di project SAMA tidak query ulang. Kalau
	// window_minutes beda, count-nya juga beda (window waktu beda),
	// makanya key cache ikut window_minutes.
	type cacheKey struct {
		projectID		string
		windowMinutes	int
	}
	countCache := make(map[cacheKey]map[string]int)

	for _, rule := range rules {
		key := cacheKey{projectID: rule.ProjectID, windowMinutes: rule.WindowMinutes}
		counts, ok := countCache[key]
		if !ok {
			since := time.Now().Add(-time.Duration(rule.WindowMinutes) * time.Minute)
			counts, err = uc.eventRepo.CountGroupedByIssueSince(ctx, rule.ProjectID, since)
			if err != nil {
				continue
			}
			countCache[key] = counts
		}
		
		for issueID, count := range counts {
			if count <= rule.Threshold {
				continue
			}

			issue, err := uc.issueRepo.GetByID(ctx, issueID)
			if err != nil {
				continue
			}

			if err := uc.notifyIfNotCooldown(ctx, rule, issue); err != nil {
				continue
			}
		}
	}
	return nil
}

// notifyIfNotCooldown adalah inti logic cooldown (Sprint 6 keputusan:
// alert_logs sebagai source of truth, granularitas per rule+issue).
// Urutan: cek cooldown -> kirim notifikasi -> BARU insert alert_logs.
// Insert HARUS setelah kirim sukses, bukan sebelum — kalau insert
// duluan lalu kirim gagal, rule ini kena cooldown palsu padahal user
// tidak pernah benar-benar dapat notifikasi.
func (uc *EvaluateAlertUsecase) notifyIfNotCooldown(ctx context.Context, rule *domain.AlertRule, issue *domain.Issue) error {
	lastSent, err := uc.alertLogRepo.GetLastSentAt(ctx, rule.ID, issue.ID)
	if err != nil {
		return err
	}
	if lastSent != nil {
		cooldownUntil := lastSent.Add(time.Duration(rule.CooldownMinutes) * time.Minute)
		if time.Now().Before(cooldownUntil) {
			return nil
		}
	}

	if err := uc.notifier.Notify(ctx, rule, issue); err != nil {
		return err
	}

	return uc.alertLogRepo.Create(ctx, &domain.AlertLog{
		AlertRuleID: 	rule.ID,
		IssueID: 		issue.ID,
	})
}