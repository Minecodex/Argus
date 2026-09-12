-- +goose Up
-- Argus is pre-release. This is the only PostgreSQL baseline and contains the
-- current schema directly; no historical Host, WinRM, self-enrollment, or
-- compatibility transition is retained.

-- +goose StatementBegin
CREATE FUNCTION public.reject_remote_access_governance_delete() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE EXCEPTION 'remote access governance objects must be archived, not deleted'
        USING ERRCODE = '55000';
END;
$$;
-- +goose StatementEnd


--
-- Name: validate_collector_resource(); Type: FUNCTION; Schema: public; Owner: -
--

-- +goose StatementBegin
CREATE FUNCTION public.validate_collector_resource() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.resource_type = 'host' AND NOT EXISTS (
        SELECT 1 FROM hosts WHERE id = NEW.resource_id AND enterprise_id = NEW.enterprise_id
    ) THEN
        RAISE EXCEPTION 'collector Host must belong to enterprise' USING ERRCODE = '23503';
    ELSIF NEW.resource_type = 'kubernetes_cluster' AND NOT EXISTS (
        SELECT 1 FROM kubernetes_clusters WHERE id = NEW.resource_id AND enterprise_id = NEW.enterprise_id
    ) THEN
        RAISE EXCEPTION 'collector Kubernetes cluster must belong to enterprise' USING ERRCODE = '23503';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd


--
-- Name: validate_data_authorization_grant(); Type: FUNCTION; Schema: public; Owner: -
--

-- +goose StatementBegin
CREATE FUNCTION public.validate_data_authorization_grant() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.subject_type = 'user' AND NOT EXISTS (SELECT 1 FROM enterprise_users WHERE id = NEW.subject_id AND enterprise_id = NEW.enterprise_id) THEN
        RAISE EXCEPTION 'authorization grant user does not belong to enterprise' USING ERRCODE = '23503';
    ELSIF NEW.subject_type = 'department' AND NOT EXISTS (SELECT 1 FROM departments WHERE id = NEW.subject_id AND enterprise_id = NEW.enterprise_id) THEN
        RAISE EXCEPTION 'authorization grant department does not belong to enterprise' USING ERRCODE = '23503';
    ELSIF NEW.subject_type = 'role' AND NOT EXISTS (SELECT 1 FROM roles WHERE id = NEW.subject_id AND enterprise_id = NEW.enterprise_id) THEN
        RAISE EXCEPTION 'authorization grant role does not belong to enterprise' USING ERRCODE = '23503';
    ELSIF NEW.subject_type = 'service_account' AND NOT EXISTS (SELECT 1 FROM service_accounts WHERE id = NEW.subject_id AND enterprise_id = NEW.enterprise_id) THEN
        RAISE EXCEPTION 'authorization grant service account does not belong to enterprise' USING ERRCODE = '23503';
    END IF;
    IF NEW.resource_type = 'host' AND NOT EXISTS (SELECT 1 FROM hosts WHERE id = NEW.resource_id AND enterprise_id = NEW.enterprise_id) THEN
        RAISE EXCEPTION 'authorization grant host does not belong to enterprise' USING ERRCODE = '23503';
    ELSIF NEW.resource_type = 'kubernetes_cluster' AND NOT EXISTS (SELECT 1 FROM kubernetes_clusters WHERE id = NEW.resource_id AND enterprise_id = NEW.enterprise_id) THEN
        RAISE EXCEPTION 'authorization grant cluster does not belong to enterprise' USING ERRCODE = '23503';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd


--
-- Name: validate_remote_access_grant(); Type: FUNCTION; Schema: public; Owner: -
--

-- +goose StatementBegin
CREATE FUNCTION public.validate_remote_access_grant() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.subject_type = 'user' AND NOT EXISTS (SELECT 1 FROM enterprise_users WHERE id = NEW.subject_id AND enterprise_id = NEW.enterprise_id) THEN
        RAISE EXCEPTION 'remote access grant user must belong to enterprise' USING ERRCODE = '23503';
    ELSIF NEW.subject_type = 'department' AND NOT EXISTS (SELECT 1 FROM departments WHERE id = NEW.subject_id AND enterprise_id = NEW.enterprise_id) THEN
        RAISE EXCEPTION 'remote access grant department must belong to enterprise' USING ERRCODE = '23503';
    END IF;
    IF EXISTS (SELECT 1 FROM unnest(NEW.host_ids) id WHERE NOT EXISTS (SELECT 1 FROM hosts WHERE hosts.id = id AND hosts.enterprise_id = NEW.enterprise_id AND hosts.status = 'active')) THEN
        RAISE EXCEPTION 'remote access grant host must belong to enterprise' USING ERRCODE = '23503';
    END IF;
    IF EXISTS (SELECT 1 FROM unnest(NEW.managed_account_ids) id WHERE NOT EXISTS (SELECT 1 FROM managed_accounts WHERE managed_accounts.id = id AND managed_accounts.enterprise_id = NEW.enterprise_id AND managed_accounts.status = 'active')) THEN
        RAISE EXCEPTION 'remote access grant account must belong to enterprise' USING ERRCODE = '23503';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd


--
-- Name: validate_role_binding_subject(); Type: FUNCTION; Schema: public; Owner: -
--

-- +goose StatementBegin
CREATE FUNCTION public.validate_role_binding_subject() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF NEW.subject_type = 'user' AND NOT EXISTS (SELECT 1 FROM enterprise_users WHERE id = NEW.subject_id AND enterprise_id = NEW.enterprise_id) THEN
        RAISE EXCEPTION 'role binding user must belong to enterprise' USING ERRCODE = '23503';
    ELSIF NEW.subject_type = 'department' AND NOT EXISTS (SELECT 1 FROM departments WHERE id = NEW.subject_id AND enterprise_id = NEW.enterprise_id) THEN
        RAISE EXCEPTION 'role binding department must belong to enterprise' USING ERRCODE = '23503';
    ELSIF NEW.subject_type = 'service_account' AND NOT EXISTS (SELECT 1 FROM service_accounts WHERE id = NEW.subject_id AND enterprise_id = NEW.enterprise_id) THEN
        RAISE EXCEPTION 'role binding service account must belong to enterprise' USING ERRCODE = '23503';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: action_bindings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.action_bindings (
    id uuid NOT NULL,
    binding_ref text NOT NULL,
    pending_action_id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    actor_user_id uuid,
    action text NOT NULL,
    request_id text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    consumed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    card_instance_id uuid,
    conversation_id uuid,
    authorization_version bigint,
    binding_source text DEFAULT 'text_fallback'::text NOT NULL,
    CONSTRAINT action_bindings_action_check CHECK ((action = ANY (ARRAY['confirm'::text, 'cancel'::text, 'approve'::text, 'reject'::text]))),
    CONSTRAINT action_bindings_authorization_version_check CHECK (((authorization_version IS NULL) OR (authorization_version > 0))),
    CONSTRAINT action_bindings_binding_source_check CHECK ((binding_source = ANY (ARRAY['text_fallback'::text, 'card'::text]))),
    CONSTRAINT action_bindings_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'consumed'::text, 'cancelled'::text, 'expired'::text, 'invalidated'::text])))
);


--
-- Name: ai_model_credentials; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ai_model_credentials (
    id uuid NOT NULL,
    model_revision_id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    key_id text NOT NULL,
    key_version integer NOT NULL,
    wrapped_dek bytea NOT NULL,
    wrap_nonce bytea,
    nonce bytea NOT NULL,
    ciphertext bytea NOT NULL,
    value_hash bytea NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    provider text DEFAULT 'local'::text NOT NULL,
    CONSTRAINT ai_model_credentials_envelope_check CHECK (((octet_length(wrapped_dek) > 0) AND (octet_length(nonce) = 12) AND (octet_length(ciphertext) > 0) AND (((provider = 'local'::text) AND (wrap_nonce IS NOT NULL) AND (octet_length(wrap_nonce) = 12)) OR ((provider = 'openbao_transit'::text) AND (wrap_nonce IS NULL))))),
    CONSTRAINT ai_model_credentials_key_version_check CHECK ((key_version > 0)),
    CONSTRAINT ai_model_credentials_provider_check CHECK ((provider = ANY (ARRAY['local'::text, 'openbao_transit'::text]))),
    CONSTRAINT ai_model_credentials_value_hash_check CHECK ((octet_length(value_hash) = 32))
);


--
-- Name: ai_model_revisions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ai_model_revisions (
    id uuid NOT NULL,
    model_id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    revision integer NOT NULL,
    base_url text NOT NULL,
    provider_model_id text NOT NULL,
    api_protocol text NOT NULL,
    context_window_tokens integer NOT NULL,
    max_output_tokens integer NOT NULL,
    input_price_per_million numeric(20,8) NOT NULL,
    output_price_per_million numeric(20,8) NOT NULL,
    capabilities jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ai_model_revisions_api_protocol_check CHECK ((api_protocol = ANY (ARRAY['chat_completions'::text, 'responses'::text]))),
    CONSTRAINT ai_model_revisions_revision_check CHECK ((revision > 0))
);


--
-- Name: ai_models; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ai_models (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    name text NOT NULL,
    base_url text NOT NULL,
    model_id text NOT NULL,
    api_protocol text NOT NULL,
    context_window_tokens integer NOT NULL,
    max_output_tokens integer NOT NULL,
    input_price_per_million numeric(20,8) NOT NULL,
    output_price_per_million numeric(20,8) NOT NULL,
    capabilities jsonb DEFAULT '{}'::jsonb NOT NULL,
    status text DEFAULT 'enabled'::text NOT NULL,
    health_status text DEFAULT 'unknown'::text NOT NULL,
    revision integer DEFAULT 1 NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    last_tested_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ai_models_api_protocol_check CHECK ((api_protocol = ANY (ARRAY['chat_completions'::text, 'responses'::text]))),
    CONSTRAINT ai_models_base_url_check CHECK (((char_length(base_url) >= 1) AND (char_length(base_url) <= 2048))),
    CONSTRAINT ai_models_check CHECK (((max_output_tokens > 0) AND (max_output_tokens < context_window_tokens))),
    CONSTRAINT ai_models_context_window_tokens_check CHECK ((context_window_tokens >= 8192)),
    CONSTRAINT ai_models_health_status_check CHECK ((health_status = ANY (ARRAY['unknown'::text, 'healthy'::text, 'unhealthy'::text]))),
    CONSTRAINT ai_models_input_price_per_million_check CHECK ((input_price_per_million >= (0)::numeric)),
    CONSTRAINT ai_models_model_id_check CHECK (((char_length(model_id) >= 1) AND (char_length(model_id) <= 256))),
    CONSTRAINT ai_models_name_check CHECK (((char_length(name) >= 1) AND (char_length(name) <= 128))),
    CONSTRAINT ai_models_output_price_per_million_check CHECK ((output_price_per_million >= (0)::numeric)),
    CONSTRAINT ai_models_revision_check CHECK ((revision > 0)),
    CONSTRAINT ai_models_status_check CHECK ((status = ANY (ARRAY['enabled'::text, 'disabled'::text]))),
    CONSTRAINT ai_models_version_check CHECK ((version > 0))
);


--
-- Name: api_keys; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.api_keys (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    service_account_id uuid NOT NULL,
    name text NOT NULL,
    prefix text NOT NULL,
    secret_hash bytea NOT NULL,
    authorization_version bigint NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    expires_at timestamp with time zone,
    last_used_at timestamp with time zone,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT api_keys_authorization_version_check CHECK ((authorization_version > 0)),
    CONSTRAINT api_keys_name_check CHECK (((char_length(name) >= 1) AND (char_length(name) <= 128))),
    CONSTRAINT api_keys_prefix_check CHECK (((char_length(prefix) >= 6) AND (char_length(prefix) <= 32))),
    CONSTRAINT api_keys_secret_hash_check CHECK ((octet_length(secret_hash) = 32)),
    CONSTRAINT api_keys_status_check CHECK ((status = ANY (ARRAY['active'::text, 'revoked'::text, 'expired'::text]))),
    CONSTRAINT api_keys_version_check CHECK ((version > 0))
);


--
-- Name: approval_decisions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.approval_decisions (
    id uuid NOT NULL,
    approval_request_id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    actor_user_id uuid NOT NULL,
    decision text NOT NULL,
    reason text DEFAULT ''::text NOT NULL,
    decided_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT approval_decisions_decision_check CHECK ((decision = ANY (ARRAY['approved'::text, 'rejected'::text])))
);


--
-- Name: approval_policies; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.approval_policies (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    name text NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    tool_ids text[] DEFAULT '{}'::text[] NOT NULL,
    risks text[] NOT NULL,
    resource_types text[] DEFAULT '{}'::text[] NOT NULL,
    minimum_approvers integer NOT NULL,
    separation_of_duty boolean DEFAULT true NOT NULL,
    approver_role_ids uuid[] NOT NULL,
    expires_after_seconds integer DEFAULT 86400 NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT approval_policies_approver_role_ids_check CHECK ((cardinality(approver_role_ids) > 0)),
    CONSTRAINT approval_policies_expires_after_seconds_check CHECK (((expires_after_seconds >= 60) AND (expires_after_seconds <= 604800))),
    CONSTRAINT approval_policies_minimum_approvers_check CHECK (((minimum_approvers >= 1) AND (minimum_approvers <= 10))),
    CONSTRAINT approval_policies_name_check CHECK (((char_length(name) >= 1) AND (char_length(name) <= 128))),
    CONSTRAINT approval_policies_risks_check CHECK ((cardinality(risks) > 0)),
    CONSTRAINT approval_policies_version_check CHECK ((version > 0))
);


--
-- Name: approval_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.approval_requests (
    id uuid NOT NULL,
    pending_action_id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT approval_requests_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'approved'::text, 'rejected'::text, 'expired'::text, 'invalidated'::text])))
);


--
-- Name: approval_requirement_snapshots; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.approval_requirement_snapshots (
    id uuid NOT NULL,
    approval_request_id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    policy_id uuid NOT NULL,
    policy_version bigint NOT NULL,
    minimum_approvers integer NOT NULL,
    separation_of_duty boolean NOT NULL,
    approver_role_ids uuid[] NOT NULL,
    approved_count integer DEFAULT 0 NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    policy_hash bytea NOT NULL,
    CONSTRAINT approval_requirement_snapshots_approved_count_check CHECK ((approved_count >= 0)),
    CONSTRAINT approval_requirement_snapshots_minimum_approvers_check CHECK (((minimum_approvers >= 1) AND (minimum_approvers <= 10))),
    CONSTRAINT approval_requirement_snapshots_policy_hash_check CHECK ((octet_length(policy_hash) = 32)),
    CONSTRAINT approval_requirement_snapshots_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'approved'::text, 'rejected'::text, 'invalidated'::text])))
);


--
-- Name: artifacts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.artifacts (
    id uuid NOT NULL,
    result_ref text NOT NULL,
    enterprise_id uuid NOT NULL,
    conversation_id uuid,
    run_id uuid,
    content_type text NOT NULL,
    data_classification text NOT NULL,
    content bytea NOT NULL,
    content_hash bytea NOT NULL,
    byte_size integer NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT artifacts_byte_size_check CHECK (((byte_size >= 0) AND (byte_size <= 4194304))),
    CONSTRAINT artifacts_content_check CHECK ((octet_length(content) <= 4194304)),
    CONSTRAINT artifacts_content_hash_check CHECK ((octet_length(content_hash) = 32)),
    CONSTRAINT artifacts_data_classification_check CHECK ((data_classification = ANY (ARRAY['public'::text, 'internal'::text, 'sensitive'::text])))
);


--
-- Name: audit_chain_heads; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.audit_chain_heads (
    chain_key text NOT NULL,
    domain text NOT NULL,
    enterprise_id uuid,
    last_event_id uuid,
    last_hash bytea NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    CONSTRAINT audit_chain_heads_check CHECK ((((domain = 'platform'::text) AND (enterprise_id IS NULL)) OR ((domain = 'enterprise'::text) AND (enterprise_id IS NOT NULL)))),
    CONSTRAINT audit_chain_heads_domain_check CHECK ((domain = ANY (ARRAY['platform'::text, 'enterprise'::text]))),
    CONSTRAINT audit_chain_heads_last_hash_check CHECK ((octet_length(last_hash) = 32))
);


--
-- Name: audit_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.audit_events (
    id uuid NOT NULL,
    domain text NOT NULL,
    enterprise_id uuid,
    actor_type text NOT NULL,
    actor_id text NOT NULL,
    action text NOT NULL,
    resource_type text,
    resource_id text,
    result text NOT NULL,
    details jsonb DEFAULT '{}'::jsonb NOT NULL,
    previous_hash bytea NOT NULL,
    event_hash bytea NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT audit_events_action_check CHECK ((action ~ '^[a-z][a-z0-9_.]+$'::text)),
    CONSTRAINT audit_events_actor_type_check CHECK ((actor_type = ANY (ARRAY['platform_user'::text, 'enterprise_user'::text, 'service_account'::text, 'connector'::text, 'direct_executor'::text, 'remote_access_gateway'::text, 'system'::text]))),
    CONSTRAINT audit_events_check CHECK ((((domain = 'platform'::text) AND (enterprise_id IS NULL)) OR ((domain = 'enterprise'::text) AND (enterprise_id IS NOT NULL)))),
    CONSTRAINT audit_events_domain_check CHECK ((domain = ANY (ARRAY['platform'::text, 'enterprise'::text]))),
    CONSTRAINT audit_events_event_hash_check CHECK ((octet_length(event_hash) = 32)),
    CONSTRAINT audit_events_previous_hash_check CHECK ((octet_length(previous_hash) = 32)),
    CONSTRAINT audit_events_result_check CHECK ((result = ANY (ARRAY['success'::text, 'failure'::text, 'denied'::text])))
);


--
-- Name: authorization_versions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.authorization_versions (
    enterprise_id uuid NOT NULL,
    subject_type text NOT NULL,
    subject_id uuid NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT authorization_versions_subject_type_check CHECK ((subject_type = ANY (ARRAY['user'::text, 'department'::text, 'role'::text, 'service_account'::text]))),
    CONSTRAINT authorization_versions_version_check CHECK ((version > 0))
);


--
-- Name: bastion_scopes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.bastion_scopes (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    name text NOT NULL,
    environment text NOT NULL,
    labels jsonb DEFAULT '{}'::jsonb NOT NULL,
    labels_hash bytea NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    connector_host_id uuid,
    active_connector_id uuid,
    fencing_generation bigint DEFAULT 1 NOT NULL,
    resource_version bigint DEFAULT 1 NOT NULL,
    deleted_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    onboarding_mode text NOT NULL,
    relay_address text DEFAULT ''::text NOT NULL,
    relay_https_port integer DEFAULT 8445 NOT NULL,
    relay_gateway_port integer DEFAULT 9445 NOT NULL,
    relay_port_generation bigint DEFAULT 0 NOT NULL,
    relay_status text DEFAULT 'pending'::text NOT NULL,
    relay_error_code text DEFAULT ''::text NOT NULL,
    removal_generation bigint DEFAULT 0 NOT NULL,
    local_cleanup text DEFAULT 'verified'::text NOT NULL,
    CONSTRAINT bastion_scopes_environment_check CHECK ((environment = ANY (ARRAY['development'::text, 'staging'::text, 'production'::text]))),
    CONSTRAINT bastion_scopes_fencing_generation_check CHECK ((fencing_generation > 0)),
    CONSTRAINT bastion_scopes_labels_check CHECK (((jsonb_typeof(labels) = 'object'::text) AND (octet_length((labels)::text) <= 4096))),
    CONSTRAINT bastion_scopes_labels_hash_check CHECK ((octet_length(labels_hash) = 32)),
    CONSTRAINT bastion_scopes_name_check CHECK (((char_length(name) >= 1) AND (char_length(name) <= 128))),
    CONSTRAINT bastion_scopes_onboarding_mode_check CHECK ((onboarding_mode = ANY (ARRAY['command'::text, 'direct_install'::text, 'direct_install_tunnel'::text]))),
    CONSTRAINT bastion_scopes_relay_address_check CHECK ((char_length(relay_address) <= 512)),
    CONSTRAINT bastion_scopes_relay_gateway_port_check CHECK (((relay_gateway_port >= 1) AND (relay_gateway_port <= 65535))),
    CONSTRAINT bastion_scopes_relay_https_port_check CHECK (((relay_https_port >= 1) AND (relay_https_port <= 65535))),
    CONSTRAINT bastion_scopes_relay_port_generation_check CHECK ((relay_port_generation >= 0)),
    CONSTRAINT bastion_scopes_relay_error_code_check CHECK ((relay_error_code = ''::text) OR (relay_error_code ~ '^[A-Z][A-Z0-9_]*$'::text)),
    CONSTRAINT bastion_scopes_relay_status_check CHECK ((relay_status = ANY (ARRAY['pending'::text, 'ready'::text, 'degraded'::text, 'offline'::text]))),
    CONSTRAINT bastion_scopes_resource_version_check CHECK ((resource_version > 0)),
    CONSTRAINT bastion_scopes_removal_generation_check CHECK ((removal_generation >= 0)),
    CONSTRAINT bastion_scopes_local_cleanup_check CHECK ((local_cleanup = ANY (ARRAY['verified'::text, 'pending'::text, 'unknown'::text]))),
    CONSTRAINT bastion_scopes_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'active'::text, 'suspected_offline'::text, 'offline'::text, 'draining'::text, 'uninstalling'::text, 'uninstalled'::text, 'removal_failed'::text, 'cleanup_unknown'::text, 'deleted'::text])))
);


--
-- Name: break_glass_sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.break_glass_sessions (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    user_id uuid NOT NULL,
    source_session_id uuid NOT NULL,
    authorization_version bigint NOT NULL,
    reason text NOT NULL,
    ticket_ref text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT break_glass_sessions_authorization_version_check CHECK ((authorization_version > 0)),
    CONSTRAINT break_glass_sessions_reason_check CHECK (((char_length(reason) >= 8) AND (char_length(reason) <= 2048))),
    CONSTRAINT break_glass_sessions_status_check CHECK ((status = ANY (ARRAY['active'::text, 'revoked'::text, 'expired'::text]))),
    CONSTRAINT break_glass_sessions_ticket_ref_check CHECK (((char_length(ticket_ref) >= 1) AND (char_length(ticket_ref) <= 256)))
);


--
-- Name: card_data_sources; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.card_data_sources (
    id uuid NOT NULL,
    card_instance_id uuid NOT NULL,
    slot_name text NOT NULL,
    tool_call_id uuid NOT NULL,
    result_ref text NOT NULL,
    field_path text NOT NULL,
    output_schema_version text NOT NULL,
    source_hash bytea NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT card_data_sources_source_hash_check CHECK ((octet_length(source_hash) = 32))
);


--
-- Name: card_demo_scenarios; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.card_demo_scenarios (
    id uuid NOT NULL,
    card_version_id uuid NOT NULL,
    scenario text NOT NULL,
    data jsonb NOT NULL,
    byte_size integer NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT card_demo_scenarios_byte_size_check CHECK (((byte_size >= 0) AND (byte_size <= 262144))),
    CONSTRAINT card_demo_scenarios_scenario_check CHECK ((scenario = ANY (ARRAY['default'::text, 'empty'::text, 'error'::text, 'large'::text, 'light'::text, 'dark'::text, 'zh-CN'::text, 'en-US'::text])))
);


--
-- Name: card_instances; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.card_instances (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    conversation_id uuid NOT NULL,
    run_id uuid,
    card_id uuid NOT NULL,
    card_version_id uuid NOT NULL,
    actor_user_id uuid NOT NULL,
    presentation_kind text NOT NULL,
    render_spec jsonb NOT NULL,
    render_spec_hash bytea NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT card_instances_presentation_kind_check CHECK ((presentation_kind = ANY (ARRAY['table'::text, 'detail'::text, 'pending_action'::text, 'metric'::text, 'generic'::text]))),
    CONSTRAINT card_instances_render_spec_hash_check CHECK ((octet_length(render_spec_hash) = 32)),
    CONSTRAINT card_instances_status_check CHECK ((status = ANY (ARRAY['active'::text, 'invalidated'::text])))
);


--
-- Name: card_presentations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.card_presentations (
    id uuid NOT NULL,
    card_instance_id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    viewer_user_id uuid NOT NULL,
    authorization_version bigint NOT NULL,
    locale text NOT NULL,
    color_scheme text NOT NULL,
    locale_fallback boolean DEFAULT false NOT NULL,
    initial_data jsonb NOT NULL,
    partial boolean DEFAULT false NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT card_presentations_authorization_version_check CHECK ((authorization_version > 0)),
    CONSTRAINT card_presentations_color_scheme_check CHECK ((color_scheme = ANY (ARRAY['light'::text, 'dark'::text]))),
    CONSTRAINT card_presentations_locale_check CHECK ((locale = ANY (ARRAY['zh-CN'::text, 'en-US'::text])))
);


--
-- Name: card_query_binding_specs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.card_query_binding_specs (
    id uuid NOT NULL,
    card_instance_id uuid NOT NULL,
    slot_name text NOT NULL,
    tool_id text NOT NULL,
    fixed_input jsonb NOT NULL,
    input_hash bytea NOT NULL,
    output_schema_version text NOT NULL,
    schema_hash bytea NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT card_query_binding_specs_input_hash_check CHECK ((octet_length(input_hash) = 32)),
    CONSTRAINT card_query_binding_specs_schema_hash_check CHECK ((octet_length(schema_hash) = 32)),
    CONSTRAINT card_query_binding_specs_tool_id_check CHECK ((tool_id ~ '^[a-z][a-z0-9_.-]+$'::text))
);


--
-- Name: card_query_bindings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.card_query_bindings (
    id uuid NOT NULL,
    binding_ref text NOT NULL,
    presentation_id uuid NOT NULL,
    binding_spec_id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    viewer_user_id uuid NOT NULL,
    authorization_version bigint NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    last_invoked_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT card_query_bindings_authorization_version_check CHECK ((authorization_version > 0)),
    CONSTRAINT card_query_bindings_status_check CHECK ((status = ANY (ARRAY['active'::text, 'expired'::text, 'invalidated'::text])))
);


--
-- Name: card_slot_bindings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.card_slot_bindings (
    id uuid NOT NULL,
    card_version_id uuid NOT NULL,
    slot_name text NOT NULL,
    slot_kind text NOT NULL,
    mode text NOT NULL,
    tool_id text NOT NULL,
    output_schema_version text NOT NULL,
    schema_hash bytea NOT NULL,
    field_path text NOT NULL,
    value_type text NOT NULL,
    semantic_type text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT card_slot_bindings_mode_check CHECK ((mode = ANY (ARRAY['strict'::text, 'preferred'::text]))),
    CONSTRAINT card_slot_bindings_schema_hash_check CHECK ((octet_length(schema_hash) = 32)),
    CONSTRAINT card_slot_bindings_slot_kind_check CHECK ((slot_kind = ANY (ARRAY['data'::text, 'query'::text, 'action'::text]))),
    CONSTRAINT card_slot_bindings_slot_name_check CHECK ((slot_name ~ '^[a-z][a-z0-9_]*$'::text)),
    CONSTRAINT card_slot_bindings_tool_id_check CHECK ((tool_id ~ '^[a-z][a-z0-9_.-]+$'::text)),
    CONSTRAINT card_slot_bindings_value_type_check CHECK ((value_type = ANY (ARRAY['string'::text, 'number'::text, 'boolean'::text, 'array'::text, 'object'::text])))
);


--
-- Name: card_validation_runs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.card_validation_runs (
    id uuid NOT NULL,
    card_version_id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    actor_user_id uuid NOT NULL,
    content_hash bytea NOT NULL,
    runtime_version text NOT NULL,
    nonce_hash bytea NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    required_scenarios text[] NOT NULL,
    passed_scenarios text[] DEFAULT '{}'::text[] NOT NULL,
    issues jsonb DEFAULT '[]'::jsonb NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    completed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT card_validation_runs_content_hash_check CHECK ((octet_length(content_hash) = 32)),
    CONSTRAINT card_validation_runs_nonce_hash_check CHECK ((octet_length(nonce_hash) = 32)),
    CONSTRAINT card_validation_runs_runtime_version_check CHECK (((char_length(runtime_version) >= 1) AND (char_length(runtime_version) <= 128))),
    CONSTRAINT card_validation_runs_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'passed'::text, 'failed'::text, 'expired'::text])))
);


--
-- Name: card_versions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.card_versions (
    id uuid NOT NULL,
    card_id uuid NOT NULL,
    revision integer NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    manifest jsonb NOT NULL,
    entrypoint_html bytea NOT NULL,
    content_hash bytea NOT NULL,
    manifest_hash bytea NOT NULL,
    created_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT card_versions_content_hash_check CHECK ((octet_length(content_hash) = 32)),
    CONSTRAINT card_versions_entrypoint_html_check CHECK ((octet_length(entrypoint_html) <= 524288)),
    CONSTRAINT card_versions_manifest_hash_check CHECK ((octet_length(manifest_hash) = 32)),
    CONSTRAINT card_versions_revision_check CHECK ((revision > 0)),
    CONSTRAINT card_versions_status_check CHECK ((status = ANY (ARRAY['draft'::text, 'validating'::text, 'validated'::text, 'active'::text, 'retired'::text])))
);


--
-- Name: collection_claims; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.collection_claims (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    physical_resource_ref text NOT NULL,
    collector_id uuid NOT NULL,
    profile_id uuid,
    claim_type text NOT NULL,
    signal text NOT NULL,
    selector jsonb DEFAULT '{}'::jsonb NOT NULL,
    selector_hash bytea NOT NULL,
    ownership text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    primary_claim_id uuid,
    rollback_plan jsonb,
    expires_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT collection_claims_check CHECK (((ownership <> 'migration'::text) OR ((primary_claim_id IS NOT NULL) AND (expires_at IS NOT NULL) AND (rollback_plan IS NOT NULL)))),
    CONSTRAINT collection_claims_claim_type_check CHECK (((char_length(claim_type) >= 1) AND (char_length(claim_type) <= 128))),
    CONSTRAINT collection_claims_ownership_check CHECK ((ownership = ANY (ARRAY['primary'::text, 'supplemental'::text, 'migration'::text]))),
    CONSTRAINT collection_claims_physical_resource_ref_check CHECK (((char_length(physical_resource_ref) >= 1) AND (char_length(physical_resource_ref) <= 256))),
    CONSTRAINT collection_claims_selector_check CHECK ((jsonb_typeof(selector) = 'object'::text)),
    CONSTRAINT collection_claims_selector_hash_check CHECK ((octet_length(selector_hash) = 32)),
    CONSTRAINT collection_claims_signal_check CHECK ((signal = ANY (ARRAY['metrics'::text, 'logs'::text, 'traces'::text]))),
    CONSTRAINT collection_claims_status_check CHECK ((status = ANY (ARRAY['active'::text, 'released'::text, 'conflict'::text, 'expired'::text])))
);


--
-- Name: collection_profiles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.collection_profiles (
    id uuid NOT NULL,
    profile_key text NOT NULL,
    version text NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    signals text[] NOT NULL,
    required_components text[] DEFAULT '{}'::text[] NOT NULL,
    supported_platforms text[] NOT NULL,
    claim_types text[] DEFAULT '{}'::text[] NOT NULL,
    config_schema_version text NOT NULL,
    support_status text NOT NULL,
    catalog_revision integer NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT collection_profiles_catalog_revision_check CHECK ((catalog_revision > 0)),
    CONSTRAINT collection_profiles_config_schema_version_check CHECK (((char_length(config_schema_version) >= 1) AND (char_length(config_schema_version) <= 64))),
    CONSTRAINT collection_profiles_description_check CHECK ((char_length(description) <= 1024)),
    CONSTRAINT collection_profiles_name_check CHECK (((char_length(name) >= 1) AND (char_length(name) <= 128))),
    CONSTRAINT collection_profiles_profile_key_check CHECK ((profile_key ~ '^[a-z][a-z0-9-]{0,62}$'::text)),
    CONSTRAINT collection_profiles_signals_check CHECK (((cardinality(signals) > 0) AND (signals <@ ARRAY['metrics'::text, 'logs'::text, 'traces'::text]))),
    CONSTRAINT collection_profiles_support_status_check CHECK ((support_status = ANY (ARRAY['supported'::text, 'validation_pending'::text, 'retired'::text]))),
    CONSTRAINT collection_profiles_supported_platforms_check CHECK (((cardinality(supported_platforms) > 0) AND (supported_platforms <@ ARRAY['linux_arm64'::text, 'linux_amd64'::text, 'windows_amd64'::text]))),
    CONSTRAINT collection_profiles_version_check CHECK (((char_length(version) >= 1) AND (char_length(version) <= 64)))
);


--
-- Name: collector_config_revisions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.collector_config_revisions (
    id uuid NOT NULL,
    collector_id uuid NOT NULL,
    revision bigint NOT NULL,
    profile_ids uuid[] NOT NULL,
    rendered_config jsonb NOT NULL,
    config_hash bytea NOT NULL,
    status text DEFAULT 'prepared'::text NOT NULL,
    failure_code text,
    rollback_revision bigint,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    applied_at timestamp with time zone,
    CONSTRAINT collector_config_revisions_config_hash_check CHECK ((octet_length(config_hash) = 32)),
    CONSTRAINT collector_config_revisions_profile_ids_check CHECK ((cardinality(profile_ids) > 0)),
    CONSTRAINT collector_config_revisions_rendered_config_check CHECK ((jsonb_typeof(rendered_config) = 'object'::text)),
    CONSTRAINT collector_config_revisions_revision_check CHECK ((revision > 0)),
    CONSTRAINT collector_config_revisions_status_check CHECK ((status = ANY (ARRAY['prepared'::text, 'applying'::text, 'effective'::text, 'failed'::text, 'rolled_back'::text, 'superseded'::text])))
);


--
-- Name: collector_distribution_versions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.collector_distribution_versions (
    id uuid NOT NULL,
    name text NOT NULL,
    version text NOT NULL,
    collector_version text NOT NULL,
    config_schema_version text NOT NULL,
    support_status text NOT NULL,
    components text[] DEFAULT '{}'::text[] NOT NULL,
    artifact_manifest jsonb NOT NULL,
    catalog_revision integer NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT collector_distribution_versions_artifact_manifest_check CHECK ((jsonb_typeof(artifact_manifest) = 'array'::text)),
    CONSTRAINT collector_distribution_versions_catalog_revision_check CHECK ((catalog_revision > 0)),
    CONSTRAINT collector_distribution_versions_collector_version_check CHECK (((char_length(collector_version) >= 1) AND (char_length(collector_version) <= 64))),
    CONSTRAINT collector_distribution_versions_config_schema_version_check CHECK (((char_length(config_schema_version) >= 1) AND (char_length(config_schema_version) <= 64))),
    CONSTRAINT collector_distribution_versions_name_check CHECK (((char_length(name) >= 1) AND (char_length(name) <= 128))),
    CONSTRAINT collector_distribution_versions_support_status_check CHECK ((support_status = ANY (ARRAY['supported'::text, 'validation_pending'::text, 'retired'::text]))),
    CONSTRAINT collector_distribution_versions_version_check CHECK (((char_length(version) >= 1) AND (char_length(version) <= 64)))
);


