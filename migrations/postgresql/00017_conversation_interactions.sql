-- +goose Up
ALTER TABLE conversations DROP CONSTRAINT conversations_mode_check;
ALTER TABLE runs DROP CONSTRAINT runs_mode_snapshot_check;
UPDATE conversations SET mode='execute' WHERE mode='investigate';
UPDATE runs SET mode_snapshot='execute' WHERE mode_snapshot='investigate';
ALTER TABLE conversations ALTER COLUMN mode SET DEFAULT 'execute';
ALTER TABLE runs ALTER COLUMN mode_snapshot SET DEFAULT 'execute';
ALTER TABLE conversations ADD CONSTRAINT conversations_mode_check CHECK(mode IN ('execute','plan'));
ALTER TABLE runs ADD CONSTRAINT runs_mode_snapshot_check CHECK(mode_snapshot IN ('execute','plan'));
ALTER TABLE conversations ADD COLUMN access_mode text NOT NULL DEFAULT 'request_approval' CHECK(access_mode IN ('request_approval','full_access'));
ALTER TABLE runs ADD COLUMN access_mode_snapshot text NOT NULL DEFAULT 'request_approval' CHECK(access_mode_snapshot IN ('request_approval','full_access'));
ALTER TABLE runs ADD COLUMN approved_plan_id uuid;
ALTER TABLE pending_actions ADD COLUMN confirmation_mode text NOT NULL DEFAULT 'request_approval' CHECK(confirmation_mode IN ('request_approval','full_access'));
ALTER TABLE pending_actions ADD COLUMN confirmation_policy_version bigint;
ALTER TABLE user_confirmations ADD COLUMN source text NOT NULL DEFAULT 'user' CHECK(source IN ('user','access_mode_policy'));

CREATE TABLE conversation_interactions (
    id uuid PRIMARY KEY,
    enterprise_id uuid NOT NULL REFERENCES enterprises(id),
    conversation_id uuid NOT NULL REFERENCES conversations(id),
    owner_user_id uuid NOT NULL REFERENCES enterprise_users(id),
    run_id uuid NOT NULL REFERENCES runs(id),
    tool_call_id uuid NOT NULL REFERENCES tool_calls(id),
    kind text NOT NULL CHECK(kind IN ('user_input','plan')),
    family_id uuid NOT NULL,
    revision integer NOT NULL DEFAULT 1 CHECK(revision>0),
    status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','answered','skipped','superseded','confirmed','cancelled')),
    content jsonb NOT NULL,
    content_hash text NOT NULL,
    response jsonb,
    implementation_run_id uuid REFERENCES runs(id),
    version bigint NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tool_call_id),
    UNIQUE(family_id,revision)
);
CREATE INDEX conversation_interactions_visible ON conversation_interactions(enterprise_id,conversation_id,created_at);
CREATE UNIQUE INDEX conversation_interactions_pending ON conversation_interactions(run_id) WHERE status='pending';
ALTER TABLE runs ADD CONSTRAINT runs_approved_plan_id_fkey FOREIGN KEY(approved_plan_id) REFERENCES conversation_interactions(id);
ALTER TABLE conversation_events DROP CONSTRAINT conversation_events_event_type_check;
ALTER TABLE conversation_events ADD CONSTRAINT conversation_events_event_type_check CHECK(event_type IN (
 'user_message','assistant_message','model_usage','tool_call_requested','tool_call_started','tool_call_result',
 'pending_action_created','user_confirmation','approval_update','execution_update','tool_presentation',
 'workspace_file_added','artifact_published','run_state_changed','context_compacted','agent_delta',
 'interaction_requested','interaction_resolved','conversation_settings_changed'));

-- +goose Down
ALTER TABLE runs DROP CONSTRAINT runs_approved_plan_id_fkey;
DROP TABLE conversation_interactions;
ALTER TABLE runs DROP COLUMN approved_plan_id, DROP COLUMN access_mode_snapshot;
ALTER TABLE user_confirmations DROP COLUMN source;
ALTER TABLE pending_actions DROP COLUMN confirmation_mode, DROP COLUMN confirmation_policy_version;
ALTER TABLE conversations DROP COLUMN access_mode;
ALTER TABLE conversations DROP CONSTRAINT conversations_mode_check;
ALTER TABLE runs DROP CONSTRAINT runs_mode_snapshot_check;
UPDATE conversations SET mode='investigate' WHERE mode='execute';
UPDATE runs SET mode_snapshot='investigate' WHERE mode_snapshot='execute';
ALTER TABLE conversations ALTER COLUMN mode SET DEFAULT 'investigate';
ALTER TABLE runs ALTER COLUMN mode_snapshot SET DEFAULT 'investigate';
ALTER TABLE conversations ADD CONSTRAINT conversations_mode_check CHECK(mode IN ('investigate','plan'));
ALTER TABLE runs ADD CONSTRAINT runs_mode_snapshot_check CHECK(mode_snapshot IN ('investigate','plan'));
