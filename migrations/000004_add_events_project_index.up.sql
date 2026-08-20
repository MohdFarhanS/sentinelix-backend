-- Dipakai EventRepository.CountGroupedByIssueSince (worker threshold
-- ticker, Sprint 6): "WHERE project_id=? AND occurred_at > ? GROUP BY
-- issue_id" — idx_events_issue yang sudah ada di-index by issue_id,
-- BUKAN project_id, jadi query ini butuh index terpisah.
CREATE INDEX idx_events_project_occurred ON events(project_id, occurred_at DESC);