export interface paths {
    "/alerts/summary": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** getAlertSummary */
        get: operations["getAlertSummary"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/alerts/configurations": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** listAlertConfigurations */
        get: operations["listAlertConfigurations"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/alerts/configurations/{id}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        /** getAlertConfiguration */
        get: operations["getAlertConfiguration"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/alerts/configurations/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** previewAlertConfiguration */
        post: operations["previewAlertConfiguration"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/alerts/commands/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** previewAlertCommand */
        post: operations["previewAlertCommand"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/alerts/trial": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** trialAlertPolicy */
        post: operations["trialAlertPolicy"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/alerts/instances": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** listAlertInstances */
        get: operations["listAlertInstances"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/alerts/history": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** listAlertHistory */
        get: operations["listAlertHistory"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/alerts/deliveries": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** listAlertDeliveries */
        get: operations["listAlertDeliveries"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/alerts/instances/{id}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        /** getAlertInstance */
        get: operations["getAlertInstance"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/alerts/deliveries/{id}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        /** getAlertDelivery */
        get: operations["getAlertDelivery"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/alerts/instances/{id}/handle": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** handleAlertInstance */
        post: operations["handleAlertInstance"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/alerts/history/export": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** exportAlertHistory */
        get: operations["exportAlertHistory"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
}
export type webhooks = Record<string, never>;
export interface components {
    schemas: {
        RequestId: string;
        ApiError: {
            code: string;
            message_key: string;
            params?: {
                [key: string]: string | number | boolean;
            };
            message?: string;
            request_id: components["schemas"]["RequestId"];
            trace_id?: string;
            /** @default false */
            retryable: boolean;
        };
        AlertSummary: {
            /** Format: int64 */
            active: number;
            /** Format: int64 */
            unacknowledged: number;
            /** Format: int64 */
            policies: number;
            /** Format: int64 */
            failed_deliveries: number;
            /** Format: date-time */
            last_evaluated_at?: string;
            /** Format: int64 */
            current_total?: number;
        };
        /** @enum {string} */
        AlertKind: "policy" | "silence" | "inhibition" | "channel" | "route" | "settings" | "template" | "grouping";
        AlertMatcher: {
            key: string;
            /** @enum {string} */
            operator: "=" | "!=" | "=~" | "!~";
            value: string;
        };
        /** @description Resolve hosts, clusters, applications or monitoring targets with the resource tools before selecting their explicit UUIDs. Dynamic selection evaluates resource LABELS inside the evaluator's existing grants; the resource name field is not a label. 指定主机、集群、应用或监控目标时先查询实际 UUID 并使用 explicit + ids；dynamic 的 matchers 只匹配资源 labels，不匹配资源名称字段。 */
        AlertResourceSelector: {
            /**
             * @description Use explicit for concrete requested resources; dynamic is for intentionally changing label-selected authorized sets.
             * @enum {string}
             */
            mode: "dynamic" | "explicit";
            types: ("host" | "kubernetes_cluster" | "application" | "monitoring_target")[];
            ids?: string[];
            /** @description Resource labels only. Do not use name as a matcher to select a named resource unless that exact label was read from the resource. */
            matchers?: components["schemas"]["AlertMatcher"][];
            source_ids?: string[];
        };
        AlertRule: {
            /** Format: uuid */
            id?: string;
            name: string;
            expression: string;
            /** @enum {string} */
            query_mode: "numeric" | "predicate";
            presence_expression?: string;
            /** @enum {string} */
            operator: ">" | ">=" | "<" | "<=" | "==" | "!=";
            /** Format: double */
            threshold: number;
            /** Format: double */
            recovery_threshold: number;
            /** @enum {string} */
            severity: "critical" | "warning" | "info";
            unit?: string;
        };
        AlertPolicySpec: {
            description?: string;
            enabled: boolean;
            resources: components["schemas"]["AlertResourceSelector"];
            rules: components["schemas"]["AlertRule"][];
            /** @enum {string} */
            mode: "independent" | "every" | "any";
            match_by?: string[];
            /** Format: int64 */
            interval_seconds: number;
            /** Format: int64 */
            for_seconds: number;
            /** Format: int64 */
            recovery_seconds: number;
            /** @enum {string} */
            no_data_behavior: "report" | "keep";
            /** @enum {string} */
            error_behavior: "report" | "keep";
            channel_ids?: string[];
            route_ids?: string[];
            labels?: {
                [key: string]: string;
            };
        };
        AlertSilenceSpec: {
            enabled: boolean;
            /** @description Explicit resource identity selection. For a named host/cluster resolve its actual UUID and include it here; label matchers do not match resource names. An explicit empty array selects all evaluator-authorized resources only when that full scope is requested. Omitted or null selection is invalid. 指定资源使用真实 UUID；明确选择全部授权范围时才传空数组。 */
            resource_ids: string[];
            policy_ids?: string[];
            metric?: string;
            /** @description Filters actual observed metric labels within resource_ids; never substitute a named host or cluster for its UUID using a guessed cluster/name label. */
            matchers?: components["schemas"]["AlertMatcher"][];
            /** Format: date-time */
            starts_at: string;
            /** Format: date-time */
            ends_at: string;
            reason: string;
        };
        AlertInhibitionSpec: {
            enabled: boolean;
            /** @description Explicit resource identity selection. For a named host/cluster resolve its actual UUID and include it here; label matchers do not match resource names. An explicit empty array selects all evaluator-authorized resources only when that full scope is requested. Omitted or null selection is invalid. 指定资源使用真实 UUID；明确选择全部授权范围时才传空数组。 */
            resource_ids: string[];
            source: components["schemas"]["AlertMatcher"][];
            target: components["schemas"]["AlertMatcher"][];
            equal: string[];
            reason: string;
        };
        AlertChannelSpec: {
            enabled: boolean;
            /** @enum {string} */
            provider: "dingtalk" | "wecom" | "feishu" | "smtp";
            /** Format: uuid */
            credential_id: string;
            display_address?: string;
            mention_ids?: string[];
            mention_all?: boolean;
            template?: string;
            /** @description Preselect this enabled channel in new portal policy drafts. Existing policies and explicit API/tool targets remain unchanged. */
            default_for_new_policies?: boolean;
        };
        AlertRouteSpec: {
            enabled: boolean;
            matchers: components["schemas"]["AlertMatcher"][];
            channel_ids: string[];
            continue: boolean;
            /** Format: int64 */
            priority: number;
        };
        AlertSettingsSpec: {
            /** Format: uuid */
            evaluator_id: string;
        };
        AlertGroupingSpec: {
            group_by: string[];
            /** Format: int64 */
            wait_seconds: number;
            /** Format: int64 */
            interval_seconds: number;
            /** Format: int64 */
            repeat_seconds: number;
            timing: ("first" | "repeat" | "recovery" | "severity")[];
        };
        AlertTemplateSpec: {
            content: string;
        };
        AlertSpec: components["schemas"]["AlertPolicySpec"] | components["schemas"]["AlertSilenceSpec"] | components["schemas"]["AlertInhibitionSpec"] | components["schemas"]["AlertChannelSpec"] | components["schemas"]["AlertRouteSpec"] | components["schemas"]["AlertSettingsSpec"] | components["schemas"]["AlertGroupingSpec"] | components["schemas"]["AlertTemplateSpec"];
        AlertConfiguration: {
            /** Format: uuid */
            id: string;
            kind: components["schemas"]["AlertKind"];
            name: string;
            spec: components["schemas"]["AlertSpec"];
            /** Format: int64 */
            version: number;
            /** Format: int64 */
            generation: number;
            health: string;
            error_code?: string;
            /** Format: date-time */
            last_evaluated_at?: string;
            /** Format: date-time */
            last_successful_at?: string;
            /** Format: date-time */
            created_at: string;
            /** Format: date-time */
            updated_at: string;
            /** Format: int64 */
            matched_instances?: number;
            /** @enum {string} */
            managed_by?: "collector_delivery";
        };
        IdempotencyKey: string;
        AlertMutation: {
            kind: components["schemas"]["AlertKind"];
            /** @enum {string} */
            operation: "create" | "update" | "delete" | "enable" | "disable";
            /** Format: uuid */
            id?: string;
            name?: string;
            spec?: components["schemas"]["AlertSpec"];
            /** Format: int64 */
            expected_version?: number;
        };
        PublicJsonValue: unknown;
        PublicJsonObject: unknown;
        /** PendingActionPublic */
        "pending-action-public.schema": {
            /** @constant */
            schema_version: "argus.pending_action/v1";
            action_ref: string;
            action_type: string;
            title: string;
            summary: string;
            /** @enum {unknown} */
            risk: "read" | "write" | "dangerous" | "critical";
            preview: components["schemas"]["PublicJsonObject"];
            diff: {
                /** @enum {unknown} */
                kind: "add" | "remove" | "change" | "note";
                text: string;
            }[];
            /** @enum {unknown} */
            status: "prepared" | "awaiting_confirmation" | "awaiting_approval" | "ready" | "executing" | "succeeded" | "failed" | "result_unknown" | "cancelled" | "expired" | "rejected" | "invalidated";
            available_actions: ("confirm" | "cancel" | "approve" | "reject")[];
            approval?: {
                required: boolean;
                policy_ref?: string;
                minimum_approvers: number;
                approved_count: number;
                separation_of_duty: boolean;
            };
            execution_ref?: string;
            result_summary?: string;
            /** Format: date-time */
            expires_at: string;
            /** Format: date-time */
            created_at: string;
            /** Format: date-time */
            updated_at: string;
        };
        AlertHandleInput: {
            /** @enum {string} */
            operation: "acknowledge" | "assign" | "comment";
            /** Format: uuid */
            owner_id?: string;
            comment?: string;
            /** Format: int64 */
            expected_handling_version: number;
        };
        AlertCommand: {
            /** @enum {string} */
            operation: "notification.test" | "notification.resend" | "event.acknowledge" | "event.assign" | "event.comment";
            /** Format: uuid */
            id: string;
            /** Format: int64 */
            expected_version?: number;
            handling?: components["schemas"]["AlertHandleInput"];
            /** Format: int64 */
            expected_handling_version?: number;
        };
        AlertMemberValue: {
            /** Format: uuid */
            rule_id: string;
            name: string;
            expression: string;
            /** Format: double */
            value: number;
            /** Format: double */
            threshold: number;
            /** Format: double */
            recovery_threshold: number;
            severity: string;
            firing: boolean;
            recovering: boolean;
            unit: string;
        };
        AlertObservation: {
            key: string;
            labels: {
                [key: string]: string;
            };
            /** Format: double */
            value: number;
            /** Format: uuid */
            rule_id: string;
            resource_ids: string[];
            source_ids: string[];
            firing: boolean;
            recovering: boolean;
            severity: string;
            rule: components["schemas"]["AlertRule"];
            members?: components["schemas"]["AlertMemberValue"][];
        };
        AlertEvaluationOutcome: {
            /** Format: uuid */
            rule_id: string;
            resource_ids: string[];
            source_ids: string[];
            health: string;
            error_code?: string;
            complete: boolean;
        };
        AlertEvaluation: {
            observations: components["schemas"]["AlertObservation"][];
            health: string;
            error_code: string;
            resource_ids: string[];
            /** Format: date-time */
            slot: string;
            outcomes?: components["schemas"]["AlertEvaluationOutcome"][];
            sources_captured?: boolean;
        };
        CollectorDeliveryCounts: {
            /** Format: int64 */
            log_records: number;
            /** Format: int64 */
            spans: number;
            /** Format: int64 */
            metric_points: number;
        };
        CollectorDeliveryReport: {
            /** @enum {string} */
            schema_version: "argus.collector_delivery/v1";
            instance_id: string;
            /** Format: int64 */
            sequence: number;
            /** Format: date-time */
            observed_since: string;
            /** Format: date-time */
            observed_at: string;
            /** Format: date-time */
            last_failure_at?: string;
            terminal_failures: components["schemas"]["CollectorDeliveryCounts"];
            receiver_refusals: components["schemas"]["CollectorDeliveryCounts"];
            enqueue_failures: components["schemas"]["CollectorDeliveryCounts"];
            /**
             * Format: int64
             * @enum {integer}
             */
            retry_limit_seconds: 300;
            /** @enum {string} */
            attribution: "collector_shared_pipeline_exact_targets_unknown";
        };
        AlertInstanceState: {
            name: string;
            policy_name: string;
            /** Format: uuid */
            rule_id: string;
            metric: string;
            labels: {
                [key: string]: string;
            };
            /** Format: double */
            value?: number;
            /** Format: double */
            threshold: number;
            /** Format: double */
            recovery_threshold: number;
            unit: string;
            /** Format: date-time */
            pending_since?: string;
            /** Format: date-time */
            recovering_since?: string;
            /** Format: date-time */
            last_slot: string;
            policy_snapshot: components["schemas"]["AlertPolicySpec"];
            notification_state: string;
            noise_reasons: string[];
            /** Format: date-time */
            last_notification_at?: string;
            closure_reason?: string;
            kind: string;
            members?: components["schemas"]["AlertMemberValue"][];
            /** Format: uuid */
            evaluator_id?: string;
            /** Format: int64 */
            authorization_version?: number;
            source_snapshot?: {
                [key: string]: unknown;
            }[] | null;
            latest_source_snapshot?: {
                [key: string]: unknown;
            }[] | null;
            /**
             * Format: date-time
             * @description Time of the last successful evaluation that produced the retained value and members. Unchanged on failed evaluations; absent when unknown. This is not the telemetry sample timestamp.
             */
            last_successful_at?: string;
            delivery_gap?: components["schemas"]["CollectorDeliveryReport"];
        };
        AlertInstance: {
            /** Format: uuid */
            id: string;
            /** Format: uuid */
            policy_id: string;
            /** Format: int64 */
            policy_version: number;
            resource_ids: string[];
            source_ids: string[];
            state: components["schemas"]["AlertInstanceState"];
            /** @enum {string} */
            lifecycle: "normal" | "pending" | "firing" | "recovering" | "resolved" | "closed";
            health: string;
            /** @enum {string} */
            severity: "critical" | "warning" | "info";
            acknowledged: boolean;
            /** Format: uuid */
            owner_id?: string;
            /** Format: date-time */
            started_at?: string;
            /** Format: date-time */
            resolved_at?: string;
            /** Format: date-time */
            last_evaluated_at: string;
            /** Format: int64 */
            version: number;
            /** Format: int64 */
            handling_version: number;
        };
        AlertEvaluationStatus: {
            /** Format: uuid */
            policy_id: string;
            name: string;
            health: string;
            error_code?: string;
            /** Format: date-time */
            last_evaluated_at: string;
            /** Format: date-time */
            last_successful_at: string;
        };
        AlertInstancePage: {
            items: components["schemas"]["AlertInstance"][];
            /** Format: int64 */
            total: number;
            next_cursor: string | null;
            complete: boolean;
            resource_ids?: string[];
            evaluations?: components["schemas"]["AlertEvaluationStatus"][];
        };
        AlertMessage: {
            subject: string;
            text: string;
            event_type: string;
            instance_ids: string[];
            policy_ids: string[];
            labels: {
                [key: string]: string;
            };
            test: boolean;
            routing_snapshot?: {
                [key: string]: number;
            };
            /** Format: uuid */
            batch_id?: string;
        };
        AlertDelivery: {
            /** Format: uuid */
            id: string;
            /** Format: uuid */
            instance_id?: string;
            /** Format: uuid */
            channel_id: string;
            resource_ids: string[];
            message: components["schemas"]["AlertMessage"];
            /** @enum {string} */
            status: "pending" | "sending" | "accepted" | "failed" | "unknown" | "muted" | "inhibited" | "canceled";
            /** Format: int64 */
            attempt: number;
            last_error: string;
            /** Format: date-time */
            last_attempt_at?: string;
            /** Format: date-time */
            created_at: string;
            /** Format: int64 */
            version: number;
            /** Format: uuid */
            batch_id?: string;
            /** Format: uuid */
            resend_of_id?: string;
        };
        AlertDeliveryPage: {
            items: components["schemas"]["AlertDelivery"][];
            /** Format: int64 */
            total: number;
            next_cursor: string | null;
            complete: boolean;
            resource_ids?: string[];
            evaluations?: components["schemas"]["AlertEvaluationStatus"][];
        };
        AlertEvent: {
            /** Format: uuid */
            id: string;
            /** Format: uuid */
            instance_id: string;
            event_type: string;
            payload: {
                [key: string]: unknown;
            };
            /** Format: uuid */
            actor_id?: string;
            origin: string;
            /** Format: date-time */
            created_at: string;
        };
        AlertInstanceDetail: {
            instance: components["schemas"]["AlertInstance"];
            events: components["schemas"]["AlertEvent"][];
        };
        AlertDeliveryAttempt: {
            /** Format: uuid */
            id: string;
            /** Format: int64 */
            attempt: number;
            status: string;
            result_code: string;
            error_code: string;
            /** Format: date-time */
            started_at: string;
            /** Format: date-time */
            finished_at: string;
        };
        AlertDeliveryDetail: {
            delivery: components["schemas"]["AlertDelivery"];
            attempts: components["schemas"]["AlertDeliveryAttempt"][];
        };
        AlertExport: {
            filename: string;
            content_type: string;
            content: string;
            /** Format: int64 */
            count: number;
        };
    };
    responses: {
        /** @description Operation result. */
        Error: {
            headers: {
                [name: string]: unknown;
            };
            content: {
                "application/json": components["schemas"]["ApiError"];
            };
        };
    };
    parameters: {
        CsrfToken: string;
        IdempotencyKey: components["schemas"]["IdempotencyKey"];
    };
    requestBodies: never;
    headers: never;
    pathItems: never;
}
export type $defs = Record<string, never>;
export interface operations {
    getAlertSummary: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Authorized alert result. Mutation previews use the existing host confirmation workflow. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["AlertSummary"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    listAlertConfigurations: {
        parameters: {
            query: {
                kind: components["schemas"]["AlertKind"];
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Authorized alert result. Mutation previews use the existing host confirmation workflow. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["AlertConfiguration"][];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    getAlertConfiguration: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Authorized alert result. Mutation previews use the existing host confirmation workflow. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["AlertConfiguration"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    previewAlertConfiguration: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CsrfToken"];
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["AlertMutation"];
            };
        };
        responses: {
            /** @description Authorized alert result. Mutation previews use the existing host confirmation workflow. */
            202: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["pending-action-public.schema"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    previewAlertCommand: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CsrfToken"];
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["AlertCommand"];
            };
        };
        responses: {
            /** @description Authorized alert result. Mutation previews use the existing host confirmation workflow. */
            202: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["pending-action-public.schema"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    trialAlertPolicy: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CsrfToken"];
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["AlertPolicySpec"];
            };
        };
        responses: {
            /** @description Authorized alert result. Mutation previews use the existing host confirmation workflow. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["AlertEvaluation"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    listAlertInstances: {
        parameters: {
            query?: {
                query?: string;
                severity?: "critical" | "warning" | "info";
                lifecycle?: string;
                notification_state?: string;
                policy_id?: string;
                resource_id?: string;
                metric?: string;
                from?: string;
                to?: string;
                acknowledged?: boolean;
                limit?: number;
                cursor?: string;
                rule_id?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Authorized alert result. Mutation previews use the existing host confirmation workflow. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["AlertInstancePage"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    listAlertHistory: {
        parameters: {
            query?: {
                query?: string;
                severity?: "critical" | "warning" | "info";
                lifecycle?: string;
                notification_state?: string;
                policy_id?: string;
                resource_id?: string;
                metric?: string;
                from?: string;
                to?: string;
                acknowledged?: boolean;
                limit?: number;
                cursor?: string;
                rule_id?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Authorized alert result. Mutation previews use the existing host confirmation workflow. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["AlertInstancePage"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    listAlertDeliveries: {
        parameters: {
            query?: {
                query?: string;
                notification_state?: string;
                policy_id?: string;
                resource_id?: string;
                from?: string;
                to?: string;
                limit?: number;
                cursor?: string;
                channel_id?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Authorized alert result. Mutation previews use the existing host confirmation workflow. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["AlertDeliveryPage"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    getAlertInstance: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Authorized alert result. Mutation previews use the existing host confirmation workflow. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["AlertInstanceDetail"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    getAlertDelivery: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Authorized alert result. Mutation previews use the existing host confirmation workflow. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["AlertDeliveryDetail"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    handleAlertInstance: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CsrfToken"];
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
            };
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["AlertHandleInput"];
            };
        };
        responses: {
            /** @description Authorized alert result. Mutation previews use the existing host confirmation workflow. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["AlertInstance"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    exportAlertHistory: {
        parameters: {
            query?: {
                query?: string;
                severity?: "critical" | "warning" | "info";
                lifecycle?: string;
                notification_state?: string;
                policy_id?: string;
                resource_id?: string;
                metric?: string;
                from?: string;
                to?: string;
                acknowledged?: boolean;
                limit?: number;
                cursor?: string;
                rule_id?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Authorized alert result. Mutation previews use the existing host confirmation workflow. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["AlertExport"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
}
