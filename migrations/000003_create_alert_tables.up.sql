CREATE TABLE alert_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID REFERENCES projects(id) ON DELETE CASCADE,
    condition_type VARCHAR(20) NOT NULL, -- 'new_issue' | 'threshold'
    threshold INT DEFAULT 1,
    window_minutes INT DEFAULT 60, -- dipakai HANYA untuk condition_type='threshold' (durasi hitung count events)
    cooldown_minutes INT DEFAULT 60, -- dipakai SEMUA condition_type (durasi idempotency/cooldown)
    channel VARCHAR(20) NOT NULL, -- 'email' | 'slack'
    channel_target TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE alert_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    alert_rule_id UUID REFERENCES alert_rules(id) ON DELETE CASCADE,
    issue_id UUID,
    sent_at TIMESTAMPTZ DEFAULT now()
);
-- Dipakai AlertLogRepository.GetLastSentAt (cooldown check per rule+issue,
-- Sprint 6): "ORDER BY sent_at DESC LIMIT 1 WHERE alert_rule_id=? AND
-- issue_id=?" — tanpa index ini bakal sequential scan begitu alert_logs
-- mulai besar.
CREATE INDEX idx_alert_logs_rule_issue ON alert_logs(alert_rule_id, issue_id, sent_at DESC);