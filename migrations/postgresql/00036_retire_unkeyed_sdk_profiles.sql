-- +goose Up
-- Application SDKs use authenticated, application-scoped endpoints. Keep
-- historical profile identities for audit while denying new activation.
UPDATE collection_profiles SET support_status='retired'
WHERE profile_key IN ('otlp-receiver','jaeger-receiver','skywalking-receiver');

-- +goose Down
-- A schema rollback must not reactivate an unauthenticated business receiver.
SELECT 1;