--
-- Name: collector_instances; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.collector_instances (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    resource_type text NOT NULL,
    resource_id uuid NOT NULL,
    distribution_version_id uuid NOT NULL,
    platform text NOT NULL,
    role text NOT NULL,
    status text DEFAULT 'pending_install'::text NOT NULL,
    desired_revision bigint DEFAULT 0 NOT NULL,
    effective_revision bigint DEFAULT 0 NOT NULL,
    authorization_version bigint DEFAULT 1 NOT NULL,
    last_seen_at timestamp with time zone,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT collector_instances_authorization_version_check CHECK ((authorization_version > 0)),
    CONSTRAINT collector_instances_desired_revision_check CHECK ((desired_revision >= 0)),
    CONSTRAINT collector_instances_effective_revision_check CHECK ((effective_revision >= 0)),
    CONSTRAINT collector_instances_platform_check CHECK ((platform = ANY (ARRAY['linux_arm64'::text, 'linux_amd64'::text, 'windows_amd64'::text]))),
    CONSTRAINT collector_instances_resource_type_check CHECK ((resource_type = ANY (ARRAY['host'::text, 'kubernetes_cluster'::text]))),
    CONSTRAINT collector_instances_role_check CHECK ((role = ANY (ARRAY['direct'::text, 'leaf'::text, 'edge_gateway'::text, 'daemonset'::text, 'kubernetes_gateway'::text]))),
    CONSTRAINT collector_instances_status_check CHECK ((status = ANY (ARRAY['pending_install'::text, 'installing'::text, 'converged'::text, 'degraded'::text, 'backlog'::text, 'result_unknown'::text, 'uninstalling'::text, 'uninstalled'::text]))),
    CONSTRAINT collector_instances_version_check CHECK ((version > 0))
);


--
-- Name: connection_tests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.connection_tests (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    target_type text NOT NULL,
    resource_id uuid,
    path text NOT NULL,
    connector_id uuid,
    connection_epoch bigint,
    credential_id uuid,
    credential_version bigint,
    request_plan jsonb NOT NULL,
    request_hash bytea NOT NULL,
    status text DEFAULT 'queued'::text NOT NULL,
    result jsonb DEFAULT '{}'::jsonb NOT NULL,
    error_code text,
    expires_at timestamp with time zone NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT connection_tests_path_check CHECK ((path = ANY (ARRAY['connector'::text, 'direct'::text, 'in_cluster'::text]))),
    CONSTRAINT connection_tests_request_hash_check CHECK ((octet_length(request_hash) = 32)),
    CONSTRAINT connection_tests_status_check CHECK ((status = ANY (ARRAY['queued'::text, 'running'::text, 'succeeded'::text, 'failed'::text, 'result_unknown'::text, 'expired'::text]))),
    CONSTRAINT connection_tests_target_type_check CHECK ((target_type = ANY (ARRAY['host'::text, 'kubernetes_cluster'::text])))
);


--
-- Name: connector_certificates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.connector_certificates (
    id uuid NOT NULL,
    connector_id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    serial_number text NOT NULL,
    issuer_generation integer NOT NULL,
    certificate_request_name text NOT NULL,
    certificate_pem text NOT NULL,
    ca_bundle_pem text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    not_before timestamp with time zone NOT NULL,
    not_after timestamp with time zone NOT NULL,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT connector_certificates_issuer_generation_check CHECK ((issuer_generation > 0)),
    CONSTRAINT connector_certificates_status_check CHECK ((status = ANY (ARRAY['active'::text, 'overlap'::text, 'revoked'::text, 'expired'::text])))
);


--
-- Name: connector_commands; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.connector_commands (
    id uuid NOT NULL,
    command_id text NOT NULL,
    enterprise_id uuid NOT NULL,
    connector_id uuid NOT NULL,
    connection_epoch bigint NOT NULL,
    operation_ref text NOT NULL,
    credential_lease_id uuid,
    command_type text NOT NULL,
    payload_schema_version text NOT NULL,
    payload jsonb NOT NULL,
    payload_hash bytea NOT NULL,
    idempotency_key text NOT NULL,
    status text DEFAULT 'queued'::text NOT NULL,
    result jsonb DEFAULT '{}'::jsonb NOT NULL,
    result_hash bytea,
    error_code text,
    expires_at timestamp with time zone NOT NULL,
    acknowledged_at timestamp with time zone,
    started_at timestamp with time zone,
    completed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT connector_commands_command_type_check CHECK ((command_type = ANY (ARRAY['host_connection_probe'::text, 'kubernetes_connection_probe'::text, 'kubernetes_resource_query'::text, 'kubernetes_pod_logs'::text, 'connector_uninstall'::text, 'collector_management'::text, 'host_connector_install'::text, 'host_connector_removal'::text, 'host_windows_rdp_configure'::text]))),
    CONSTRAINT connector_commands_connection_epoch_check CHECK ((connection_epoch > 0)),
    CONSTRAINT connector_commands_payload_hash_check CHECK ((octet_length(payload_hash) = 32)),
    CONSTRAINT connector_commands_status_check CHECK ((status = ANY (ARRAY['queued'::text, 'dispatched'::text, 'acknowledged'::text, 'running'::text, 'succeeded'::text, 'failed'::text, 'timed_out'::text, 'delivery_unknown'::text, 'result_unknown'::text, 'expired'::text])))
);


--
-- Name: connector_control_tunnels; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.connector_control_tunnels (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    connector_id uuid NOT NULL,
    bastion_scope_id uuid,
    host_id uuid NOT NULL,
    credential_id uuid NOT NULL,
    credential_version bigint NOT NULL,
    target_address text NOT NULL,
    target_port integer NOT NULL,
    target_username text NOT NULL,
    pinned_host_key text NOT NULL,
    enroll_forward_target text NOT NULL,
    gateway_forward_target text NOT NULL,
    status text DEFAULT 'desired'::text NOT NULL,
    epoch bigint DEFAULT 0 NOT NULL,
    fence bigint DEFAULT 0 NOT NULL,
    lease_owner text DEFAULT ''::text NOT NULL,
    lease_expires_at timestamp with time zone,
    last_claim_at timestamp with time zone,
    last_established_at timestamp with time zone,
    last_heartbeat_at timestamp with time zone,
    last_drop_reason text DEFAULT ''::text NOT NULL,
    reconnect_attempt integer DEFAULT 0 NOT NULL,
    next_claim_at timestamp with time zone,
    bytes_relayed bigint DEFAULT 0 NOT NULL,
    throttled_events bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT connector_control_tunnels_bytes_relayed_check CHECK ((bytes_relayed >= 0)),
    CONSTRAINT connector_control_tunnels_credential_version_check CHECK ((credential_version > 0)),
    CONSTRAINT connector_control_tunnels_enroll_forward_target_check CHECK (((char_length(enroll_forward_target) >= 1) AND (char_length(enroll_forward_target) <= 256))),
    CONSTRAINT connector_control_tunnels_epoch_check CHECK ((epoch >= 0)),
    CONSTRAINT connector_control_tunnels_fence_check CHECK ((fence >= 0)),
    CONSTRAINT connector_control_tunnels_gateway_forward_target_check CHECK (((char_length(gateway_forward_target) >= 1) AND (char_length(gateway_forward_target) <= 256))),
    CONSTRAINT connector_control_tunnels_pinned_host_key_check CHECK (((char_length(pinned_host_key) >= 1) AND (char_length(pinned_host_key) <= 512))),
    CONSTRAINT connector_control_tunnels_reconnect_attempt_check CHECK (((reconnect_attempt >= 0) AND (reconnect_attempt <= 30))),
    CONSTRAINT connector_control_tunnels_status_check CHECK ((status = ANY (ARRAY['desired'::text, 'establishing'::text, 'established'::text, 'degraded'::text, 'down'::text, 'removed'::text]))),
    CONSTRAINT connector_control_tunnels_target_address_check CHECK (((char_length(target_address) >= 1) AND (char_length(target_address) <= 512))),
    CONSTRAINT connector_control_tunnels_target_port_check CHECK (((target_port >= 1) AND (target_port <= 65535))),
    CONSTRAINT connector_control_tunnels_target_username_check CHECK (((char_length(target_username) >= 1) AND (char_length(target_username) <= 256))),
    CONSTRAINT connector_control_tunnels_throttled_events_check CHECK ((throttled_events >= 0))
);


--
-- Name: connector_enrollment_tokens; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.connector_enrollment_tokens (
    id uuid NOT NULL,
    preallocated_connector_id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    role text NOT NULL,
    purpose text NOT NULL,
    bastion_scope_id uuid,
    kubernetes_cluster_id uuid,
    preallocated_host_id uuid,
    token_hash bytea NOT NULL,
    policy jsonb DEFAULT '{}'::jsonb NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    consumed_at timestamp with time zone,
    consumed_device_hash bytea,
    registered_connector_id uuid,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    release_version_id uuid,
    CONSTRAINT connector_enrollment_tokens_purpose_check CHECK ((purpose = ANY (ARRAY['host_registration'::text, 'initial_registration'::text, 'connector_replacement'::text, 'kubernetes_registration'::text, 'pki_repair'::text]))),
    CONSTRAINT connector_enrollment_tokens_role_check CHECK ((role = ANY (ARRAY['host'::text, 'bastion'::text, 'kubernetes'::text]))),
    CONSTRAINT connector_enrollment_tokens_status_check CHECK ((status = ANY (ARRAY['active'::text, 'consumed'::text, 'revoked'::text, 'expired'::text]))),
    CONSTRAINT connector_enrollment_tokens_subject_check CHECK ((((role = 'host'::text) AND (preallocated_host_id IS NOT NULL) AND (kubernetes_cluster_id IS NULL)) OR ((role = 'bastion'::text) AND (bastion_scope_id IS NOT NULL) AND (preallocated_host_id IS NOT NULL) AND (kubernetes_cluster_id IS NULL)) OR ((role = 'kubernetes'::text) AND (bastion_scope_id IS NULL) AND (preallocated_host_id IS NULL) AND (kubernetes_cluster_id IS NOT NULL)))),
    CONSTRAINT connector_enrollment_tokens_token_hash_check CHECK ((octet_length(token_hash) = 32))
);


--
-- Name: connector_install_operation_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.connector_install_operation_events (
    id uuid NOT NULL,
    operation_id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    sequence bigint NOT NULL,
    stage text NOT NULL,
    status text NOT NULL,
    error_code text,
    occurred_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT connector_install_operation_events_sequence_check CHECK ((sequence > 0)),
    CONSTRAINT connector_install_operation_events_stage_check CHECK ((stage = ANY (ARRAY['queued'::text, 'ssh_connecting'::text, 'artifact_verifying'::text, 'artifact_transferring'::text, 'service_installing'::text, 'control_tunnel_establishing'::text, 'enrolling'::text, 'waiting_connector_online'::text, 'completed'::text]))),
    CONSTRAINT connector_install_operation_events_status_check CHECK ((status = ANY (ARRAY['started'::text, 'succeeded'::text, 'failed'::text, 'retrying'::text])))
);


--
-- Name: connector_install_operation_secrets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.connector_install_operation_secrets (
    operation_id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    key_version integer NOT NULL,
    nonce bytea NOT NULL,
    ciphertext bytea NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    consumed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT connector_install_operation_secrets_key_version_check CHECK ((key_version > 0))
);


--
-- Name: connector_install_operations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.connector_install_operations (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    connector_id uuid NOT NULL,
    bastion_scope_id uuid NOT NULL,
    host_id uuid NOT NULL,
    pending_action_id uuid NOT NULL,
    retry_of uuid,
    release_version_id uuid NOT NULL,
    connection_test_id uuid NOT NULL,
    install_mode text NOT NULL,
    status text DEFAULT 'queued'::text NOT NULL,
    stage text DEFAULT 'queued'::text NOT NULL,
    plan jsonb NOT NULL,
    plan_hash bytea NOT NULL,
    attempts integer DEFAULT 0 NOT NULL,
    lease_owner text DEFAULT ''::text NOT NULL,
    fence bigint DEFAULT 0 NOT NULL,
    lease_expires_at timestamp with time zone,
    error_code text,
    connector_online_at timestamp with time zone,
    expires_at timestamp with time zone NOT NULL,
    completed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT connector_install_operations_attempts_check CHECK (((attempts >= 0) AND (attempts <= 3))),
    CONSTRAINT connector_install_operations_fence_check CHECK ((fence >= 0)),
    CONSTRAINT connector_install_operations_install_mode_check CHECK ((install_mode = ANY (ARRAY['direct_install'::text, 'direct_install_tunnel'::text]))),
    CONSTRAINT connector_install_operations_plan_check CHECK ((jsonb_typeof(plan) = 'object'::text)),
    CONSTRAINT connector_install_operations_plan_hash_check CHECK ((octet_length(plan_hash) = 32)),
    CONSTRAINT connector_install_operations_stage_check CHECK ((stage = ANY (ARRAY['queued'::text, 'ssh_connecting'::text, 'artifact_verifying'::text, 'artifact_transferring'::text, 'service_installing'::text, 'control_tunnel_establishing'::text, 'enrolling'::text, 'waiting_connector_online'::text, 'completed'::text]))),
    CONSTRAINT connector_install_operations_status_check CHECK ((status = ANY (ARRAY['queued'::text, 'running'::text, 'succeeded'::text, 'failed'::text, 'result_unknown'::text, 'expired'::text, 'cancelled'::text])))
);


--
-- Name: connector_release_versions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.connector_release_versions (
    id uuid NOT NULL,
    version text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    manifest jsonb NOT NULL,
    manifest_hash bytea NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT connector_release_versions_manifest_check CHECK ((jsonb_typeof(manifest) = 'object'::text)),
    CONSTRAINT connector_release_versions_manifest_hash_check CHECK ((octet_length(manifest_hash) = 32)),
    CONSTRAINT connector_release_versions_status_check CHECK ((status = ANY (ARRAY['active'::text, 'retired'::text]))),
    CONSTRAINT connector_release_versions_version_check CHECK (((char_length(version) >= 1) AND (char_length(version) <= 128)))
);


--
-- Name: connector_sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.connector_sessions (
    connector_id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    gateway_instance_id text NOT NULL,
    connection_epoch bigint NOT NULL,
    capabilities text[] DEFAULT '{}'::text[] NOT NULL,
    connected_at timestamp with time zone NOT NULL,
    last_heartbeat_at timestamp with time zone NOT NULL,
    draining boolean DEFAULT false NOT NULL,
    CONSTRAINT connector_sessions_connection_epoch_check CHECK ((connection_epoch > 0))
);


--
-- Name: connectors; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.connectors (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    role text NOT NULL,
    name text NOT NULL,
    host_id uuid,
    bastion_scope_id uuid,
    kubernetes_cluster_id uuid,
    instance_id text NOT NULL,
    device_fingerprint_hash bytea NOT NULL,
    public_key_hash bytea NOT NULL,
    software_version text DEFAULT ''::text NOT NULL,
    capabilities text[] DEFAULT '{}'::text[] NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    connection_epoch bigint DEFAULT 0 NOT NULL,
    certificate_expires_at timestamp with time zone NOT NULL,
    certificate_rotation_requested_at timestamp with time zone,
    connected_at timestamp with time zone,
    last_heartbeat_at timestamp with time zone,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT connectors_connection_epoch_check CHECK ((connection_epoch >= 0)),
    CONSTRAINT connectors_device_fingerprint_hash_check CHECK ((octet_length(device_fingerprint_hash) = 32)),
    CONSTRAINT connectors_name_check CHECK (((char_length(name) >= 1) AND (char_length(name) <= 128))),
    CONSTRAINT connectors_public_key_hash_check CHECK ((octet_length(public_key_hash) = 32)),
    CONSTRAINT connectors_role_check CHECK ((role = ANY (ARRAY['host'::text, 'bastion'::text, 'kubernetes'::text]))),
    CONSTRAINT connectors_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'online'::text, 'suspected_offline'::text, 'offline'::text, 'draining'::text, 'uninstalled'::text, 'revoked'::text]))),
    CONSTRAINT connectors_subject_check CHECK ((((role = ANY (ARRAY['host'::text, 'bastion'::text])) AND (host_id IS NOT NULL) AND (kubernetes_cluster_id IS NULL)) OR ((role = 'kubernetes'::text) AND (host_id IS NULL) AND (bastion_scope_id IS NULL) AND (kubernetes_cluster_id IS NOT NULL)))),
    CONSTRAINT connectors_version_check CHECK ((version > 0))
);


--
-- Name: context_snapshots; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.context_snapshots (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    conversation_id uuid NOT NULL,
    run_id uuid NOT NULL,
    revision integer NOT NULL,
    source_from_sequence bigint NOT NULL,
    source_through_sequence bigint NOT NULL,
    first_kept_sequence bigint NOT NULL,
    typed_checkpoint jsonb NOT NULL,
    narrative_summary text NOT NULL,
    compaction_model_id uuid NOT NULL,
    compaction_model_revision integer NOT NULL,
    prompt_version text NOT NULL,
    estimated_tokens_before integer NOT NULL,
    actual_tokens_after integer NOT NULL,
    source_hash bytea NOT NULL,
    snapshot_hash bytea NOT NULL,
    status text NOT NULL,
    error_code text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT context_snapshots_actual_tokens_after_check CHECK ((actual_tokens_after >= 0)),
    CONSTRAINT context_snapshots_check CHECK ((source_through_sequence >= source_from_sequence)),
    CONSTRAINT context_snapshots_check1 CHECK ((first_kept_sequence > source_through_sequence)),
    CONSTRAINT context_snapshots_compaction_model_revision_check CHECK ((compaction_model_revision > 0)),
    CONSTRAINT context_snapshots_estimated_tokens_before_check CHECK ((estimated_tokens_before >= 0)),
    CONSTRAINT context_snapshots_revision_check CHECK ((revision > 0)),
    CONSTRAINT context_snapshots_snapshot_hash_check CHECK ((octet_length(snapshot_hash) = 32)),
    CONSTRAINT context_snapshots_source_from_sequence_check CHECK ((source_from_sequence > 0)),
    CONSTRAINT context_snapshots_source_hash_check CHECK ((octet_length(source_hash) = 32)),
    CONSTRAINT context_snapshots_status_check CHECK ((status = ANY (ARRAY['active'::text, 'superseded'::text, 'failed'::text])))
);


--
-- Name: conversation_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.conversation_events (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    conversation_id uuid NOT NULL,
    run_id uuid,
    step_id uuid,
    sequence bigint NOT NULL,
    event_type text NOT NULL,
    actor_type text NOT NULL,
    actor_id text,
    payload jsonb NOT NULL,
    content_hash bytea NOT NULL,
    artifact_ref text,
    data_classification text DEFAULT 'internal'::text NOT NULL,
    occurred_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT conversation_events_actor_type_check CHECK ((actor_type = ANY (ARRAY['user'::text, 'model'::text, 'service'::text, 'worker'::text, 'system'::text]))),
    CONSTRAINT conversation_events_content_hash_check CHECK ((octet_length(content_hash) = 32)),
    CONSTRAINT conversation_events_data_classification_check CHECK ((data_classification = ANY (ARRAY['public'::text, 'internal'::text, 'sensitive'::text]))),
    CONSTRAINT conversation_events_event_type_check CHECK ((event_type = ANY (ARRAY['user_message'::text, 'assistant_message'::text, 'model_usage'::text, 'tool_call_requested'::text, 'tool_call_started'::text, 'tool_call_result'::text, 'pending_action_created'::text, 'user_confirmation'::text, 'approval_update'::text, 'execution_update'::text, 'card_draft_created'::text, 'card_instance_created'::text, 'card_presentation_invalidated'::text, 'card_action_result'::text, 'run_state_changed'::text, 'context_compacted'::text, 'agent_delta'::text]))),
    CONSTRAINT conversation_events_sequence_check CHECK ((sequence > 0))
);


--
-- Name: conversations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.conversations (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    owner_user_id uuid NOT NULL,
    title text NOT NULL,
    selected_model_id uuid NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT conversations_status_check CHECK ((status = ANY (ARRAY['active'::text, 'archived'::text]))),
    CONSTRAINT conversations_title_check CHECK (((char_length(title) >= 1) AND (char_length(title) <= 256))),
    CONSTRAINT conversations_version_check CHECK ((version > 0))
);


--
-- Name: credential_leases; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.credential_leases (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    credential_id uuid NOT NULL,
    secret_version_id uuid NOT NULL,
    operation_ref text NOT NULL,
    target_resource_type text NOT NULL,
    target_resource_id uuid NOT NULL,
    recipient_type text NOT NULL,
    recipient_id text NOT NULL,
    protocol text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    consumed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT credential_leases_protocol_check CHECK ((protocol = ANY (ARRAY['ssh'::text, 'windows'::text, 'kubernetes'::text, 'http'::text]))),
    CONSTRAINT credential_leases_recipient_type_check CHECK ((recipient_type = ANY (ARRAY['connector'::text, 'direct_executor'::text, 'connector_gateway'::text]))),
    CONSTRAINT credential_leases_status_check CHECK ((status = ANY (ARRAY['active'::text, 'consumed'::text, 'expired'::text, 'revoked'::text])))
);


--
-- Name: credentials; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.credentials (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    name text NOT NULL,
    protocol text NOT NULL,
    username text DEFAULT ''::text NOT NULL,
    secret_id uuid NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT credentials_name_check CHECK (((char_length(name) >= 1) AND (char_length(name) <= 128))),
    CONSTRAINT credentials_protocol_check CHECK ((protocol = ANY (ARRAY['ssh'::text, 'windows'::text, 'kubernetes'::text, 'http'::text]))),
    CONSTRAINT credentials_status_check CHECK ((status = ANY (ARRAY['active'::text, 'disabled'::text]))),
    CONSTRAINT credentials_version_check CHECK ((version > 0))
);


--
-- Name: data_authorization_grants; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.data_authorization_grants (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    subject_type text NOT NULL,
    subject_id uuid NOT NULL,
    resource_type text NOT NULL,
    resource_id uuid NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT data_authorization_grants_resource_type_check CHECK ((resource_type = ANY (ARRAY['host'::text, 'kubernetes_cluster'::text]))),
    CONSTRAINT data_authorization_grants_status_check CHECK ((status = ANY (ARRAY['active'::text, 'disabled'::text]))),
    CONSTRAINT data_authorization_grants_subject_type_check CHECK ((subject_type = ANY (ARRAY['user'::text, 'department'::text, 'role'::text, 'service_account'::text]))),
    CONSTRAINT data_authorization_grants_version_check CHECK ((version > 0))
);


--
-- Name: departments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.departments (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT departments_name_check CHECK (((char_length(name) >= 1) AND (char_length(name) <= 128))),
    CONSTRAINT departments_status_check CHECK ((status = ANY (ARRAY['active'::text, 'disabled'::text]))),
    CONSTRAINT departments_version_check CHECK ((version > 0))
);


--
-- Name: enterprise_telemetry_tables; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.enterprise_telemetry_tables (
    enterprise_id uuid NOT NULL,
    schema_version integer NOT NULL,
    status text NOT NULL,
    ready_at timestamp with time zone,
    last_error text,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT enterprise_telemetry_tables_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'ready'::text, 'deleting'::text, 'error'::text])))
);


--
-- Name: enterprise_users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.enterprise_users (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    department_id uuid NOT NULL,
    username text NOT NULL,
    display_name text NOT NULL,
    email text,
    status text DEFAULT 'active'::text NOT NULL,
    mfa_enabled boolean DEFAULT false NOT NULL,
    authorization_version bigint DEFAULT 1 NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    last_login_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT enterprise_users_authorization_version_check CHECK ((authorization_version > 0)),
    CONSTRAINT enterprise_users_display_name_check CHECK (((char_length(display_name) >= 1) AND (char_length(display_name) <= 128))),
    CONSTRAINT enterprise_users_status_check CHECK ((status = ANY (ARRAY['active'::text, 'disabled'::text]))),
    CONSTRAINT enterprise_users_username_check CHECK (((char_length(username) >= 3) AND (char_length(username) <= 128))),
    CONSTRAINT enterprise_users_version_check CHECK ((version > 0))
);


--
-- Name: enterprises; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.enterprises (
    id uuid NOT NULL,
    name text NOT NULL,
    code text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    timezone text NOT NULL,
    default_locale text DEFAULT 'zh-CN'::text NOT NULL,
    remark text DEFAULT ''::text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT enterprises_code_check CHECK ((((char_length(code) >= 1) AND (char_length(code) <= 63)) AND (code ~ '^[a-z0-9]+(-[a-z0-9]+)*$'::text))),
    CONSTRAINT enterprises_default_locale_check CHECK ((default_locale = ANY (ARRAY['zh-CN'::text, 'en-US'::text]))),
    CONSTRAINT enterprises_name_check CHECK (((char_length(name) >= 1) AND (char_length(name) <= 128))),
    CONSTRAINT enterprises_status_check CHECK ((status = ANY (ARRAY['active'::text, 'suspended'::text, 'disabled'::text]))),
    CONSTRAINT enterprises_version_check CHECK ((version > 0))
);


--
-- Name: execution_one_time_results; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.execution_one_time_results (
    id uuid NOT NULL,
    execution_id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    authorization_version bigint NOT NULL,
    result_kind text NOT NULL,
    key_version integer NOT NULL,
    nonce bytea NOT NULL,
    ciphertext bytea NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    consumed_by_user_id uuid,
    consumed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT execution_one_time_results_authorization_version_check CHECK ((authorization_version > 0)),
    CONSTRAINT execution_one_time_results_check CHECK (((consumed_by_user_id IS NULL) = (consumed_at IS NULL))),
    CONSTRAINT execution_one_time_results_key_version_check CHECK ((key_version > 0)),
    CONSTRAINT execution_one_time_results_result_kind_check CHECK ((result_kind = ANY (ARRAY['connector_install_command'::text, 'host_removal_command'::text])))
);


--
-- Name: executions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.executions (
    id uuid NOT NULL,
    execution_ref text NOT NULL,
    pending_action_id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    run_id uuid,
    status text DEFAULT 'pending'::text NOT NULL,
    idempotency_key text NOT NULL,
    result_ref text,
    connector_command_id uuid,
    error_code text,
    started_at timestamp with time zone,
    completed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    telemetry_collector_operation_id uuid,
    connector_install_operation_id uuid,
    host_onboarding_operation_id uuid,
    host_removal_operation_id uuid,
    CONSTRAINT execution_external_operation CHECK ((num_nonnulls(connector_command_id, telemetry_collector_operation_id, connector_install_operation_id, host_onboarding_operation_id, host_removal_operation_id) <= 1)),
    CONSTRAINT executions_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'running'::text, 'succeeded'::text, 'failed'::text, 'result_unknown'::text, 'cancelled'::text])))
);


--
-- Name: host_onboarding_operation_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_onboarding_operation_events (
    id uuid NOT NULL,
    operation_id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    sequence bigint NOT NULL,
    stage text NOT NULL,
    status text NOT NULL,
    error_code text,
    occurred_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT host_onboarding_operation_events_sequence_check CHECK ((sequence > 0)),
    CONSTRAINT host_onboarding_operation_events_stage_check CHECK ((stage = ANY (ARRAY['queued'::text, 'probing'::text, 'transferring'::text, 'installing'::text, 'enrolling'::text, 'waiting_online'::text, 'completed'::text]))),
    CONSTRAINT host_onboarding_operation_events_status_check CHECK ((status = ANY (ARRAY['started'::text, 'succeeded'::text, 'failed'::text, 'retrying'::text])))
);


--
-- Name: host_onboarding_operation_secrets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_onboarding_operation_secrets (
    operation_id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    key_version integer NOT NULL,
    nonce bytea NOT NULL,
    ciphertext bytea NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    consumed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT host_onboarding_operation_secrets_key_version_check CHECK ((key_version > 0))
);


--
-- Name: host_onboarding_operations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_onboarding_operations (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    host_id uuid NOT NULL,
    connector_id uuid NOT NULL,
    pending_action_id uuid NOT NULL,
    retry_of uuid,
    release_version_id uuid NOT NULL,
    connection_test_id uuid,
    install_method text NOT NULL,
    ssh_path text NOT NULL,
    target_platform text NOT NULL,
    control_path text NOT NULL,
    bastion_scope_id uuid,
    status text DEFAULT 'queued'::text NOT NULL,
    stage text DEFAULT 'queued'::text NOT NULL,
    plan jsonb NOT NULL,
    plan_hash bytea NOT NULL,
    attempts integer DEFAULT 0 NOT NULL,
    lease_owner text DEFAULT ''::text NOT NULL,
    fence bigint DEFAULT 0 NOT NULL,
    lease_expires_at timestamp with time zone,
    error_code text,
    connector_online_at timestamp with time zone,
    expires_at timestamp with time zone NOT NULL,
    completed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT host_onboarding_operations_attempts_check CHECK (((attempts >= 0) AND (attempts <= 3))),
    CONSTRAINT host_onboarding_operations_check CHECK ((((install_method = 'manual'::text) AND (ssh_path = 'none'::text) AND (connection_test_id IS NULL)) OR ((install_method = 'ssh'::text) AND (ssh_path <> 'none'::text) AND (connection_test_id IS NOT NULL)))),
    CONSTRAINT host_onboarding_operations_check1 CHECK ((((control_path = 'bastion_relay'::text) AND (bastion_scope_id IS NOT NULL)) OR ((control_path <> 'bastion_relay'::text) AND (bastion_scope_id IS NULL)))),
    CONSTRAINT host_onboarding_operations_control_path_check CHECK ((control_path = ANY (ARRAY['direct'::text, 'bastion_relay'::text, 'executor_tunnel'::text]))),
    CONSTRAINT host_onboarding_operations_fence_check CHECK ((fence >= 0)),
    CONSTRAINT host_onboarding_operations_install_method_check CHECK ((install_method = ANY (ARRAY['manual'::text, 'ssh'::text]))),
    CONSTRAINT host_onboarding_operations_plan_check CHECK ((jsonb_typeof(plan) = 'object'::text)),
    CONSTRAINT host_onboarding_operations_plan_hash_check CHECK ((octet_length(plan_hash) = 32)),
    CONSTRAINT host_onboarding_operations_ssh_path_check CHECK ((ssh_path = ANY (ARRAY['none'::text, 'direct_executor'::text, 'bastion_connector'::text]))),
    CONSTRAINT host_onboarding_operations_stage_check CHECK ((stage = ANY (ARRAY['queued'::text, 'probing'::text, 'transferring'::text, 'installing'::text, 'enrolling'::text, 'waiting_online'::text, 'completed'::text]))),
    CONSTRAINT host_onboarding_operations_status_check CHECK ((status = ANY (ARRAY['queued'::text, 'running'::text, 'succeeded'::text, 'failed'::text, 'result_unknown'::text, 'expired'::text, 'cancelled'::text]))),
    CONSTRAINT host_onboarding_operations_target_platform_check CHECK ((target_platform = ANY (ARRAY['linux_amd64'::text, 'linux_arm64'::text, 'windows_amd64'::text])))
);


--
-- Name: host_runtime_observations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_runtime_observations (
    host_id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    connector_id uuid NOT NULL,
    platform text NOT NULL,
    openssh_status text NOT NULL,
    rdp_status text NOT NULL,
    rdp_nla_enabled boolean DEFAULT false NOT NULL,
    rdp_firewall_enabled boolean DEFAULT false NOT NULL,
    rdp_service_running boolean DEFAULT false NOT NULL,
    observed_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT host_runtime_observations_openssh_status_check CHECK ((openssh_status = ANY (ARRAY['available'::text, 'unavailable'::text, 'unknown'::text]))),
    CONSTRAINT host_runtime_observations_platform_check CHECK ((platform = ANY (ARRAY['linux'::text, 'windows'::text]))),
    CONSTRAINT host_runtime_observations_rdp_status_check CHECK ((rdp_status = ANY (ARRAY['enabled'::text, 'disabled'::text, 'unavailable'::text, 'unknown'::text])))
);


--
-- Name: hosts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.hosts (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    name text NOT NULL,
    hostname text DEFAULT ''::text NOT NULL,
    address text DEFAULT ''::text,
    port integer DEFAULT 0 NOT NULL,
    platform text NOT NULL,
    bastion_scope_id uuid,
    connector_id uuid,
    environment text NOT NULL,
    labels jsonb DEFAULT '{}'::jsonb NOT NULL,
    labels_hash bytea NOT NULL,
    labels_version bigint DEFAULT 1 NOT NULL,
    resource_version bigint DEFAULT 1 NOT NULL,
    connection_status text DEFAULT 'unknown'::text NOT NULL,
    pinned_host_key text DEFAULT ''::text NOT NULL,
    last_seen_at timestamp with time zone,
    status text DEFAULT 'active'::text NOT NULL,
    deleted_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    architecture text,
    role text NOT NULL,
    control_path text NOT NULL,
    removal_generation bigint DEFAULT 0 NOT NULL,
    local_cleanup text DEFAULT 'verified'::text NOT NULL,
    CONSTRAINT hosts_address_check CHECK (((address IS NULL) OR ((char_length(address) >= 0) AND (char_length(address) <= 512)))),
    CONSTRAINT hosts_connection_status_check CHECK ((connection_status = ANY (ARRAY['online'::text, 'offline'::text, 'onboarding'::text, 'degraded'::text, 'unknown'::text]))),
    CONSTRAINT hosts_control_path_check CHECK ((((role = 'bastion'::text) AND (platform = 'linux'::text) AND (bastion_scope_id IS NOT NULL) AND (control_path = ANY (ARRAY['direct'::text, 'executor_tunnel'::text]))) OR ((role = 'managed_host'::text) AND (control_path = 'bastion_relay'::text) AND (bastion_scope_id IS NOT NULL)) OR ((role = 'managed_host'::text) AND (control_path = ANY (ARRAY['direct'::text, 'executor_tunnel'::text])) AND (bastion_scope_id IS NULL)))),
    CONSTRAINT hosts_environment_check CHECK ((environment = ANY (ARRAY['development'::text, 'staging'::text, 'production'::text]))),
    CONSTRAINT hosts_labels_check CHECK (((jsonb_typeof(labels) = 'object'::text) AND (octet_length((labels)::text) <= 4096))),
    CONSTRAINT hosts_labels_hash_check CHECK ((octet_length(labels_hash) = 32)),
    CONSTRAINT hosts_labels_version_check CHECK ((labels_version > 0)),
    CONSTRAINT hosts_name_check CHECK (((char_length(name) >= 1) AND (char_length(name) <= 128))),
    CONSTRAINT hosts_platform_check CHECK ((platform = ANY (ARRAY['linux'::text, 'windows'::text]))),
    CONSTRAINT hosts_port_check CHECK (((port >= 0) AND (port <= 65535))),
    CONSTRAINT hosts_resource_version_check CHECK ((resource_version > 0)),
    CONSTRAINT hosts_role_check CHECK ((role = ANY (ARRAY['managed_host'::text, 'bastion'::text]))),
    CONSTRAINT hosts_removal_generation_check CHECK ((removal_generation >= 0)),
    CONSTRAINT hosts_local_cleanup_check CHECK ((local_cleanup = ANY (ARRAY['verified'::text, 'pending'::text, 'unknown'::text]))),
    CONSTRAINT hosts_status_check CHECK ((status = ANY (ARRAY['active'::text, 'disabled'::text, 'draining'::text, 'uninstalling'::text, 'uninstalled'::text, 'removal_failed'::text, 'cleanup_unknown'::text, 'deleted'::text])))
);


