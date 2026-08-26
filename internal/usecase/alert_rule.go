package usecase

import (
	"context"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
	"github.com/rs/zerolog"
)

type AlertRuleUsecase struct {
	alertRuleRepo domain.AlertRuleRepository
	projectRepo   domain.ProjectRepository
	auditLogRepo  domain.AuditLogRepository
	logger        zerolog.Logger
}

func NewAlertRuleUsecase(
	alertRuleRepo domain.AlertRuleRepository,
	projectRepo domain.ProjectRepository,
	auditLogRepo domain.AuditLogRepository,
	logger zerolog.Logger,
) *AlertRuleUsecase {
	return &AlertRuleUsecase{
		alertRuleRepo: alertRuleRepo,
		projectRepo:   projectRepo,
		auditLogRepo:  auditLogRepo,
		logger:        logger,
	}
}

// writeAuditLog — best-effort, dipusatkan di satu method supaya Create/
// Update/Delete tidak duplikasi pola try-log-swallow yang sama 3x. Error
// SENGAJA tidak di-return ke caller (lihat 06-ROADMAP.md §6 diskusi Sprint
// 9) — audit write gagal tidak boleh menggagalkan operasi utama yang sudah
// sukses. Structured fields dipilih (bukan string interpolation) supaya
// kegagalan yang recurring per action gampang di-filter kalau dicek manual.
func (uc *AlertRuleUsecase) writeAuditLog(ctx context.Context, actorUserID, action, resourceID string, metadata map[string]any) {
	if err := uc.auditLogRepo.Create(ctx, &domain.AuditLog{
		ActorUserID:  &actorUserID,
		Action:       action,
		ResourceType: domain.ResourceTypeAlertRule,
		ResourceID:   resourceID,
		Metadata:     metadata,
	}); err != nil {
		uc.logger.Error().
			Err(err).
			Str("action", action).
			Str("resource_id", resourceID).
			Str("actor_user_id", actorUserID).
			Msg("failed to write audit log")
	}
}

type CreateAlertRuleInput struct {
	UserID          string
	ProjectID       string
	ConditionType   string
	Threshold       int
	WindowMinutes   int
	CooldownMinutes int
	Channel         string
	ChannelTarget   string
}

func (uc *AlertRuleUsecase) Create(ctx context.Context, in CreateAlertRuleInput) (*domain.AlertRule, error) {
	project, err := uc.projectRepo.GetByID(ctx, in.ProjectID)
	if err != nil {
		return nil, err
	}
	if project.UserID != in.UserID {
		return nil, ErrForbidden
	}

	rule := &domain.AlertRule{
		ProjectID:       in.ProjectID,
		ConditionType:   in.ConditionType,
		Threshold:       in.Threshold,
		WindowMinutes:   in.WindowMinutes,
		CooldownMinutes: in.CooldownMinutes,
		Channel:         in.Channel,
		ChannelTarget:   in.ChannelTarget,
	}
	if err := rule.Validate(); err != nil {
		return nil, err
	}

	if err := uc.alertRuleRepo.Create(ctx, rule); err != nil {
		return nil, err
	}

	uc.writeAuditLog(ctx, in.UserID, domain.ActionAlertRuleCreated, rule.ID, map[string]any{
		"condition_type": rule.ConditionType,
		"threshold":      rule.Threshold,
		"channel":        rule.Channel,
	})

	return rule, nil
}

type ListAlertRulesInput struct {
	UserID    string
	ProjectID string
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

type UpdateAlertRuleInput struct {
	UserID          string
	AlertRuleID     string
	ConditionType   *string
	Threshold       *int
	WindowMinutes   *int
	CooldownMinutes *int
	Channel         *string
	ChannelTarget   *string
}

func (uc *AlertRuleUsecase) Update(ctx context.Context, in UpdateAlertRuleInput) (*domain.AlertRule, error) {
	rule, err := uc.getOwnedAlertRule(ctx, in.UserID, in.AlertRuleID)
	if err != nil {
		return nil, err
	}

	// changedFields dikumpulin buat metadata audit log — cuma field yang
	// BENERAN dikirim di request (bukan seluruh state rule setelah merge),
	// supaya audit trail nunjukin "apa yang diubah", bukan "snapshot penuh
	// tiap kali update" yang kurang informatif buat histori.
	changedFields := map[string]any{}

	if in.ConditionType != nil {
		rule.ConditionType = *in.ConditionType
		changedFields["condition_type"] = *in.ConditionType
	}
	if in.Threshold != nil {
		rule.Threshold = *in.Threshold
		changedFields["threshold"] = *in.Threshold
	}
	if in.WindowMinutes != nil {
		rule.WindowMinutes = *in.WindowMinutes
		changedFields["window_minutes"] = *in.WindowMinutes
	}
	if in.CooldownMinutes != nil {
		rule.CooldownMinutes = *in.CooldownMinutes
		changedFields["cooldown_minutes"] = *in.CooldownMinutes
	}
	if in.Channel != nil {
		rule.Channel = *in.Channel
		changedFields["channel"] = *in.Channel
	}
	if in.ChannelTarget != nil {
		rule.ChannelTarget = *in.ChannelTarget
		changedFields["channel_target"] = *in.ChannelTarget
	}

	if err := rule.Validate(); err != nil {
		return nil, err
	}

	if err := uc.alertRuleRepo.Update(ctx, rule); err != nil {
		return nil, err
	}

	uc.writeAuditLog(ctx, in.UserID, domain.ActionAlertRuleUpdated, rule.ID, changedFields)

	return rule, nil
}

type DeleteAlertRuleInput struct {
	UserID      string
	AlertRuleID string
}

func (uc *AlertRuleUsecase) Delete(ctx context.Context, in DeleteAlertRuleInput) error {
	rule, err := uc.getOwnedAlertRule(ctx, in.UserID, in.AlertRuleID)
	if err != nil {
		return err
	}

	if err := uc.alertRuleRepo.Delete(ctx, in.AlertRuleID); err != nil {
		return err
	}

	// Metadata nyimpen snapshot terakhir SEBELUM dihapus — beda dari
	// Create/Update yang metadata-nya "apa yang terjadi", di sini
	// penting justru "apa yang HILANG", karena setelah ini resource-nya
	// sudah tidak ada buat dicek lagi.
	uc.writeAuditLog(ctx, in.UserID, domain.ActionAlertRuleDeleted, rule.ID, map[string]any{
		"condition_type": rule.ConditionType,
		"threshold":      rule.Threshold,
		"channel":        rule.Channel,
	})

	return nil
}
