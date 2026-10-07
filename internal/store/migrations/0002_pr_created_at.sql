-- A fila de review ordena PRs por idade (desde quando estão abertos).
-- O schema inicial só guardava updated_at, que muda a cada push.
ALTER TABLE pull_requests ADD COLUMN created_at TEXT NOT NULL DEFAULT '';