--
-- Name: host_removal_operations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.host_removal_operations (
    id uuid PRIMARY KEY,
    enterprise_id uuid NOT NULL,
    pending_action_id uuid NOT NULL,
    target_type text NOT NULL,
    host_id uuid NOT NULL,
    bastion_scope_id uuid,
    connector_id uuid NOT NULL,
    removal_mode text DEFAULT 'uninstall'::text NOT NULL,
    delivery_method text NOT NULL,
    ssh_path text NOT NULL,
    target_platform text NOT NULL,
    control_path text NOT NULL,
    connection_test_id uuid,
    credential_id uuid,
    credential_version bigint,
    pinned_host_key text DEFAULT ''::text NOT NULL,
    resource_version bigint NOT NULL,
    connector_version bigint NOT NULL,
    connection_epoch bigint NOT NULL,
    removal_generation bigint NOT NULL,
    trust_bundle_epoch bigint NOT NULL,
    plan jsonb NOT NULL,
    plan_hash bytea NOT NULL,
    status text DEFAULT 'queued'::text NOT NULL,
    stage text DEFAULT 'queued'::text NOT NULL,
    attempts integer DEFAULT 0 NOT NULL,
    lease_owner text DEFAULT ''::text NOT NULL,
    lease_expires_at timestamp with time zone,
    error_code text,
    local_cleanup text DEFAULT 'pending'::text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    completed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT host_removal_operations_id_enterprise_key UNIQUE (id, enterprise_id),
    CONSTRAINT host_removal_operations_pending_action_key UNIQUE (pending_action_id),
    CONSTRAINT host_removal_operations_target_type_check CHECK ((target_type = ANY (ARRAY['managed_host'::text, 'bastion_scope'::text]))),
    CONSTRAINT host_removal_operations_removal_mode_check CHECK ((removal_mode = ANY (ARRAY['uninstall'::text, 'forget'::text]))),
    CONSTRAINT host_removal_operations_delivery_method_check CHECK ((delivery_method = ANY (ARRAY['manual'::text, 'ssh'::text, 'server_only'::text]))),
    CONSTRAINT host_removal_operations_ssh_path_check CHECK ((ssh_path = ANY (ARRAY['none'::text, 'direct_executor'::text, 'bastion_connector'::text]))),
    CONSTRAINT host_removal_operations_target_platform_check CHECK ((target_platform = ANY (ARRAY['linux_amd64'::text, 'linux_arm64'::text, 'windows_amd64'::text]))),
    CONSTRAINT host_removal_operations_control_path_check CHECK ((control_path = ANY (ARRAY['direct'::text, 'bastion_relay'::text, 'executor_tunnel'::text]))),
    CONSTRAINT host_removal_operations_status_check CHECK ((status = ANY (ARRAY['queued'::text, 'running'::text, 'awaiting_manual_execution'::text, 'succeeded'::text, 'failed'::text, 'cleanup_unknown'::text]))),
    CONSTRAINT host_removal_operations_stage_check CHECK ((stage = ANY (ARRAY['queued'::text, 'draining'::text, 'terminating_sessions'::text, 'awaiting_manual_execution'::text, 'uninstalling_workloads'::text, 'stopping_relay'::text, 'uninstalling_connector'::text, 'verifying_cleanup'::text, 'revoking_identities'::text, 'completed'::text]))),
    CONSTRAINT host_removal_operations_local_cleanup_check CHECK ((local_cleanup = ANY (ARRAY['pending'::text, 'verified'::text, 'unknown'::text]))),
    CONSTRAINT host_removal_operations_plan_check CHECK ((jsonb_typeof(plan) = 'object'::text)),
    CONSTRAINT host_removal_operations_plan_hash_check CHECK ((octet_length(plan_hash) = 32)),
    CONSTRAINT host_removal_operations_attempts_check CHECK (((attempts >= 0) AND (attempts <= 10))),
    CONSTRAINT host_removal_operations_version_check CHECK ((resource_version > 0 AND connector_version > 0 AND connection_epoch > 0 AND removal_generation > 0 AND trust_bundle_epoch > 0)),
    CONSTRAINT host_removal_operations_target_check CHECK (((target_type <> 'bastion_scope'::text) OR (bastion_scope_id IS NOT NULL))),
    CONSTRAINT host_removal_operations_delivery_check CHECK ((((delivery_method = 'manual'::text) AND (ssh_path = 'none'::text) AND (connection_test_id IS NULL)) OR ((delivery_method = 'ssh'::text) AND (ssh_path <> 'none'::text) AND (connection_test_id IS NOT NULL) AND (credential_id IS NOT NULL) AND (credential_version IS NOT NULL)) OR ((delivery_method = 'server_only'::text) AND (removal_mode = 'forget'::text) AND (ssh_path = 'none'::text))))
);

CREATE TABLE public.host_removal_operation_steps (
    operation_id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    stage text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    attempt integer DEFAULT 0 NOT NULL,
    postcondition jsonb DEFAULT '{}'::jsonb NOT NULL,
    result_hash bytea,
    error_code text,
    started_at timestamp with time zone,
    completed_at timestamp with time zone,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT host_removal_operation_steps_pkey PRIMARY KEY (operation_id, stage),
    CONSTRAINT host_removal_operation_steps_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'running'::text, 'succeeded'::text, 'failed'::text, 'unknown'::text]))),
    CONSTRAINT host_removal_operation_steps_attempt_check CHECK ((attempt >= 0)),
    CONSTRAINT host_removal_operation_steps_postcondition_check CHECK ((jsonb_typeof(postcondition) = 'object'::text))
);

CREATE TABLE public.host_removal_operation_events (
    id uuid PRIMARY KEY,
    operation_id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    sequence bigint NOT NULL,
    stage text NOT NULL,
    status text NOT NULL,
    error_code text,
    occurred_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT host_removal_operation_events_operation_sequence_key UNIQUE (operation_id, sequence),
    CONSTRAINT host_removal_operation_events_sequence_check CHECK ((sequence > 0)),
    CONSTRAINT host_removal_operation_events_status_check CHECK ((status = ANY (ARRAY['started'::text, 'succeeded'::text, 'failed'::text, 'unknown'::text, 'resumed'::text])))
);

CREATE TABLE public.host_removal_tokens (
    id uuid PRIMARY KEY,
    operation_id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    purpose text NOT NULL,
    token_hash bytea NOT NULL,
    key_version integer NOT NULL,
    nonce bytea NOT NULL,
    ciphertext bytea NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    consumed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT host_removal_tokens_token_hash_key UNIQUE (token_hash),
    CONSTRAINT host_removal_tokens_purpose_check CHECK ((purpose = ANY (ARRAY['bootstrap'::text, 'resume'::text, 'receipt'::text]))),
    CONSTRAINT host_removal_tokens_status_check CHECK ((status = ANY (ARRAY['active'::text, 'consumed'::text, 'revoked'::text, 'expired'::text]))),
    CONSTRAINT host_removal_tokens_hash_check CHECK ((octet_length(token_hash) = 32)),
    CONSTRAINT host_removal_tokens_key_version_check CHECK ((key_version > 0))
);

CREATE TABLE public.host_managed_change_journals (
    id uuid PRIMARY KEY,
    enterprise_id uuid NOT NULL,
    host_id uuid NOT NULL,
    connector_id uuid NOT NULL,
    change_type text NOT NULL,
    before_state jsonb NOT NULL,
    applied_state jsonb NOT NULL,
    state_hash bytea NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    restored_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT host_managed_change_journals_change_type_check CHECK ((change_type = 'windows_rdp'::text)),
    CONSTRAINT host_managed_change_journals_state_check CHECK ((jsonb_typeof(before_state) = 'object'::text AND jsonb_typeof(applied_state) = 'object'::text)),
    CONSTRAINT host_managed_change_journals_hash_check CHECK ((octet_length(state_hash) = 32)),
    CONSTRAINT host_managed_change_journals_status_check CHECK ((status = ANY (ARRAY['active'::text, 'restored'::text, 'drifted'::text])))
);


--
-- Name: idempotency_records; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.idempotency_records (
    audience text NOT NULL,
    subject_id text NOT NULL,
    operation text NOT NULL,
    idempotency_key text NOT NULL,
    request_hash bytea NOT NULL,
    response_status integer,
    response_nonce bytea,
    response_ciphertext bytea,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    response_provider text,
    response_key_id text,
    response_key_version integer,
    CONSTRAINT idempotency_records_audience_check CHECK ((audience = ANY (ARRAY['setup'::text, 'platform'::text, 'enterprise'::text, 'api_key'::text]))),
    CONSTRAINT idempotency_records_provider_check CHECK (((response_provider IS NULL) OR (response_provider = ANY (ARRAY['local_test'::text, 'openbao_transit'::text])))),
    CONSTRAINT idempotency_records_request_hash_check CHECK ((octet_length(request_hash) = 32)),
    CONSTRAINT idempotency_records_response_check CHECK ((((response_ciphertext IS NULL) AND (response_nonce IS NULL) AND (response_status IS NULL) AND (response_provider IS NULL) AND (response_key_id IS NULL) AND (response_key_version IS NULL)) OR ((response_ciphertext IS NOT NULL) AND (response_status IS NOT NULL) AND (((response_provider IS NULL) AND (response_nonce IS NOT NULL) AND (response_key_id IS NULL) AND (response_key_version IS NULL)) OR ((response_provider IS NOT NULL) AND (response_nonce IS NULL) AND (response_key_id IS NOT NULL) AND (response_key_version > 0))))))
);


--
-- Name: interactive_cards; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.interactive_cards (
    id uuid NOT NULL,
    enterprise_id uuid,
    source text NOT NULL,
    slug text NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    lifecycle text DEFAULT 'draft'::text NOT NULL,
    enabled boolean DEFAULT false NOT NULL,
    availability text DEFAULT 'disabled'::text NOT NULL,
    active_version_id uuid,
    latest_revision integer DEFAULT 1 NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT interactive_cards_availability_check CHECK ((availability = ANY (ARRAY['available'::text, 'disabled'::text, 'dependency_pending'::text, 'invalidated'::text]))),
    CONSTRAINT interactive_cards_check CHECK ((((source = 'system'::text) AND (enterprise_id IS NULL) AND (created_by IS NULL)) OR ((source = 'enterprise'::text) AND (enterprise_id IS NOT NULL) AND (created_by IS NOT NULL)))),
    CONSTRAINT interactive_cards_description_check CHECK ((char_length(description) <= 2048)),
    CONSTRAINT interactive_cards_latest_revision_check CHECK ((latest_revision > 0)),
    CONSTRAINT interactive_cards_lifecycle_check CHECK ((lifecycle = ANY (ARRAY['draft'::text, 'active'::text, 'deprecated'::text]))),
    CONSTRAINT interactive_cards_name_check CHECK (((char_length(name) >= 1) AND (char_length(name) <= 128))),
    CONSTRAINT interactive_cards_slug_check CHECK ((slug ~ '^[a-z][a-z0-9-]{0,62}$'::text)),
    CONSTRAINT interactive_cards_source_check CHECK ((source = ANY (ARRAY['system'::text, 'enterprise'::text]))),
    CONSTRAINT interactive_cards_version_check CHECK ((version > 0))
);


--
-- Name: kubernetes_clusters; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.kubernetes_clusters (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    name text NOT NULL,
    api_server text NOT NULL,
    connection_mode text NOT NULL,
    bastion_scope_id uuid,
    connector_id uuid,
    credential_id uuid,
    default_namespace text DEFAULT ''::text NOT NULL,
    environment text NOT NULL,
    labels jsonb DEFAULT '{}'::jsonb NOT NULL,
    labels_hash bytea NOT NULL,
    labels_version bigint DEFAULT 1 NOT NULL,
    resource_version bigint DEFAULT 1 NOT NULL,
    connection_status text DEFAULT 'disconnected'::text NOT NULL,
    kubernetes_version text DEFAULT ''::text NOT NULL,
    node_count integer DEFAULT 0 NOT NULL,
    ready_node_count integer DEFAULT 0 NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    deleted_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT kubernetes_clusters_api_server_check CHECK (((char_length(api_server) >= 1) AND (char_length(api_server) <= 2048))),
    CONSTRAINT kubernetes_clusters_check CHECK ((((connection_mode = 'via_bastion'::text) AND (bastion_scope_id IS NOT NULL) AND (credential_id IS NOT NULL)) OR ((connection_mode = 'direct'::text) AND (bastion_scope_id IS NULL) AND (credential_id IS NOT NULL)) OR ((connection_mode = 'in_cluster'::text) AND (bastion_scope_id IS NULL) AND (credential_id IS NULL)))),
    CONSTRAINT kubernetes_clusters_connection_mode_check CHECK ((connection_mode = ANY (ARRAY['via_bastion'::text, 'direct'::text, 'in_cluster'::text]))),
    CONSTRAINT kubernetes_clusters_connection_status_check CHECK ((connection_status = ANY (ARRAY['pending_connector'::text, 'connected'::text, 'degraded'::text, 'disconnected'::text]))),
    CONSTRAINT kubernetes_clusters_environment_check CHECK ((environment = ANY (ARRAY['development'::text, 'staging'::text, 'production'::text]))),
    CONSTRAINT kubernetes_clusters_labels_check CHECK (((jsonb_typeof(labels) = 'object'::text) AND (octet_length((labels)::text) <= 4096))),
    CONSTRAINT kubernetes_clusters_labels_hash_check CHECK ((octet_length(labels_hash) = 32)),
    CONSTRAINT kubernetes_clusters_labels_version_check CHECK ((labels_version > 0)),
    CONSTRAINT kubernetes_clusters_name_check CHECK (((char_length(name) >= 1) AND (char_length(name) <= 128))),
    CONSTRAINT kubernetes_clusters_node_count_check CHECK ((node_count >= 0)),
    CONSTRAINT kubernetes_clusters_ready_node_count_check CHECK ((ready_node_count >= 0)),
    CONSTRAINT kubernetes_clusters_resource_version_check CHECK ((resource_version > 0)),
    CONSTRAINT kubernetes_clusters_status_check CHECK ((status = ANY (ARRAY['active'::text, 'disabled'::text, 'deleted'::text])))
);


--
-- Name: kubernetes_node_host_bindings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.kubernetes_node_host_bindings (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    kubernetes_cluster_id uuid NOT NULL,
    node_uid text NOT NULL,
    node_name text NOT NULL,
    host_id uuid,
    matched_by text NOT NULL,
    evidence jsonb NOT NULL,
    evidence_hash bytea NOT NULL,
    confidence integer NOT NULL,
    status text DEFAULT 'proposed'::text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT kubernetes_node_host_bindings_confidence_check CHECK (((confidence >= 0) AND (confidence <= 100))),
    CONSTRAINT kubernetes_node_host_bindings_evidence_check CHECK ((jsonb_typeof(evidence) = 'object'::text)),
    CONSTRAINT kubernetes_node_host_bindings_evidence_hash_check CHECK ((octet_length(evidence_hash) = 32)),
    CONSTRAINT kubernetes_node_host_bindings_matched_by_check CHECK ((matched_by = ANY (ARRAY['system_uuid'::text, 'provider_id'::text, 'machine_id'::text, 'collector_host_id'::text, 'ip'::text, 'manual'::text]))),
    CONSTRAINT kubernetes_node_host_bindings_node_name_check CHECK (((char_length(node_name) >= 1) AND (char_length(node_name) <= 253))),
    CONSTRAINT kubernetes_node_host_bindings_node_uid_check CHECK (((char_length(node_uid) >= 1) AND (char_length(node_uid) <= 256))),
    CONSTRAINT kubernetes_node_host_bindings_status_check CHECK ((status = ANY (ARRAY['proposed'::text, 'verified'::text, 'rejected'::text, 'stale'::text]))),
    CONSTRAINT kubernetes_node_host_bindings_version_check CHECK ((version > 0))
);


--
-- Name: managed_accounts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.managed_accounts (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    host_id uuid NOT NULL,
    username text NOT NULL,
    privilege_level text NOT NULL,
    credential_id uuid NOT NULL,
    allowed_protocols text[] NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT managed_accounts_allowed_protocols_check CHECK (((cardinality(allowed_protocols) > 0) AND (allowed_protocols <@ ARRAY['shell'::text, 'ssh'::text, 'rdp'::text]))),
    CONSTRAINT managed_accounts_privilege_level_check CHECK ((privilege_level = ANY (ARRAY['standard'::text, 'sudo'::text, 'administrator'::text]))),
    CONSTRAINT managed_accounts_status_check CHECK ((status = ANY (ARRAY['active'::text, 'disabled'::text]))),
    CONSTRAINT managed_accounts_username_check CHECK (((char_length(username) >= 1) AND (char_length(username) <= 256))),
    CONSTRAINT managed_accounts_version_check CHECK ((version > 0))
);


--
-- Name: mfa_challenges; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mfa_challenges (
    id uuid NOT NULL,
    challenge_hash bytea NOT NULL,
    audience text NOT NULL,
    user_id uuid NOT NULL,
    purpose text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    consumed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT mfa_challenges_audience_check CHECK ((audience = ANY (ARRAY['platform'::text, 'enterprise'::text]))),
    CONSTRAINT mfa_challenges_challenge_hash_check CHECK ((octet_length(challenge_hash) = 32)),
    CONSTRAINT mfa_challenges_purpose_check CHECK ((purpose = ANY (ARRAY['login'::text, 'step_up'::text])))
);


--
-- Name: mfa_credentials; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mfa_credentials (
    id uuid NOT NULL,
    audience text NOT NULL,
    user_id uuid NOT NULL,
    provider text NOT NULL,
    key_id text NOT NULL,
    key_version integer NOT NULL,
    encrypted_secret bytea NOT NULL,
    last_totp_counter bigint,
    enrollment_hash bytea,
    enrollment_expires_at timestamp with time zone,
    status text NOT NULL,
    verified_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT mfa_credentials_audience_check CHECK ((audience = ANY (ARRAY['platform'::text, 'enterprise'::text]))),
    CONSTRAINT mfa_credentials_enrollment_hash_check CHECK (((enrollment_hash IS NULL) OR (octet_length(enrollment_hash) = 32))),
    CONSTRAINT mfa_credentials_key_version_check CHECK ((key_version > 0)),
    CONSTRAINT mfa_credentials_provider_check CHECK ((provider = ANY (ARRAY['openbao_transit'::text, 'local_test'::text]))),
    CONSTRAINT mfa_credentials_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'active'::text, 'disabled'::text])))
);


--
-- Name: mfa_recovery_codes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mfa_recovery_codes (
    id uuid NOT NULL,
    credential_id uuid NOT NULL,
    code_hash bytea NOT NULL,
    consumed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT mfa_recovery_codes_code_hash_check CHECK ((octet_length(code_hash) = 32))
);


--
-- Name: model_calls; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.model_calls (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    run_id uuid NOT NULL,
    step_id uuid NOT NULL,
    model_id uuid NOT NULL,
    model_revision integer NOT NULL,
    call_kind text NOT NULL,
    projection_hash bytea NOT NULL,
    input_tokens bigint DEFAULT 0 NOT NULL,
    output_tokens bigint DEFAULT 0 NOT NULL,
    input_price_snapshot numeric(20,8) NOT NULL,
    output_price_snapshot numeric(20,8) NOT NULL,
    amount numeric(20,8) DEFAULT 0 NOT NULL,
    latency_ms bigint DEFAULT 0 NOT NULL,
    stop_reason text,
    status text NOT NULL,
    error_code text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    completed_at timestamp with time zone,
    CONSTRAINT model_calls_amount_check CHECK ((amount >= (0)::numeric)),
    CONSTRAINT model_calls_call_kind_check CHECK ((call_kind = ANY (ARRAY['inference'::text, 'compaction'::text]))),
    CONSTRAINT model_calls_input_tokens_check CHECK ((input_tokens >= 0)),
    CONSTRAINT model_calls_latency_ms_check CHECK ((latency_ms >= 0)),
    CONSTRAINT model_calls_output_tokens_check CHECK ((output_tokens >= 0)),
    CONSTRAINT model_calls_projection_hash_check CHECK ((octet_length(projection_hash) = 32)),
    CONSTRAINT model_calls_status_check CHECK ((status = ANY (ARRAY['reserved'::text, 'running'::text, 'succeeded'::text, 'failed'::text, 'cancelled'::text])))
);


--
-- Name: model_compatibility_results; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.model_compatibility_results (
    id uuid NOT NULL,
    model_revision_id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    compatible boolean NOT NULL,
    checks jsonb NOT NULL,
    error_code text,
    tested_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: model_quota_reservations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.model_quota_reservations (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    model_call_id uuid NOT NULL,
    model_id uuid NOT NULL,
    department_id uuid NOT NULL,
    user_id uuid NOT NULL,
    month date NOT NULL,
    reserved_amount numeric(20,8) NOT NULL,
    settled_amount numeric(20,8),
    status text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT model_quota_reservations_reserved_amount_check CHECK ((reserved_amount >= (0)::numeric)),
    CONSTRAINT model_quota_reservations_status_check CHECK ((status = ANY (ARRAY['active'::text, 'settled'::text, 'released'::text])))
);


--
-- Name: model_quotas; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.model_quotas (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    model_id uuid NOT NULL,
    subject_type text NOT NULL,
    subject_id uuid NOT NULL,
    monthly_amount numeric(20,8) NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT model_quotas_monthly_amount_check CHECK ((monthly_amount >= (0)::numeric)),
    CONSTRAINT model_quotas_subject_type_check CHECK ((subject_type = ANY (ARRAY['department'::text, 'user'::text]))),
    CONSTRAINT model_quotas_version_check CHECK ((version > 0))
);


--
-- Name: outbox_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.outbox_events (
    id uuid NOT NULL,
    topic text NOT NULL,
    aggregate_type text NOT NULL,
    aggregate_id text NOT NULL,
    payload jsonb NOT NULL,
    attempts integer DEFAULT 0 NOT NULL,
    available_at timestamp with time zone DEFAULT now() NOT NULL,
    claimed_at timestamp with time zone,
    published_at timestamp with time zone,
    last_error text,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: password_credentials; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.password_credentials (
    id uuid NOT NULL,
    audience text NOT NULL,
    subject_id uuid NOT NULL,
    encoded_hash text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    temporary boolean DEFAULT false NOT NULL,
    expires_at timestamp with time zone,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT password_credentials_audience_check CHECK ((audience = ANY (ARRAY['platform'::text, 'enterprise'::text]))),
    CONSTRAINT password_credentials_check CHECK (((temporary AND (expires_at IS NOT NULL)) OR (NOT temporary))),
    CONSTRAINT password_credentials_status_check CHECK ((status = ANY (ARRAY['active'::text, 'revoked'::text]))),
    CONSTRAINT password_credentials_version_check CHECK ((version > 0))
);


--
-- Name: pending_action_plans; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.pending_action_plans (
    id uuid NOT NULL,
    pending_action_id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    preview_call_id text NOT NULL,
    commit_tool text NOT NULL,
    authorization_version bigint NOT NULL,
    plan_schema_version text NOT NULL,
    plan_hash bytea NOT NULL,
    immutable_plan jsonb NOT NULL,
    resource_scope_snapshot jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT pending_action_plans_authorization_version_check CHECK ((authorization_version > 0)),
    CONSTRAINT pending_action_plans_commit_tool_check CHECK ((commit_tool ~ '^[a-z][a-z0-9_.]+[.]commit$'::text)),
    CONSTRAINT pending_action_plans_plan_hash_check CHECK ((octet_length(plan_hash) = 32))
);


--
-- Name: pending_action_tokens; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.pending_action_tokens (
    id uuid NOT NULL,
    pending_action_id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    token_hash bytea NOT NULL,
    key_version integer NOT NULL,
    nonce bytea NOT NULL,
    ciphertext bytea NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    consumed_at timestamp with time zone,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT pending_action_tokens_key_version_check CHECK ((key_version > 0)),
    CONSTRAINT pending_action_tokens_status_check CHECK ((status = ANY (ARRAY['active'::text, 'consumed'::text, 'revoked'::text, 'expired'::text]))),
    CONSTRAINT pending_action_tokens_token_hash_check CHECK ((octet_length(token_hash) = 32))
);


--
-- Name: pending_actions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.pending_actions (
    id uuid NOT NULL,
    action_ref text NOT NULL,
    enterprise_id uuid NOT NULL,
    creator_subject_id uuid CONSTRAINT pending_actions_creator_user_id_not_null NOT NULL,
    authorization_version bigint NOT NULL,
    action_type text NOT NULL,
    title text NOT NULL,
    summary text NOT NULL,
    risk text NOT NULL,
    preview jsonb NOT NULL,
    diff jsonb DEFAULT '[]'::jsonb NOT NULL,
    status text NOT NULL,
    resource_type text NOT NULL,
    resource_id uuid,
    expected_resource_version bigint,
    impact_hash bytea NOT NULL,
    result_resource_type text,
    result_resource_id uuid,
    result_resource_version bigint,
    result_summary text DEFAULT ''::text NOT NULL,
    error_code text,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    creator_subject_type text DEFAULT 'user'::text NOT NULL,
    run_id uuid,
    confirmation_required boolean DEFAULT true NOT NULL,
    policy_snapshot_hash bytea,
    CONSTRAINT pending_actions_action_type_check CHECK ((action_type ~ '^[a-z][a-z0-9_.]+$'::text)),
    CONSTRAINT pending_actions_authorization_version_check CHECK ((authorization_version > 0)),
    CONSTRAINT pending_actions_creator_subject_type_check CHECK ((creator_subject_type = ANY (ARRAY['user'::text, 'service_account'::text]))),
    CONSTRAINT pending_actions_impact_hash_check CHECK ((octet_length(impact_hash) = 32)),
    CONSTRAINT pending_actions_policy_hash_check CHECK (((policy_snapshot_hash IS NULL) OR (octet_length(policy_snapshot_hash) = 32))),
    CONSTRAINT pending_actions_risk_check CHECK ((risk = ANY (ARRAY['write'::text, 'dangerous'::text, 'critical'::text]))),
    CONSTRAINT pending_actions_status_check CHECK ((status = ANY (ARRAY['prepared'::text, 'awaiting_confirmation'::text, 'awaiting_approval'::text, 'ready'::text, 'executing'::text, 'succeeded'::text, 'failed'::text, 'result_unknown'::text, 'cancelled'::text, 'expired'::text, 'rejected'::text, 'invalidated'::text])))
);


--
-- Name: permissions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.permissions (
    id text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    registry_version integer NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT permissions_id_check CHECK ((id ~ '^[a-z][a-z0-9_.]+$'::text)),
    CONSTRAINT permissions_registry_version_check CHECK ((registry_version > 0))
);


--
-- Name: pki_certificate_identities; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.pki_certificate_identities (
    serial_number text NOT NULL,
    subject_kind text NOT NULL,
    subject_id text NOT NULL,
    enterprise_id uuid,
    uri_san text DEFAULT ''::text NOT NULL,
    dns_sans text[] DEFAULT '{}'::text[] NOT NULL,
    extended_key_usage text NOT NULL,
    issuer_generation integer NOT NULL,
    certificate_sha256 text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    not_before timestamp with time zone NOT NULL,
    not_after timestamp with time zone NOT NULL,
    revoked_at timestamp with time zone,
    revocation_reason text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT pki_certificate_identities_certificate_sha256_check CHECK ((certificate_sha256 ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT pki_certificate_identities_check CHECK ((not_after > not_before)),
    CONSTRAINT pki_certificate_identities_check1 CHECK (((status = 'revoked'::text) = (revoked_at IS NOT NULL))),
    CONSTRAINT pki_certificate_identities_extended_key_usage_check CHECK ((extended_key_usage = ANY (ARRAY['serverAuth'::text, 'clientAuth'::text]))),
    CONSTRAINT pki_certificate_identities_issuer_generation_check CHECK ((issuer_generation > 0)),
    CONSTRAINT pki_certificate_identities_serial_number_check CHECK (((char_length(serial_number) >= 1) AND (char_length(serial_number) <= 256))),
    CONSTRAINT pki_certificate_identities_status_check CHECK ((status = ANY (ARRAY['active'::text, 'overlap'::text, 'revoked'::text, 'expired'::text]))),
    CONSTRAINT pki_certificate_identities_subject_id_check CHECK (((char_length(subject_id) >= 1) AND (char_length(subject_id) <= 256))),
    CONSTRAINT pki_certificate_identities_subject_kind_check CHECK ((subject_kind = ANY (ARRAY['service'::text, 'connector'::text, 'collector'::text])))
);


--
-- Name: pki_node_trust_acks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.pki_node_trust_acks (
    node_kind text NOT NULL,
    node_id text NOT NULL,
    enterprise_id uuid,
    epoch bigint NOT NULL,
    bundle_sha256 text NOT NULL,
    ca_fingerprints text[] DEFAULT '{}'::text[] NOT NULL,
    status text NOT NULL,
    required_for_cutover boolean DEFAULT false NOT NULL,
    error text DEFAULT ''::text NOT NULL,
    first_seen_at timestamp with time zone DEFAULT now() NOT NULL,
    acknowledged_at timestamp with time zone,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT pki_node_trust_acks_bundle_sha256_check CHECK ((bundle_sha256 ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT pki_node_trust_acks_check CHECK (((status = 'acked'::text) = (acknowledged_at IS NOT NULL))),
    CONSTRAINT pki_node_trust_acks_node_id_check CHECK (((char_length(node_id) >= 1) AND (char_length(node_id) <= 256))),
    CONSTRAINT pki_node_trust_acks_node_kind_check CHECK ((node_kind = ANY (ARRAY['connector'::text, 'collector'::text, 'kubernetes_connector'::text, 'control_plane'::text]))),
    CONSTRAINT pki_node_trust_acks_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'acked'::text, 'failed'::text, 'trust_expired'::text])))
);


--
-- Name: pki_trust_bundles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.pki_trust_bundles (
    epoch bigint NOT NULL,
    state text NOT NULL,
    direction text DEFAULT 'forward'::text NOT NULL,
    bundle_pem text NOT NULL,
    bundle_sha256 text NOT NULL,
    current_ca_fingerprints text[] NOT NULL,
    next_ca_fingerprints text[] DEFAULT '{}'::text[] NOT NULL,
    started_at timestamp with time zone DEFAULT now() NOT NULL,
    retire_at timestamp with time zone,
    last_error text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT pki_trust_bundles_bundle_pem_check CHECK (((char_length(bundle_pem) >= 128) AND (char_length(bundle_pem) <= 131072))),
    CONSTRAINT pki_trust_bundles_bundle_sha256_check CHECK ((bundle_sha256 ~ '^[a-f0-9]{64}$'::text)),
    CONSTRAINT pki_trust_bundles_check CHECK (((state = ANY (ARRAY['overlapping'::text, 'retiring'::text])) = (retire_at IS NOT NULL))),
    CONSTRAINT pki_trust_bundles_check1 CHECK (((state <> 'overlapping'::text) OR (cardinality(next_ca_fingerprints) > 0))),
    CONSTRAINT pki_trust_bundles_check2 CHECK (((direction = 'forward'::text) OR (state = ANY (ARRAY['overlapping'::text, 'retiring'::text])))),
    CONSTRAINT pki_trust_bundles_current_ca_fingerprints_check CHECK ((cardinality(current_ca_fingerprints) > 0)),
    CONSTRAINT pki_trust_bundles_direction_check CHECK ((direction = ANY (ARRAY['forward'::text, 'rollback'::text]))),
    CONSTRAINT pki_trust_bundles_epoch_check CHECK ((epoch > 0)),
    CONSTRAINT pki_trust_bundles_state_check CHECK ((state = ANY (ARRAY['stable'::text, 'preparing'::text, 'overlapping'::text, 'retiring'::text, 'failed'::text])))
);


--
-- Name: platform_settings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.platform_settings (
    singleton boolean DEFAULT true NOT NULL,
    platform_name text NOT NULL,
    default_locale text NOT NULL,
    timezone text NOT NULL,
    external_url text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT platform_settings_default_locale_check CHECK ((default_locale = ANY (ARRAY['zh-CN'::text, 'en-US'::text]))),
    CONSTRAINT platform_settings_platform_name_check CHECK (((char_length(platform_name) >= 1) AND (char_length(platform_name) <= 128))),
    CONSTRAINT platform_settings_singleton_check CHECK (singleton),
    CONSTRAINT platform_settings_version_check CHECK ((version > 0))
);


--
-- Name: platform_state; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.platform_state (
    singleton boolean DEFAULT true NOT NULL,
    state text NOT NULL,
    initialized_at timestamp with time zone,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT platform_state_singleton_check CHECK (singleton),
    CONSTRAINT platform_state_state_check CHECK ((state = ANY (ARRAY['uninitialized'::text, 'initializing'::text, 'initialized'::text])))
);


--
-- Name: platform_users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.platform_users (
    id uuid NOT NULL,
    username text NOT NULL,
    display_name text NOT NULL,
    email text,
    role text DEFAULT 'platform_super_admin'::text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    mfa_enabled boolean DEFAULT false NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT platform_users_display_name_check CHECK (((char_length(display_name) >= 1) AND (char_length(display_name) <= 128))),
    CONSTRAINT platform_users_role_check CHECK ((role = 'platform_super_admin'::text)),
    CONSTRAINT platform_users_status_check CHECK ((status = ANY (ARRAY['active'::text, 'disabled'::text]))),
    CONSTRAINT platform_users_username_check CHECK (((char_length(username) >= 3) AND (char_length(username) <= 128))),
    CONSTRAINT platform_users_version_check CHECK ((version > 0))
);


--
-- Name: remote_access_approval_workflows; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.remote_access_approval_workflows (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    approver_role_ids uuid[] NOT NULL,
    minimum_approvals integer NOT NULL,
    separation_of_duties boolean DEFAULT true NOT NULL,
    approval_timeout_seconds integer CONSTRAINT remote_access_approval_workfl_approval_timeout_seconds_not_null NOT NULL,
    timeout_effect text NOT NULL,
    escalation_role_ids uuid[] DEFAULT '{}'::uuid[] NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    escalation_after_seconds integer DEFAULT 30 CONSTRAINT remote_access_approval_workfl_escalation_after_seconds_not_null NOT NULL,
    CONSTRAINT remote_access_approval_workflows_approval_timeout_seconds_check CHECK (((approval_timeout_seconds >= 60) AND (approval_timeout_seconds <= 604800))),
    CONSTRAINT remote_access_approval_workflows_approver_role_ids_check CHECK (((cardinality(approver_role_ids) > 0) AND (cardinality(approver_role_ids) <= 64))),
    CONSTRAINT remote_access_approval_workflows_check CHECK ((minimum_approvals <= cardinality(approver_role_ids))),
    CONSTRAINT remote_access_approval_workflows_description_check CHECK ((char_length(description) <= 2048)),
    CONSTRAINT remote_access_approval_workflows_minimum_approvals_check CHECK (((minimum_approvals >= 1) AND (minimum_approvals <= 16))),
    CONSTRAINT remote_access_approval_workflows_name_check CHECK (((char_length(name) >= 1) AND (char_length(name) <= 128))),
    CONSTRAINT remote_access_approval_workflows_status_check CHECK ((status = ANY (ARRAY['draft'::text, 'enabled'::text, 'disabled'::text, 'archived'::text]))),
    CONSTRAINT remote_access_approval_workflows_timeout_effect_check CHECK ((timeout_effect = ANY (ARRAY['reject'::text, 'expire'::text]))),
    CONSTRAINT remote_access_approval_workflows_version_check CHECK ((version > 0)),
    CONSTRAINT remote_access_workflow_escalation_window_ck CHECK (((escalation_after_seconds >= 30) AND (escalation_after_seconds < approval_timeout_seconds)))
);


--
-- Name: remote_access_command_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.remote_access_command_events (
    id uuid NOT NULL,
    session_id uuid NOT NULL,
    sequence bigint NOT NULL,
    event_type text NOT NULL,
    command_hash bytea,
    occurred_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT remote_access_command_events_command_hash_check CHECK (((command_hash IS NULL) OR (octet_length(command_hash) = 32))),
    CONSTRAINT remote_access_command_events_event_type_check CHECK ((event_type = ANY (ARRAY['input'::text, 'output'::text, 'resize'::text, 'marker'::text, 'state'::text]))),
    CONSTRAINT remote_access_command_events_sequence_check CHECK ((sequence > 0))
);


--
-- Name: remote_access_decisions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.remote_access_decisions (
    id uuid NOT NULL,
    request_id uuid NOT NULL,
    requirement_id uuid NOT NULL,
    decision text NOT NULL,
    comment text DEFAULT ''::text NOT NULL,
    decided_by uuid NOT NULL,
    decided_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT remote_access_decisions_comment_check CHECK ((char_length(comment) <= 2048)),
    CONSTRAINT remote_access_decisions_decision_check CHECK ((decision = ANY (ARRAY['approve'::text, 'reject'::text])))
);


--
-- Name: remote_access_grants; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.remote_access_grants (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    subject_type text NOT NULL,
    subject_id uuid NOT NULL,
    host_ids uuid[] DEFAULT '{}'::uuid[] NOT NULL,
    managed_account_ids uuid[] NOT NULL,
    protocols text[] NOT NULL,
    actions text[] DEFAULT ARRAY['terminal'::text] NOT NULL,
    valid_from timestamp with time zone NOT NULL,
    valid_until timestamp with time zone NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    status text DEFAULT 'enabled'::text NOT NULL,
    CONSTRAINT remote_access_grants_actions_check CHECK ((actions = ARRAY['terminal'::text])),
    CONSTRAINT remote_access_grants_check CHECK ((valid_until > valid_from)),
    CONSTRAINT remote_access_grants_managed_account_ids_check CHECK ((cardinality(managed_account_ids) > 0)),
    CONSTRAINT remote_access_grants_protocols_check CHECK (((cardinality(protocols) > 0) AND (protocols <@ ARRAY['shell'::text, 'ssh'::text, 'rdp'::text]))),
    CONSTRAINT remote_access_grants_status_check CHECK ((status = ANY (ARRAY['draft'::text, 'enabled'::text, 'disabled'::text, 'archived'::text]))),
    CONSTRAINT remote_access_grants_subject_type_check CHECK ((subject_type = ANY (ARRAY['user'::text, 'department'::text]))),
    CONSTRAINT remote_access_grants_version_check CHECK ((version > 0))
);


--
-- Name: remote_access_leases; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.remote_access_leases (
    id uuid NOT NULL,
    request_id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    user_id uuid NOT NULL,
    grant_id uuid NOT NULL,
    host_id uuid NOT NULL,
    managed_account_id uuid NOT NULL,
    protocol text NOT NULL,
    action text DEFAULT 'terminal'::text NOT NULL,
    authorization_version bigint NOT NULL,
    issued_at timestamp with time zone DEFAULT statement_timestamp() NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    revoked_at timestamp with time zone,
    revoke_reason text,
    decision_snapshot jsonb DEFAULT '{}'::jsonb NOT NULL,
    session_profile_snapshot jsonb DEFAULT '{}'::jsonb NOT NULL,
    decision_snapshot_hash bytea,
    CONSTRAINT remote_access_lease_decision_snapshot_ck CHECK ((jsonb_typeof(decision_snapshot) = 'object'::text)),
    CONSTRAINT remote_access_lease_profile_snapshot_ck CHECK ((jsonb_typeof(session_profile_snapshot) = 'object'::text)),
    CONSTRAINT remote_access_lease_snapshot_hash_ck CHECK (((decision_snapshot_hash IS NULL) OR (octet_length(decision_snapshot_hash) = 32))),
    CONSTRAINT remote_access_leases_action_check CHECK ((action = 'terminal'::text)),
    CONSTRAINT remote_access_leases_authorization_version_check CHECK ((authorization_version > 0)),
    CONSTRAINT remote_access_leases_check CHECK ((expires_at <= (issued_at + '00:15:00'::interval))),
    CONSTRAINT remote_access_leases_protocol_check CHECK ((protocol = ANY (ARRAY['shell'::text, 'ssh'::text, 'rdp'::text])))
);


--
-- Name: remote_access_recording_chunks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.remote_access_recording_chunks (
    recording_id uuid NOT NULL,
    sequence bigint NOT NULL,
    object_key text NOT NULL,
    nonce bytea NOT NULL,
    ciphertext_size bigint NOT NULL,
    event_count integer NOT NULL,
    started_at timestamp with time zone NOT NULL,
    ended_at timestamp with time zone NOT NULL,
    previous_hash bytea NOT NULL,
    chunk_hash bytea NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT remote_access_recording_chunks_check CHECK ((ended_at >= started_at)),
    CONSTRAINT remote_access_recording_chunks_chunk_hash_check CHECK ((octet_length(chunk_hash) = 32)),
    CONSTRAINT remote_access_recording_chunks_ciphertext_size_check CHECK ((ciphertext_size > 0)),
    CONSTRAINT remote_access_recording_chunks_event_count_check CHECK ((event_count > 0)),
    CONSTRAINT remote_access_recording_chunks_nonce_check CHECK ((octet_length(nonce) = 12)),
    CONSTRAINT remote_access_recording_chunks_previous_hash_check CHECK ((octet_length(previous_hash) = 32)),
    CONSTRAINT remote_access_recording_chunks_sequence_check CHECK ((sequence > 0))
);


--
-- Name: remote_access_recordings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.remote_access_recordings (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    session_id uuid NOT NULL,
    status text DEFAULT 'recording'::text NOT NULL,
    format text DEFAULT 'asciicast_v2'::text NOT NULL,
    key_provider text NOT NULL,
    key_id text NOT NULL,
    key_version integer NOT NULL,
    wrapped_dek bytea NOT NULL,
    chunk_count integer DEFAULT 0 NOT NULL,
    event_count bigint DEFAULT 0 NOT NULL,
    size_bytes bigint DEFAULT 0 NOT NULL,
    duration_ms bigint DEFAULT 0 NOT NULL,
    final_hash bytea,
    retention_until timestamp with time zone DEFAULT (now() + '90 days'::interval) NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    completed_at timestamp with time zone,
    CONSTRAINT remote_access_recordings_chunk_count_check CHECK ((chunk_count >= 0)),
    CONSTRAINT remote_access_recordings_duration_ms_check CHECK ((duration_ms >= 0)),
    CONSTRAINT remote_access_recordings_event_count_check CHECK ((event_count >= 0)),
    CONSTRAINT remote_access_recordings_final_hash_check CHECK (((final_hash IS NULL) OR (octet_length(final_hash) = 32))),
    CONSTRAINT remote_access_recordings_format_check CHECK ((format = ANY (ARRAY['asciicast_v2'::text, 'guacamole_v1'::text]))),
    CONSTRAINT remote_access_recordings_key_provider_check CHECK ((key_provider = ANY (ARRAY['local'::text, 'openbao_transit'::text]))),
    CONSTRAINT remote_access_recordings_key_version_check CHECK ((key_version > 0)),
    CONSTRAINT remote_access_recordings_size_bytes_check CHECK ((size_bytes >= 0)),
    CONSTRAINT remote_access_recordings_status_check CHECK ((status = ANY (ARRAY['recording'::text, 'available'::text, 'incomplete'::text, 'failed'::text, 'expired'::text])))
);


--
-- Name: remote_access_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.remote_access_requests (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    requester_id uuid NOT NULL,
    grant_id uuid NOT NULL,
    host_id uuid NOT NULL,
    managed_account_id uuid NOT NULL,
    protocol text NOT NULL,
    action text DEFAULT 'terminal'::text NOT NULL,
    reason text NOT NULL,
    status text NOT NULL,
    authorization_version bigint NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    decision_outcome text,
    decision_reason_codes jsonb DEFAULT '[]'::jsonb NOT NULL,
    decision_snapshot jsonb DEFAULT '{}'::jsonb NOT NULL,
    decision_snapshot_hash bytea,
    matched_grant_snapshots jsonb DEFAULT '[]'::jsonb NOT NULL,
    matched_rule_snapshots jsonb DEFAULT '[]'::jsonb NOT NULL,
    decision_at timestamp with time zone,
    CONSTRAINT remote_access_request_decision_snapshot_ck CHECK ((jsonb_typeof(decision_snapshot) = 'object'::text)),
    CONSTRAINT remote_access_request_snapshot_hash_ck CHECK (((decision_snapshot_hash IS NULL) OR (octet_length(decision_snapshot_hash) = 32))),
    CONSTRAINT remote_access_requests_action_check CHECK ((action = 'terminal'::text)),
    CONSTRAINT remote_access_requests_authorization_version_check CHECK ((authorization_version > 0)),
    CONSTRAINT remote_access_requests_protocol_check CHECK ((protocol = ANY (ARRAY['shell'::text, 'ssh'::text, 'rdp'::text]))),
    CONSTRAINT remote_access_requests_reason_check CHECK (((char_length(reason) >= 1) AND (char_length(reason) <= 2048))),
    CONSTRAINT remote_access_requests_status_check CHECK ((status = ANY (ARRAY['requested'::text, 'awaiting_mfa'::text, 'awaiting_approval'::text, 'authorized'::text, 'rejected'::text, 'expired'::text, 'invalidated'::text])))
);


--
-- Name: remote_access_requirement_snapshots; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.remote_access_requirement_snapshots (
    id uuid NOT NULL,
    request_id uuid NOT NULL,
    approver_role_ids uuid[] DEFAULT '{}'::uuid[] NOT NULL,
    minimum_approvals integer NOT NULL,
    separation_of_duties boolean CONSTRAINT remote_access_requirement_snapsho_separation_of_duties_not_null NOT NULL,
    require_mfa boolean NOT NULL,
    max_session_seconds integer CONSTRAINT remote_access_requirement_snapshot_max_session_seconds_not_null NOT NULL,
    idle_timeout_seconds integer CONSTRAINT remote_access_requirement_snapsho_idle_timeout_seconds_not_null NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    rule_id uuid,
    rule_version bigint,
    workflow_id uuid,
    workflow_version bigint,
    session_profile_id uuid,
    session_profile_version bigint,
    approval_snapshot jsonb DEFAULT '{}'::jsonb NOT NULL,
    deadline_at timestamp with time zone,
    escalated_at timestamp with time zone,
    timeout_effect text DEFAULT 'expire'::text NOT NULL,
    escalation_role_ids uuid[] DEFAULT '{}'::uuid[] CONSTRAINT remote_access_requirement_snapshot_escalation_role_ids_not_null NOT NULL,
    escalation_at timestamp with time zone NOT NULL,
    CONSTRAINT remote_access_requirement_escalation_window_ck CHECK ((escalation_at < deadline_at)),
    CONSTRAINT remote_access_requirement_profile_version_ck CHECK (((session_profile_version IS NULL) OR (session_profile_version > 0))),
    CONSTRAINT remote_access_requirement_rule_version_ck CHECK (((rule_version IS NULL) OR (rule_version > 0))),
    CONSTRAINT remote_access_requirement_snapshots_idle_timeout_seconds_check CHECK (((idle_timeout_seconds >= 60) AND (idle_timeout_seconds <= max_session_seconds))),
    CONSTRAINT remote_access_requirement_snapshots_max_session_seconds_check CHECK (((max_session_seconds >= 60) AND (max_session_seconds <= 86400))),
    CONSTRAINT remote_access_requirement_snapshots_minimum_approvals_check CHECK (((minimum_approvals >= 1) AND (minimum_approvals <= 16))),
    CONSTRAINT remote_access_requirement_snapshots_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'satisfied'::text, 'rejected'::text, 'invalidated'::text]))),
    CONSTRAINT remote_access_requirement_snapshots_timeout_effect_check CHECK ((timeout_effect = ANY (ARRAY['reject'::text, 'expire'::text]))),
    CONSTRAINT remote_access_requirement_workflow_source_ck CHECK (((workflow_id IS NOT NULL) AND (workflow_version IS NOT NULL))),
    CONSTRAINT remote_access_requirement_workflow_version_ck CHECK (((workflow_version IS NULL) OR (workflow_version > 0)))
);


