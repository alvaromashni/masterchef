-- Schema inicial, conforme a seção 9 do CONTEXT.md.
-- Datas são guardadas como TEXT em ISO 8601 (o SQLite não tem tipo de data próprio).

CREATE TABLE issues (
  id            TEXT PRIMARY KEY,      -- id do Linear
  identifier    TEXT NOT NULL,         -- ex.: ABC-123
  product_slug  TEXT NOT NULL,
  title         TEXT NOT NULL,
  state_name    TEXT NOT NULL,
  state_type    TEXT NOT NULL,         -- backlog, unstarted, started, completed, canceled
  scope_label   TEXT,                  -- de scope:<nome>, se houver
  url           TEXT NOT NULL,
  description   TEXT,
  updated_at    TEXT NOT NULL          -- ISO 8601
);

CREATE TABLE pull_requests (
  id            INTEGER PRIMARY KEY,   -- id do GitHub
  repo          TEXT NOT NULL,         -- owner/name
  number        INTEGER NOT NULL,
  product_slug  TEXT NOT NULL,
  scope         TEXT NOT NULL,
  title         TEXT NOT NULL,
  branch        TEXT NOT NULL,
  state         TEXT NOT NULL,         -- open, closed, merged
  draft         INTEGER NOT NULL,
  additions     INTEGER NOT NULL,
  deletions     INTEGER NOT NULL,
  risk_level    TEXT NOT NULL,
  risk_reasons  TEXT NOT NULL,         -- JSON array de strings
  url           TEXT NOT NULL,
  updated_at    TEXT NOT NULL,
  UNIQUE (repo, number)
);

CREATE TABLE issue_prs (
  issue_id  TEXT NOT NULL,
  pr_id     INTEGER NOT NULL,
  PRIMARY KEY (issue_id, pr_id)
);

CREATE TABLE events (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  occurred_at  TEXT NOT NULL,          -- quando o coletor detectou
  product_slug TEXT NOT NULL,
  scope        TEXT,                   -- nulo = "a classificar"
  kind         TEXT NOT NULL,          -- issue_created, issue_state_changed, pr_opened, pr_merged, pr_closed, pr_updated
  ref          TEXT NOT NULL,          -- ABC-123 ou owner/repo#42
  summary      TEXT NOT NULL,          -- ex.: "In Progress → In Review"
  url          TEXT NOT NULL
);

CREATE TABLE meta (
  key    TEXT PRIMARY KEY,             -- last_visit, last_sync_ok, last_sync_error
  value  TEXT NOT NULL
);
