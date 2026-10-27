-- +goose Up
-- The manager pre-selected when someone creates a Code project. Work projects
-- never use this; their pickers start empty.
ALTER TABLE app_settings ADD COLUMN default_code_manager TEXT NOT NULL DEFAULT 'claude-code';

-- +goose Down
ALTER TABLE app_settings DROP COLUMN default_code_manager;