--
-- Name: remote_access_routes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.remote_access_routes (
    session_id uuid NOT NULL,
    gateway_instance text NOT NULL,
    connector_id uuid,
    connector_epoch bigint,
    session_fence bigint NOT NULL,
    lease_expires_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT remote_access_routes_session_fence_check CHECK ((session_fence > 0))
);


--
-- Name: remote_access_rules; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.remote_access_rules (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    priority integer DEFAULT 100 NOT NULL,
    protocols text[] NOT NULL,
    actions text[] DEFAULT ARRAY['terminal'::text] NOT NULL,
    source_cidrs text[] DEFAULT '{}'::text[] NOT NULL,
    time_windows jsonb DEFAULT '[]'::jsonb NOT NULL,
    effects text[] NOT NULL,
    approval_workflow_id uuid,
    session_profile_id uuid,
    status text DEFAULT 'draft'::text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT remote_access_rules_actions_check CHECK ((actions = ARRAY['terminal'::text])),
    CONSTRAINT remote_access_rules_check CHECK (((NOT ('require_approval'::text = ANY (effects))) OR (approval_workflow_id IS NOT NULL))),
    CONSTRAINT remote_access_rules_check1 CHECK (((NOT ('deny'::text = ANY (effects))) OR ((cardinality(effects) = 1) AND (approval_workflow_id IS NULL) AND (session_profile_id IS NULL)))),
    CONSTRAINT remote_access_rules_description_check CHECK ((char_length(description) <= 2048)),
    CONSTRAINT remote_access_rules_effects_check CHECK (((cardinality(effects) <= 4) AND (effects <@ ARRAY['deny'::text, 'require_mfa'::text, 'require_approval'::text, 'notify'::text]) AND ((cardinality(effects) > 0) OR (session_profile_id IS NOT NULL)))),
    CONSTRAINT remote_access_rules_effects_check1 CHECK (((NOT ('deny'::text = ANY (effects))) OR (cardinality(effects) = 1))),
    CONSTRAINT remote_access_rules_name_check CHECK (((char_length(name) >= 1) AND (char_length(name) <= 128))),
    CONSTRAINT remote_access_rules_priority_check CHECK (((priority >= 0) AND (priority <= 10000))),
    CONSTRAINT remote_access_rules_protocols_check CHECK (((cardinality(protocols) > 0) AND (protocols <@ ARRAY['shell'::text, 'ssh'::text, 'rdp'::text]))),
    CONSTRAINT remote_access_rules_source_cidrs_check CHECK ((cardinality(source_cidrs) <= 64)),
    CONSTRAINT remote_access_rules_status_check CHECK ((status = ANY (ARRAY['draft'::text, 'enabled'::text, 'disabled'::text, 'archived'::text]))),
    CONSTRAINT remote_access_rules_time_windows_check CHECK ((jsonb_typeof(time_windows) = 'array'::text)),
    CONSTRAINT remote_access_rules_version_check CHECK ((version > 0))
);


--
-- Name: remote_access_session_profiles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.remote_access_session_profiles (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    max_session_seconds integer NOT NULL,
    idle_timeout_seconds integer NOT NULL,
    recording_mode text NOT NULL,
    command_audit_mode text NOT NULL,
    clipboard_mode text NOT NULL,
    file_upload_mode text NOT NULL,
    file_download_mode text NOT NULL,
    port_forward_mode text NOT NULL,
    session_share_mode text NOT NULL,
    retention_days integer NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT remote_access_session_profiles_check CHECK ((idle_timeout_seconds <= max_session_seconds)),
    CONSTRAINT remote_access_session_profiles_clipboard_mode_check CHECK ((clipboard_mode = ANY (ARRAY['enabled'::text, 'disabled'::text]))),
    CONSTRAINT remote_access_session_profiles_command_audit_mode_check CHECK ((command_audit_mode = ANY (ARRAY['required'::text, 'optional'::text, 'disabled'::text]))),
    CONSTRAINT remote_access_session_profiles_description_check CHECK ((char_length(description) <= 2048)),
    CONSTRAINT remote_access_session_profiles_file_download_mode_check CHECK ((file_download_mode = ANY (ARRAY['enabled'::text, 'disabled'::text]))),
    CONSTRAINT remote_access_session_profiles_file_upload_mode_check CHECK ((file_upload_mode = ANY (ARRAY['enabled'::text, 'disabled'::text]))),
    CONSTRAINT remote_access_session_profiles_idle_timeout_seconds_check CHECK (((idle_timeout_seconds >= 60) AND (idle_timeout_seconds <= 86400))),
    CONSTRAINT remote_access_session_profiles_max_session_seconds_check CHECK (((max_session_seconds >= 60) AND (max_session_seconds <= 86400))),
    CONSTRAINT remote_access_session_profiles_name_check CHECK (((char_length(name) >= 1) AND (char_length(name) <= 128))),
    CONSTRAINT remote_access_session_profiles_port_forward_mode_check CHECK ((port_forward_mode = ANY (ARRAY['enabled'::text, 'disabled'::text]))),
    CONSTRAINT remote_access_session_profiles_recording_mode_check CHECK ((recording_mode = ANY (ARRAY['required'::text, 'optional'::text, 'disabled'::text]))),
    CONSTRAINT remote_access_session_profiles_retention_days_check CHECK (((retention_days >= 1) AND (retention_days <= 3650))),
    CONSTRAINT remote_access_session_profiles_session_share_mode_check CHECK ((session_share_mode = ANY (ARRAY['enabled'::text, 'disabled'::text]))),
    CONSTRAINT remote_access_session_profiles_status_check CHECK ((status = ANY (ARRAY['draft'::text, 'enabled'::text, 'disabled'::text, 'archived'::text]))),
    CONSTRAINT remote_access_session_profiles_version_check CHECK ((version > 0))
);


--
-- Name: remote_access_sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.remote_access_sessions (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    user_id uuid NOT NULL,
    http_session_id uuid NOT NULL,
    lease_id uuid NOT NULL,
    host_id uuid NOT NULL,
    managed_account_id uuid NOT NULL,
    protocol text NOT NULL,
    control_path text CONSTRAINT remote_access_sessions_connection_mode_not_null NOT NULL,
    connector_id uuid,
    connector_epoch bigint,
    status text NOT NULL,
    session_fence bigint DEFAULT 1 NOT NULL,
    authorization_version bigint NOT NULL,
    idle_timeout_seconds integer NOT NULL,
    max_duration_seconds integer NOT NULL,
    connect_before timestamp with time zone NOT NULL,
    connected_at timestamp with time zone,
    terminated_at timestamp with time zone,
    termination_reason text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    decision_snapshot jsonb DEFAULT '{}'::jsonb NOT NULL,
    session_profile_snapshot jsonb DEFAULT '{}'::jsonb NOT NULL,
    decision_snapshot_hash bytea,
    recording_mode text DEFAULT 'required'::text NOT NULL,
    command_audit_mode text DEFAULT 'required'::text NOT NULL,
    clipboard_mode text DEFAULT 'disabled'::text NOT NULL,
    file_upload_mode text DEFAULT 'disabled'::text NOT NULL,
    file_download_mode text DEFAULT 'disabled'::text NOT NULL,
    port_forward_mode text DEFAULT 'disabled'::text NOT NULL,
    session_share_mode text DEFAULT 'disabled'::text NOT NULL,
    retention_days integer DEFAULT 90 NOT NULL,
    gateway_instance text,
    reason text DEFAULT ''::text NOT NULL,
    CONSTRAINT remote_access_session_decision_snapshot_ck CHECK ((jsonb_typeof(decision_snapshot) = 'object'::text)),
    CONSTRAINT remote_access_session_profile_snapshot_ck CHECK ((jsonb_typeof(session_profile_snapshot) = 'object'::text)),
    CONSTRAINT remote_access_session_snapshot_hash_ck CHECK (((decision_snapshot_hash IS NULL) OR (octet_length(decision_snapshot_hash) = 32))),
    CONSTRAINT remote_access_sessions_authorization_version_check CHECK ((authorization_version > 0)),
    CONSTRAINT remote_access_sessions_clipboard_mode_check CHECK ((clipboard_mode = ANY (ARRAY['enabled'::text, 'disabled'::text]))),
    CONSTRAINT remote_access_sessions_command_audit_mode_check CHECK ((command_audit_mode = ANY (ARRAY['required'::text, 'optional'::text, 'disabled'::text]))),
    CONSTRAINT remote_access_sessions_control_path_check CHECK ((control_path = ANY (ARRAY['direct'::text, 'bastion_relay'::text, 'executor_tunnel'::text]))),
    CONSTRAINT remote_access_sessions_file_download_mode_check CHECK ((file_download_mode = ANY (ARRAY['enabled'::text, 'disabled'::text]))),
    CONSTRAINT remote_access_sessions_file_upload_mode_check CHECK ((file_upload_mode = ANY (ARRAY['enabled'::text, 'disabled'::text]))),
    CONSTRAINT remote_access_sessions_idle_timeout_seconds_check CHECK (((idle_timeout_seconds >= 60) AND (idle_timeout_seconds <= 900))),
    CONSTRAINT remote_access_sessions_max_duration_seconds_check CHECK (((max_duration_seconds >= 60) AND (max_duration_seconds <= 3600))),
    CONSTRAINT remote_access_sessions_port_forward_mode_check CHECK ((port_forward_mode = ANY (ARRAY['enabled'::text, 'disabled'::text]))),
    CONSTRAINT remote_access_sessions_protocol_check CHECK ((protocol = ANY (ARRAY['shell'::text, 'ssh'::text, 'rdp'::text]))),
    CONSTRAINT remote_access_sessions_recording_mode_check CHECK ((recording_mode = ANY (ARRAY['required'::text, 'optional'::text, 'disabled'::text]))),
    CONSTRAINT remote_access_sessions_retention_days_check CHECK (((retention_days >= 1) AND (retention_days <= 3650))),
    CONSTRAINT remote_access_sessions_session_fence_check CHECK ((session_fence > 0)),
    CONSTRAINT remote_access_sessions_session_share_mode_check CHECK ((session_share_mode = ANY (ARRAY['enabled'::text, 'disabled'::text]))),
    CONSTRAINT remote_access_sessions_status_check CHECK ((status = ANY (ARRAY['requested'::text, 'awaiting_approval'::text, 'authorized'::text, 'connecting'::text, 'active'::text, 'terminating'::text, 'terminated'::text, 'failed'::text, 'expired'::text, 'connection_lost'::text, 'invalidated'::text])))
);


--
-- Name: remote_access_tickets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.remote_access_tickets (
    id uuid NOT NULL,
    session_id uuid NOT NULL,
    ticket_hash bytea NOT NULL,
    http_session_id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    user_id uuid NOT NULL,
    host_id uuid NOT NULL,
    managed_account_id uuid NOT NULL,
    protocol text NOT NULL,
    lease_id uuid NOT NULL,
    authorization_version bigint NOT NULL,
    session_fence bigint NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    consumed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT statement_timestamp() NOT NULL,
    CONSTRAINT remote_access_tickets_authorization_version_check CHECK ((authorization_version > 0)),
    CONSTRAINT remote_access_tickets_check CHECK ((expires_at <= (created_at + '00:01:00'::interval))),
    CONSTRAINT remote_access_tickets_protocol_check CHECK ((protocol = ANY (ARRAY['shell'::text, 'ssh'::text, 'rdp'::text]))),
    CONSTRAINT remote_access_tickets_session_fence_check CHECK ((session_fence > 0)),
    CONSTRAINT remote_access_tickets_ticket_hash_check CHECK ((octet_length(ticket_hash) = 32))
);


--
-- Name: role_bindings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.role_bindings (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    subject_type text NOT NULL,
    subject_id uuid NOT NULL,
    role_id uuid NOT NULL,
    valid_from timestamp with time zone,
    valid_until timestamp with time zone,
    status text DEFAULT 'active'::text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT role_bindings_check CHECK (((valid_until IS NULL) OR (valid_from IS NULL) OR (valid_until > valid_from))),
    CONSTRAINT role_bindings_status_check CHECK ((status = ANY (ARRAY['active'::text, 'disabled'::text]))),
    CONSTRAINT role_bindings_subject_type_check CHECK ((subject_type = ANY (ARRAY['user'::text, 'department'::text, 'service_account'::text]))),
    CONSTRAINT role_bindings_version_check CHECK ((version > 0))
);


--
-- Name: role_permissions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.role_permissions (
    role_id uuid NOT NULL,
    permission_id text NOT NULL
);


--
-- Name: roles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.roles (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    identity_key text,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    builtin boolean DEFAULT false NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT roles_check CHECK (((builtin AND (identity_key IS NOT NULL)) OR ((NOT builtin) AND (identity_key IS NULL)))),
    CONSTRAINT roles_name_check CHECK (((char_length(name) >= 1) AND (char_length(name) <= 128))),
    CONSTRAINT roles_status_check CHECK ((status = ANY (ARRAY['active'::text, 'disabled'::text]))),
    CONSTRAINT roles_version_check CHECK ((version > 0))
);


--
-- Name: run_steps; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.run_steps (
    id uuid NOT NULL,
    run_id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    sequence integer NOT NULL,
    step_type text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    attempt integer DEFAULT 0 NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT run_steps_attempt_check CHECK ((attempt >= 0)),
    CONSTRAINT run_steps_sequence_check CHECK ((sequence > 0)),
    CONSTRAINT run_steps_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'leased'::text, 'running'::text, 'waiting_input'::text, 'waiting_approval'::text, 'succeeded'::text, 'failed'::text, 'cancelled'::text, 'timed_out'::text]))),
    CONSTRAINT run_steps_step_type_check CHECK ((step_type = ANY (ARRAY['model_call'::text, 'tool_call'::text, 'pending_action'::text, 'execution'::text, 'context_compaction'::text, 'verification'::text]))),
    CONSTRAINT run_steps_version_check CHECK ((version > 0))
);


--
-- Name: runs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.runs (
    id uuid NOT NULL,
    conversation_id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    actor_user_id uuid NOT NULL,
    model_id uuid NOT NULL,
    model_revision integer NOT NULL,
    locale text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    current_step_id uuid,
    authorization_version bigint NOT NULL,
    checkpoint jsonb DEFAULT '{}'::jsonb NOT NULL,
    stop_reason text,
    error_code text,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT runs_authorization_version_check CHECK ((authorization_version > 0)),
    CONSTRAINT runs_locale_check CHECK ((locale = ANY (ARRAY['zh-CN'::text, 'en-US'::text]))),
    CONSTRAINT runs_model_revision_check CHECK ((model_revision > 0)),
    CONSTRAINT runs_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'running'::text, 'waiting_input'::text, 'waiting_approval'::text, 'waiting_system'::text, 'succeeded'::text, 'failed'::text, 'cancelled'::text, 'timed_out'::text]))),
    CONSTRAINT runs_version_check CHECK ((version > 0))
);


--
-- Name: runtime_tasks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.runtime_tasks (
    id uuid NOT NULL,
    enterprise_id uuid,
    queue text NOT NULL,
    run_id uuid,
    step_id uuid,
    payload jsonb DEFAULT '{}'::jsonb NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    attempt integer DEFAULT 0 NOT NULL,
    max_attempts integer DEFAULT 5 NOT NULL,
    lease_owner text,
    lease_until timestamp with time zone,
    fence_token bigint DEFAULT 0 NOT NULL,
    available_at timestamp with time zone DEFAULT now() NOT NULL,
    last_error_code text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT runtime_tasks_attempt_check CHECK ((attempt >= 0)),
    CONSTRAINT runtime_tasks_check CHECK ((((run_id IS NULL) AND (step_id IS NULL)) OR (enterprise_id IS NOT NULL))),
    CONSTRAINT runtime_tasks_fence_token_check CHECK ((fence_token >= 0)),
    CONSTRAINT runtime_tasks_max_attempts_check CHECK ((max_attempts > 0)),
    CONSTRAINT runtime_tasks_queue_check CHECK ((queue = ANY (ARRAY['agent'::text, 'action'::text, 'compaction'::text, 'sandbox'::text]))),
    CONSTRAINT runtime_tasks_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'leased'::text, 'running'::text, 'succeeded'::text, 'failed'::text, 'cancelled'::text, 'timed_out'::text])))
);


--
-- Name: sandbox_backends; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sandbox_backends (
    id uuid NOT NULL,
    name text NOT NULL,
    endpoint text NOT NULL,
    credential_provider text,
    credential_key_id text,
    credential_key_version integer,
    credential_wrapped_dek bytea,
    credential_wrap_nonce bytea,
    credential_nonce bytea,
    credential_ciphertext bytea,
    credential_value_hash bytea,
    status text DEFAULT 'enabled'::text NOT NULL,
    health_status text DEFAULT 'unknown'::text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT sandbox_backends_credential_envelope_check CHECK ((((credential_provider IS NULL) AND (credential_key_id IS NULL) AND (credential_key_version IS NULL) AND (credential_wrapped_dek IS NULL) AND (credential_wrap_nonce IS NULL) AND (credential_nonce IS NULL) AND (credential_ciphertext IS NULL) AND (credential_value_hash IS NULL)) OR ((credential_provider = ANY (ARRAY['local'::text, 'openbao_transit'::text])) AND (credential_key_id IS NOT NULL) AND (credential_key_version IS NOT NULL) AND (credential_key_version > 0) AND (credential_wrapped_dek IS NOT NULL) AND (octet_length(credential_wrapped_dek) > 0) AND (credential_nonce IS NOT NULL) AND (octet_length(credential_nonce) = 12) AND (credential_ciphertext IS NOT NULL) AND (octet_length(credential_ciphertext) > 0) AND (credential_value_hash IS NOT NULL) AND (octet_length(credential_value_hash) = 32) AND (((credential_provider = 'local'::text) AND (credential_wrap_nonce IS NOT NULL) AND (octet_length(credential_wrap_nonce) = 12)) OR ((credential_provider = 'openbao_transit'::text) AND (credential_wrap_nonce IS NULL)))))),
    CONSTRAINT sandbox_backends_endpoint_check CHECK (((char_length(endpoint) >= 1) AND (char_length(endpoint) <= 2048))),
    CONSTRAINT sandbox_backends_health_status_check CHECK ((health_status = ANY (ARRAY['unknown'::text, 'healthy'::text, 'unhealthy'::text]))),
    CONSTRAINT sandbox_backends_name_check CHECK (((char_length(name) >= 1) AND (char_length(name) <= 128))),
    CONSTRAINT sandbox_backends_status_check CHECK ((status = ANY (ARRAY['enabled'::text, 'disabled'::text]))),
    CONSTRAINT sandbox_backends_version_check CHECK ((version > 0))
);


--
-- Name: sandbox_images; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sandbox_images (
    id uuid NOT NULL,
    backend_id uuid NOT NULL,
    name text NOT NULL,
    image_ref text NOT NULL,
    digest text NOT NULL,
    status text DEFAULT 'enabled'::text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT sandbox_images_digest_check CHECK ((digest ~ '^sha256:[a-f0-9]{64}$'::text)),
    CONSTRAINT sandbox_images_status_check CHECK ((status = ANY (ARRAY['enabled'::text, 'disabled'::text]))),
    CONSTRAINT sandbox_images_version_check CHECK ((version > 0))
);


--
-- Name: sandbox_profiles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sandbox_profiles (
    id uuid NOT NULL,
    name text NOT NULL,
    backend_id uuid NOT NULL,
    image_id uuid NOT NULL,
    task_kinds text[] NOT NULL,
    cpu_millis integer NOT NULL,
    memory_mib integer NOT NULL,
    timeout_seconds integer NOT NULL,
    network_mode text NOT NULL,
    status text DEFAULT 'enabled'::text NOT NULL,
    revision integer DEFAULT 1 NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT sandbox_profiles_cpu_millis_check CHECK (((cpu_millis >= 100) AND (cpu_millis <= 16000))),
    CONSTRAINT sandbox_profiles_memory_mib_check CHECK (((memory_mib >= 128) AND (memory_mib <= 65536))),
    CONSTRAINT sandbox_profiles_network_mode_check CHECK ((network_mode = ANY (ARRAY['none'::text, 'restricted'::text]))),
    CONSTRAINT sandbox_profiles_revision_check CHECK ((revision > 0)),
    CONSTRAINT sandbox_profiles_status_check CHECK ((status = ANY (ARRAY['enabled'::text, 'disabled'::text]))),
    CONSTRAINT sandbox_profiles_task_kinds_check CHECK ((cardinality(task_kinds) > 0)),
    CONSTRAINT sandbox_profiles_timeout_seconds_check CHECK (((timeout_seconds >= 10) AND (timeout_seconds <= 3600))),
    CONSTRAINT sandbox_profiles_version_check CHECK ((version > 0))
);


--
-- Name: sandbox_quotas; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sandbox_quotas (
    enterprise_id uuid NOT NULL,
    max_concurrent_sessions integer NOT NULL,
    monthly_session_seconds bigint NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT sandbox_quotas_max_concurrent_sessions_check CHECK (((max_concurrent_sessions >= 0) AND (max_concurrent_sessions <= 10000))),
    CONSTRAINT sandbox_quotas_monthly_session_seconds_check CHECK ((monthly_session_seconds >= 0)),
    CONSTRAINT sandbox_quotas_version_check CHECK ((version > 0))
);


--
-- Name: sandbox_sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sandbox_sessions (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    task_id uuid NOT NULL,
    profile_id uuid NOT NULL,
    profile_revision integer NOT NULL,
    upstream_session_id text NOT NULL,
    status text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    started_at timestamp with time zone,
    terminated_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT sandbox_sessions_profile_revision_check CHECK ((profile_revision > 0)),
    CONSTRAINT sandbox_sessions_status_check CHECK ((status = ANY (ARRAY['creating'::text, 'running'::text, 'terminating'::text, 'terminated'::text, 'failed'::text, 'unknown'::text])))
);


--
-- Name: sandbox_usage; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sandbox_usage (
    enterprise_id uuid NOT NULL,
    month date NOT NULL,
    session_count bigint DEFAULT 0 NOT NULL,
    session_seconds bigint DEFAULT 0 NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT sandbox_usage_session_count_check CHECK ((session_count >= 0)),
    CONSTRAINT sandbox_usage_session_seconds_check CHECK ((session_seconds >= 0))
);


--
-- Name: secret_versions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.secret_versions (
    id uuid NOT NULL,
    secret_id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    version integer NOT NULL,
    provider text DEFAULT 'local'::text NOT NULL,
    key_id text NOT NULL,
    key_version integer NOT NULL,
    wrapped_dek bytea NOT NULL,
    wrap_nonce bytea,
    nonce bytea NOT NULL,
    ciphertext bytea NOT NULL,
    value_hash bytea NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT secret_versions_envelope_check CHECK (((octet_length(wrapped_dek) > 0) AND (octet_length(nonce) = 12) AND (octet_length(ciphertext) > 0) AND (((provider = 'local'::text) AND (wrap_nonce IS NOT NULL) AND (octet_length(wrap_nonce) = 12)) OR ((provider = 'openbao_transit'::text) AND (wrap_nonce IS NULL))))),
    CONSTRAINT secret_versions_key_version_check CHECK ((key_version > 0)),
    CONSTRAINT secret_versions_provider_check CHECK ((provider = ANY (ARRAY['local'::text, 'openbao_transit'::text]))),
    CONSTRAINT secret_versions_value_hash_check CHECK ((octet_length(value_hash) = 32)),
    CONSTRAINT secret_versions_version_check CHECK ((version > 0))
);


