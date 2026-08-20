CREATE TABLE issues (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID REFERENCES projects(id) ON DELETE CASCADE,
    fingerprint VARCHAR(64) NOT NULL,
    title TEXT NOT NULL,
    level VARCHAR(20) DEFAULT 'error',
    status VARCHAR(20) DEFAULT 'unresolved',
    first_seen TIMESTAMPTZ DEFAULT now(),
    last_seen TIMESTAMPTZ DEFAULT now(),
    count INT DEFAULT 1,
    UNIQUE(project_id, fingerprint)
);
CREATE INDEX idx_issues_project_lastseen ON issues(project_id, last_seen DESC);

CREATE TABLE events (
    id UUID DEFAULT gen_random_uuid(),
    issue_id UUID NOT NULL,
    project_id UUID NOT NULL,
    payload JSONB,
    stack_trace TEXT,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
) PARTITION BY RANGE (occurred_at);
CREATE INDEX idx_events_issue ON events(issue_id, occurred_at DESC);

-- Partisi bulan berjalan + bulan depan, plus partisi DEFAULT sebagai
-- catch-all supaya INSERT tidak pernah gagal gara-gara "no partition found".
CREATE TABLE events_2026_08 PARTITION OF events
    FOR VALUES FROM ('2026-08-01') TO ('2026-09-01');
CREATE TABLE events_2026_09 PARTITION OF events
    FOR VALUES FROM ('2026-09-01') TO ('2026-10-01');
CREATE TABLE events_default PARTITION OF events DEFAULT;