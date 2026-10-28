-- +goose Up
CREATE TABLE model_metric_outbox (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 enterprise_id uuid NOT NULL REFERENCES enterprises(id),
 entity_id uuid NOT NULL,
 kind text NOT NULL CHECK(kind IN ('call','quota','reservation')),
 department_id uuid,
 payload jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 delivered_at timestamptz
);
CREATE INDEX model_metric_outbox_pending ON model_metric_outbox(id) WHERE delivered_at IS NULL;
CREATE INDEX model_metric_outbox_scope ON model_metric_outbox(enterprise_id,department_id,id);
CREATE INDEX model_metric_outbox_entity ON model_metric_outbox(enterprise_id,kind,entity_id,id DESC);

-- The durable snapshot is committed with the terminal call. No prompts, output,
-- endpoint, credentials or arbitrary model payload are copied into analytics.
-- +goose StatementBegin
CREATE FUNCTION enqueue_model_metric_call() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE actor_id uuid; dept_id uuid;
BEGIN
 IF NEW.completed_at IS NULL OR NEW.status NOT IN ('succeeded','failed','cancelled') THEN RETURN NEW; END IF;
 SELECT COALESCE(q.user_id,r.actor_user_id),COALESCE(q.department_id,u.department_id)
 INTO actor_id,dept_id FROM runs r JOIN enterprise_users u ON u.id=r.actor_user_id AND u.enterprise_id=r.enterprise_id
 LEFT JOIN model_quota_reservations q ON q.model_call_id=NEW.id AND q.enterprise_id=NEW.enterprise_id WHERE r.id=NEW.run_id;
 INSERT INTO model_metric_outbox(enterprise_id,entity_id,kind,department_id,payload)
 VALUES(NEW.enterprise_id,NEW.id,'call',dept_id,jsonb_build_object(
 'enterprise_id',NEW.enterprise_id,'id',NEW.id,'model_id',NEW.model_id,'department_id',dept_id,'user_id',actor_id,
 'completed_at',NEW.completed_at,'status',NEW.status,'call_kind',NEW.call_kind,'error_code',COALESCE(NEW.error_code,''),
 'input_tokens',NEW.input_tokens,'output_tokens',NEW.output_tokens,'cached_tokens',NEW.cached_input_tokens,
 'input_source',NEW.input_usage_source,'output_source',NEW.output_usage_source,'cached_source',NEW.cached_input_usage_source,
 'amount',NEW.amount::text,'latency_ms',NEW.latency_ms,'dispatched',NEW.dispatched_at IS NOT NULL));
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER model_metric_call_outbox AFTER INSERT OR UPDATE ON model_calls FOR EACH ROW EXECUTE FUNCTION enqueue_model_metric_call();

-- +goose StatementBegin
CREATE FUNCTION enqueue_model_metric_quota() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE item model_quotas; deleted boolean;
BEGIN
 deleted := TG_OP='DELETE';
 IF deleted THEN item:=OLD; ELSE item:=NEW; END IF;
 INSERT INTO model_metric_outbox(enterprise_id,entity_id,kind,department_id,payload)
 VALUES(item.enterprise_id,item.id,'quota',CASE WHEN item.subject_type='department' THEN item.subject_id END,
 jsonb_build_object('enterprise_id',item.enterprise_id,'id',item.id,'model_id',item.model_id,
 'subject_type',item.subject_type,'subject_id',item.subject_id,'amount',item.monthly_amount::text,
 'effective_at',clock_timestamp(),'deleted',deleted));
 RETURN item;
END $$;
-- +goose StatementEnd
CREATE TRIGGER model_metric_quota_outbox AFTER INSERT OR UPDATE OR DELETE ON model_quotas FOR EACH ROW EXECUTE FUNCTION enqueue_model_metric_quota();

-- +goose StatementBegin
CREATE FUNCTION enqueue_model_metric_reservation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO model_metric_outbox(enterprise_id,entity_id,kind,department_id,payload)
 VALUES(NEW.enterprise_id,NEW.id,'reservation',NEW.department_id,jsonb_build_object(
 'enterprise_id',NEW.enterprise_id,'id',NEW.id,'model_id',NEW.model_id,'department_id',NEW.department_id,'user_id',NEW.user_id,
 'month',to_char(NEW.month,'YYYY-MM'),'reserved_amount',NEW.reserved_amount::text,'settled_amount',COALESCE(NEW.settled_amount,0)::text,'status',NEW.status));
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER model_metric_reservation_outbox AFTER INSERT OR UPDATE ON model_quota_reservations FOR EACH ROW EXECUTE FUNCTION enqueue_model_metric_reservation();

-- Historical backfill is durable and replayable, using exactly the live snapshot format.
INSERT INTO model_metric_outbox(enterprise_id,entity_id,kind,department_id,payload)
SELECT c.enterprise_id,c.id,'call',COALESCE(q.department_id,u.department_id),jsonb_build_object(
 'enterprise_id',c.enterprise_id,'id',c.id,'model_id',c.model_id,'department_id',COALESCE(q.department_id,u.department_id),'user_id',COALESCE(q.user_id,r.actor_user_id),
 'completed_at',c.completed_at,'status',c.status,'call_kind',c.call_kind,'error_code',COALESCE(c.error_code,''),
 'input_tokens',c.input_tokens,'output_tokens',c.output_tokens,'cached_tokens',c.cached_input_tokens,
 'input_source',c.input_usage_source,'output_source',c.output_usage_source,'cached_source',c.cached_input_usage_source,
 'amount',c.amount::text,'latency_ms',c.latency_ms,'dispatched',c.dispatched_at IS NOT NULL)
FROM model_calls c JOIN runs r ON r.id=c.run_id JOIN enterprise_users u ON u.id=r.actor_user_id AND u.enterprise_id=c.enterprise_id
LEFT JOIN model_quota_reservations q ON q.model_call_id=c.id AND q.enterprise_id=c.enterprise_id
WHERE c.completed_at IS NOT NULL AND c.status IN ('succeeded','failed','cancelled');
INSERT INTO model_metric_outbox(enterprise_id,entity_id,kind,department_id,payload)
SELECT enterprise_id,id,'quota',CASE WHEN subject_type='department' THEN subject_id END,
jsonb_build_object('enterprise_id',enterprise_id,'id',id,'model_id',model_id,'subject_type',subject_type,'subject_id',subject_id,
 'amount',monthly_amount::text,'effective_at',updated_at,'deleted',false) FROM model_quotas;
INSERT INTO model_metric_outbox(enterprise_id,entity_id,kind,department_id,payload)
SELECT enterprise_id,id,'reservation',department_id,jsonb_build_object(
 'enterprise_id',enterprise_id,'id',id,'model_id',model_id,'department_id',department_id,'user_id',user_id,
 'month',to_char(month,'YYYY-MM'),'reserved_amount',reserved_amount::text,'settled_amount',COALESCE(settled_amount,0)::text,'status',status)
FROM model_quota_reservations;

-- +goose Down
DROP TRIGGER model_metric_reservation_outbox ON model_quota_reservations;
DROP FUNCTION enqueue_model_metric_reservation();
DROP TRIGGER model_metric_quota_outbox ON model_quotas;
DROP TRIGGER model_metric_call_outbox ON model_calls;
DROP FUNCTION enqueue_model_metric_quota();
DROP FUNCTION enqueue_model_metric_call();
DROP TABLE model_metric_outbox;