--
-- Name: secrets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.secrets (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    name text NOT NULL,
    type text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    current_version integer DEFAULT 1 NOT NULL,
    last_accessed_at timestamp with time zone,
    version bigint DEFAULT 1 NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT secrets_current_version_check CHECK ((current_version > 0)),
    CONSTRAINT secrets_name_check CHECK (((char_length(name) >= 1) AND (char_length(name) <= 128))),
    CONSTRAINT secrets_status_check CHECK ((status = ANY (ARRAY['active'::text, 'disabled'::text]))),
    CONSTRAINT secrets_type_check CHECK ((type = ANY (ARRAY['ssh_password'::text, 'ssh_private_key'::text, 'windows_password'::text, 'kubeconfig'::text, 'api_token'::text, 'basic_auth'::text]))),
    CONSTRAINT secrets_version_check CHECK ((version > 0))
);


--
-- Name: service_accounts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.service_accounts (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    allowed_tool_ids text[] DEFAULT '{}'::text[] NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    authorization_version bigint DEFAULT 1 NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT service_accounts_authorization_version_check CHECK ((authorization_version > 0)),
    CONSTRAINT service_accounts_name_check CHECK (((char_length(name) >= 1) AND (char_length(name) <= 128))),
    CONSTRAINT service_accounts_status_check CHECK ((status = ANY (ARRAY['active'::text, 'disabled'::text]))),
    CONSTRAINT service_accounts_version_check CHECK ((version > 0))
);


--
-- Name: sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sessions (
    id uuid NOT NULL,
    token_hash bytea NOT NULL,
    csrf_hash bytea NOT NULL,
    audience text NOT NULL,
    user_id uuid NOT NULL,
    enterprise_id uuid,
    department_id uuid,
    authorization_version bigint,
    locale text NOT NULL,
    idle_expires_at timestamp with time zone NOT NULL,
    absolute_expires_at timestamp with time zone NOT NULL,
    last_seen_at timestamp with time zone NOT NULL,
    revoked_at timestamp with time zone,
    revoke_reason text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    authenticated_at timestamp with time zone DEFAULT now() NOT NULL,
    step_up_expires_at timestamp with time zone,
    amr text[] DEFAULT ARRAY['password'::text] NOT NULL,
    CONSTRAINT sessions_audience_check CHECK ((audience = ANY (ARRAY['platform'::text, 'enterprise'::text]))),
    CONSTRAINT sessions_check CHECK ((((audience = 'platform'::text) AND (enterprise_id IS NULL) AND (department_id IS NULL) AND (authorization_version IS NULL)) OR ((audience = 'enterprise'::text) AND (enterprise_id IS NOT NULL) AND (department_id IS NOT NULL) AND (authorization_version IS NOT NULL)))),
    CONSTRAINT sessions_csrf_hash_check CHECK ((octet_length(csrf_hash) = 32)),
    CONSTRAINT sessions_locale_check CHECK ((locale = ANY (ARRAY['zh-CN'::text, 'en-US'::text]))),
    CONSTRAINT sessions_token_hash_check CHECK ((octet_length(token_hash) = 32))
);


--
-- Name: telemetry_certificates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.telemetry_certificates (
    id uuid NOT NULL,
    collector_id uuid NOT NULL,
    serial_number text NOT NULL,
    uri_san text NOT NULL,
    csr_hash bytea NOT NULL,
    certificate_hash bytea NOT NULL,
    certificate_request_name text NOT NULL,
    issuer_generation integer NOT NULL,
    not_before timestamp with time zone NOT NULL,
    not_after timestamp with time zone NOT NULL,
    revoked_at timestamp with time zone,
    revoke_reason text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    certificate_usage text NOT NULL,
    CONSTRAINT telemetry_certificates_certificate_hash_check CHECK ((octet_length(certificate_hash) = 32)),
    CONSTRAINT telemetry_certificates_certificate_usage_check CHECK ((certificate_usage = ANY (ARRAY['clientAuth'::text, 'serverAuth'::text]))),
    CONSTRAINT telemetry_certificates_check CHECK ((not_after <= (not_before + '24:00:00'::interval))),
    CONSTRAINT telemetry_certificates_csr_hash_check CHECK ((octet_length(csr_hash) = 32)),
    CONSTRAINT telemetry_certificates_issuer_generation_check CHECK ((issuer_generation > 0))
);


--
-- Name: telemetry_collector_operations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.telemetry_collector_operations (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    collector_id uuid NOT NULL,
    pending_action_id uuid NOT NULL,
    operation text NOT NULL,
    executor_kind text NOT NULL,
    status text DEFAULT 'queued'::text NOT NULL,
    plan jsonb NOT NULL,
    plan_hash bytea NOT NULL,
    lease_owner text,
    fence bigint DEFAULT 0 NOT NULL,
    lease_expires_at timestamp with time zone,
    result_hash bytea,
    error_code text,
    attempts integer DEFAULT 0 NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    completed_at timestamp with time zone,
    CONSTRAINT telemetry_collector_operations_attempts_check CHECK ((attempts >= 0)),
    CONSTRAINT telemetry_collector_operations_executor_kind_check CHECK ((executor_kind = ANY (ARRAY['direct'::text, 'bootstrap'::text]))),
    CONSTRAINT telemetry_collector_operations_fence_check CHECK ((fence >= 0)),
    CONSTRAINT telemetry_collector_operations_operation_check CHECK ((operation = ANY (ARRAY['install'::text, 'configure'::text, 'upgrade'::text, 'repair'::text, 'uninstall'::text]))),
    CONSTRAINT telemetry_collector_operations_plan_check CHECK ((jsonb_typeof(plan) = 'object'::text)),
    CONSTRAINT telemetry_collector_operations_plan_hash_check CHECK ((octet_length(plan_hash) = 32)),
    CONSTRAINT telemetry_collector_operations_result_hash_check CHECK (((result_hash IS NULL) OR (octet_length(result_hash) = 32))),
    CONSTRAINT telemetry_collector_operations_status_check CHECK ((status = ANY (ARRAY['queued'::text, 'running'::text, 'succeeded'::text, 'failed'::text, 'result_unknown'::text, 'expired'::text])))
);


--
-- Name: telemetry_dlq_records; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.telemetry_dlq_records (
    id uuid NOT NULL,
    signal text NOT NULL,
    topic text NOT NULL,
    partition integer NOT NULL,
    source_offset bigint NOT NULL,
    dlq_topic text NOT NULL,
    dlq_partition integer NOT NULL,
    dlq_offset bigint NOT NULL,
    record_hash bytea NOT NULL,
    error_code text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    first_seen_at timestamp with time zone DEFAULT now() NOT NULL,
    replayed_at timestamp with time zone,
    CONSTRAINT telemetry_dlq_records_dlq_offset_check CHECK ((dlq_offset >= 0)),
    CONSTRAINT telemetry_dlq_records_dlq_partition_check CHECK ((dlq_partition >= 0)),
    CONSTRAINT telemetry_dlq_records_partition_check CHECK ((partition >= 0)),
    CONSTRAINT telemetry_dlq_records_record_hash_check CHECK ((octet_length(record_hash) = 32)),
    CONSTRAINT telemetry_dlq_records_signal_check CHECK ((signal = ANY (ARRAY['metrics'::text, 'logs'::text, 'traces'::text]))),
    CONSTRAINT telemetry_dlq_records_source_offset_check CHECK ((source_offset >= 0)),
    CONSTRAINT telemetry_dlq_records_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'replaying'::text, 'replayed'::text, 'discarded'::text, 'failed'::text])))
);


--
-- Name: telemetry_enrollment_tokens; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.telemetry_enrollment_tokens (
    id uuid NOT NULL,
    collector_id uuid NOT NULL,
    token_hash bytea NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    consumed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT statement_timestamp() NOT NULL,
    CONSTRAINT telemetry_enrollment_tokens_check CHECK ((expires_at <= (created_at + '00:10:00'::interval))),
    CONSTRAINT telemetry_enrollment_tokens_token_hash_check CHECK ((octet_length(token_hash) = 32))
);


--
-- Name: telemetry_retention_policies; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.telemetry_retention_policies (
    enterprise_id uuid NOT NULL,
    metrics_days integer DEFAULT 30 NOT NULL,
    logs_days integer DEFAULT 14 NOT NULL,
    traces_days integer DEFAULT 7 NOT NULL,
    max_rows integer DEFAULT 50000 NOT NULL,
    max_scan_bytes bigint DEFAULT 268435456 NOT NULL,
    max_execution_ms integer DEFAULT 10000 NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT telemetry_retention_policies_logs_days_check CHECK (((logs_days >= 1) AND (logs_days <= 3650))),
    CONSTRAINT telemetry_retention_policies_max_execution_ms_check CHECK (((max_execution_ms >= 100) AND (max_execution_ms <= 30000))),
    CONSTRAINT telemetry_retention_policies_max_rows_check CHECK (((max_rows >= 1) AND (max_rows <= 100000))),
    CONSTRAINT telemetry_retention_policies_max_scan_bytes_check CHECK (((max_scan_bytes >= 1048576) AND (max_scan_bytes <= 1073741824))),
    CONSTRAINT telemetry_retention_policies_metrics_days_check CHECK (((metrics_days >= 1) AND (metrics_days <= 3650))),
    CONSTRAINT telemetry_retention_policies_traces_days_check CHECK (((traces_days >= 1) AND (traces_days <= 3650))),
    CONSTRAINT telemetry_retention_policies_version_check CHECK ((version > 0))
);


--
-- Name: telemetry_route_tests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.telemetry_route_tests (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    route_id uuid NOT NULL,
    status text NOT NULL,
    result_code text,
    result_hash bytea,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    completed_at timestamp with time zone,
    CONSTRAINT telemetry_route_tests_result_hash_check CHECK (((result_hash IS NULL) OR (octet_length(result_hash) = 32))),
    CONSTRAINT telemetry_route_tests_status_check CHECK ((status = ANY (ARRAY['queued'::text, 'running'::text, 'succeeded'::text, 'failed'::text, 'expired'::text])))
);


--
-- Name: telemetry_routes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.telemetry_routes (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    collector_id uuid NOT NULL,
    kind text NOT NULL,
    gateway_collector_id uuid,
    status text DEFAULT 'pending'::text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    last_tested_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    transport text NOT NULL,
    loopback_port integer,
    CONSTRAINT telemetry_routes_kind_check CHECK ((kind = ANY (ARRAY['direct_argus'::text, 'bastion_gateway'::text, 'kubernetes_gateway'::text]))),
    CONSTRAINT telemetry_routes_kind_transport_check CHECK (((kind <> 'kubernetes_gateway'::text) OR (transport = 'direct'::text))),
    CONSTRAINT telemetry_routes_loopback_check CHECK ((((transport = 'direct'::text) AND (loopback_port IS NULL)) OR ((transport = ANY (ARRAY['executor_tunnel'::text, 'bastion_tunnel'::text])) AND ((loopback_port >= 1) AND (loopback_port <= 65534))))),
    CONSTRAINT telemetry_routes_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'testing'::text, 'active'::text, 'degraded'::text, 'invalidated'::text]))),
    CONSTRAINT telemetry_routes_transport_check CHECK ((transport = ANY (ARRAY['direct'::text, 'executor_tunnel'::text, 'bastion_tunnel'::text]))),
    CONSTRAINT telemetry_routes_version_check CHECK ((version > 0))
);


--
-- Name: telemetry_tunnels; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.telemetry_tunnels (
    id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    host_id uuid NOT NULL,
    collector_id uuid NOT NULL,
    connector_id uuid,
    credential_id uuid NOT NULL,
    credential_version bigint NOT NULL,
    target_address text NOT NULL,
    target_port integer NOT NULL,
    target_username text NOT NULL,
    pinned_host_key text NOT NULL,
    initiator text NOT NULL,
    transport text NOT NULL,
    loopback_port integer NOT NULL,
    forward_target text NOT NULL,
    status text DEFAULT 'desired'::text NOT NULL,
    epoch bigint DEFAULT 1 NOT NULL,
    lease_owner text DEFAULT ''::text NOT NULL,
    owner_connection_epoch bigint DEFAULT 0 NOT NULL,
    fence bigint DEFAULT 0 NOT NULL,
    lease_expires_at timestamp with time zone,
    last_claim_at timestamp with time zone,
    last_established_at timestamp with time zone,
    last_heartbeat_at timestamp with time zone,
    last_drop_reason text DEFAULT ''::text NOT NULL,
    reconnect_attempt integer DEFAULT 0 NOT NULL,
    next_claim_at timestamp with time zone,
    bytes_relayed bigint DEFAULT 0 NOT NULL,
    throttled_events bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT telemetry_tunnels_bytes_relayed_check CHECK ((bytes_relayed >= 0)),
    CONSTRAINT telemetry_tunnels_check CHECK (((initiator <> 'direct_executor'::text) OR (transport = 'executor_tunnel'::text))),
    CONSTRAINT telemetry_tunnels_check1 CHECK (((initiator <> 'connector'::text) OR ((transport = 'bastion_tunnel'::text) AND (connector_id IS NOT NULL)))),
    CONSTRAINT telemetry_tunnels_check2 CHECK (((initiator <> 'direct_executor'::text) OR (connector_id IS NULL))),
    CONSTRAINT telemetry_tunnels_credential_version_check CHECK ((credential_version > 0)),
    CONSTRAINT telemetry_tunnels_epoch_check CHECK ((epoch > 0)),
    CONSTRAINT telemetry_tunnels_fence_check CHECK ((fence >= 0)),
    CONSTRAINT telemetry_tunnels_forward_target_check CHECK (((char_length(forward_target) >= 1) AND (char_length(forward_target) <= 256))),
    CONSTRAINT telemetry_tunnels_initiator_check CHECK ((initiator = ANY (ARRAY['direct_executor'::text, 'connector'::text]))),
    CONSTRAINT telemetry_tunnels_loopback_port_check CHECK (((loopback_port >= 1) AND (loopback_port <= 65534))),
    CONSTRAINT telemetry_tunnels_owner_connection_epoch_check CHECK ((owner_connection_epoch >= 0)),
    CONSTRAINT telemetry_tunnels_pinned_host_key_check CHECK (((char_length(pinned_host_key) >= 1) AND (char_length(pinned_host_key) <= 512))),
    CONSTRAINT telemetry_tunnels_reconnect_attempt_check CHECK (((reconnect_attempt >= 0) AND (reconnect_attempt <= 30))),
    CONSTRAINT telemetry_tunnels_status_check CHECK ((status = ANY (ARRAY['desired'::text, 'establishing'::text, 'established'::text, 'degraded'::text, 'down'::text, 'removed'::text]))),
    CONSTRAINT telemetry_tunnels_target_address_check CHECK (((char_length(target_address) >= 1) AND (char_length(target_address) <= 512))),
    CONSTRAINT telemetry_tunnels_target_port_check CHECK (((target_port >= 1) AND (target_port <= 65535))),
    CONSTRAINT telemetry_tunnels_target_username_check CHECK (((char_length(target_username) >= 1) AND (char_length(target_username) <= 256))),
    CONSTRAINT telemetry_tunnels_throttled_events_check CHECK ((throttled_events >= 0)),
    CONSTRAINT telemetry_tunnels_transport_check CHECK ((transport = ANY (ARRAY['executor_tunnel'::text, 'bastion_tunnel'::text])))
);


--
-- Name: telemetry_usage_daily; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.telemetry_usage_daily (
    enterprise_id uuid NOT NULL,
    usage_date date NOT NULL,
    ingested_bytes bigint DEFAULT 0 NOT NULL,
    metric_points bigint DEFAULT 0 NOT NULL,
    log_records bigint DEFAULT 0 NOT NULL,
    spans bigint DEFAULT 0 NOT NULL,
    estimated_storage_bytes bigint DEFAULT 0 NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT telemetry_usage_daily_estimated_storage_bytes_check CHECK ((estimated_storage_bytes >= 0)),
    CONSTRAINT telemetry_usage_daily_ingested_bytes_check CHECK ((ingested_bytes >= 0)),
    CONSTRAINT telemetry_usage_daily_log_records_check CHECK ((log_records >= 0)),
    CONSTRAINT telemetry_usage_daily_metric_points_check CHECK ((metric_points >= 0)),
    CONSTRAINT telemetry_usage_daily_spans_check CHECK ((spans >= 0))
);


--
-- Name: temporary_credentials; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.temporary_credentials (
    id uuid NOT NULL,
    audience text NOT NULL,
    user_id uuid NOT NULL,
    challenge_hash bytea,
    status text DEFAULT 'active'::text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    consumed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT temporary_credentials_audience_check CHECK ((audience = ANY (ARRAY['platform'::text, 'enterprise'::text]))),
    CONSTRAINT temporary_credentials_challenge_hash_check CHECK (((challenge_hash IS NULL) OR (octet_length(challenge_hash) = 32))),
    CONSTRAINT temporary_credentials_status_check CHECK ((status = ANY (ARRAY['active'::text, 'consumed'::text, 'expired'::text, 'revoked'::text])))
);


--
-- Name: tool_calls; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.tool_calls (
    id uuid NOT NULL,
    call_id text NOT NULL,
    enterprise_id uuid NOT NULL,
    run_id uuid NOT NULL,
    step_id uuid NOT NULL,
    tool_id text NOT NULL,
    input jsonb NOT NULL,
    input_hash bytea NOT NULL,
    status text NOT NULL,
    error_code text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT tool_calls_input_hash_check CHECK ((octet_length(input_hash) = 32)),
    CONSTRAINT tool_calls_status_check CHECK ((status = ANY (ARRAY['requested'::text, 'running'::text, 'succeeded'::text, 'failed'::text, 'cancelled'::text]))),
    CONSTRAINT tool_calls_tool_id_check CHECK ((tool_id ~ '^[a-z][a-z0-9_.-]+$'::text))
);


--
-- Name: tool_results; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.tool_results (
    id uuid NOT NULL,
    tool_call_id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    artifact_id uuid NOT NULL,
    projection jsonb NOT NULL,
    projection_hash bytea NOT NULL,
    projection_bytes integer NOT NULL,
    partial boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT tool_results_projection_bytes_check CHECK (((projection_bytes >= 0) AND (projection_bytes <= 65536))),
    CONSTRAINT tool_results_projection_hash_check CHECK ((octet_length(projection_hash) = 32))
);


