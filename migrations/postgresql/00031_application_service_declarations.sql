-- +goose Up
ALTER TABLE monitoring_application_services ALTER COLUMN first_seen_at DROP NOT NULL;
ALTER TABLE monitoring_application_services ALTER COLUMN first_seen_at DROP DEFAULT;
ALTER TABLE monitoring_application_services ALTER COLUMN last_seen_at DROP NOT NULL;
ALTER TABLE monitoring_application_services ALTER COLUMN last_seen_at DROP DEFAULT;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM monitoring_application_services WHERE first_seen_at IS NULL OR last_seen_at IS NULL) THEN
  RAISE EXCEPTION 'Unobserved service declarations exist; do not invent observation timestamps during downgrade';
 END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE monitoring_application_services ALTER COLUMN first_seen_at SET NOT NULL;
ALTER TABLE monitoring_application_services ALTER COLUMN first_seen_at SET DEFAULT now();
ALTER TABLE monitoring_application_services ALTER COLUMN last_seen_at SET NOT NULL;
ALTER TABLE monitoring_application_services ALTER COLUMN last_seen_at SET DEFAULT now();
