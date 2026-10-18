-- +goose Up
ALTER TABLE conversation_interactions ADD COLUMN authorization_version bigint NOT NULL DEFAULT 1;
UPDATE conversation_interactions i SET authorization_version=r.authorization_version FROM runs r WHERE r.id=i.run_id;
-- +goose Down
ALTER TABLE conversation_interactions DROP COLUMN authorization_version;