--
-- Name: user_confirmations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_confirmations (
    id uuid NOT NULL,
    pending_action_id uuid NOT NULL,
    enterprise_id uuid NOT NULL,
    actor_user_id uuid NOT NULL,
    authorization_version bigint NOT NULL,
    confirmed_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: action_bindings action_bindings_binding_ref_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.action_bindings
    ADD CONSTRAINT action_bindings_binding_ref_key UNIQUE (binding_ref);


--
-- Name: action_bindings action_bindings_id_enterprise_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.action_bindings
    ADD CONSTRAINT action_bindings_id_enterprise_id_key UNIQUE (id, enterprise_id);


--
-- Name: action_bindings action_bindings_pending_action_id_request_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.action_bindings
    ADD CONSTRAINT action_bindings_pending_action_id_request_id_key UNIQUE (pending_action_id, request_id);


--
-- Name: action_bindings action_bindings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.action_bindings
    ADD CONSTRAINT action_bindings_pkey PRIMARY KEY (id);


--
-- Name: ai_model_credentials ai_model_credentials_model_revision_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_model_credentials
    ADD CONSTRAINT ai_model_credentials_model_revision_id_key UNIQUE (model_revision_id);


--
-- Name: ai_model_credentials ai_model_credentials_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_model_credentials
    ADD CONSTRAINT ai_model_credentials_pkey PRIMARY KEY (id);


--
-- Name: ai_model_revisions ai_model_revisions_id_enterprise_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_model_revisions
    ADD CONSTRAINT ai_model_revisions_id_enterprise_id_key UNIQUE (id, enterprise_id);


--
-- Name: ai_model_revisions ai_model_revisions_model_id_revision_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_model_revisions
    ADD CONSTRAINT ai_model_revisions_model_id_revision_key UNIQUE (model_id, revision);


--
-- Name: ai_model_revisions ai_model_revisions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_model_revisions
    ADD CONSTRAINT ai_model_revisions_pkey PRIMARY KEY (id);


--
-- Name: ai_models ai_models_id_enterprise_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_models
    ADD CONSTRAINT ai_models_id_enterprise_id_key UNIQUE (id, enterprise_id);


--
-- Name: ai_models ai_models_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_models
    ADD CONSTRAINT ai_models_pkey PRIMARY KEY (id);


--
-- Name: api_keys api_keys_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_keys
    ADD CONSTRAINT api_keys_pkey PRIMARY KEY (id);


--
-- Name: api_keys api_keys_prefix_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_keys
    ADD CONSTRAINT api_keys_prefix_key UNIQUE (prefix);


--
-- Name: approval_decisions approval_decisions_approval_request_id_actor_user_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_decisions
    ADD CONSTRAINT approval_decisions_approval_request_id_actor_user_id_key UNIQUE (approval_request_id, actor_user_id);


--
-- Name: approval_decisions approval_decisions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_decisions
    ADD CONSTRAINT approval_decisions_pkey PRIMARY KEY (id);


--
-- Name: approval_policies approval_policies_id_enterprise_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_policies
    ADD CONSTRAINT approval_policies_id_enterprise_id_key UNIQUE (id, enterprise_id);


--
-- Name: approval_policies approval_policies_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_policies
    ADD CONSTRAINT approval_policies_pkey PRIMARY KEY (id);


--
-- Name: approval_requests approval_requests_pending_action_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_requests
    ADD CONSTRAINT approval_requests_pending_action_id_key UNIQUE (pending_action_id);


--
-- Name: approval_requests approval_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_requests
    ADD CONSTRAINT approval_requests_pkey PRIMARY KEY (id);


--
-- Name: approval_requirement_snapshots approval_requirement_snapshot_approval_request_id_policy_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_requirement_snapshots
    ADD CONSTRAINT approval_requirement_snapshot_approval_request_id_policy_id_key UNIQUE (approval_request_id, policy_id);


--
-- Name: approval_requirement_snapshots approval_requirement_snapshots_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_requirement_snapshots
    ADD CONSTRAINT approval_requirement_snapshots_pkey PRIMARY KEY (id);


--
-- Name: artifacts artifacts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.artifacts
    ADD CONSTRAINT artifacts_pkey PRIMARY KEY (id);


--
-- Name: artifacts artifacts_result_ref_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.artifacts
    ADD CONSTRAINT artifacts_result_ref_key UNIQUE (result_ref);


--
-- Name: audit_chain_heads audit_chain_heads_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audit_chain_heads
    ADD CONSTRAINT audit_chain_heads_pkey PRIMARY KEY (chain_key);


--
-- Name: audit_events audit_events_event_hash_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audit_events
    ADD CONSTRAINT audit_events_event_hash_key UNIQUE (event_hash);


--
-- Name: audit_events audit_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audit_events
    ADD CONSTRAINT audit_events_pkey PRIMARY KEY (id);


--
-- Name: authorization_versions authorization_versions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.authorization_versions
    ADD CONSTRAINT authorization_versions_pkey PRIMARY KEY (enterprise_id, subject_type, subject_id);


--
-- Name: bastion_scopes bastion_scopes_id_enterprise_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bastion_scopes
    ADD CONSTRAINT bastion_scopes_id_enterprise_id_key UNIQUE (id, enterprise_id);


--
-- Name: bastion_scopes bastion_scopes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bastion_scopes
    ADD CONSTRAINT bastion_scopes_pkey PRIMARY KEY (id);


--
-- Name: break_glass_sessions break_glass_sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.break_glass_sessions
    ADD CONSTRAINT break_glass_sessions_pkey PRIMARY KEY (id);


--
-- Name: card_data_sources card_data_sources_card_instance_id_slot_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_data_sources
    ADD CONSTRAINT card_data_sources_card_instance_id_slot_name_key UNIQUE (card_instance_id, slot_name);


--
-- Name: card_data_sources card_data_sources_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_data_sources
    ADD CONSTRAINT card_data_sources_pkey PRIMARY KEY (id);


--
-- Name: card_demo_scenarios card_demo_scenarios_card_version_id_scenario_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_demo_scenarios
    ADD CONSTRAINT card_demo_scenarios_card_version_id_scenario_key UNIQUE (card_version_id, scenario);


--
-- Name: card_demo_scenarios card_demo_scenarios_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_demo_scenarios
    ADD CONSTRAINT card_demo_scenarios_pkey PRIMARY KEY (id);


--
-- Name: card_instances card_instances_id_enterprise_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_instances
    ADD CONSTRAINT card_instances_id_enterprise_id_key UNIQUE (id, enterprise_id);


--
-- Name: card_instances card_instances_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_instances
    ADD CONSTRAINT card_instances_pkey PRIMARY KEY (id);


--
-- Name: card_presentations card_presentations_id_enterprise_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_presentations
    ADD CONSTRAINT card_presentations_id_enterprise_id_key UNIQUE (id, enterprise_id);


--
-- Name: card_presentations card_presentations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_presentations
    ADD CONSTRAINT card_presentations_pkey PRIMARY KEY (id);


--
-- Name: card_query_binding_specs card_query_binding_specs_card_instance_id_slot_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_query_binding_specs
    ADD CONSTRAINT card_query_binding_specs_card_instance_id_slot_name_key UNIQUE (card_instance_id, slot_name);


--
-- Name: card_query_binding_specs card_query_binding_specs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_query_binding_specs
    ADD CONSTRAINT card_query_binding_specs_pkey PRIMARY KEY (id);


--
-- Name: card_query_bindings card_query_bindings_binding_ref_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_query_bindings
    ADD CONSTRAINT card_query_bindings_binding_ref_key UNIQUE (binding_ref);


--
-- Name: card_query_bindings card_query_bindings_id_enterprise_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_query_bindings
    ADD CONSTRAINT card_query_bindings_id_enterprise_id_key UNIQUE (id, enterprise_id);


--
-- Name: card_query_bindings card_query_bindings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_query_bindings
    ADD CONSTRAINT card_query_bindings_pkey PRIMARY KEY (id);


--
-- Name: card_slot_bindings card_slot_bindings_card_version_id_slot_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_slot_bindings
    ADD CONSTRAINT card_slot_bindings_card_version_id_slot_name_key UNIQUE (card_version_id, slot_name);


--
-- Name: card_slot_bindings card_slot_bindings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_slot_bindings
    ADD CONSTRAINT card_slot_bindings_pkey PRIMARY KEY (id);


--
-- Name: card_validation_runs card_validation_runs_id_enterprise_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_validation_runs
    ADD CONSTRAINT card_validation_runs_id_enterprise_id_key UNIQUE (id, enterprise_id);


--
-- Name: card_validation_runs card_validation_runs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_validation_runs
    ADD CONSTRAINT card_validation_runs_pkey PRIMARY KEY (id);


--
-- Name: card_versions card_versions_card_id_revision_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_versions
    ADD CONSTRAINT card_versions_card_id_revision_key UNIQUE (card_id, revision);


--
-- Name: card_versions card_versions_id_card_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_versions
    ADD CONSTRAINT card_versions_id_card_id_key UNIQUE (id, card_id);


--
-- Name: card_versions card_versions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_versions
    ADD CONSTRAINT card_versions_pkey PRIMARY KEY (id);


--
-- Name: collection_claims collection_claims_id_enterprise_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.collection_claims
    ADD CONSTRAINT collection_claims_id_enterprise_id_key UNIQUE (id, enterprise_id);


--
-- Name: collection_claims collection_claims_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.collection_claims
    ADD CONSTRAINT collection_claims_pkey PRIMARY KEY (id);


--
-- Name: collection_profiles collection_profiles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.collection_profiles
    ADD CONSTRAINT collection_profiles_pkey PRIMARY KEY (id);


--
-- Name: collection_profiles collection_profiles_profile_key_version_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.collection_profiles
    ADD CONSTRAINT collection_profiles_profile_key_version_key UNIQUE (profile_key, version);


--
-- Name: collector_config_revisions collector_config_revisions_collector_id_revision_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.collector_config_revisions
    ADD CONSTRAINT collector_config_revisions_collector_id_revision_key UNIQUE (collector_id, revision);


--
-- Name: collector_config_revisions collector_config_revisions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.collector_config_revisions
    ADD CONSTRAINT collector_config_revisions_pkey PRIMARY KEY (id);


--
-- Name: collector_distribution_versions collector_distribution_versions_name_version_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.collector_distribution_versions
    ADD CONSTRAINT collector_distribution_versions_name_version_key UNIQUE (name, version);


--
-- Name: collector_distribution_versions collector_distribution_versions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.collector_distribution_versions
    ADD CONSTRAINT collector_distribution_versions_pkey PRIMARY KEY (id);


--
-- Name: collector_instances collector_instances_enterprise_id_resource_type_resource_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.collector_instances
    ADD CONSTRAINT collector_instances_enterprise_id_resource_type_resource_id_key UNIQUE (enterprise_id, resource_type, resource_id);


--
-- Name: collector_instances collector_instances_id_enterprise_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.collector_instances
    ADD CONSTRAINT collector_instances_id_enterprise_id_key UNIQUE (id, enterprise_id);


--
-- Name: collector_instances collector_instances_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.collector_instances
    ADD CONSTRAINT collector_instances_pkey PRIMARY KEY (id);


--
-- Name: connection_tests connection_tests_id_enterprise_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connection_tests
    ADD CONSTRAINT connection_tests_id_enterprise_key UNIQUE (id, enterprise_id);


--
-- Name: connection_tests connection_tests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connection_tests
    ADD CONSTRAINT connection_tests_pkey PRIMARY KEY (id);


--
-- Name: connector_certificates connector_certificates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_certificates
    ADD CONSTRAINT connector_certificates_pkey PRIMARY KEY (id);


--
-- Name: connector_certificates connector_certificates_serial_number_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_certificates
    ADD CONSTRAINT connector_certificates_serial_number_key UNIQUE (serial_number);


--
-- Name: connector_commands connector_commands_command_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_commands
    ADD CONSTRAINT connector_commands_command_id_key UNIQUE (command_id);


--
-- Name: connector_commands connector_commands_connector_id_idempotency_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_commands
    ADD CONSTRAINT connector_commands_connector_id_idempotency_key_key UNIQUE (connector_id, idempotency_key);


--
-- Name: connector_commands connector_commands_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_commands
    ADD CONSTRAINT connector_commands_pkey PRIMARY KEY (id);


--
-- Name: connector_control_tunnels connector_control_tunnels_connector_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_control_tunnels
    ADD CONSTRAINT connector_control_tunnels_connector_id_key UNIQUE (connector_id);


--
-- Name: connector_control_tunnels connector_control_tunnels_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_control_tunnels
    ADD CONSTRAINT connector_control_tunnels_pkey PRIMARY KEY (id);


--
-- Name: connector_enrollment_tokens connector_enrollment_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_enrollment_tokens
    ADD CONSTRAINT connector_enrollment_tokens_pkey PRIMARY KEY (id);


--
-- Name: connector_enrollment_tokens connector_enrollment_tokens_token_hash_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_enrollment_tokens
    ADD CONSTRAINT connector_enrollment_tokens_token_hash_key UNIQUE (token_hash);


--
-- Name: connector_install_operation_events connector_install_operation_events_operation_id_sequence_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_install_operation_events
    ADD CONSTRAINT connector_install_operation_events_operation_id_sequence_key UNIQUE (operation_id, sequence);


--
-- Name: connector_install_operation_events connector_install_operation_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_install_operation_events
    ADD CONSTRAINT connector_install_operation_events_pkey PRIMARY KEY (id);


--
-- Name: connector_install_operation_secrets connector_install_operation_secrets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_install_operation_secrets
    ADD CONSTRAINT connector_install_operation_secrets_pkey PRIMARY KEY (operation_id);


--
-- Name: connector_install_operations connector_install_operations_pending_action_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_install_operations
    ADD CONSTRAINT connector_install_operations_pending_action_id_key UNIQUE (pending_action_id);


--
-- Name: connector_install_operations connector_install_operations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_install_operations
    ADD CONSTRAINT connector_install_operations_pkey PRIMARY KEY (id);


--
-- Name: connector_release_versions connector_release_versions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_release_versions
    ADD CONSTRAINT connector_release_versions_pkey PRIMARY KEY (id);


--
-- Name: connector_release_versions connector_release_versions_version_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_release_versions
    ADD CONSTRAINT connector_release_versions_version_key UNIQUE (version);


--
-- Name: connector_sessions connector_sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_sessions
    ADD CONSTRAINT connector_sessions_pkey PRIMARY KEY (connector_id);


--
-- Name: connectors connectors_id_enterprise_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connectors
    ADD CONSTRAINT connectors_id_enterprise_id_key UNIQUE (id, enterprise_id);


--
-- Name: connectors connectors_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connectors
    ADD CONSTRAINT connectors_pkey PRIMARY KEY (id);


--
-- Name: context_snapshots context_snapshots_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.context_snapshots
    ADD CONSTRAINT context_snapshots_pkey PRIMARY KEY (id);


--
-- Name: context_snapshots context_snapshots_run_id_revision_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.context_snapshots
    ADD CONSTRAINT context_snapshots_run_id_revision_key UNIQUE (run_id, revision);


--
-- Name: context_snapshots context_snapshots_run_id_source_hash_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.context_snapshots
    ADD CONSTRAINT context_snapshots_run_id_source_hash_key UNIQUE (run_id, source_hash);


--
-- Name: conversation_events conversation_events_conversation_id_sequence_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.conversation_events
    ADD CONSTRAINT conversation_events_conversation_id_sequence_key UNIQUE (conversation_id, sequence);


--
-- Name: conversation_events conversation_events_id_enterprise_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.conversation_events
    ADD CONSTRAINT conversation_events_id_enterprise_id_key UNIQUE (id, enterprise_id);


--
-- Name: conversation_events conversation_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.conversation_events
    ADD CONSTRAINT conversation_events_pkey PRIMARY KEY (id);


--
-- Name: conversations conversations_id_enterprise_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.conversations
    ADD CONSTRAINT conversations_id_enterprise_id_key UNIQUE (id, enterprise_id);


--
-- Name: conversations conversations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.conversations
    ADD CONSTRAINT conversations_pkey PRIMARY KEY (id);


--
-- Name: credential_leases credential_leases_id_enterprise_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.credential_leases
    ADD CONSTRAINT credential_leases_id_enterprise_id_key UNIQUE (id, enterprise_id);


--
-- Name: credential_leases credential_leases_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.credential_leases
    ADD CONSTRAINT credential_leases_pkey PRIMARY KEY (id);


--
-- Name: credentials credentials_id_enterprise_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.credentials
    ADD CONSTRAINT credentials_id_enterprise_id_key UNIQUE (id, enterprise_id);


--
-- Name: credentials credentials_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.credentials
    ADD CONSTRAINT credentials_pkey PRIMARY KEY (id);


--
-- Name: data_authorization_grants data_authorization_grants_enterprise_id_subject_type_subjec_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.data_authorization_grants
    ADD CONSTRAINT data_authorization_grants_enterprise_id_subject_type_subjec_key UNIQUE (enterprise_id, subject_type, subject_id, resource_type, resource_id);


--
-- Name: data_authorization_grants data_authorization_grants_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.data_authorization_grants
    ADD CONSTRAINT data_authorization_grants_pkey PRIMARY KEY (id);


--
-- Name: departments departments_id_enterprise_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.departments
    ADD CONSTRAINT departments_id_enterprise_id_key UNIQUE (id, enterprise_id);


--
-- Name: departments departments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.departments
    ADD CONSTRAINT departments_pkey PRIMARY KEY (id);


--
-- Name: enterprise_telemetry_tables enterprise_telemetry_tables_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.enterprise_telemetry_tables
    ADD CONSTRAINT enterprise_telemetry_tables_pkey PRIMARY KEY (enterprise_id);


--
-- Name: enterprise_users enterprise_users_id_enterprise_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.enterprise_users
    ADD CONSTRAINT enterprise_users_id_enterprise_id_key UNIQUE (id, enterprise_id);


--
-- Name: enterprise_users enterprise_users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.enterprise_users
    ADD CONSTRAINT enterprise_users_pkey PRIMARY KEY (id);


--
-- Name: enterprises enterprises_code_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.enterprises
    ADD CONSTRAINT enterprises_code_key UNIQUE (code);


--
-- Name: enterprises enterprises_id_status_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.enterprises
    ADD CONSTRAINT enterprises_id_status_key UNIQUE (id, status);


--
-- Name: enterprises enterprises_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.enterprises
    ADD CONSTRAINT enterprises_pkey PRIMARY KEY (id);


--
-- Name: execution_one_time_results execution_one_time_results_execution_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.execution_one_time_results
    ADD CONSTRAINT execution_one_time_results_execution_id_key UNIQUE (execution_id);


--
-- Name: execution_one_time_results execution_one_time_results_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.execution_one_time_results
    ADD CONSTRAINT execution_one_time_results_pkey PRIMARY KEY (id);


--
-- Name: executions executions_enterprise_id_idempotency_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.executions
    ADD CONSTRAINT executions_enterprise_id_idempotency_key_key UNIQUE (enterprise_id, idempotency_key);


--
-- Name: executions executions_execution_ref_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.executions
    ADD CONSTRAINT executions_execution_ref_key UNIQUE (execution_ref);


--
-- Name: executions executions_id_enterprise_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.executions
    ADD CONSTRAINT executions_id_enterprise_id_key UNIQUE (id, enterprise_id);


--
-- Name: executions executions_pending_action_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.executions
    ADD CONSTRAINT executions_pending_action_id_key UNIQUE (pending_action_id);


--
-- Name: executions executions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.executions
    ADD CONSTRAINT executions_pkey PRIMARY KEY (id);


--
-- Name: host_onboarding_operation_events host_onboarding_operation_events_operation_id_sequence_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_onboarding_operation_events
    ADD CONSTRAINT host_onboarding_operation_events_operation_id_sequence_key UNIQUE (operation_id, sequence);


--
-- Name: host_onboarding_operation_events host_onboarding_operation_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_onboarding_operation_events
    ADD CONSTRAINT host_onboarding_operation_events_pkey PRIMARY KEY (id);


--
-- Name: host_onboarding_operation_secrets host_onboarding_operation_secrets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_onboarding_operation_secrets
    ADD CONSTRAINT host_onboarding_operation_secrets_pkey PRIMARY KEY (operation_id);


--
-- Name: host_onboarding_operations host_onboarding_operations_pending_action_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_onboarding_operations
    ADD CONSTRAINT host_onboarding_operations_pending_action_id_key UNIQUE (pending_action_id);


--
-- Name: host_onboarding_operations host_onboarding_operations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_onboarding_operations
    ADD CONSTRAINT host_onboarding_operations_pkey PRIMARY KEY (id);


--
-- Name: host_runtime_observations host_runtime_observations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_runtime_observations
    ADD CONSTRAINT host_runtime_observations_pkey PRIMARY KEY (host_id);


--
-- Name: hosts hosts_id_enterprise_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.hosts
    ADD CONSTRAINT hosts_id_enterprise_id_key UNIQUE (id, enterprise_id);


--
-- Name: hosts hosts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.hosts
    ADD CONSTRAINT hosts_pkey PRIMARY KEY (id);


--
-- Name: idempotency_records idempotency_records_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.idempotency_records
    ADD CONSTRAINT idempotency_records_pkey PRIMARY KEY (audience, subject_id, operation, idempotency_key);


--
-- Name: interactive_cards interactive_cards_id_enterprise_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.interactive_cards
    ADD CONSTRAINT interactive_cards_id_enterprise_id_key UNIQUE (id, enterprise_id);


--
-- Name: interactive_cards interactive_cards_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.interactive_cards
    ADD CONSTRAINT interactive_cards_pkey PRIMARY KEY (id);


--
-- Name: kubernetes_clusters kubernetes_clusters_id_enterprise_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kubernetes_clusters
    ADD CONSTRAINT kubernetes_clusters_id_enterprise_id_key UNIQUE (id, enterprise_id);


--
-- Name: kubernetes_clusters kubernetes_clusters_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kubernetes_clusters
    ADD CONSTRAINT kubernetes_clusters_pkey PRIMARY KEY (id);


--
-- Name: kubernetes_node_host_bindings kubernetes_node_host_bindings_enterprise_id_kubernetes_clus_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kubernetes_node_host_bindings
    ADD CONSTRAINT kubernetes_node_host_bindings_enterprise_id_kubernetes_clus_key UNIQUE (enterprise_id, kubernetes_cluster_id, node_uid);


--
-- Name: kubernetes_node_host_bindings kubernetes_node_host_bindings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kubernetes_node_host_bindings
    ADD CONSTRAINT kubernetes_node_host_bindings_pkey PRIMARY KEY (id);


--
-- Name: managed_accounts managed_accounts_host_id_username_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.managed_accounts
    ADD CONSTRAINT managed_accounts_host_id_username_key UNIQUE (host_id, username);


--
-- Name: managed_accounts managed_accounts_id_enterprise_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.managed_accounts
    ADD CONSTRAINT managed_accounts_id_enterprise_id_key UNIQUE (id, enterprise_id);


--
-- Name: managed_accounts managed_accounts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.managed_accounts
    ADD CONSTRAINT managed_accounts_pkey PRIMARY KEY (id);


--
-- Name: mfa_challenges mfa_challenges_challenge_hash_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mfa_challenges
    ADD CONSTRAINT mfa_challenges_challenge_hash_key UNIQUE (challenge_hash);


--
-- Name: mfa_challenges mfa_challenges_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mfa_challenges
    ADD CONSTRAINT mfa_challenges_pkey PRIMARY KEY (id);


--
-- Name: mfa_credentials mfa_credentials_audience_user_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mfa_credentials
    ADD CONSTRAINT mfa_credentials_audience_user_id_key UNIQUE (audience, user_id);


--
-- Name: mfa_credentials mfa_credentials_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mfa_credentials
    ADD CONSTRAINT mfa_credentials_pkey PRIMARY KEY (id);


--
-- Name: mfa_recovery_codes mfa_recovery_codes_credential_id_code_hash_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mfa_recovery_codes
    ADD CONSTRAINT mfa_recovery_codes_credential_id_code_hash_key UNIQUE (credential_id, code_hash);


--
-- Name: mfa_recovery_codes mfa_recovery_codes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mfa_recovery_codes
    ADD CONSTRAINT mfa_recovery_codes_pkey PRIMARY KEY (id);


--
-- Name: model_calls model_calls_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.model_calls
    ADD CONSTRAINT model_calls_pkey PRIMARY KEY (id);


--
-- Name: model_compatibility_results model_compatibility_results_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.model_compatibility_results
    ADD CONSTRAINT model_compatibility_results_pkey PRIMARY KEY (id);


--
-- Name: model_quota_reservations model_quota_reservations_model_call_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.model_quota_reservations
    ADD CONSTRAINT model_quota_reservations_model_call_id_key UNIQUE (model_call_id);


--
-- Name: model_quota_reservations model_quota_reservations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.model_quota_reservations
    ADD CONSTRAINT model_quota_reservations_pkey PRIMARY KEY (id);


--
-- Name: model_quotas model_quotas_enterprise_id_model_id_subject_type_subject_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.model_quotas
    ADD CONSTRAINT model_quotas_enterprise_id_model_id_subject_type_subject_id_key UNIQUE (enterprise_id, model_id, subject_type, subject_id);


--
-- Name: model_quotas model_quotas_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.model_quotas
    ADD CONSTRAINT model_quotas_pkey PRIMARY KEY (id);


--
-- Name: outbox_events outbox_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.outbox_events
    ADD CONSTRAINT outbox_events_pkey PRIMARY KEY (id);


--
-- Name: password_credentials password_credentials_audience_subject_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.password_credentials
    ADD CONSTRAINT password_credentials_audience_subject_id_key UNIQUE (audience, subject_id);


--
-- Name: password_credentials password_credentials_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.password_credentials
    ADD CONSTRAINT password_credentials_pkey PRIMARY KEY (id);


--
-- Name: pending_action_plans pending_action_plans_pending_action_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pending_action_plans
    ADD CONSTRAINT pending_action_plans_pending_action_id_key UNIQUE (pending_action_id);


--
-- Name: pending_action_plans pending_action_plans_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pending_action_plans
    ADD CONSTRAINT pending_action_plans_pkey PRIMARY KEY (id);


--
-- Name: pending_action_tokens pending_action_tokens_pending_action_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pending_action_tokens
    ADD CONSTRAINT pending_action_tokens_pending_action_id_key UNIQUE (pending_action_id);


--
-- Name: pending_action_tokens pending_action_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pending_action_tokens
    ADD CONSTRAINT pending_action_tokens_pkey PRIMARY KEY (id);


--
-- Name: pending_action_tokens pending_action_tokens_token_hash_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pending_action_tokens
    ADD CONSTRAINT pending_action_tokens_token_hash_key UNIQUE (token_hash);


--
-- Name: pending_actions pending_actions_action_ref_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pending_actions
    ADD CONSTRAINT pending_actions_action_ref_key UNIQUE (action_ref);


--
-- Name: pending_actions pending_actions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pending_actions
    ADD CONSTRAINT pending_actions_pkey PRIMARY KEY (id);


--
-- Name: permissions permissions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.permissions
    ADD CONSTRAINT permissions_pkey PRIMARY KEY (id);


--
-- Name: pki_certificate_identities pki_certificate_identities_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pki_certificate_identities
    ADD CONSTRAINT pki_certificate_identities_pkey PRIMARY KEY (serial_number);


--
-- Name: pki_node_trust_acks pki_node_trust_acks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pki_node_trust_acks
    ADD CONSTRAINT pki_node_trust_acks_pkey PRIMARY KEY (node_kind, node_id, epoch);


--
-- Name: pki_trust_bundles pki_trust_bundles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pki_trust_bundles
    ADD CONSTRAINT pki_trust_bundles_pkey PRIMARY KEY (epoch);


--
-- Name: platform_settings platform_settings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.platform_settings
    ADD CONSTRAINT platform_settings_pkey PRIMARY KEY (singleton);


--
-- Name: platform_state platform_state_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.platform_state
    ADD CONSTRAINT platform_state_pkey PRIMARY KEY (singleton);


--
-- Name: platform_users platform_users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.platform_users
    ADD CONSTRAINT platform_users_pkey PRIMARY KEY (id);


--
-- Name: remote_access_approval_workflows remote_access_approval_workflows_id_enterprise_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_approval_workflows
    ADD CONSTRAINT remote_access_approval_workflows_id_enterprise_id_key UNIQUE (id, enterprise_id);


--
-- Name: remote_access_approval_workflows remote_access_approval_workflows_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_approval_workflows
    ADD CONSTRAINT remote_access_approval_workflows_pkey PRIMARY KEY (id);


--
-- Name: remote_access_command_events remote_access_command_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_command_events
    ADD CONSTRAINT remote_access_command_events_pkey PRIMARY KEY (id);


--
-- Name: remote_access_command_events remote_access_command_events_session_id_sequence_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_command_events
    ADD CONSTRAINT remote_access_command_events_session_id_sequence_key UNIQUE (session_id, sequence);


--
-- Name: remote_access_decisions remote_access_decisions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_decisions
    ADD CONSTRAINT remote_access_decisions_pkey PRIMARY KEY (id);


--
-- Name: remote_access_decisions remote_access_decisions_requirement_id_decided_by_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_decisions
    ADD CONSTRAINT remote_access_decisions_requirement_id_decided_by_key UNIQUE (requirement_id, decided_by);


--
-- Name: remote_access_grants remote_access_grants_id_enterprise_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_grants
    ADD CONSTRAINT remote_access_grants_id_enterprise_id_key UNIQUE (id, enterprise_id);


--
-- Name: remote_access_grants remote_access_grants_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_grants
    ADD CONSTRAINT remote_access_grants_pkey PRIMARY KEY (id);


--
-- Name: remote_access_leases remote_access_leases_id_enterprise_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_leases
    ADD CONSTRAINT remote_access_leases_id_enterprise_id_key UNIQUE (id, enterprise_id);


--
-- Name: remote_access_leases remote_access_leases_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_leases
    ADD CONSTRAINT remote_access_leases_pkey PRIMARY KEY (id);


--
-- Name: remote_access_leases remote_access_leases_request_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_leases
    ADD CONSTRAINT remote_access_leases_request_id_key UNIQUE (request_id);


--
-- Name: remote_access_recording_chunks remote_access_recording_chunks_object_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_recording_chunks
    ADD CONSTRAINT remote_access_recording_chunks_object_key_key UNIQUE (object_key);


--
-- Name: remote_access_recording_chunks remote_access_recording_chunks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_recording_chunks
    ADD CONSTRAINT remote_access_recording_chunks_pkey PRIMARY KEY (recording_id, sequence);


--
-- Name: remote_access_recordings remote_access_recordings_id_enterprise_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_recordings
    ADD CONSTRAINT remote_access_recordings_id_enterprise_id_key UNIQUE (id, enterprise_id);


--
-- Name: remote_access_recordings remote_access_recordings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_recordings
    ADD CONSTRAINT remote_access_recordings_pkey PRIMARY KEY (id);


--
-- Name: remote_access_recordings remote_access_recordings_session_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_recordings
    ADD CONSTRAINT remote_access_recordings_session_id_key UNIQUE (session_id);


--
-- Name: remote_access_requests remote_access_requests_id_enterprise_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_requests
    ADD CONSTRAINT remote_access_requests_id_enterprise_id_key UNIQUE (id, enterprise_id);


--
-- Name: remote_access_requests remote_access_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_requests
    ADD CONSTRAINT remote_access_requests_pkey PRIMARY KEY (id);


--
-- Name: remote_access_requirement_snapshots remote_access_requirement_snapshots_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_requirement_snapshots
    ADD CONSTRAINT remote_access_requirement_snapshots_pkey PRIMARY KEY (id);


--
-- Name: remote_access_routes remote_access_routes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_routes
    ADD CONSTRAINT remote_access_routes_pkey PRIMARY KEY (session_id);


--
-- Name: remote_access_rules remote_access_rules_id_enterprise_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_rules
    ADD CONSTRAINT remote_access_rules_id_enterprise_id_key UNIQUE (id, enterprise_id);


--
-- Name: remote_access_rules remote_access_rules_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_rules
    ADD CONSTRAINT remote_access_rules_pkey PRIMARY KEY (id);


--
-- Name: remote_access_session_profiles remote_access_session_profiles_id_enterprise_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_session_profiles
    ADD CONSTRAINT remote_access_session_profiles_id_enterprise_id_key UNIQUE (id, enterprise_id);


--
-- Name: remote_access_session_profiles remote_access_session_profiles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_session_profiles
    ADD CONSTRAINT remote_access_session_profiles_pkey PRIMARY KEY (id);


--
-- Name: remote_access_sessions remote_access_sessions_id_enterprise_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_sessions
    ADD CONSTRAINT remote_access_sessions_id_enterprise_id_key UNIQUE (id, enterprise_id);


--
-- Name: remote_access_sessions remote_access_sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_sessions
    ADD CONSTRAINT remote_access_sessions_pkey PRIMARY KEY (id);


--
-- Name: remote_access_tickets remote_access_tickets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_tickets
    ADD CONSTRAINT remote_access_tickets_pkey PRIMARY KEY (id);


--
-- Name: remote_access_tickets remote_access_tickets_ticket_hash_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_tickets
    ADD CONSTRAINT remote_access_tickets_ticket_hash_key UNIQUE (ticket_hash);


--
-- Name: role_bindings role_bindings_id_enterprise_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.role_bindings
    ADD CONSTRAINT role_bindings_id_enterprise_id_key UNIQUE (id, enterprise_id);


--
-- Name: role_bindings role_bindings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.role_bindings
    ADD CONSTRAINT role_bindings_pkey PRIMARY KEY (id);


--
-- Name: role_permissions role_permissions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.role_permissions
    ADD CONSTRAINT role_permissions_pkey PRIMARY KEY (role_id, permission_id);


--
-- Name: roles roles_enterprise_id_identity_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.roles
    ADD CONSTRAINT roles_enterprise_id_identity_key_key UNIQUE (enterprise_id, identity_key);


--
-- Name: roles roles_id_enterprise_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.roles
    ADD CONSTRAINT roles_id_enterprise_id_key UNIQUE (id, enterprise_id);


--
-- Name: roles roles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.roles
    ADD CONSTRAINT roles_pkey PRIMARY KEY (id);


--
-- Name: run_steps run_steps_id_enterprise_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.run_steps
    ADD CONSTRAINT run_steps_id_enterprise_id_key UNIQUE (id, enterprise_id);


--
-- Name: run_steps run_steps_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.run_steps
    ADD CONSTRAINT run_steps_pkey PRIMARY KEY (id);


--
-- Name: run_steps run_steps_run_id_sequence_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.run_steps
    ADD CONSTRAINT run_steps_run_id_sequence_key UNIQUE (run_id, sequence);


--
-- Name: runs runs_id_enterprise_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.runs
    ADD CONSTRAINT runs_id_enterprise_id_key UNIQUE (id, enterprise_id);


--
-- Name: runs runs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.runs
    ADD CONSTRAINT runs_pkey PRIMARY KEY (id);


--
-- Name: runtime_tasks runtime_tasks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.runtime_tasks
    ADD CONSTRAINT runtime_tasks_pkey PRIMARY KEY (id);


--
-- Name: sandbox_backends sandbox_backends_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sandbox_backends
    ADD CONSTRAINT sandbox_backends_name_key UNIQUE (name);


--
-- Name: sandbox_backends sandbox_backends_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sandbox_backends
    ADD CONSTRAINT sandbox_backends_pkey PRIMARY KEY (id);


--
-- Name: sandbox_images sandbox_images_backend_id_digest_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sandbox_images
    ADD CONSTRAINT sandbox_images_backend_id_digest_key UNIQUE (backend_id, digest);


--
-- Name: sandbox_images sandbox_images_backend_id_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sandbox_images
    ADD CONSTRAINT sandbox_images_backend_id_name_key UNIQUE (backend_id, name);


--
-- Name: sandbox_images sandbox_images_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sandbox_images
    ADD CONSTRAINT sandbox_images_pkey PRIMARY KEY (id);


--
-- Name: sandbox_profiles sandbox_profiles_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sandbox_profiles
    ADD CONSTRAINT sandbox_profiles_name_key UNIQUE (name);


--
-- Name: sandbox_profiles sandbox_profiles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sandbox_profiles
    ADD CONSTRAINT sandbox_profiles_pkey PRIMARY KEY (id);


--
-- Name: sandbox_quotas sandbox_quotas_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sandbox_quotas
    ADD CONSTRAINT sandbox_quotas_pkey PRIMARY KEY (enterprise_id);


--
-- Name: sandbox_sessions sandbox_sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sandbox_sessions
    ADD CONSTRAINT sandbox_sessions_pkey PRIMARY KEY (id);


--
-- Name: sandbox_sessions sandbox_sessions_task_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sandbox_sessions
    ADD CONSTRAINT sandbox_sessions_task_id_key UNIQUE (task_id);


--
-- Name: sandbox_sessions sandbox_sessions_upstream_session_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sandbox_sessions
    ADD CONSTRAINT sandbox_sessions_upstream_session_id_key UNIQUE (upstream_session_id);


--
-- Name: sandbox_usage sandbox_usage_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sandbox_usage
    ADD CONSTRAINT sandbox_usage_pkey PRIMARY KEY (enterprise_id, month);


--
-- Name: secret_versions secret_versions_id_enterprise_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.secret_versions
    ADD CONSTRAINT secret_versions_id_enterprise_id_key UNIQUE (id, enterprise_id);


--
-- Name: secret_versions secret_versions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.secret_versions
    ADD CONSTRAINT secret_versions_pkey PRIMARY KEY (id);


--
-- Name: secret_versions secret_versions_secret_id_version_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.secret_versions
    ADD CONSTRAINT secret_versions_secret_id_version_key UNIQUE (secret_id, version);


--
-- Name: secrets secrets_id_enterprise_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.secrets
    ADD CONSTRAINT secrets_id_enterprise_id_key UNIQUE (id, enterprise_id);


--
-- Name: secrets secrets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.secrets
    ADD CONSTRAINT secrets_pkey PRIMARY KEY (id);


--
-- Name: service_accounts service_accounts_id_enterprise_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.service_accounts
    ADD CONSTRAINT service_accounts_id_enterprise_id_key UNIQUE (id, enterprise_id);


--
-- Name: service_accounts service_accounts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.service_accounts
    ADD CONSTRAINT service_accounts_pkey PRIMARY KEY (id);


--
-- Name: sessions sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sessions
    ADD CONSTRAINT sessions_pkey PRIMARY KEY (id);


--
-- Name: sessions sessions_token_hash_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sessions
    ADD CONSTRAINT sessions_token_hash_key UNIQUE (token_hash);


--
-- Name: telemetry_certificates telemetry_certificates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telemetry_certificates
    ADD CONSTRAINT telemetry_certificates_pkey PRIMARY KEY (id);


--
-- Name: telemetry_certificates telemetry_certificates_serial_number_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telemetry_certificates
    ADD CONSTRAINT telemetry_certificates_serial_number_key UNIQUE (serial_number);


--
-- Name: telemetry_collector_operations telemetry_collector_operations_pending_action_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telemetry_collector_operations
    ADD CONSTRAINT telemetry_collector_operations_pending_action_id_key UNIQUE (pending_action_id);


--
-- Name: telemetry_collector_operations telemetry_collector_operations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telemetry_collector_operations
    ADD CONSTRAINT telemetry_collector_operations_pkey PRIMARY KEY (id);


--
-- Name: telemetry_dlq_records telemetry_dlq_records_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telemetry_dlq_records
    ADD CONSTRAINT telemetry_dlq_records_pkey PRIMARY KEY (id);


--
-- Name: telemetry_dlq_records telemetry_dlq_records_topic_partition_source_offset_record__key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telemetry_dlq_records
    ADD CONSTRAINT telemetry_dlq_records_topic_partition_source_offset_record__key UNIQUE (topic, partition, source_offset, record_hash);


--
-- Name: telemetry_enrollment_tokens telemetry_enrollment_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telemetry_enrollment_tokens
    ADD CONSTRAINT telemetry_enrollment_tokens_pkey PRIMARY KEY (id);


--
-- Name: telemetry_enrollment_tokens telemetry_enrollment_tokens_token_hash_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telemetry_enrollment_tokens
    ADD CONSTRAINT telemetry_enrollment_tokens_token_hash_key UNIQUE (token_hash);


--
-- Name: telemetry_retention_policies telemetry_retention_policies_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telemetry_retention_policies
    ADD CONSTRAINT telemetry_retention_policies_pkey PRIMARY KEY (enterprise_id);


--
-- Name: telemetry_route_tests telemetry_route_tests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telemetry_route_tests
    ADD CONSTRAINT telemetry_route_tests_pkey PRIMARY KEY (id);


--
-- Name: telemetry_routes telemetry_routes_collector_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telemetry_routes
    ADD CONSTRAINT telemetry_routes_collector_id_key UNIQUE (collector_id);


--
-- Name: telemetry_routes telemetry_routes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telemetry_routes
    ADD CONSTRAINT telemetry_routes_pkey PRIMARY KEY (id);


--
-- Name: telemetry_tunnels telemetry_tunnels_collector_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telemetry_tunnels
    ADD CONSTRAINT telemetry_tunnels_collector_id_key UNIQUE (collector_id);


--
-- Name: telemetry_tunnels telemetry_tunnels_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telemetry_tunnels
    ADD CONSTRAINT telemetry_tunnels_pkey PRIMARY KEY (id);


--
-- Name: telemetry_usage_daily telemetry_usage_daily_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telemetry_usage_daily
    ADD CONSTRAINT telemetry_usage_daily_pkey PRIMARY KEY (enterprise_id, usage_date);


--
-- Name: temporary_credentials temporary_credentials_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.temporary_credentials
    ADD CONSTRAINT temporary_credentials_pkey PRIMARY KEY (id);


--
-- Name: tool_calls tool_calls_call_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tool_calls
    ADD CONSTRAINT tool_calls_call_id_key UNIQUE (call_id);


--
-- Name: tool_calls tool_calls_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tool_calls
    ADD CONSTRAINT tool_calls_pkey PRIMARY KEY (id);


--
-- Name: tool_results tool_results_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tool_results
    ADD CONSTRAINT tool_results_pkey PRIMARY KEY (id);


--
-- Name: tool_results tool_results_tool_call_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tool_results
    ADD CONSTRAINT tool_results_tool_call_id_key UNIQUE (tool_call_id);


--
-- Name: user_confirmations user_confirmations_pending_action_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_confirmations
    ADD CONSTRAINT user_confirmations_pending_action_id_key UNIQUE (pending_action_id);


--
-- Name: user_confirmations user_confirmations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_confirmations
    ADD CONSTRAINT user_confirmations_pkey PRIMARY KEY (id);


--
-- Name: ai_models_name_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX ai_models_name_unique ON public.ai_models USING btree (enterprise_id, lower(name));


--
-- Name: approval_policies_name_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX approval_policies_name_unique ON public.approval_policies USING btree (enterprise_id, lower(name));


--
-- Name: audit_events_enterprise_cursor; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX audit_events_enterprise_cursor ON public.audit_events USING btree (enterprise_id, created_at DESC, id DESC) WHERE (domain = 'enterprise'::text);


--
-- Name: audit_events_platform_cursor; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX audit_events_platform_cursor ON public.audit_events USING btree (created_at DESC, id DESC) WHERE (domain = 'platform'::text);


--
-- Name: bastion_scopes_labels_gin; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX bastion_scopes_labels_gin ON public.bastion_scopes USING gin (labels jsonb_path_ops);


--
-- Name: bastion_scopes_name_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX bastion_scopes_name_unique ON public.bastion_scopes USING btree (enterprise_id, lower(name)) WHERE (status <> 'deleted'::text);


--
-- Name: break_glass_active_subject; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX break_glass_active_subject ON public.break_glass_sessions USING btree (enterprise_id, user_id, expires_at) WHERE (status = 'active'::text);


--
-- Name: card_instances_conversation; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX card_instances_conversation ON public.card_instances USING btree (conversation_id, created_at, id);


--
-- Name: card_query_bindings_expiry; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX card_query_bindings_expiry ON public.card_query_bindings USING btree (expires_at) WHERE (status = 'active'::text);


--
-- Name: collection_claims_active_migration_per_collector; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX collection_claims_active_migration_per_collector ON public.collection_claims USING btree (enterprise_id, physical_resource_ref, claim_type, selector_hash, collector_id) WHERE ((ownership = 'migration'::text) AND (status = 'active'::text));


--
-- Name: collection_claims_active_primary; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX collection_claims_active_primary ON public.collection_claims USING btree (enterprise_id, physical_resource_ref, claim_type, selector_hash) WHERE ((ownership = 'primary'::text) AND (status = 'active'::text));


--
-- Name: collection_claims_resource; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX collection_claims_resource ON public.collection_claims USING btree (enterprise_id, physical_resource_ref, status);


--
-- Name: collector_instances_inventory; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX collector_instances_inventory ON public.collector_instances USING btree (enterprise_id, resource_type, status, updated_at DESC);


--
-- Name: connection_tests_expiry; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX connection_tests_expiry ON public.connection_tests USING btree (enterprise_id, expires_at);


--
-- Name: connector_certificates_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX connector_certificates_active ON public.connector_certificates USING btree (connector_id, status, not_after);


--
-- Name: connector_commands_dispatch; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX connector_commands_dispatch ON public.connector_commands USING btree (connector_id, status, created_at);


--
-- Name: connector_control_tunnels_claim_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX connector_control_tunnels_claim_idx ON public.connector_control_tunnels USING btree (last_claim_at NULLS FIRST, id) WHERE (status = ANY (ARRAY['desired'::text, 'establishing'::text, 'degraded'::text, 'down'::text]));


--
-- Name: connector_control_tunnels_scope_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX connector_control_tunnels_scope_idx ON public.connector_control_tunnels USING btree (enterprise_id, bastion_scope_id);


--
-- Name: connector_install_operations_claim_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX connector_install_operations_claim_idx ON public.connector_install_operations USING btree (created_at, id) WHERE (status = 'queued'::text);


--
-- Name: connector_install_operations_scope_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX connector_install_operations_scope_idx ON public.connector_install_operations USING btree (enterprise_id, bastion_scope_id, created_at DESC);


--
-- Name: connector_one_active_enrollment; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX connector_one_active_enrollment ON public.connector_enrollment_tokens USING btree (enterprise_id, role, COALESCE(bastion_scope_id, kubernetes_cluster_id)) WHERE (status = 'active'::text);


--
-- Name: connectors_enterprise_instance_live_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX connectors_enterprise_instance_live_unique ON public.connectors USING btree (enterprise_id, instance_id) WHERE (status <> ALL (ARRAY['revoked'::text, 'uninstalled'::text]));


--
-- Name: context_snapshots_one_active; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX context_snapshots_one_active ON public.context_snapshots USING btree (run_id) WHERE (status = 'active'::text);


--
-- Name: conversation_events_cursor; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX conversation_events_cursor ON public.conversation_events USING btree (conversation_id, sequence);


--
-- Name: conversations_owner_cursor; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX conversations_owner_cursor ON public.conversations USING btree (enterprise_id, owner_user_id, updated_at DESC, id DESC);


--
-- Name: credential_leases_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX credential_leases_active ON public.credential_leases USING btree (enterprise_id, recipient_type, recipient_id, status, expires_at);


--
-- Name: credential_leases_expiry_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX credential_leases_expiry_idx ON public.credential_leases USING btree (expires_at) WHERE (status = 'active'::text);


--
-- Name: credentials_name_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX credentials_name_unique ON public.credentials USING btree (enterprise_id, lower(name));


--
-- Name: data_authorization_grants_resource_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX data_authorization_grants_resource_idx ON public.data_authorization_grants USING btree (enterprise_id, resource_type, resource_id, status);


--
-- Name: data_authorization_grants_subject_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX data_authorization_grants_subject_idx ON public.data_authorization_grants USING btree (enterprise_id, subject_type, subject_id, resource_type, status);


--
-- Name: departments_name_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX departments_name_unique ON public.departments USING btree (enterprise_id, lower(name));


--
-- Name: departments_one_default_per_enterprise; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX departments_one_default_per_enterprise ON public.departments USING btree (enterprise_id) WHERE is_default;


--
-- Name: enterprise_users_username_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX enterprise_users_username_unique ON public.enterprise_users USING btree (lower(username));


--
-- Name: host_onboarding_operations_claim_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX host_onboarding_operations_claim_idx ON public.host_onboarding_operations USING btree (created_at, id) WHERE (status = 'queued'::text);


--
-- Name: host_onboarding_operations_host_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX host_onboarding_operations_host_idx ON public.host_onboarding_operations USING btree (enterprise_id, host_id, created_at DESC);


--
-- Name: hosts_bastion_root_live_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX hosts_bastion_root_live_unique ON public.hosts USING btree (bastion_scope_id) WHERE ((role = 'bastion'::text) AND (status <> 'deleted'::text));


--
-- Name: hosts_labels_gin; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX hosts_labels_gin ON public.hosts USING gin (labels jsonb_path_ops);


--
-- Name: hosts_name_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX hosts_name_unique ON public.hosts USING btree (enterprise_id, lower(name)) WHERE (status <> 'deleted'::text);


--
-- Name: hosts_scope_index; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX hosts_scope_index ON public.hosts USING btree (enterprise_id, bastion_scope_id, created_at, id) WHERE (status <> 'deleted'::text);


--
-- Name: interactive_cards_catalog; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX interactive_cards_catalog ON public.interactive_cards USING btree (enterprise_id, source, enabled, availability, updated_at DESC);


--
-- Name: interactive_cards_enterprise_slug_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX interactive_cards_enterprise_slug_unique ON public.interactive_cards USING btree (enterprise_id, slug) WHERE (source = 'enterprise'::text);


--
-- Name: interactive_cards_system_slug_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX interactive_cards_system_slug_unique ON public.interactive_cards USING btree (slug) WHERE (source = 'system'::text);


--
-- Name: kubernetes_clusters_labels_gin; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX kubernetes_clusters_labels_gin ON public.kubernetes_clusters USING gin (labels jsonb_path_ops);


--
-- Name: kubernetes_clusters_name_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX kubernetes_clusters_name_unique ON public.kubernetes_clusters USING btree (enterprise_id, lower(name)) WHERE (status <> 'deleted'::text);


--
-- Name: mfa_challenges_subject; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX mfa_challenges_subject ON public.mfa_challenges USING btree (audience, user_id, expires_at DESC);


--
-- Name: mfa_credentials_enrollment_hash; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX mfa_credentials_enrollment_hash ON public.mfa_credentials USING btree (enrollment_hash) WHERE (enrollment_hash IS NOT NULL);


--
-- Name: outbox_pending; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX outbox_pending ON public.outbox_events USING btree (available_at, created_at) WHERE (published_at IS NULL);


--
-- Name: pending_actions_enterprise_cursor; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX pending_actions_enterprise_cursor ON public.pending_actions USING btree (enterprise_id, created_at DESC, id DESC);


--
-- Name: pki_certificate_identities_expiry_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX pki_certificate_identities_expiry_idx ON public.pki_certificate_identities USING btree (not_after) WHERE (status = ANY (ARRAY['active'::text, 'overlap'::text]));


--
-- Name: pki_certificate_identities_subject_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX pki_certificate_identities_subject_idx ON public.pki_certificate_identities USING btree (subject_kind, subject_id, status);


--
-- Name: pki_node_trust_acks_epoch_status_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX pki_node_trust_acks_epoch_status_idx ON public.pki_node_trust_acks USING btree (epoch, status, node_kind);


--
-- Name: platform_users_username_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX platform_users_username_unique ON public.platform_users USING btree (lower(username));


--
-- Name: remote_access_grants_subject; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX remote_access_grants_subject ON public.remote_access_grants USING btree (enterprise_id, subject_type, subject_id, status, valid_until);


--
-- Name: remote_access_leases_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX remote_access_leases_active ON public.remote_access_leases USING btree (enterprise_id, user_id, expires_at) WHERE (revoked_at IS NULL);


--
-- Name: remote_access_profiles_name_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX remote_access_profiles_name_unique ON public.remote_access_session_profiles USING btree (enterprise_id, lower(name));


--
-- Name: remote_access_requests_expiry; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX remote_access_requests_expiry ON public.remote_access_requests USING btree (enterprise_id, status, expires_at);


--
-- Name: remote_access_requests_requester; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX remote_access_requests_requester ON public.remote_access_requests USING btree (enterprise_id, requester_id, created_at DESC, id DESC);


--
-- Name: remote_access_requirement_workflow_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX remote_access_requirement_workflow_unique ON public.remote_access_requirement_snapshots USING btree (request_id, workflow_id) WHERE (workflow_id IS NOT NULL);


--
-- Name: remote_access_requirements_deadline; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX remote_access_requirements_deadline ON public.remote_access_requirement_snapshots USING btree (deadline_at, status);


--
-- Name: remote_access_requirements_escalation_scan; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX remote_access_requirements_escalation_scan ON public.remote_access_requirement_snapshots USING btree (escalation_at, id) WHERE ((status = 'pending'::text) AND (escalated_at IS NULL));


--
-- Name: remote_access_rules_match; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX remote_access_rules_match ON public.remote_access_rules USING btree (enterprise_id, status, priority, id);


--
-- Name: remote_access_rules_name_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX remote_access_rules_name_unique ON public.remote_access_rules USING btree (enterprise_id, lower(name));


--
-- Name: remote_access_sessions_capacity; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX remote_access_sessions_capacity ON public.remote_access_sessions USING btree (enterprise_id, user_id, host_id) WHERE (status = ANY (ARRAY['connecting'::text, 'active'::text, 'terminating'::text]));


--
-- Name: remote_access_tickets_session; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX remote_access_tickets_session ON public.remote_access_tickets USING btree (session_id, created_at DESC);


--
-- Name: remote_access_workflows_name_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX remote_access_workflows_name_unique ON public.remote_access_approval_workflows USING btree (enterprise_id, lower(name));


--
-- Name: role_bindings_subject_role_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX role_bindings_subject_role_unique ON public.role_bindings USING btree (enterprise_id, subject_type, subject_id, role_id);


--
-- Name: roles_name_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX roles_name_unique ON public.roles USING btree (enterprise_id, lower(name));


--
-- Name: runs_one_active_per_conversation; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX runs_one_active_per_conversation ON public.runs USING btree (conversation_id) WHERE (status = ANY (ARRAY['pending'::text, 'running'::text, 'waiting_input'::text, 'waiting_approval'::text, 'waiting_system'::text]));


--
-- Name: runtime_tasks_claim; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX runtime_tasks_claim ON public.runtime_tasks USING btree (queue, available_at, created_at) WHERE ((status = 'pending'::text) OR ((status = ANY (ARRAY['leased'::text, 'running'::text])) AND (lease_until IS NOT NULL)));


--
-- Name: sandbox_sessions_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX sandbox_sessions_active ON public.sandbox_sessions USING btree (enterprise_id, expires_at) WHERE (status = ANY (ARRAY['creating'::text, 'running'::text, 'terminating'::text, 'unknown'::text]));


--
-- Name: secrets_name_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX secrets_name_unique ON public.secrets USING btree (enterprise_id, lower(name));


--
-- Name: service_accounts_name_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX service_accounts_name_unique ON public.service_accounts USING btree (enterprise_id, lower(name));


--
-- Name: sessions_active_token; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX sessions_active_token ON public.sessions USING btree (token_hash) WHERE (revoked_at IS NULL);


--
-- Name: sessions_subject; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX sessions_subject ON public.sessions USING btree (audience, user_id) WHERE (revoked_at IS NULL);


--
-- Name: telemetry_certificates_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX telemetry_certificates_active ON public.telemetry_certificates USING btree (collector_id, not_after) WHERE (revoked_at IS NULL);


--
-- Name: telemetry_collector_operations_queue; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX telemetry_collector_operations_queue ON public.telemetry_collector_operations USING btree (status, created_at) WHERE (status = ANY (ARRAY['queued'::text, 'running'::text, 'result_unknown'::text]));


--
-- Name: telemetry_tunnels_claim_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX telemetry_tunnels_claim_idx ON public.telemetry_tunnels USING btree (last_claim_at NULLS FIRST) WHERE (status = ANY (ARRAY['desired'::text, 'establishing'::text, 'degraded'::text, 'down'::text]));


--
-- Name: telemetry_tunnels_enterprise_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX telemetry_tunnels_enterprise_idx ON public.telemetry_tunnels USING btree (enterprise_id, host_id);


--
-- Name: temporary_credentials_subject; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX temporary_credentials_subject ON public.temporary_credentials USING btree (audience, user_id, status);


--
-- Name: collector_instances collector_resource_enterprise; Type: TRIGGER; Schema: public; Owner: -
--

CREATE CONSTRAINT TRIGGER collector_resource_enterprise AFTER INSERT OR UPDATE ON public.collector_instances DEFERRABLE INITIALLY IMMEDIATE FOR EACH ROW EXECUTE FUNCTION public.validate_collector_resource();


--
-- Name: data_authorization_grants data_authorization_grant_integrity; Type: TRIGGER; Schema: public; Owner: -
--

CREATE CONSTRAINT TRIGGER data_authorization_grant_integrity AFTER INSERT OR UPDATE ON public.data_authorization_grants DEFERRABLE INITIALLY IMMEDIATE FOR EACH ROW EXECUTE FUNCTION public.validate_data_authorization_grant();


--
-- Name: remote_access_grants remote_access_grant_enterprise; Type: TRIGGER; Schema: public; Owner: -
--

CREATE CONSTRAINT TRIGGER remote_access_grant_enterprise AFTER INSERT OR UPDATE ON public.remote_access_grants DEFERRABLE INITIALLY IMMEDIATE FOR EACH ROW EXECUTE FUNCTION public.validate_remote_access_grant();


--
-- Name: remote_access_grants remote_access_grants_reject_delete; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER remote_access_grants_reject_delete BEFORE DELETE ON public.remote_access_grants FOR EACH ROW EXECUTE FUNCTION public.reject_remote_access_governance_delete();


--
-- Name: remote_access_rules remote_access_rules_reject_delete; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER remote_access_rules_reject_delete BEFORE DELETE ON public.remote_access_rules FOR EACH ROW EXECUTE FUNCTION public.reject_remote_access_governance_delete();


--
-- Name: remote_access_session_profiles remote_access_session_profiles_reject_delete; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER remote_access_session_profiles_reject_delete BEFORE DELETE ON public.remote_access_session_profiles FOR EACH ROW EXECUTE FUNCTION public.reject_remote_access_governance_delete();


--
-- Name: remote_access_approval_workflows remote_access_workflows_reject_delete; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER remote_access_workflows_reject_delete BEFORE DELETE ON public.remote_access_approval_workflows FOR EACH ROW EXECUTE FUNCTION public.reject_remote_access_governance_delete();


--
-- Name: role_bindings role_binding_subject_enterprise; Type: TRIGGER; Schema: public; Owner: -
--

CREATE CONSTRAINT TRIGGER role_binding_subject_enterprise AFTER INSERT OR UPDATE ON public.role_bindings DEFERRABLE INITIALLY IMMEDIATE FOR EACH ROW EXECUTE FUNCTION public.validate_role_binding_subject();


--
-- Name: action_bindings action_bindings_card_instance_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.action_bindings
    ADD CONSTRAINT action_bindings_card_instance_id_fkey FOREIGN KEY (card_instance_id) REFERENCES public.card_instances(id);


--
-- Name: action_bindings action_bindings_conversation_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.action_bindings
    ADD CONSTRAINT action_bindings_conversation_id_fkey FOREIGN KEY (conversation_id) REFERENCES public.conversations(id);


--
-- Name: action_bindings action_bindings_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.action_bindings
    ADD CONSTRAINT action_bindings_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: action_bindings action_bindings_pending_action_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.action_bindings
    ADD CONSTRAINT action_bindings_pending_action_id_fkey FOREIGN KEY (pending_action_id) REFERENCES public.pending_actions(id);


--
-- Name: ai_model_credentials ai_model_credentials_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_model_credentials
    ADD CONSTRAINT ai_model_credentials_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: ai_model_credentials ai_model_credentials_model_revision_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_model_credentials
    ADD CONSTRAINT ai_model_credentials_model_revision_id_fkey FOREIGN KEY (model_revision_id) REFERENCES public.ai_model_revisions(id);


--
-- Name: ai_model_revisions ai_model_revisions_model_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_model_revisions
    ADD CONSTRAINT ai_model_revisions_model_id_enterprise_id_fkey FOREIGN KEY (model_id, enterprise_id) REFERENCES public.ai_models(id, enterprise_id);


--
-- Name: ai_models ai_models_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_models
    ADD CONSTRAINT ai_models_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: api_keys api_keys_service_account_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_keys
    ADD CONSTRAINT api_keys_service_account_id_enterprise_id_fkey FOREIGN KEY (service_account_id, enterprise_id) REFERENCES public.service_accounts(id, enterprise_id);


--
-- Name: approval_decisions approval_decisions_actor_user_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_decisions
    ADD CONSTRAINT approval_decisions_actor_user_id_enterprise_id_fkey FOREIGN KEY (actor_user_id, enterprise_id) REFERENCES public.enterprise_users(id, enterprise_id);


--
-- Name: approval_decisions approval_decisions_approval_request_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_decisions
    ADD CONSTRAINT approval_decisions_approval_request_id_fkey FOREIGN KEY (approval_request_id) REFERENCES public.approval_requests(id);


--
-- Name: approval_decisions approval_decisions_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_decisions
    ADD CONSTRAINT approval_decisions_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: approval_policies approval_policies_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_policies
    ADD CONSTRAINT approval_policies_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: approval_requests approval_requests_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_requests
    ADD CONSTRAINT approval_requests_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: approval_requests approval_requests_pending_action_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_requests
    ADD CONSTRAINT approval_requests_pending_action_id_fkey FOREIGN KEY (pending_action_id) REFERENCES public.pending_actions(id);


--
-- Name: approval_requirement_snapshots approval_requirement_snapshots_approval_request_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_requirement_snapshots
    ADD CONSTRAINT approval_requirement_snapshots_approval_request_id_fkey FOREIGN KEY (approval_request_id) REFERENCES public.approval_requests(id);


--
-- Name: approval_requirement_snapshots approval_requirement_snapshots_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_requirement_snapshots
    ADD CONSTRAINT approval_requirement_snapshots_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: artifacts artifacts_conversation_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.artifacts
    ADD CONSTRAINT artifacts_conversation_id_enterprise_id_fkey FOREIGN KEY (conversation_id, enterprise_id) REFERENCES public.conversations(id, enterprise_id);


--
-- Name: artifacts artifacts_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.artifacts
    ADD CONSTRAINT artifacts_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: artifacts artifacts_run_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.artifacts
    ADD CONSTRAINT artifacts_run_id_enterprise_id_fkey FOREIGN KEY (run_id, enterprise_id) REFERENCES public.runs(id, enterprise_id);


--
-- Name: authorization_versions authorization_versions_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.authorization_versions
    ADD CONSTRAINT authorization_versions_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: bastion_scopes bastion_scope_active_connector_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bastion_scopes
    ADD CONSTRAINT bastion_scope_active_connector_fk FOREIGN KEY (active_connector_id, enterprise_id) REFERENCES public.connectors(id, enterprise_id);


--
-- Name: bastion_scopes bastion_scope_connector_host_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bastion_scopes
    ADD CONSTRAINT bastion_scope_connector_host_fk FOREIGN KEY (connector_host_id, enterprise_id) REFERENCES public.hosts(id, enterprise_id);


--
-- Name: bastion_scopes bastion_scopes_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bastion_scopes
    ADD CONSTRAINT bastion_scopes_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: break_glass_sessions break_glass_sessions_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.break_glass_sessions
    ADD CONSTRAINT break_glass_sessions_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: break_glass_sessions break_glass_sessions_source_session_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.break_glass_sessions
    ADD CONSTRAINT break_glass_sessions_source_session_id_fkey FOREIGN KEY (source_session_id) REFERENCES public.sessions(id);


--
-- Name: break_glass_sessions break_glass_sessions_user_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.break_glass_sessions
    ADD CONSTRAINT break_glass_sessions_user_id_enterprise_id_fkey FOREIGN KEY (user_id, enterprise_id) REFERENCES public.enterprise_users(id, enterprise_id);


--
-- Name: card_data_sources card_data_sources_card_instance_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_data_sources
    ADD CONSTRAINT card_data_sources_card_instance_id_fkey FOREIGN KEY (card_instance_id) REFERENCES public.card_instances(id) ON DELETE CASCADE;


--
-- Name: card_data_sources card_data_sources_result_ref_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_data_sources
    ADD CONSTRAINT card_data_sources_result_ref_fkey FOREIGN KEY (result_ref) REFERENCES public.artifacts(result_ref);


--
-- Name: card_data_sources card_data_sources_tool_call_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_data_sources
    ADD CONSTRAINT card_data_sources_tool_call_id_fkey FOREIGN KEY (tool_call_id) REFERENCES public.tool_calls(id);


--
-- Name: card_demo_scenarios card_demo_scenarios_card_version_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_demo_scenarios
    ADD CONSTRAINT card_demo_scenarios_card_version_id_fkey FOREIGN KEY (card_version_id) REFERENCES public.card_versions(id) ON DELETE CASCADE;


--
-- Name: card_instances card_instances_actor_user_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_instances
    ADD CONSTRAINT card_instances_actor_user_id_enterprise_id_fkey FOREIGN KEY (actor_user_id, enterprise_id) REFERENCES public.enterprise_users(id, enterprise_id);


--
-- Name: card_instances card_instances_card_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_instances
    ADD CONSTRAINT card_instances_card_id_fkey FOREIGN KEY (card_id) REFERENCES public.interactive_cards(id);


--
-- Name: card_instances card_instances_card_version_id_card_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_instances
    ADD CONSTRAINT card_instances_card_version_id_card_id_fkey FOREIGN KEY (card_version_id, card_id) REFERENCES public.card_versions(id, card_id);


--
-- Name: card_instances card_instances_conversation_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_instances
    ADD CONSTRAINT card_instances_conversation_id_enterprise_id_fkey FOREIGN KEY (conversation_id, enterprise_id) REFERENCES public.conversations(id, enterprise_id);


--
-- Name: card_instances card_instances_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_instances
    ADD CONSTRAINT card_instances_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: card_instances card_instances_run_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_instances
    ADD CONSTRAINT card_instances_run_id_enterprise_id_fkey FOREIGN KEY (run_id, enterprise_id) REFERENCES public.runs(id, enterprise_id);


--
-- Name: card_presentations card_presentations_card_instance_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_presentations
    ADD CONSTRAINT card_presentations_card_instance_id_fkey FOREIGN KEY (card_instance_id) REFERENCES public.card_instances(id);


--
-- Name: card_presentations card_presentations_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_presentations
    ADD CONSTRAINT card_presentations_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: card_presentations card_presentations_viewer_user_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_presentations
    ADD CONSTRAINT card_presentations_viewer_user_id_enterprise_id_fkey FOREIGN KEY (viewer_user_id, enterprise_id) REFERENCES public.enterprise_users(id, enterprise_id);


--
-- Name: card_query_binding_specs card_query_binding_specs_card_instance_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_query_binding_specs
    ADD CONSTRAINT card_query_binding_specs_card_instance_id_fkey FOREIGN KEY (card_instance_id) REFERENCES public.card_instances(id) ON DELETE CASCADE;


--
-- Name: card_query_bindings card_query_bindings_binding_spec_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_query_bindings
    ADD CONSTRAINT card_query_bindings_binding_spec_id_fkey FOREIGN KEY (binding_spec_id) REFERENCES public.card_query_binding_specs(id);


--
-- Name: card_query_bindings card_query_bindings_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_query_bindings
    ADD CONSTRAINT card_query_bindings_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: card_query_bindings card_query_bindings_presentation_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_query_bindings
    ADD CONSTRAINT card_query_bindings_presentation_id_fkey FOREIGN KEY (presentation_id) REFERENCES public.card_presentations(id) ON DELETE CASCADE;


--
-- Name: card_query_bindings card_query_bindings_viewer_user_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_query_bindings
    ADD CONSTRAINT card_query_bindings_viewer_user_id_enterprise_id_fkey FOREIGN KEY (viewer_user_id, enterprise_id) REFERENCES public.enterprise_users(id, enterprise_id);


--
-- Name: card_slot_bindings card_slot_bindings_card_version_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_slot_bindings
    ADD CONSTRAINT card_slot_bindings_card_version_id_fkey FOREIGN KEY (card_version_id) REFERENCES public.card_versions(id) ON DELETE CASCADE;


--
-- Name: card_validation_runs card_validation_runs_actor_user_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_validation_runs
    ADD CONSTRAINT card_validation_runs_actor_user_id_enterprise_id_fkey FOREIGN KEY (actor_user_id, enterprise_id) REFERENCES public.enterprise_users(id, enterprise_id);


--
-- Name: card_validation_runs card_validation_runs_card_version_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_validation_runs
    ADD CONSTRAINT card_validation_runs_card_version_id_fkey FOREIGN KEY (card_version_id) REFERENCES public.card_versions(id);


--
-- Name: card_validation_runs card_validation_runs_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_validation_runs
    ADD CONSTRAINT card_validation_runs_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: card_versions card_versions_card_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.card_versions
    ADD CONSTRAINT card_versions_card_id_fkey FOREIGN KEY (card_id) REFERENCES public.interactive_cards(id);


--
-- Name: collection_claims collection_claims_collector_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.collection_claims
    ADD CONSTRAINT collection_claims_collector_id_enterprise_id_fkey FOREIGN KEY (collector_id, enterprise_id) REFERENCES public.collector_instances(id, enterprise_id);


--
-- Name: collection_claims collection_claims_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.collection_claims
    ADD CONSTRAINT collection_claims_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: collection_claims collection_claims_primary_claim_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.collection_claims
    ADD CONSTRAINT collection_claims_primary_claim_id_enterprise_id_fkey FOREIGN KEY (primary_claim_id, enterprise_id) REFERENCES public.collection_claims(id, enterprise_id);


--
-- Name: collection_claims collection_claims_profile_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.collection_claims
    ADD CONSTRAINT collection_claims_profile_id_fkey FOREIGN KEY (profile_id) REFERENCES public.collection_profiles(id);


--
-- Name: collector_config_revisions collector_config_revisions_collector_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.collector_config_revisions
    ADD CONSTRAINT collector_config_revisions_collector_id_fkey FOREIGN KEY (collector_id) REFERENCES public.collector_instances(id);


--
-- Name: collector_instances collector_instances_distribution_version_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.collector_instances
    ADD CONSTRAINT collector_instances_distribution_version_id_fkey FOREIGN KEY (distribution_version_id) REFERENCES public.collector_distribution_versions(id);


--
-- Name: collector_instances collector_instances_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.collector_instances
    ADD CONSTRAINT collector_instances_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: connection_tests connection_tests_connector_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connection_tests
    ADD CONSTRAINT connection_tests_connector_id_enterprise_id_fkey FOREIGN KEY (connector_id, enterprise_id) REFERENCES public.connectors(id, enterprise_id);


--
-- Name: connection_tests connection_tests_credential_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connection_tests
    ADD CONSTRAINT connection_tests_credential_id_enterprise_id_fkey FOREIGN KEY (credential_id, enterprise_id) REFERENCES public.credentials(id, enterprise_id);


--
-- Name: connection_tests connection_tests_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connection_tests
    ADD CONSTRAINT connection_tests_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: connector_certificates connector_certificates_connector_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_certificates
    ADD CONSTRAINT connector_certificates_connector_id_enterprise_id_fkey FOREIGN KEY (connector_id, enterprise_id) REFERENCES public.connectors(id, enterprise_id);


--
-- Name: connector_commands connector_command_credential_lease_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_commands
    ADD CONSTRAINT connector_command_credential_lease_fk FOREIGN KEY (credential_lease_id, enterprise_id) REFERENCES public.credential_leases(id, enterprise_id);


--
-- Name: connector_commands connector_commands_connector_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_commands
    ADD CONSTRAINT connector_commands_connector_id_enterprise_id_fkey FOREIGN KEY (connector_id, enterprise_id) REFERENCES public.connectors(id, enterprise_id);


--
-- Name: connector_control_tunnels connector_control_tunnels_bastion_scope_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_control_tunnels
    ADD CONSTRAINT connector_control_tunnels_bastion_scope_id_enterprise_id_fkey FOREIGN KEY (bastion_scope_id, enterprise_id) REFERENCES public.bastion_scopes(id, enterprise_id);


--
-- Name: connector_control_tunnels connector_control_tunnels_credential_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_control_tunnels
    ADD CONSTRAINT connector_control_tunnels_credential_id_enterprise_id_fkey FOREIGN KEY (credential_id, enterprise_id) REFERENCES public.credentials(id, enterprise_id);


--
-- Name: connector_control_tunnels connector_control_tunnels_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_control_tunnels
    ADD CONSTRAINT connector_control_tunnels_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: connector_control_tunnels connector_control_tunnels_host_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_control_tunnels
    ADD CONSTRAINT connector_control_tunnels_host_id_enterprise_id_fkey FOREIGN KEY (host_id, enterprise_id) REFERENCES public.hosts(id, enterprise_id);


--
-- Name: connector_enrollment_tokens connector_enrollment_tokens_bastion_scope_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_enrollment_tokens
    ADD CONSTRAINT connector_enrollment_tokens_bastion_scope_id_enterprise_id_fkey FOREIGN KEY (bastion_scope_id, enterprise_id) REFERENCES public.bastion_scopes(id, enterprise_id);


--
-- Name: connector_enrollment_tokens connector_enrollment_tokens_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_enrollment_tokens
    ADD CONSTRAINT connector_enrollment_tokens_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: connector_enrollment_tokens connector_enrollment_tokens_kubernetes_cluster_id_enterpri_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_enrollment_tokens
    ADD CONSTRAINT connector_enrollment_tokens_kubernetes_cluster_id_enterpri_fkey FOREIGN KEY (kubernetes_cluster_id, enterprise_id) REFERENCES public.kubernetes_clusters(id, enterprise_id);


--
-- Name: connector_enrollment_tokens connector_enrollment_tokens_release_version_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_enrollment_tokens
    ADD CONSTRAINT connector_enrollment_tokens_release_version_id_fkey FOREIGN KEY (release_version_id) REFERENCES public.connector_release_versions(id);


--
-- Name: connector_install_operation_events connector_install_operation_events_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_install_operation_events
    ADD CONSTRAINT connector_install_operation_events_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: connector_install_operation_events connector_install_operation_events_operation_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_install_operation_events
    ADD CONSTRAINT connector_install_operation_events_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES public.connector_install_operations(id) ON DELETE CASCADE;


--
-- Name: connector_install_operation_secrets connector_install_operation_secrets_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_install_operation_secrets
    ADD CONSTRAINT connector_install_operation_secrets_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: connector_install_operation_secrets connector_install_operation_secrets_operation_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_install_operation_secrets
    ADD CONSTRAINT connector_install_operation_secrets_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES public.connector_install_operations(id) ON DELETE CASCADE;


--
-- Name: connector_install_operations connector_install_operations_bastion_scope_id_enterprise_i_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_install_operations
    ADD CONSTRAINT connector_install_operations_bastion_scope_id_enterprise_i_fkey FOREIGN KEY (bastion_scope_id, enterprise_id) REFERENCES public.bastion_scopes(id, enterprise_id);


--
-- Name: connector_install_operations connector_install_operations_connection_test_id_enterprise_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_install_operations
    ADD CONSTRAINT connector_install_operations_connection_test_id_enterprise_fkey FOREIGN KEY (connection_test_id, enterprise_id) REFERENCES public.connection_tests(id, enterprise_id);


--
-- Name: connector_install_operations connector_install_operations_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_install_operations
    ADD CONSTRAINT connector_install_operations_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: connector_install_operations connector_install_operations_host_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_install_operations
    ADD CONSTRAINT connector_install_operations_host_id_enterprise_id_fkey FOREIGN KEY (host_id, enterprise_id) REFERENCES public.hosts(id, enterprise_id);


--
-- Name: connector_install_operations connector_install_operations_pending_action_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_install_operations
    ADD CONSTRAINT connector_install_operations_pending_action_id_fkey FOREIGN KEY (pending_action_id) REFERENCES public.pending_actions(id);


--
-- Name: connector_install_operations connector_install_operations_release_version_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_install_operations
    ADD CONSTRAINT connector_install_operations_release_version_id_fkey FOREIGN KEY (release_version_id) REFERENCES public.connector_release_versions(id);


--
-- Name: connector_install_operations connector_install_operations_retry_of_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_install_operations
    ADD CONSTRAINT connector_install_operations_retry_of_fkey FOREIGN KEY (retry_of) REFERENCES public.connector_install_operations(id);


--
-- Name: connector_sessions connector_sessions_connector_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_sessions
    ADD CONSTRAINT connector_sessions_connector_id_enterprise_id_fkey FOREIGN KEY (connector_id, enterprise_id) REFERENCES public.connectors(id, enterprise_id);


--
-- Name: connectors connectors_bastion_scope_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connectors
    ADD CONSTRAINT connectors_bastion_scope_id_enterprise_id_fkey FOREIGN KEY (bastion_scope_id, enterprise_id) REFERENCES public.bastion_scopes(id, enterprise_id);


--
-- Name: connectors connectors_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connectors
    ADD CONSTRAINT connectors_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: connectors connectors_host_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connectors
    ADD CONSTRAINT connectors_host_id_enterprise_id_fkey FOREIGN KEY (host_id, enterprise_id) REFERENCES public.hosts(id, enterprise_id);


--
-- Name: connectors connectors_kubernetes_cluster_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connectors
    ADD CONSTRAINT connectors_kubernetes_cluster_id_enterprise_id_fkey FOREIGN KEY (kubernetes_cluster_id, enterprise_id) REFERENCES public.kubernetes_clusters(id, enterprise_id);


--
-- Name: context_snapshots context_snapshots_conversation_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.context_snapshots
    ADD CONSTRAINT context_snapshots_conversation_id_enterprise_id_fkey FOREIGN KEY (conversation_id, enterprise_id) REFERENCES public.conversations(id, enterprise_id);


--
-- Name: context_snapshots context_snapshots_run_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.context_snapshots
    ADD CONSTRAINT context_snapshots_run_id_enterprise_id_fkey FOREIGN KEY (run_id, enterprise_id) REFERENCES public.runs(id, enterprise_id);


--
-- Name: conversation_events conversation_events_conversation_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.conversation_events
    ADD CONSTRAINT conversation_events_conversation_id_enterprise_id_fkey FOREIGN KEY (conversation_id, enterprise_id) REFERENCES public.conversations(id, enterprise_id);


--
-- Name: conversation_events conversation_events_run_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.conversation_events
    ADD CONSTRAINT conversation_events_run_id_enterprise_id_fkey FOREIGN KEY (run_id, enterprise_id) REFERENCES public.runs(id, enterprise_id);


--
-- Name: conversations conversations_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.conversations
    ADD CONSTRAINT conversations_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: conversations conversations_owner_user_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.conversations
    ADD CONSTRAINT conversations_owner_user_id_enterprise_id_fkey FOREIGN KEY (owner_user_id, enterprise_id) REFERENCES public.enterprise_users(id, enterprise_id);


--
-- Name: conversations conversations_selected_model_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.conversations
    ADD CONSTRAINT conversations_selected_model_id_enterprise_id_fkey FOREIGN KEY (selected_model_id, enterprise_id) REFERENCES public.ai_models(id, enterprise_id);


--
-- Name: credential_leases credential_leases_credential_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.credential_leases
    ADD CONSTRAINT credential_leases_credential_id_enterprise_id_fkey FOREIGN KEY (credential_id, enterprise_id) REFERENCES public.credentials(id, enterprise_id);


--
-- Name: credential_leases credential_leases_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.credential_leases
    ADD CONSTRAINT credential_leases_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: credential_leases credential_leases_secret_version_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.credential_leases
    ADD CONSTRAINT credential_leases_secret_version_id_enterprise_id_fkey FOREIGN KEY (secret_version_id, enterprise_id) REFERENCES public.secret_versions(id, enterprise_id);


--
-- Name: credentials credentials_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.credentials
    ADD CONSTRAINT credentials_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: credentials credentials_secret_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.credentials
    ADD CONSTRAINT credentials_secret_id_enterprise_id_fkey FOREIGN KEY (secret_id, enterprise_id) REFERENCES public.secrets(id, enterprise_id);


--
-- Name: data_authorization_grants data_authorization_grants_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.data_authorization_grants
    ADD CONSTRAINT data_authorization_grants_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: departments departments_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.departments
    ADD CONSTRAINT departments_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: enterprise_telemetry_tables enterprise_telemetry_tables_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.enterprise_telemetry_tables
    ADD CONSTRAINT enterprise_telemetry_tables_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id) ON DELETE CASCADE;


