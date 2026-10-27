-- +goose Up
ALTER TABLE dashboard_bindings DROP CONSTRAINT dashboard_bindings_resource_type_check;
ALTER TABLE dashboard_bindings ADD CONSTRAINT dashboard_bindings_resource_type_check CHECK(resource_type IN ('host','kubernetes_cluster','application','monitoring_target'));
-- +goose StatementBegin
CREATE FUNCTION validate_dashboard_resource_reference() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.resource_type='host' AND NOT EXISTS(SELECT 1 FROM hosts WHERE id=NEW.resource_id AND enterprise_id=NEW.enterprise_id) THEN
  RAISE EXCEPTION 'dashboard host outside enterprise' USING ERRCODE='23503';
 ELSIF NEW.resource_type='kubernetes_cluster' AND NOT EXISTS(SELECT 1 FROM kubernetes_clusters WHERE id=NEW.resource_id AND enterprise_id=NEW.enterprise_id) THEN
  RAISE EXCEPTION 'dashboard cluster outside enterprise' USING ERRCODE='23503';
 ELSIF NEW.resource_type='application' AND NOT EXISTS(SELECT 1 FROM monitoring_applications WHERE id=NEW.resource_id AND enterprise_id=NEW.enterprise_id) THEN
  RAISE EXCEPTION 'dashboard application outside enterprise' USING ERRCODE='23503';
 ELSIF NEW.resource_type='monitoring_target' AND NOT EXISTS(SELECT 1 FROM monitoring_targets WHERE id=NEW.resource_id AND enterprise_id=NEW.enterprise_id) THEN
  RAISE EXCEPTION 'dashboard monitoring target outside enterprise' USING ERRCODE='23503';
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER dashboard_resource_reference BEFORE INSERT OR UPDATE ON dashboard_bindings FOR EACH ROW EXECUTE FUNCTION validate_dashboard_resource_reference();

-- +goose Down
-- The old constraint deliberately refuses rollback while new-kind bindings exist.
ALTER TABLE dashboard_bindings DROP CONSTRAINT dashboard_bindings_resource_type_check;
ALTER TABLE dashboard_bindings ADD CONSTRAINT dashboard_bindings_resource_type_check CHECK(resource_type IN ('host','kubernetes_cluster'));
DROP TRIGGER dashboard_resource_reference ON dashboard_bindings;
DROP FUNCTION validate_dashboard_resource_reference();
