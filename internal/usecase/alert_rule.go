package usecase

import (
	"context"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
)

type AlertRuleUsecase struct {
	alertRuleRepo 	domain.AlertRuleRepository
	projectRepo 	domain.ProjectRepository
}

func NewAlertRuleUsecase(alertRuleRepo domain.AlertRuleRepository, projectRepo domain.ProjectRepository) *AlertRuleUsecase {
	return &AlertRuleUsecase{alertRuleRepo: alertRuleRepo, projectRepo: projectRepo}
}

type CreateAlertRuleInput struct {
	UserID			string
	ProjectID		string
	ConditionType	string
	Threshold		int
	WindowMinutes	int
	CooldownMinutes	int
	Channel			string
	ChannelTarget	string
}

// Create validasi ownership project dulu (pola sama seperti
// IssueUsecase.List), baru validasi field-level lewat AlertRule.Validate()
// sebelum insert. Urutan ini penting: user yang bukan pemilik project
// tidak boleh tahu apakah payload-nya valid atau tidak (403 duluan,
// baru 400 kalau memang miliknya).
func (uc *AlertRuleUsecase) Create(ctx context.Context, in CreateAlertRuleInput) (*domain.AlertRule, error) {
	project, err := uc.projectRepo.GetByID(ctx, in.ProjectID)
	if err != nil {
		return nil, err
	}
	if project.UserID != in.UserID {
		return nil, ErrForbidden
	}

	rule := &domain.AlertRule{
		ProjectID:		in.ProjectID,
		ConditionType: 	in.ConditionType,
		Threshold: 		in.Threshold,
		WindowMinutes: 	in.WindowMinutes,
		CooldownMinutes: in.CooldownMinutes,
		Channel: 		in.Channel,
		ChannelTarget: 	in.ChannelTarget,
	}
	if err := rule.Validate(); err != nil {
		return nil, err
	}

	if err := uc.alertRuleRepo.Create(ctx, rule); err != nil {
		return nil, err
	}
	return rule, nil
}

type ListAlertRulesInput struct {
	UserID		string
	ProjectID	string
}

func (uc *AlertRuleUsecase) List(ctx context.Context, in ListAlertRulesInput) ([]*domain.AlertRule, error) {
project, err := uc.projectRepo.GetByID(ctx, in.ProjectID)
if err != nil {
	return nil, err
}
if project.UserID != in.UserID {
	return nil, ErrForbidden
}

return uc.alertRuleRepo.ListByProjectID(ctx, in.ProjectID)
}

// getOwnedAlertRule ambil alert rule by ID lalu verifikasi project-nya
// milik user yang login — pola sama persis seperti
// IssueUsecase.getOwnedIssue (alert rule itu anak dari project, cek
// ownership selalu lewat rule -> project -> user).
func (uc *AlertRuleUsecase) getOwnedAlertRule(ctx context.Context, userID, ruleID string) (*domain.AlertRule, error) {
	rule, err := uc.alertRuleRepo.GetByID(ctx, ruleID)
	if err != nil {
		return nil, err
	}

	project, err := uc.projectRepo.GetByID(ctx, rule.ProjectID)
	if err != nil {
		return nil, err
	}
	if project.UserID != userID {
		return nil, ErrForbidden
	}

	return rule, nil
}

// UpdateAlertRuleInput pakai pointer field biar bisa bedain "field tidak
// dikirim" (nil, tidak diubah) vs "field dikirim nilai zero-value"
// (misal threshold: 0 secara eksplisit) — semantik PATCH partial update
// sesuai 04-API-DESIGN.md.
type UpdateAlertRuleInput struct {
	UserID			string
	AlertRuleID		string
	ConditionType	*string
	Threshold		*int
	WindowMinutes	*int
	CooldownMinutes	*int
	Channel			*string
	ChannelTarget	*string
}

// Update ambil rule existing lewat getOwnedAlertRule (sekaligus ownership
// check), merge field yang dikirim, lalu validasi ulang SEBELUM ditulis
// ke DB — Validate() dipanggil terhadap state FINAL (bukan cuma field
// yang berubah), jadi kalau misalnya user ubah condition_type ke
// "threshold" tapi tidak kirim window_minutes baru, validasi tetap benar
// menolak kalau window_minutes lama-nya 0.
func (uc *AlertRuleUsecase) Update(ctx context.Context, in UpdateAlertRuleInput) (*domain.AlertRule, error) {
	rule, err := uc.getOwnedAlertRule(ctx, in.UserID, in.AlertRuleID)
	if err != nil {
		return nil, err
	}

	if in.ConditionType != nil {
		rule.ConditionType = *in.ConditionType
	}
	if in.Threshold != nil {
		rule.Threshold = *in.Threshold
	}
	if in.WindowMinutes != nil {
		rule.WindowMinutes = *in.WindowMinutes
	}
	if in.CooldownMinutes != nil {
		rule.CooldownMinutes = *in.CooldownMinutes
	}
	if in.Channel != nil {
		rule.Channel = *in.Channel
	}
	if in.ChannelTarget != nil {
		rule.ChannelTarget = *in.ChannelTarget
	}

	if err := rule.Validate(); err != nil {
		return nil, err
	}

	if err := uc.alertRuleRepo.Update(ctx, rule); err != nil {
		return nil, err
	}
	return rule, nil
}

type DeleteAlertRuleInput struct {
	UserID		string
	AlertRuleID	string
}

func (uc *AlertRuleUsecase) Delete(ctx context.Context, in DeleteAlertRuleInput) error {
	if _, err := uc.getOwnedAlertRule(ctx, in.UserID, in.AlertRuleID); err != nil {
		return err
	}
	return uc.alertRuleRepo.Delete(ctx, in.AlertRuleID)
}