--
-- Name: enterprise_users enterprise_users_department_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.enterprise_users
    ADD CONSTRAINT enterprise_users_department_id_enterprise_id_fkey FOREIGN KEY (department_id, enterprise_id) REFERENCES public.departments(id, enterprise_id);


--
-- Name: enterprise_users enterprise_users_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.enterprise_users
    ADD CONSTRAINT enterprise_users_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: execution_one_time_results execution_one_time_results_consumed_by_user_id_enterprise__fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.execution_one_time_results
    ADD CONSTRAINT execution_one_time_results_consumed_by_user_id_enterprise__fkey FOREIGN KEY (consumed_by_user_id, enterprise_id) REFERENCES public.enterprise_users(id, enterprise_id);


--
-- Name: execution_one_time_results execution_one_time_results_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.execution_one_time_results
    ADD CONSTRAINT execution_one_time_results_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: execution_one_time_results execution_one_time_results_execution_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.execution_one_time_results
    ADD CONSTRAINT execution_one_time_results_execution_id_enterprise_id_fkey FOREIGN KEY (execution_id, enterprise_id) REFERENCES public.executions(id, enterprise_id);


--
-- Name: executions executions_connector_command_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.executions
    ADD CONSTRAINT executions_connector_command_id_fkey FOREIGN KEY (connector_command_id) REFERENCES public.connector_commands(id);


--
-- Name: executions executions_connector_install_operation_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.executions
    ADD CONSTRAINT executions_connector_install_operation_id_fkey FOREIGN KEY (connector_install_operation_id) REFERENCES public.connector_install_operations(id);


--
-- Name: executions executions_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.executions
    ADD CONSTRAINT executions_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: executions executions_host_onboarding_operation_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.executions
    ADD CONSTRAINT executions_host_onboarding_operation_id_fkey FOREIGN KEY (host_onboarding_operation_id) REFERENCES public.host_onboarding_operations(id);


--
-- Name: executions executions_pending_action_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.executions
    ADD CONSTRAINT executions_pending_action_id_fkey FOREIGN KEY (pending_action_id) REFERENCES public.pending_actions(id);


--
-- Name: executions executions_run_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.executions
    ADD CONSTRAINT executions_run_id_enterprise_id_fkey FOREIGN KEY (run_id, enterprise_id) REFERENCES public.runs(id, enterprise_id);


--
-- Name: executions executions_telemetry_collector_operation_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.executions
    ADD CONSTRAINT executions_telemetry_collector_operation_id_fkey FOREIGN KEY (telemetry_collector_operation_id) REFERENCES public.telemetry_collector_operations(id);


--
-- Name: host_onboarding_operation_events host_onboarding_operation_events_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_onboarding_operation_events
    ADD CONSTRAINT host_onboarding_operation_events_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: host_onboarding_operation_events host_onboarding_operation_events_operation_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_onboarding_operation_events
    ADD CONSTRAINT host_onboarding_operation_events_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES public.host_onboarding_operations(id) ON DELETE CASCADE;


--
-- Name: host_onboarding_operation_secrets host_onboarding_operation_secrets_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_onboarding_operation_secrets
    ADD CONSTRAINT host_onboarding_operation_secrets_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: host_onboarding_operation_secrets host_onboarding_operation_secrets_operation_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_onboarding_operation_secrets
    ADD CONSTRAINT host_onboarding_operation_secrets_operation_id_fkey FOREIGN KEY (operation_id) REFERENCES public.host_onboarding_operations(id) ON DELETE CASCADE;


--
-- Name: host_onboarding_operations host_onboarding_operations_bastion_scope_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_onboarding_operations
    ADD CONSTRAINT host_onboarding_operations_bastion_scope_id_enterprise_id_fkey FOREIGN KEY (bastion_scope_id, enterprise_id) REFERENCES public.bastion_scopes(id, enterprise_id);


--
-- Name: host_onboarding_operations host_onboarding_operations_connection_test_id_enterprise_i_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_onboarding_operations
    ADD CONSTRAINT host_onboarding_operations_connection_test_id_enterprise_i_fkey FOREIGN KEY (connection_test_id, enterprise_id) REFERENCES public.connection_tests(id, enterprise_id);


--
-- Name: host_onboarding_operations host_onboarding_operations_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_onboarding_operations
    ADD CONSTRAINT host_onboarding_operations_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: host_onboarding_operations host_onboarding_operations_host_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_onboarding_operations
    ADD CONSTRAINT host_onboarding_operations_host_id_enterprise_id_fkey FOREIGN KEY (host_id, enterprise_id) REFERENCES public.hosts(id, enterprise_id);


--
-- Name: host_onboarding_operations host_onboarding_operations_pending_action_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_onboarding_operations
    ADD CONSTRAINT host_onboarding_operations_pending_action_id_fkey FOREIGN KEY (pending_action_id) REFERENCES public.pending_actions(id);


--
-- Name: host_onboarding_operations host_onboarding_operations_release_version_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_onboarding_operations
    ADD CONSTRAINT host_onboarding_operations_release_version_id_fkey FOREIGN KEY (release_version_id) REFERENCES public.connector_release_versions(id);


--
-- Name: host_onboarding_operations host_onboarding_operations_retry_of_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_onboarding_operations
    ADD CONSTRAINT host_onboarding_operations_retry_of_fkey FOREIGN KEY (retry_of) REFERENCES public.host_onboarding_operations(id);


--
-- Name: host_runtime_observations host_runtime_observations_connector_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_runtime_observations
    ADD CONSTRAINT host_runtime_observations_connector_id_enterprise_id_fkey FOREIGN KEY (connector_id, enterprise_id) REFERENCES public.connectors(id, enterprise_id) ON DELETE CASCADE;


--
-- Name: host_runtime_observations host_runtime_observations_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_runtime_observations
    ADD CONSTRAINT host_runtime_observations_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: host_runtime_observations host_runtime_observations_host_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.host_runtime_observations
    ADD CONSTRAINT host_runtime_observations_host_id_enterprise_id_fkey FOREIGN KEY (host_id, enterprise_id) REFERENCES public.hosts(id, enterprise_id) ON DELETE CASCADE;


--
-- Name: hosts hosts_bastion_scope_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.hosts
    ADD CONSTRAINT hosts_bastion_scope_id_enterprise_id_fkey FOREIGN KEY (bastion_scope_id, enterprise_id) REFERENCES public.bastion_scopes(id, enterprise_id);


--
-- Name: hosts hosts_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.hosts
    ADD CONSTRAINT hosts_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: interactive_cards interactive_cards_active_version_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.interactive_cards
    ADD CONSTRAINT interactive_cards_active_version_fk FOREIGN KEY (active_version_id, id) REFERENCES public.card_versions(id, card_id);


