-- +goose NO TRANSACTION
-- +goose Up
-- +goose StatementBegin
PRAGMA foreign_keys=OFF;
PRAGMA legacy_alter_table=ON;

-- These triggers read the projects table. Modern SQLite rewrites every trigger
-- that names a dropped table, and that rewrite fails once projects is gone.
DROP TRIGGER IF EXISTS agent_model_catalog_cdc_insert;
DROP TRIGGER IF EXISTS agent_model_catalog_cdc_update;

CREATE TABLE projects_new (
    id              TEXT PRIMARY KEY,
    path            TEXT NOT NULL,
    repo_origin_url TEXT NOT NULL DEFAULT '',
    display_name    TEXT NOT NULL DEFAULT '',
    registered_at   TIMESTAMP NOT NULL,
    archived_at     TIMESTAMP,
    config          TEXT,
    kind            TEXT NOT NULL DEFAULT 'single_repo'
        CHECK (kind IN ('single_repo', 'workspace', 'scratch', 'work'))
);

INSERT INTO projects_new (id, path, repo_origin_url, display_name, registered_at, archived_at, config, kind)
SELECT id, path, repo_origin_url, display_name, registered_at, archived_at, config, kind
FROM projects;

DROP TABLE projects;
ALTER TABLE projects_new RENAME TO projects;

CREATE TRIGGER agent_model_catalog_cdc_insert
AFTER INSERT ON agent_model_catalog
BEGIN
    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)
    SELECT projects.id, NULL, 'session_updated',
        json_object('kind', 'model_catalog', 'agentId', NEW.agent_id, 'projectId', projects.id),
        datetime('now')
    FROM projects
    WHERE (NEW.project_id = '' AND projects.archived_at IS NULL)
       OR projects.id = NEW.project_id;
END;

CREATE TRIGGER agent_model_catalog_cdc_update
AFTER UPDATE ON agent_model_catalog
BEGIN
    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)
    SELECT projects.id, NULL, 'session_updated',
        json_object('kind', 'model_catalog', 'agentId', NEW.agent_id, 'projectId', projects.id),
        datetime('now')
    FROM projects
    WHERE (NEW.project_id = '' AND projects.archived_at IS NULL)
       OR projects.id = NEW.project_id;
END;

PRAGMA legacy_alter_table=OFF;
PRAGMA foreign_keys=ON;
PRAGMA foreign_key_check;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- SQLite cannot narrow this CHECK safely once work rows may exist. Keep the
-- widened constraint in place, matching the existing best-effort project-kind
-- down migration style.
-- +goose StatementEnd
