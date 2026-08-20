CREATE TABLE monitors (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID REFERENCES projects(id) ON DELETE CASCADE,
    url TEXT NOT NULL,
    interval_sec INT DEFAULT 60,
    channel VARCHAR(20) NOT NULL,
    channel_target TEXT NOT NULL,
    failure_threshold INT DEFAULT 3,
    status VARCHAR(20) DEFAULT 'unknown',
    created_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE monitor_checks (
    id UUID DEFAULT gen_random_uuid(),
    monitor_id UUID NOT NULL,
    status_code INT,
    latency_ms INT,
    is_up BOOLEAN,
    checked_at TIMESTAMPTZ NOT NULL DEFAULT now()
) PARTITION BY RANGE (checked_at);
CREATE INDEX idx_monitor_checks_monitor ON monitor_checks(monitor_id, checked_at DESC);

-- Partisi bulan berjalan + bulan depan, plus partisi DEFAULT sebagai
-- catch-all supaya INSERT tidak pernah gagal gara-gara "no partition
-- found" — pola sama persis dengan events (000002_issues_events.up.sql).
CREATE TABLE monitor_checks_2026_08 PARTITION OF monitor_checks
    FOR VALUES FROM ('2026-08-01') TO ('2026-09-01');
CREATE TABLE monitor_checks_2026_09 PARTITION OF monitor_checks
    FOR VALUES FROM ('2026-09-01') TO ('2026-10-01');
CREATE TABLE monitor_checks_default PARTITION OF monitor_checks DEFAULT;