--
-- Name: interactive_cards interactive_cards_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.interactive_cards
    ADD CONSTRAINT interactive_cards_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: kubernetes_clusters kubernetes_clusters_bastion_scope_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kubernetes_clusters
    ADD CONSTRAINT kubernetes_clusters_bastion_scope_id_enterprise_id_fkey FOREIGN KEY (bastion_scope_id, enterprise_id) REFERENCES public.bastion_scopes(id, enterprise_id);


--
-- Name: kubernetes_clusters kubernetes_clusters_credential_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kubernetes_clusters
    ADD CONSTRAINT kubernetes_clusters_credential_id_enterprise_id_fkey FOREIGN KEY (credential_id, enterprise_id) REFERENCES public.credentials(id, enterprise_id);


--
-- Name: kubernetes_clusters kubernetes_clusters_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kubernetes_clusters
    ADD CONSTRAINT kubernetes_clusters_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: kubernetes_node_host_bindings kubernetes_node_host_bindings_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kubernetes_node_host_bindings
    ADD CONSTRAINT kubernetes_node_host_bindings_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: kubernetes_node_host_bindings kubernetes_node_host_bindings_host_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kubernetes_node_host_bindings
    ADD CONSTRAINT kubernetes_node_host_bindings_host_id_enterprise_id_fkey FOREIGN KEY (host_id, enterprise_id) REFERENCES public.hosts(id, enterprise_id);


--
-- Name: kubernetes_node_host_bindings kubernetes_node_host_bindings_kubernetes_cluster_id_enterp_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kubernetes_node_host_bindings
    ADD CONSTRAINT kubernetes_node_host_bindings_kubernetes_cluster_id_enterp_fkey FOREIGN KEY (kubernetes_cluster_id, enterprise_id) REFERENCES public.kubernetes_clusters(id, enterprise_id);


--
-- Name: managed_accounts managed_accounts_credential_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.managed_accounts
    ADD CONSTRAINT managed_accounts_credential_id_enterprise_id_fkey FOREIGN KEY (credential_id, enterprise_id) REFERENCES public.credentials(id, enterprise_id);


--
-- Name: managed_accounts managed_accounts_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.managed_accounts
    ADD CONSTRAINT managed_accounts_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: managed_accounts managed_accounts_host_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.managed_accounts
    ADD CONSTRAINT managed_accounts_host_id_enterprise_id_fkey FOREIGN KEY (host_id, enterprise_id) REFERENCES public.hosts(id, enterprise_id);


--
-- Name: mfa_recovery_codes mfa_recovery_codes_credential_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mfa_recovery_codes
    ADD CONSTRAINT mfa_recovery_codes_credential_id_fkey FOREIGN KEY (credential_id) REFERENCES public.mfa_credentials(id) ON DELETE CASCADE;


--
-- Name: model_calls model_calls_model_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.model_calls
    ADD CONSTRAINT model_calls_model_id_enterprise_id_fkey FOREIGN KEY (model_id, enterprise_id) REFERENCES public.ai_models(id, enterprise_id);


--
-- Name: model_calls model_calls_run_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.model_calls
    ADD CONSTRAINT model_calls_run_id_enterprise_id_fkey FOREIGN KEY (run_id, enterprise_id) REFERENCES public.runs(id, enterprise_id);


--
-- Name: model_calls model_calls_step_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.model_calls
    ADD CONSTRAINT model_calls_step_id_enterprise_id_fkey FOREIGN KEY (step_id, enterprise_id) REFERENCES public.run_steps(id, enterprise_id);


--
-- Name: model_compatibility_results model_compatibility_results_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.model_compatibility_results
    ADD CONSTRAINT model_compatibility_results_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: model_compatibility_results model_compatibility_results_model_revision_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.model_compatibility_results
    ADD CONSTRAINT model_compatibility_results_model_revision_id_fkey FOREIGN KEY (model_revision_id) REFERENCES public.ai_model_revisions(id);


--
-- Name: model_quota_reservations model_quota_reservations_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.model_quota_reservations
    ADD CONSTRAINT model_quota_reservations_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: model_quota_reservations model_quota_reservations_model_call_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.model_quota_reservations
    ADD CONSTRAINT model_quota_reservations_model_call_id_fkey FOREIGN KEY (model_call_id) REFERENCES public.model_calls(id);


--
-- Name: model_quota_reservations model_quota_reservations_model_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.model_quota_reservations
    ADD CONSTRAINT model_quota_reservations_model_id_enterprise_id_fkey FOREIGN KEY (model_id, enterprise_id) REFERENCES public.ai_models(id, enterprise_id);


--
-- Name: model_quotas model_quotas_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.model_quotas
    ADD CONSTRAINT model_quotas_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: model_quotas model_quotas_model_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.model_quotas
    ADD CONSTRAINT model_quotas_model_id_enterprise_id_fkey FOREIGN KEY (model_id, enterprise_id) REFERENCES public.ai_models(id, enterprise_id);


--
-- Name: pending_action_plans pending_action_plans_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pending_action_plans
    ADD CONSTRAINT pending_action_plans_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: pending_action_plans pending_action_plans_pending_action_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pending_action_plans
    ADD CONSTRAINT pending_action_plans_pending_action_id_fkey FOREIGN KEY (pending_action_id) REFERENCES public.pending_actions(id);


--
-- Name: pending_action_tokens pending_action_tokens_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pending_action_tokens
    ADD CONSTRAINT pending_action_tokens_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: pending_action_tokens pending_action_tokens_pending_action_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pending_action_tokens
    ADD CONSTRAINT pending_action_tokens_pending_action_id_fkey FOREIGN KEY (pending_action_id) REFERENCES public.pending_actions(id);


--
-- Name: pending_actions pending_actions_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pending_actions
    ADD CONSTRAINT pending_actions_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: pending_actions pending_actions_run_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pending_actions
    ADD CONSTRAINT pending_actions_run_fk FOREIGN KEY (run_id) REFERENCES public.runs(id);


--
-- Name: pki_certificate_identities pki_certificate_identities_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pki_certificate_identities
    ADD CONSTRAINT pki_certificate_identities_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: pki_node_trust_acks pki_node_trust_acks_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pki_node_trust_acks
    ADD CONSTRAINT pki_node_trust_acks_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: pki_node_trust_acks pki_node_trust_acks_epoch_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pki_node_trust_acks
    ADD CONSTRAINT pki_node_trust_acks_epoch_fkey FOREIGN KEY (epoch) REFERENCES public.pki_trust_bundles(epoch) ON DELETE CASCADE;


--
-- Name: remote_access_approval_workflows remote_access_approval_workflows_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_approval_workflows
    ADD CONSTRAINT remote_access_approval_workflows_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: remote_access_command_events remote_access_command_events_session_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_command_events
    ADD CONSTRAINT remote_access_command_events_session_id_fkey FOREIGN KEY (session_id) REFERENCES public.remote_access_sessions(id);


--
-- Name: remote_access_decisions remote_access_decisions_decided_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_decisions
    ADD CONSTRAINT remote_access_decisions_decided_by_fkey FOREIGN KEY (decided_by) REFERENCES public.enterprise_users(id);


--
-- Name: remote_access_decisions remote_access_decisions_request_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_decisions
    ADD CONSTRAINT remote_access_decisions_request_id_fkey FOREIGN KEY (request_id) REFERENCES public.remote_access_requests(id);


--
-- Name: remote_access_decisions remote_access_decisions_requirement_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_decisions
    ADD CONSTRAINT remote_access_decisions_requirement_id_fkey FOREIGN KEY (requirement_id) REFERENCES public.remote_access_requirement_snapshots(id);


--
-- Name: remote_access_grants remote_access_grants_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_grants
    ADD CONSTRAINT remote_access_grants_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: remote_access_leases remote_access_leases_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_leases
    ADD CONSTRAINT remote_access_leases_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: remote_access_leases remote_access_leases_grant_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_leases
    ADD CONSTRAINT remote_access_leases_grant_id_enterprise_id_fkey FOREIGN KEY (grant_id, enterprise_id) REFERENCES public.remote_access_grants(id, enterprise_id);


--
-- Name: remote_access_leases remote_access_leases_host_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_leases
    ADD CONSTRAINT remote_access_leases_host_id_enterprise_id_fkey FOREIGN KEY (host_id, enterprise_id) REFERENCES public.hosts(id, enterprise_id);


--
-- Name: remote_access_leases remote_access_leases_managed_account_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_leases
    ADD CONSTRAINT remote_access_leases_managed_account_id_enterprise_id_fkey FOREIGN KEY (managed_account_id, enterprise_id) REFERENCES public.managed_accounts(id, enterprise_id);


--
-- Name: remote_access_leases remote_access_leases_request_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_leases
    ADD CONSTRAINT remote_access_leases_request_id_fkey FOREIGN KEY (request_id) REFERENCES public.remote_access_requests(id);


--
-- Name: remote_access_leases remote_access_leases_user_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_leases
    ADD CONSTRAINT remote_access_leases_user_id_enterprise_id_fkey FOREIGN KEY (user_id, enterprise_id) REFERENCES public.enterprise_users(id, enterprise_id);


--
-- Name: remote_access_recording_chunks remote_access_recording_chunks_recording_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_recording_chunks
    ADD CONSTRAINT remote_access_recording_chunks_recording_id_fkey FOREIGN KEY (recording_id) REFERENCES public.remote_access_recordings(id);


--
-- Name: remote_access_recordings remote_access_recordings_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_recordings
    ADD CONSTRAINT remote_access_recordings_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: remote_access_recordings remote_access_recordings_session_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_recordings
    ADD CONSTRAINT remote_access_recordings_session_id_enterprise_id_fkey FOREIGN KEY (session_id, enterprise_id) REFERENCES public.remote_access_sessions(id, enterprise_id);


--
-- Name: remote_access_requests remote_access_requests_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_requests
    ADD CONSTRAINT remote_access_requests_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: remote_access_requests remote_access_requests_grant_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_requests
    ADD CONSTRAINT remote_access_requests_grant_id_enterprise_id_fkey FOREIGN KEY (grant_id, enterprise_id) REFERENCES public.remote_access_grants(id, enterprise_id);


--
-- Name: remote_access_requests remote_access_requests_host_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_requests
    ADD CONSTRAINT remote_access_requests_host_id_enterprise_id_fkey FOREIGN KEY (host_id, enterprise_id) REFERENCES public.hosts(id, enterprise_id);


--
-- Name: remote_access_requests remote_access_requests_managed_account_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_requests
    ADD CONSTRAINT remote_access_requests_managed_account_id_enterprise_id_fkey FOREIGN KEY (managed_account_id, enterprise_id) REFERENCES public.managed_accounts(id, enterprise_id);


--
-- Name: remote_access_requests remote_access_requests_requester_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_requests
    ADD CONSTRAINT remote_access_requests_requester_id_enterprise_id_fkey FOREIGN KEY (requester_id, enterprise_id) REFERENCES public.enterprise_users(id, enterprise_id);


--
-- Name: remote_access_requirement_snapshots remote_access_requirement_profile_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_requirement_snapshots
    ADD CONSTRAINT remote_access_requirement_profile_fk FOREIGN KEY (session_profile_id) REFERENCES public.remote_access_session_profiles(id);


--
-- Name: remote_access_requirement_snapshots remote_access_requirement_rule_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_requirement_snapshots
    ADD CONSTRAINT remote_access_requirement_rule_fk FOREIGN KEY (rule_id) REFERENCES public.remote_access_rules(id);


--
-- Name: remote_access_requirement_snapshots remote_access_requirement_snapshots_request_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_requirement_snapshots
    ADD CONSTRAINT remote_access_requirement_snapshots_request_id_fkey FOREIGN KEY (request_id) REFERENCES public.remote_access_requests(id);


--
-- Name: remote_access_requirement_snapshots remote_access_requirement_workflow_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_requirement_snapshots
    ADD CONSTRAINT remote_access_requirement_workflow_fk FOREIGN KEY (workflow_id) REFERENCES public.remote_access_approval_workflows(id);


--
-- Name: remote_access_routes remote_access_routes_session_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_routes
    ADD CONSTRAINT remote_access_routes_session_id_fkey FOREIGN KEY (session_id) REFERENCES public.remote_access_sessions(id);


--
-- Name: remote_access_rules remote_access_rules_approval_workflow_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_rules
    ADD CONSTRAINT remote_access_rules_approval_workflow_id_enterprise_id_fkey FOREIGN KEY (approval_workflow_id, enterprise_id) REFERENCES public.remote_access_approval_workflows(id, enterprise_id);


--
-- Name: remote_access_rules remote_access_rules_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_rules
    ADD CONSTRAINT remote_access_rules_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: remote_access_rules remote_access_rules_session_profile_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_rules
    ADD CONSTRAINT remote_access_rules_session_profile_id_enterprise_id_fkey FOREIGN KEY (session_profile_id, enterprise_id) REFERENCES public.remote_access_session_profiles(id, enterprise_id);


--
-- Name: remote_access_session_profiles remote_access_session_profiles_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_session_profiles
    ADD CONSTRAINT remote_access_session_profiles_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: remote_access_sessions remote_access_sessions_connector_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_sessions
    ADD CONSTRAINT remote_access_sessions_connector_id_enterprise_id_fkey FOREIGN KEY (connector_id, enterprise_id) REFERENCES public.connectors(id, enterprise_id);


--
-- Name: remote_access_sessions remote_access_sessions_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_sessions
    ADD CONSTRAINT remote_access_sessions_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: remote_access_sessions remote_access_sessions_host_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_sessions
    ADD CONSTRAINT remote_access_sessions_host_id_enterprise_id_fkey FOREIGN KEY (host_id, enterprise_id) REFERENCES public.hosts(id, enterprise_id);


--
-- Name: remote_access_sessions remote_access_sessions_http_session_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_sessions
    ADD CONSTRAINT remote_access_sessions_http_session_id_fkey FOREIGN KEY (http_session_id) REFERENCES public.sessions(id);


--
-- Name: remote_access_sessions remote_access_sessions_lease_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_sessions
    ADD CONSTRAINT remote_access_sessions_lease_id_enterprise_id_fkey FOREIGN KEY (lease_id, enterprise_id) REFERENCES public.remote_access_leases(id, enterprise_id);


--
-- Name: remote_access_sessions remote_access_sessions_managed_account_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_sessions
    ADD CONSTRAINT remote_access_sessions_managed_account_id_enterprise_id_fkey FOREIGN KEY (managed_account_id, enterprise_id) REFERENCES public.managed_accounts(id, enterprise_id);


--
-- Name: remote_access_sessions remote_access_sessions_user_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_sessions
    ADD CONSTRAINT remote_access_sessions_user_id_enterprise_id_fkey FOREIGN KEY (user_id, enterprise_id) REFERENCES public.enterprise_users(id, enterprise_id);


--
-- Name: remote_access_tickets remote_access_tickets_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_tickets
    ADD CONSTRAINT remote_access_tickets_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: remote_access_tickets remote_access_tickets_http_session_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_tickets
    ADD CONSTRAINT remote_access_tickets_http_session_id_fkey FOREIGN KEY (http_session_id) REFERENCES public.sessions(id);


--
-- Name: remote_access_tickets remote_access_tickets_lease_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_tickets
    ADD CONSTRAINT remote_access_tickets_lease_id_fkey FOREIGN KEY (lease_id) REFERENCES public.remote_access_leases(id);


--
-- Name: remote_access_tickets remote_access_tickets_session_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.remote_access_tickets
    ADD CONSTRAINT remote_access_tickets_session_id_fkey FOREIGN KEY (session_id) REFERENCES public.remote_access_sessions(id);


--
-- Name: role_bindings role_bindings_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.role_bindings
    ADD CONSTRAINT role_bindings_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: role_bindings role_bindings_role_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.role_bindings
    ADD CONSTRAINT role_bindings_role_id_enterprise_id_fkey FOREIGN KEY (role_id, enterprise_id) REFERENCES public.roles(id, enterprise_id);


--
-- Name: role_permissions role_permissions_permission_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.role_permissions
    ADD CONSTRAINT role_permissions_permission_id_fkey FOREIGN KEY (permission_id) REFERENCES public.permissions(id);


--
-- Name: role_permissions role_permissions_role_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.role_permissions
    ADD CONSTRAINT role_permissions_role_id_fkey FOREIGN KEY (role_id) REFERENCES public.roles(id);


--
-- Name: roles roles_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.roles
    ADD CONSTRAINT roles_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: run_steps run_steps_run_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.run_steps
    ADD CONSTRAINT run_steps_run_id_enterprise_id_fkey FOREIGN KEY (run_id, enterprise_id) REFERENCES public.runs(id, enterprise_id);


--
-- Name: runs runs_conversation_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.runs
    ADD CONSTRAINT runs_conversation_id_enterprise_id_fkey FOREIGN KEY (conversation_id, enterprise_id) REFERENCES public.conversations(id, enterprise_id);


--
-- Name: runs runs_current_step_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.runs
    ADD CONSTRAINT runs_current_step_fk FOREIGN KEY (current_step_id) REFERENCES public.run_steps(id);


--
-- Name: runs runs_model_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.runs
    ADD CONSTRAINT runs_model_id_enterprise_id_fkey FOREIGN KEY (model_id, enterprise_id) REFERENCES public.ai_models(id, enterprise_id);


--
-- Name: runtime_tasks runtime_tasks_run_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.runtime_tasks
    ADD CONSTRAINT runtime_tasks_run_id_enterprise_id_fkey FOREIGN KEY (run_id, enterprise_id) REFERENCES public.runs(id, enterprise_id);


--
-- Name: runtime_tasks runtime_tasks_step_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.runtime_tasks
    ADD CONSTRAINT runtime_tasks_step_id_enterprise_id_fkey FOREIGN KEY (step_id, enterprise_id) REFERENCES public.run_steps(id, enterprise_id);


--
-- Name: sandbox_images sandbox_images_backend_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sandbox_images
    ADD CONSTRAINT sandbox_images_backend_id_fkey FOREIGN KEY (backend_id) REFERENCES public.sandbox_backends(id);


--
-- Name: sandbox_profiles sandbox_profiles_backend_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sandbox_profiles
    ADD CONSTRAINT sandbox_profiles_backend_id_fkey FOREIGN KEY (backend_id) REFERENCES public.sandbox_backends(id);


--
-- Name: sandbox_profiles sandbox_profiles_image_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sandbox_profiles
    ADD CONSTRAINT sandbox_profiles_image_id_fkey FOREIGN KEY (image_id) REFERENCES public.sandbox_images(id);


--
-- Name: sandbox_quotas sandbox_quotas_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sandbox_quotas
    ADD CONSTRAINT sandbox_quotas_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: sandbox_sessions sandbox_sessions_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sandbox_sessions
    ADD CONSTRAINT sandbox_sessions_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: sandbox_sessions sandbox_sessions_profile_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sandbox_sessions
    ADD CONSTRAINT sandbox_sessions_profile_id_fkey FOREIGN KEY (profile_id) REFERENCES public.sandbox_profiles(id);


--
-- Name: sandbox_sessions sandbox_sessions_task_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sandbox_sessions
    ADD CONSTRAINT sandbox_sessions_task_id_fkey FOREIGN KEY (task_id) REFERENCES public.runtime_tasks(id);


--
-- Name: sandbox_usage sandbox_usage_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sandbox_usage
    ADD CONSTRAINT sandbox_usage_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: secret_versions secret_versions_secret_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.secret_versions
    ADD CONSTRAINT secret_versions_secret_id_enterprise_id_fkey FOREIGN KEY (secret_id, enterprise_id) REFERENCES public.secrets(id, enterprise_id);


--
-- Name: secrets secrets_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.secrets
    ADD CONSTRAINT secrets_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: service_accounts service_accounts_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.service_accounts
    ADD CONSTRAINT service_accounts_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: sessions sessions_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sessions
    ADD CONSTRAINT sessions_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: telemetry_certificates telemetry_certificates_collector_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telemetry_certificates
    ADD CONSTRAINT telemetry_certificates_collector_id_fkey FOREIGN KEY (collector_id) REFERENCES public.collector_instances(id);


--
-- Name: telemetry_collector_operations telemetry_collector_operations_collector_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telemetry_collector_operations
    ADD CONSTRAINT telemetry_collector_operations_collector_id_enterprise_id_fkey FOREIGN KEY (collector_id, enterprise_id) REFERENCES public.collector_instances(id, enterprise_id);


--
-- Name: telemetry_collector_operations telemetry_collector_operations_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telemetry_collector_operations
    ADD CONSTRAINT telemetry_collector_operations_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: telemetry_collector_operations telemetry_collector_operations_pending_action_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telemetry_collector_operations
    ADD CONSTRAINT telemetry_collector_operations_pending_action_id_fkey FOREIGN KEY (pending_action_id) REFERENCES public.pending_actions(id);


--
-- Name: telemetry_enrollment_tokens telemetry_enrollment_tokens_collector_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telemetry_enrollment_tokens
    ADD CONSTRAINT telemetry_enrollment_tokens_collector_id_fkey FOREIGN KEY (collector_id) REFERENCES public.collector_instances(id);


--
-- Name: telemetry_retention_policies telemetry_retention_policies_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telemetry_retention_policies
    ADD CONSTRAINT telemetry_retention_policies_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: telemetry_route_tests telemetry_route_tests_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telemetry_route_tests
    ADD CONSTRAINT telemetry_route_tests_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: telemetry_route_tests telemetry_route_tests_route_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telemetry_route_tests
    ADD CONSTRAINT telemetry_route_tests_route_id_fkey FOREIGN KEY (route_id) REFERENCES public.telemetry_routes(id);


--
-- Name: telemetry_routes telemetry_routes_collector_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telemetry_routes
    ADD CONSTRAINT telemetry_routes_collector_id_enterprise_id_fkey FOREIGN KEY (collector_id, enterprise_id) REFERENCES public.collector_instances(id, enterprise_id);


--
-- Name: telemetry_routes telemetry_routes_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telemetry_routes
    ADD CONSTRAINT telemetry_routes_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: telemetry_routes telemetry_routes_gateway_collector_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telemetry_routes
    ADD CONSTRAINT telemetry_routes_gateway_collector_id_enterprise_id_fkey FOREIGN KEY (gateway_collector_id, enterprise_id) REFERENCES public.collector_instances(id, enterprise_id);


--
-- Name: telemetry_tunnels telemetry_tunnels_collector_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telemetry_tunnels
    ADD CONSTRAINT telemetry_tunnels_collector_id_enterprise_id_fkey FOREIGN KEY (collector_id, enterprise_id) REFERENCES public.collector_instances(id, enterprise_id);


--
-- Name: telemetry_tunnels telemetry_tunnels_connector_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telemetry_tunnels
    ADD CONSTRAINT telemetry_tunnels_connector_id_enterprise_id_fkey FOREIGN KEY (connector_id, enterprise_id) REFERENCES public.connectors(id, enterprise_id);


--
-- Name: telemetry_tunnels telemetry_tunnels_credential_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telemetry_tunnels
    ADD CONSTRAINT telemetry_tunnels_credential_id_enterprise_id_fkey FOREIGN KEY (credential_id, enterprise_id) REFERENCES public.credentials(id, enterprise_id);


--
-- Name: telemetry_tunnels telemetry_tunnels_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telemetry_tunnels
    ADD CONSTRAINT telemetry_tunnels_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: telemetry_tunnels telemetry_tunnels_host_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telemetry_tunnels
    ADD CONSTRAINT telemetry_tunnels_host_id_enterprise_id_fkey FOREIGN KEY (host_id, enterprise_id) REFERENCES public.hosts(id, enterprise_id);


--
-- Name: telemetry_usage_daily telemetry_usage_daily_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telemetry_usage_daily
    ADD CONSTRAINT telemetry_usage_daily_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: tool_calls tool_calls_run_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tool_calls
    ADD CONSTRAINT tool_calls_run_id_enterprise_id_fkey FOREIGN KEY (run_id, enterprise_id) REFERENCES public.runs(id, enterprise_id);


--
-- Name: tool_calls tool_calls_step_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tool_calls
    ADD CONSTRAINT tool_calls_step_id_enterprise_id_fkey FOREIGN KEY (step_id, enterprise_id) REFERENCES public.run_steps(id, enterprise_id);


--
-- Name: tool_results tool_results_artifact_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tool_results
    ADD CONSTRAINT tool_results_artifact_id_fkey FOREIGN KEY (artifact_id) REFERENCES public.artifacts(id);


--
-- Name: tool_results tool_results_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tool_results
    ADD CONSTRAINT tool_results_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: tool_results tool_results_tool_call_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tool_results
    ADD CONSTRAINT tool_results_tool_call_id_fkey FOREIGN KEY (tool_call_id) REFERENCES public.tool_calls(id);


--
-- Name: user_confirmations user_confirmations_actor_user_id_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_confirmations
    ADD CONSTRAINT user_confirmations_actor_user_id_enterprise_id_fkey FOREIGN KEY (actor_user_id, enterprise_id) REFERENCES public.enterprise_users(id, enterprise_id);


--
-- Name: user_confirmations user_confirmations_enterprise_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_confirmations
    ADD CONSTRAINT user_confirmations_enterprise_id_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);


--
-- Name: user_confirmations user_confirmations_pending_action_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_confirmations
    ADD CONSTRAINT user_confirmations_pending_action_id_fkey FOREIGN KEY (pending_action_id) REFERENCES public.pending_actions(id);

-- Host/Bastion removal is a durable, fenced workflow. These foreign keys are
-- declared after the schema dump so every referenced baseline table exists.
ALTER TABLE ONLY public.host_removal_operations
    ADD CONSTRAINT host_removal_operations_enterprise_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id),
    ADD CONSTRAINT host_removal_operations_pending_action_fkey FOREIGN KEY (pending_action_id) REFERENCES public.pending_actions(id),
    ADD CONSTRAINT host_removal_operations_host_fkey FOREIGN KEY (host_id, enterprise_id) REFERENCES public.hosts(id, enterprise_id),
    ADD CONSTRAINT host_removal_operations_bastion_scope_fkey FOREIGN KEY (bastion_scope_id, enterprise_id) REFERENCES public.bastion_scopes(id, enterprise_id),
    ADD CONSTRAINT host_removal_operations_connector_fkey FOREIGN KEY (connector_id, enterprise_id) REFERENCES public.connectors(id, enterprise_id),
    ADD CONSTRAINT host_removal_operations_connection_test_fkey FOREIGN KEY (connection_test_id) REFERENCES public.connection_tests(id),
    ADD CONSTRAINT host_removal_operations_credential_fkey FOREIGN KEY (credential_id, enterprise_id) REFERENCES public.credentials(id, enterprise_id);

ALTER TABLE ONLY public.host_removal_operation_steps
    ADD CONSTRAINT host_removal_operation_steps_operation_fkey FOREIGN KEY (operation_id) REFERENCES public.host_removal_operations(id) ON DELETE CASCADE,
    ADD CONSTRAINT host_removal_operation_steps_enterprise_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);

ALTER TABLE ONLY public.host_removal_operation_events
    ADD CONSTRAINT host_removal_operation_events_operation_fkey FOREIGN KEY (operation_id) REFERENCES public.host_removal_operations(id) ON DELETE CASCADE,
    ADD CONSTRAINT host_removal_operation_events_enterprise_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);

ALTER TABLE ONLY public.host_removal_tokens
    ADD CONSTRAINT host_removal_tokens_operation_fkey FOREIGN KEY (operation_id) REFERENCES public.host_removal_operations(id) ON DELETE CASCADE,
    ADD CONSTRAINT host_removal_tokens_enterprise_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id);

ALTER TABLE ONLY public.host_managed_change_journals
    ADD CONSTRAINT host_managed_change_journals_enterprise_fkey FOREIGN KEY (enterprise_id) REFERENCES public.enterprises(id),
    ADD CONSTRAINT host_managed_change_journals_host_fkey FOREIGN KEY (host_id, enterprise_id) REFERENCES public.hosts(id, enterprise_id),
    ADD CONSTRAINT host_managed_change_journals_connector_fkey FOREIGN KEY (connector_id, enterprise_id) REFERENCES public.connectors(id, enterprise_id);

ALTER TABLE ONLY public.executions
    ADD CONSTRAINT executions_host_removal_operation_fkey FOREIGN KEY (host_removal_operation_id) REFERENCES public.host_removal_operations(id);

CREATE UNIQUE INDEX host_removal_operations_active_target_idx ON public.host_removal_operations (enterprise_id, target_type, host_id)
WHERE status IN ('queued','running','awaiting_manual_execution','failed','cleanup_unknown');
CREATE INDEX host_removal_operations_claim_idx ON public.host_removal_operations (created_at, id)
WHERE status = 'queued';
CREATE INDEX host_removal_operations_host_idx ON public.host_removal_operations (enterprise_id, host_id, created_at DESC);
CREATE INDEX host_removal_operation_events_operation_idx ON public.host_removal_operation_events (operation_id, sequence);
CREATE INDEX host_removal_tokens_active_idx ON public.host_removal_tokens (token_hash, expires_at) WHERE status = 'active';
CREATE UNIQUE INDEX host_managed_change_journals_active_idx ON public.host_managed_change_journals (enterprise_id, host_id, change_type)
WHERE status = 'active';

INSERT INTO public.platform_state (singleton, state) VALUES (true, 'uninitialized');
INSERT INTO public.permissions (id, description, registry_version) VALUES ('host.read', 'Read hosts', 2);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('host.manage', 'Manage hosts', 2);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('host.test', 'Test host connections', 2);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('kubernetes.read', 'Read Kubernetes clusters and resources', 2);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('kubernetes.manage', 'Manage Kubernetes clusters', 2);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('kubernetes.logs', 'Read bounded Kubernetes Pod logs', 2);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('secret.read', 'Read Secret metadata', 2);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('secret.manage', 'Create and rotate Secrets', 2);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('credential.read', 'Read Credential metadata', 2);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('credential.manage', 'Manage Credentials', 2);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('credential.use', 'Use Credentials through the broker', 2);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('managed_account.read', 'Read managed accounts', 2);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('managed_account.manage', 'Manage managed accounts', 2);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('bastion_scope.read', 'Read Bastion Scopes', 2);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('bastion_scope.manage', 'Manage Bastion Scopes', 2);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('connector.read', 'Read Connector diagnostics', 2);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('connector.manage', 'Manage Connector lifecycle', 2);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('pending_action.read', 'Read resource Pending Actions', 2);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('pending_action.confirm', 'Confirm resource Pending Actions', 2);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('conversation.read', 'Read conversations and immutable events', 3);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('conversation.use', 'Create messages and run the Model Agent', 3);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('model.read', 'Read enabled AI model metadata and own availability', 3);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('model.manage', 'Manage AI models and pricing', 3);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('model.quota.manage', 'Manage department and user model quotas', 3);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('model.usage.read', 'Read governed model usage', 3);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('approval_policy.read', 'Read approval policies', 3);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('approval_policy.manage', 'Manage approval policies', 3);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('approval.read', 'Read approval requests', 3);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('approval.decide', 'Approve or reject eligible requests', 3);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('execution.read', 'Read deterministic execution state', 3);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('interactive_card.read', 'Read the interactive Card catalog', 4);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('interactive_card.create', 'Create enterprise Card drafts through Chat', 4);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('interactive_card.update', 'Create Card configuration revisions', 4);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('interactive_card.publish', 'Validate, activate, disable, and roll back Cards', 4);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('interactive_card.deprecate', 'Deprecate enterprise Cards', 4);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('telemetry.collector.read', 'Read Collector catalog and status', 6);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('telemetry.collector.manage', 'Manage Collector lifecycle and routes', 6);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('telemetry.query.metrics', 'Query authorized metrics', 6);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('telemetry.query.logs', 'Query authorized logs', 6);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('telemetry.query.traces', 'Query authorized traces', 6);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('telemetry.sensitive_fields.read', 'Read governed sensitive telemetry fields', 6);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('telemetry.usage.read', 'Read telemetry usage and retention', 6);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('remote_access.rule.read', 'Read remote access rules', 8);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('remote_access.rule.manage', 'Manage remote access rules', 8);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('remote_access.workflow.read', 'Read remote access approval workflows', 8);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('remote_access.workflow.manage', 'Manage remote access approval workflows', 8);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('remote_access.session_profile.read', 'Read remote access session profiles', 8);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('remote_access.session_profile.manage', 'Manage remote access session profiles', 8);
INSERT INTO public.permissions (id, description, registry_version) VALUES ('remote_access.governance.references.read', 'Read remote access governance references', 8);

REVOKE UPDATE, DELETE ON public.audit_events FROM PUBLIC;

-- Runtime login roles are created by the database bootstrap chart. A plain
-- developer database intentionally skips grants for roles that do not exist.
-- +goose StatementBegin
DO $roles$
DECLARE role_name text;
BEGIN
  FOREACH role_name IN ARRAY ARRAY['argus_server','argus_worker','argus_gateway','argus_direct_executor'] LOOP
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = role_name) THEN
      EXECUTE format('GRANT CONNECT ON DATABASE argus TO %I', role_name);
      EXECUTE format('GRANT USAGE ON SCHEMA public TO %I', role_name);
      EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO %I', role_name);
      EXECUTE format('GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO %I', role_name);
    END IF;
  END LOOP;
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'argus_telemetry_ingest') THEN
    GRANT CONNECT ON DATABASE argus TO argus_telemetry_ingest;
    GRANT USAGE ON SCHEMA public TO argus_telemetry_ingest;
    GRANT SELECT ON public.collector_instances, public.telemetry_certificates, public.telemetry_routes TO argus_telemetry_ingest;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'argus_telemetry_writer') THEN
    GRANT CONNECT ON DATABASE argus TO argus_telemetry_writer;
    GRANT USAGE ON SCHEMA public TO argus_telemetry_writer;
    GRANT SELECT, INSERT, UPDATE ON public.telemetry_retention_policies TO argus_telemetry_writer;
    GRANT SELECT, INSERT, UPDATE ON public.telemetry_usage_daily, public.telemetry_dlq_records TO argus_telemetry_writer;
    GRANT SELECT ON public.enterprise_telemetry_tables TO argus_telemetry_writer;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'argus_telemetry_query') THEN
    GRANT CONNECT ON DATABASE argus TO argus_telemetry_query;
    GRANT USAGE ON SCHEMA public TO argus_telemetry_query;
    GRANT SELECT ON public.enterprises TO argus_telemetry_query;
    GRANT SELECT, INSERT, UPDATE ON public.enterprise_telemetry_tables TO argus_telemetry_query;
    GRANT SELECT, INSERT, UPDATE ON public.audit_chain_heads TO argus_telemetry_query;
    GRANT INSERT ON public.audit_events TO argus_telemetry_query;
  END IF;
END
$roles$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $clean$
DECLARE object_name record;
BEGIN
  FOR object_name IN
    SELECT schemaname, tablename FROM pg_tables
    WHERE schemaname = 'public' AND tablename <> 'goose_db_version'
  LOOP
    EXECUTE format('DROP TABLE IF EXISTS %I.%I CASCADE', object_name.schemaname, object_name.tablename);
  END LOOP;
  FOR object_name IN
    SELECT namespace.nspname AS schema_name, procedure.proname AS function_name,
           pg_get_function_identity_arguments(procedure.oid) AS arguments
    FROM pg_proc procedure
    JOIN pg_namespace namespace ON namespace.oid = procedure.pronamespace
    WHERE namespace.nspname = 'public'
  LOOP
    EXECUTE format('DROP FUNCTION IF EXISTS %I.%I(%s) CASCADE', object_name.schema_name, object_name.function_name, object_name.arguments);
  END LOOP;
END
$clean$;
-- +goose StatementEnd
