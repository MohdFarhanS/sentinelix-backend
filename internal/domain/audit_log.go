package domain

import (
	"context"
	"time"
)

// Action — daftar tertutup, single source of truth. Usecase HARUS pakai
// constant ini, bukan string literal — cegah typo yang lolos compile
// tapi rusak di runtime.
const (
	ActionAlertRuleCreated			= "alert_rule.created"
	ActionAlertRuleUpdated			= "alert_rule.updated"
	ActionAlertRuleDeleted			= "alert_rule.deleted"
	ActionProjectAPIKeyCreated		= "project.api_key_created"
	ActionProjectDeleted       		= "project.deleted"
)

// ResourceType — sama alasannya seperti Action di atas.
const (
	ResourceTypeAlertRule	= "alert-rule"
	ResourceTypeProject		= "project"
)

type AuditLog struct {
	ID				string
	ActorUserID		*string
	Action			string
	ResourceType	string
	ResourceID		string
	Metadata		map[string]any
	CreatedAt		time.Time
}

type AuditLogRepository interface {
	Create(ctx context.Context, log *AuditLog) error
}