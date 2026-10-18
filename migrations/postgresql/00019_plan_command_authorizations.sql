-- +goose Up
CREATE TABLE plan_command_authorizations (
    id uuid PRIMARY KEY,
    enterprise_id uuid NOT NULL REFERENCES enterprises(id),
    conversation_id uuid NOT NULL REFERENCES conversations(id),
    owner_user_id uuid NOT NULL REFERENCES enterprise_users(id),
    plan_id uuid NOT NULL REFERENCES conversation_interactions(id),
    sequence integer NOT NULL CHECK(sequence>0),
    block_id text NOT NULL,
    spec jsonb NOT NULL,
    spec_hash text NOT NULL,
    implementation_run_id uuid REFERENCES runs(id),
    used_request_id uuid REFERENCES remote_execution_requests(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(plan_id,sequence),
    UNIQUE(plan_id,block_id),
    UNIQUE(used_request_id)
);
ALTER TABLE pending_actions ADD COLUMN approved_plan_command_id uuid REFERENCES plan_command_authorizations(id);
ALTER TABLE user_confirmations DROP CONSTRAINT user_confirmations_source_check;
ALTER TABLE user_confirmations ADD CONSTRAINT user_confirmations_source_check CHECK(source IN ('user','access_mode_policy','plan_approval'));

-- +goose Down
ALTER TABLE pending_actions DROP COLUMN approved_plan_command_id;
DROP TABLE plan_command_authorizations;
ALTER TABLE user_confirmations DROP CONSTRAINT user_confirmations_source_check;
UPDATE user_confirmations SET source='user' WHERE source='plan_approval';
ALTER TABLE user_confirmations ADD CONSTRAINT user_confirmations_source_check CHECK(source IN ('user','access_mode_policy'));
