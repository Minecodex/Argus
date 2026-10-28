-- +goose Up
CREATE INDEX audit_events_operation_ref_idx
ON audit_events (enterprise_id, (COALESCE(details->>'action_ref', details->>'pending_action_ref')), created_at DESC, id DESC)
WHERE domain = 'enterprise';

-- +goose Down
DROP INDEX audit_events_operation_ref_idx;
