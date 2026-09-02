CREATE TABLE IF NOT EXISTS workspace_tags (
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  tag TEXT NOT NULL,
  PRIMARY KEY (workspace_id, tag),
  CHECK (length(tag) >= 1 AND length(tag) <= 20)
);
CREATE INDEX IF NOT EXISTS idx_workspace_tags_tag ON workspace_tags(tag);
