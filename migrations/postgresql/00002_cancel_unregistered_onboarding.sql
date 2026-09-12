-- +goose Up
ALTER TABLE connector_install_operations DROP CONSTRAINT connector_install_operations_status_check;
ALTER TABLE connector_install_operations ADD CONSTRAINT connector_install_operations_status_check
  CHECK (status = ANY (ARRAY['queued','running','succeeded','failed','result_unknown','expired','cancelled']::text[]));

ALTER TABLE host_onboarding_operations DROP CONSTRAINT host_onboarding_operations_status_check;
ALTER TABLE host_onboarding_operations ADD CONSTRAINT host_onboarding_operations_status_check
  CHECK (status = ANY (ARRAY['queued','running','succeeded','failed','result_unknown','expired','cancelled']::text[]));

-- +goose Down
UPDATE connector_install_operations SET status='failed',error_code=COALESCE(error_code,'CONNECTOR_INSTALL_CANCELLED_BY_DELETE') WHERE status='cancelled';
ALTER TABLE connector_install_operations DROP CONSTRAINT connector_install_operations_status_check;
ALTER TABLE connector_install_operations ADD CONSTRAINT connector_install_operations_status_check
  CHECK (status = ANY (ARRAY['queued','running','succeeded','failed','result_unknown','expired']::text[]));

UPDATE host_onboarding_operations SET status='failed',error_code=COALESCE(error_code,'HOST_ONBOARDING_CANCELLED_BY_DELETE') WHERE status='cancelled';
ALTER TABLE host_onboarding_operations DROP CONSTRAINT host_onboarding_operations_status_check;
ALTER TABLE host_onboarding_operations ADD CONSTRAINT host_onboarding_operations_status_check
  CHECK (status = ANY (ARRAY['queued','running','succeeded','failed','result_unknown','expired']::text[]));
