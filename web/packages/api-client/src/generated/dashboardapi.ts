import type { operations } from "./dashboardapi_operations.js";
export type { operations };

export interface paths {
    "/dashboards": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** listDashboards. */
        get: operations["listDashboards"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/dashboards/{id}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        /** getDashboard. */
        get: operations["getDashboard"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/dashboards/{id}/revisions": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        /** listDashboardRevisions. */
        get: operations["listDashboardRevisions"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/dashboard-drafts": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** listDashboardDrafts. */
        get: operations["listDashboardDrafts"];
        put?: never;
        /** createDashboardDraft. */
        post: operations["createDashboardDraft"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/dashboard-drafts/{id}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        /** getDashboardDraft. */
        get: operations["getDashboardDraft"];
        put?: never;
        post?: never;
        /** discardDashboardDraft. */
        delete: operations["discardDashboardDraft"];
        options?: never;
        head?: never;
        /** saveDashboardDraft. */
        patch: operations["saveDashboardDraft"];
        trace?: never;
    };
    "/dashboard-drafts/{id}/preview": {
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
        /** previewDashboardPublication. */
        post: operations["previewDashboardPublication"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/dashboard-drafts/{id}/rebase": {
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
        /** rebaseDashboardDraft. */
        post: operations["rebaseDashboardDraft"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/dashboard-spec/validate": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** validateDashboardSpec. */
        post: operations["validateDashboardSpec"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/dashboard-folders": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** List dashboard folders. */
        get: operations["listDashboardFolders"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/dashboard-folders/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** Preview a folder change without publishing it. */
        post: operations["previewDashboardFolder"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/dashboards/{id}/actions/preview": {
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
        /** Preview dashboard archive or restore. */
        post: operations["previewDashboardLifecycle"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/dashboards/{id}/execute": {
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
        /** executeDashboard. */
        post: operations["executeDashboard"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/dashboard-drafts/{id}/sample": {
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
        /** sampleDashboardDraft. */
        post: operations["sampleDashboardDraft"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/dashboards/catalog/query": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** queryDashboardCatalog. */
        post: operations["queryDashboardCatalog"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/dashboards/{id}/drilldown": {
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
        /** executeDashboardDrilldown. */
        post: operations["executeDashboardDrilldown"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/dashboard-drafts/{id}/drilldowns/generate": {
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
        /** generateDashboardDrilldowns. */
        post: operations["generateDashboardDrilldowns"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/hosts/{id}/dashboard-bindings": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        /** List authorized active dashboard shortcuts. */
        get: operations["listHostDashboardBindings"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/hosts/{id}/dashboard-bindings/preview": {
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
        /** Preview a resource dashboard binding change. */
        post: operations["previewHostDashboardBinding"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/kubernetes/clusters/{id}/dashboard-bindings": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        /** List authorized active dashboard shortcuts. */
        get: operations["listClusterDashboardBindings"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/kubernetes/clusters/{id}/dashboard-bindings/preview": {
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
        /** Preview a resource dashboard binding change. */
        post: operations["previewClusterDashboardBinding"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/dashboards/{id}/bindings": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        /** List shortcuts visible to the current subject on both sides. */
        get: operations["listDashboardBindings"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/dashboard-spec/convert-panel": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** Convert every query in a panel only when the editing sources are losslessly representable. */
        post: operations["convertDashboardPanel"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/conversations/{id}/dashboard-queries": {
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
        /** startDashboardQueryJob. */
        post: operations["startDashboardQueryJob"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/conversations/{id}/dashboard-queries/{job_id}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
                job_id: string;
            };
            cookie?: never;
        };
        /** getDashboardQueryJob. */
        get: operations["getDashboardQueryJob"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/conversations/{id}/dashboard-queries/{job_id}/cancel": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
                job_id: string;
            };
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** cancelDashboardQueryJob. */
        post: operations["cancelDashboardQueryJob"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/conversations/{id}/dashboard-queries/{job_id}/resume": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
                job_id: string;
            };
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** resumeDashboardQueryJob. */
        post: operations["resumeDashboardQueryJob"];
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
        DashboardAnalysisResolveInput: {
            /** Format: uuid */
            dashboard_id: string;
            /** Format: int64 */
            expected_version: number;
            changes: components["schemas"]["DashboardConditionPatch"];
            evidence: {
                /** @description JSON pointer for each changed condition; / for reset_all */
                path: string;
                /** @description Exact excerpt from this Run's user message; a model interpretation with provenance, not proof of semantic correctness */
                quote: string;
            }[];
        };
        DashboardAnalysisQueryInput: {
            /** Format: uuid */
            dashboard_id: string;
            /**
             * Format: uuid
             * @description Required for a root query; obtained from context.resolve in the same Run
             */
            context_ref?: string;
            /** @description Optional strategy for this query only; never inherited */
            panel_ids?: string[];
            drilldown?: components["schemas"]["DashboardQueryDrilldownInput"];
        };
        DashboardAnalysisCandidateInput: {
            /** Format: uuid */
            dashboard_id: string;
            /** Format: uuid */
            context_ref: string;
            name: string;
            panel_id?: string;
        };
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
        DashboardItem: {
            /** Format: uuid */
            id: string;
            name: string;
            description: string;
            /** Format: uuid */
            folder_id?: string;
            /** Format: uuid */
            active_revision_id: string;
            /** Format: int64 */
            version: number;
            lifecycle: string;
            /** Format: date-time */
            updated_at: string;
        };
        /** @example {
         *       "kind": "relative",
         *       "seconds": 3600
         *     } */
        DashboardTimeRange: {
            /**
             * @description relative requires seconds (1..604800); absolute requires from and to with a positive interval no longer than seven days.
             * @enum {string}
             */
            kind: "relative" | "absolute";
            /** Format: int64 */
            seconds?: number;
            /** Format: date-time */
            from?: string;
            /** Format: date-time */
            to?: string;
        };
        DashboardSelection: {
            all: boolean;
            values: string[];
        };
        DashboardSourceBinding: {
            /**
             * @description Use a registered source type from catalog.resources, such as otlp, prometheus, hostmetrics, filelog, skywalking or jaeger; never a resource ID.
             * @example otlp
             */
            source_type: string;
            /**
             * @description Use the capability version returned by catalog.resources; current supported sources use v1.
             * @example v1
             */
            capability_version: string;
        };
        DashboardFilter: {
            field: string;
            operator: string;
            value?: string;
            variable?: string;
            local_parameter?: string;
            values?: string[];
            drilldown_input?: string;
        };
        DashboardParameterBinding: {
            parameter: string;
            variable?: string;
            local_parameter?: string;
            identity_mapping?: boolean;
            value_map?: {
                [key: string]: string;
            };
            drilldown_input?: string;
        };
        DashboardCandidateQuery: {
            /** @enum {string} */
            signal: "metrics" | "logs" | "traces";
            source_binding: components["schemas"]["DashboardSourceBinding"];
            metric?: string;
            field: string;
            filters: components["schemas"]["DashboardFilter"][];
            parameter_bindings?: components["schemas"]["DashboardParameterBinding"][];
        };
        DashboardVariable: {
            id: string;
            name: string;
            label: string;
            multiple: boolean;
            include_all: boolean;
            default: components["schemas"]["DashboardSelection"];
            query: components["schemas"]["DashboardCandidateQuery"];
        };
        DashboardLocalFilter: {
            id: string;
            label: string;
            /** @enum {string} */
            kind: "query" | "text" | "number";
            default: components["schemas"]["DashboardSelection"];
            query?: components["schemas"]["DashboardCandidateQuery"];
            multiple?: boolean;
            required?: boolean;
        };
        DashboardBuilder: {
            /** @enum {string} */
            operation: "value" | "sum" | "avg" | "min" | "max" | "rate" | "topk" | "p95" | "records" | "count" | "count_by" | "count_over_time" | "list" | "detail" | "apm_services" | "apm_instances" | "apm_endpoints" | "apm_red" | "apm_topology" | "trace_graph" | "log_context" | "error_rate";
            metric?: string;
            metric_type?: string;
            filters: components["schemas"]["DashboardFilter"][];
            group_by: string[];
            /** Format: int64 */
            window_seconds?: number;
            /** Format: int64 */
            bucket_seconds?: number;
            /** Format: int64 */
            top_n?: number;
            trace_id?: string;
            /** @description Optional explicit log, APM or trace-list limit. Log context uses its own before/after bound. */
            limit?: number;
            context_before?: number;
            context_after?: number;
            /** @description Fixed error classification for same-counter error_rate. Only literal matchers; shared dynamic scope belongs in filters. Output is a 0..1 ratio; zero traffic produces no data. */
            error_filters?: components["schemas"]["DashboardFilter"][];
        };
        DashboardDSL: {
            expression: string;
            pipeline?: string;
            operation?: string;
            variables?: {
                [key: string]: unknown;
            };
        };
        /** @description Exactly one of builder or dsl, matching the containing Panel.authoring_mode. */
        DashboardDefinition: {
            builder?: components["schemas"]["DashboardBuilder"];
            dsl?: components["schemas"]["DashboardDSL"];
        };
        /** @example {
         *       "kind": "auto",
         *       "target_points": 120,
         *       "min_step_seconds": 10
         *     } */
        DashboardStepPolicy: {
            /**
             * @description Metrics range queries require fixed or auto. fixed requires seconds=1..86400. auto requires target_points=1..10000 AND min_step_seconds>=1. Empty is only for targets that do not use a range step.
             * @enum {string}
             */
            kind: "" | "fixed" | "auto";
            /**
             * Format: int64
             * @description Required for fixed: interval in seconds, from 1 to 86400.
             * @example 30
             */
            seconds?: number;
            /**
             * Format: int64
             * @description Required for auto: target data points, from 1 to 10000.
             * @example 120
             */
            target_points?: number;
            /**
             * Format: int64
             * @description Required for auto: minimum interval in seconds, at least 1.
             * @example 10
             */
            min_step_seconds?: number;
        };
        DashboardTarget: {
            id: string;
            /** @enum {string} */
            language: "promql" | "kql" | "skywalking_graphql";
            /** @description Omit on display targets: they inherit Panel.signal. Required on a detail target. */
            signal?: string;
            /** @description Omit on display targets: they inherit Panel.source_binding. Required on a detail target, including cross-signal drilldowns. */
            source_binding?: components["schemas"]["DashboardSourceBinding"];
            source_definition: components["schemas"]["DashboardDefinition"];
            /**
             * @description PromQL uses instant or range; a metric timeseries uses range. Logs and Trace use an empty string.
             * @enum {string}
             */
            query_mode: "" | "instant" | "range";
            range_step_policy: components["schemas"]["DashboardStepPolicy"];
            parameter_bindings: components["schemas"]["DashboardParameterBinding"][];
        };
        DashboardDrilldownTimeWindow: {
            input: string;
            seconds?: number;
            duration_input?: string;
        };
        DashboardDrilldown: {
            id: string;
            title: string;
            detail_query_ref: string;
            scope_policy: string;
            inputs: {
                [key: string]: string;
            };
            origin_query_ref: string;
            kind?: string;
            time_window?: components["schemas"]["DashboardDrilldownTimeWindow"];
        };
        DashboardRectangle: {
            /** Format: int64 */
            x: number;
            /** Format: int64 */
            y: number;
            /**
             * Format: int64
             * @description Width in grid columns; at least min_w and x+w must be <=12.
             * @example 12
             */
            w: number;
            /**
             * Format: int64
             * @description Height in grid rows; at least min_h, at most 1000.
             * @example 32
             */
            h: number;
            /**
             * Format: int64
             * @description Minimum width, at least 1.
             * @example 3
             */
            min_w: number;
            /**
             * Format: int64
             * @description Minimum height, at least 1.
             * @example 12
             */
            min_h: number;
        };
        DashboardThreshold: {
            /** Format: double */
            value: number;
            /** @enum {string} */
            tone: "info" | "success" | "warning" | "danger";
        };
        /** @description Presentation only. Reductions operate independently on each returned series; mean ignores missing samples and is not time weighted, sum is sample sum not counter increase. Last preserves a missing final sample. Bounds and thresholds use raw query units. Unset bounds are automatic. Draw style, stack and smooth apply only to time series. Does not change query mode, query hash or exported data. */
        DashboardDisplayOptions: {
            /** @enum {string} */
            reducer?: "last" | "min" | "max" | "mean" | "sum";
            /** Format: double */
            min?: number;
            /** Format: double */
            max?: number;
            /** @enum {string} */
            draw_style?: "line" | "area" | "bar";
            stack?: boolean;
            smooth?: boolean;
        };
        DashboardPanel: {
            id: string;
            title: string;
            description: string;
            type: string;
            /** @enum {string} */
            signal: "metrics" | "logs" | "traces";
            /**
             * @description All display and detail targets in a Panel use this single editing source.
             * @enum {string}
             */
            authoring_mode: "builder" | "dsl";
            applicable_resource_types: string[];
            source_binding: components["schemas"]["DashboardSourceBinding"];
            local_filters: components["schemas"]["DashboardLocalFilter"][];
            targets: components["schemas"]["DashboardTarget"][];
            detail_query_targets: components["schemas"]["DashboardTarget"][];
            drilldowns: components["schemas"]["DashboardDrilldown"][];
            layout: components["schemas"]["DashboardRectangle"];
            unit: string;
            /** Format: int64 */
            decimals: number;
            legend: boolean;
            thresholds: components["schemas"]["DashboardThreshold"][];
            display?: components["schemas"]["DashboardDisplayOptions"];
        };
        DashboardGrid: {
            /**
             * Format: int64
             * @description The grid always has 12 columns.
             * @example 12
             */
            columns: number;
            /**
             * Format: int64
             * @description Grid row height, from 1 to 64; the editor default is 8.
             * @example 8
             */
            row_height: number;
        };
        DashboardSpec: {
            /**
             * @description Exact configuration version; do not abbreviate to v1.
             * @example argus.telemetry_dashboard/v1
             * @enum {string}
             */
            schema_version: "argus.telemetry_dashboard/v1";
            default_time_range: components["schemas"]["DashboardTimeRange"];
            /**
             * Format: int64
             * @description 0 disables refresh; otherwise 5..86400 seconds.
             * @example 0
             */
            default_refresh_seconds: number;
            variables: components["schemas"]["DashboardVariable"][];
            panels: components["schemas"]["DashboardPanel"][];
            layout: components["schemas"]["DashboardGrid"];
        };
        DashboardIssue: {
            path: string;
            code: string;
            message: string;
        };
        DashboardValidationReport: {
            valid: boolean;
            issues: components["schemas"]["DashboardIssue"][];
            compiler_version: string;
        };
        DashboardRevision: {
            /** Format: uuid */
            id: string;
            /** Format: uuid */
            dashboard_id: string;
            /** Format: int64 */
            revision_number: number;
            name: string;
            description: string;
            spec: components["schemas"]["DashboardSpec"];
            spec_hash: string;
            validation: components["schemas"]["DashboardValidationReport"];
            sample: {
                [key: string]: unknown;
            };
            /** Format: date-time */
            created_at: string;
        };
        DashboardDetail: {
            dashboard: components["schemas"]["DashboardItem"];
            revision: components["schemas"]["DashboardRevision"];
        };
        DashboardBinding: {
            resource_type: string;
            /** Format: uuid */
            resource_id: string;
        };
        DashboardDraft: {
            /** Format: uuid */
            id: string;
            /** Format: uuid */
            dashboard_id?: string;
            /** Format: uuid */
            base_revision_id?: string;
            /** Format: int64 */
            base_object_version: number;
            /** Format: int64 */
            draft_version: number;
            status: string;
            name: string;
            description: string;
            /** Format: uuid */
            folder_id?: string;
            spec: components["schemas"]["DashboardSpec"];
            proposed_bindings: components["schemas"]["DashboardBinding"][];
            /** Format: date-time */
            updated_at: string;
        };
        DashboardDraftInput: {
            /** Format: uuid */
            dashboard_id?: string;
            name: string;
            description: string;
            /** Format: uuid */
            folder_id?: string;
            spec: components["schemas"]["DashboardSpec"];
            proposed_bindings: components["schemas"]["DashboardBinding"][];
            /** Format: int64 */
            expected_version?: number;
        };
        DashboardVersionInput: {
            /** Format: int64 */
            expected_version: number;
        };
        IdempotencyKey: string;
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
        DashboardRebaseInput: {
            /** Format: int64 */
            expected_version: number;
            /** Format: int64 */
            object_version: number;
            /** Format: uuid */
            revision_id: string;
        };
        DashboardFolder: {
            /** Format: uuid */
            id: string;
            name: string;
            description: string;
            /** Format: int32 */
            sort_order: number;
            /** @enum {string} */
            status: "active" | "archived";
            /** Format: int64 */
            version: number;
        };
        DashboardLifecycleInput: {
            /** Format: uuid */
            id?: string;
            /** @enum {string} */
            operation: "folder.create" | "folder.update" | "folder.archive" | "folder.restore" | "archive" | "restore";
            /** Format: int64 */
            expected_version: number;
            name: string;
            description: string;
            /** Format: int32 */
            sort_order: number;
        };
        DashboardExecutionInput: {
            /** @description Reconcile shared variable candidates without executing panels. panel_ids is ignored in this mode. */
            candidates_only?: boolean;
            /** Format: date-time */
            from?: string;
            /** Format: date-time */
            to?: string;
            resource_ids?: string[];
            panel_ids?: string[];
            variables?: {
                [key: string]: components["schemas"]["DashboardSelection"];
            };
            local_values?: {
                [key: string]: {
                    [key: string]: components["schemas"]["DashboardSelection"];
                };
            };
        };
        DashboardResourceScope: {
            /** Format: uuid */
            id: string;
            /** @enum {string} */
            type: "host" | "kubernetes_cluster";
        };
        DashboardResolvedSource: {
            /** Format: uuid */
            id: string;
            /** Format: int64 */
            revision: number;
            /** Format: uuid */
            resource_id: string;
            /** Format: uuid */
            generation: string;
            type: string;
            capability_version: string;
            /** @description Whether this source belongs to the current installed generation when this execution was frozen; not proof of active collection. */
            current_installation?: boolean;
            /**
             * Format: date-time
             * @description Registration time of this source configuration revision.
             */
            registered_at?: string;
        };
        DashboardTargetExecution: {
            id: string;
            status: string;
            code?: string;
            query_hash: string;
            result_type: string;
            data: unknown;
            meta: {
                [key: string]: unknown;
            };
            /** @description Actual frozen PromQL range step in seconds. */
            step_seconds?: number;
        };
        DashboardPanelExecution: {
            id: string;
            status: string;
            sources: components["schemas"]["DashboardResolvedSource"][];
            targets: components["schemas"]["DashboardTargetExecution"][];
            detail_sources: {
                [key: string]: components["schemas"]["DashboardResolvedSource"][];
            };
        };
        DashboardCandidateState: {
            /** @enum {string} */
            status: "success" | "partial" | "unavailable" | "skipped_budget";
            values: string[];
            complete: boolean;
            selected_exists: {
                [key: string]: boolean;
            };
            reset: boolean;
            sources: components["schemas"]["DashboardResolvedSource"][];
            code?: string;
        };
        DashboardExecution: {
            /** Format: uuid */
            dashboard_id: string;
            /** Format: uuid */
            revision_id: string;
            /** Format: date-time */
            from: string;
            /** Format: date-time */
            to: string;
            resources: components["schemas"]["DashboardResourceScope"][];
            panels: components["schemas"]["DashboardPanelExecution"][];
            partial: boolean;
            execution_hash: string;
            variables: {
                [key: string]: components["schemas"]["DashboardSelection"];
            };
            local_values: {
                [key: string]: {
                    [key: string]: components["schemas"]["DashboardSelection"];
                };
            };
            variable_candidates: {
                [key: string]: components["schemas"]["DashboardCandidateState"];
            };
            local_candidates: {
                [key: string]: {
                    [key: string]: components["schemas"]["DashboardCandidateState"];
                };
            };
            /** Format: uuid */
            execution_id: string;
            context_token?: string;
            /** Format: date-time */
            context_expires_at?: string;
        };
        DashboardDraftSample: {
            /** Format: uuid */
            draft_id: string;
            /** Format: int64 */
            draft_version: number;
            validation: components["schemas"]["DashboardValidationReport"];
            sample: {
                [key: string]: unknown;
            };
        };
        DashboardCatalogFilter: {
            field: string;
            values: string[];
            /** @enum {string} */
            operator?: "=" | "!=" | ">" | ">=" | "<" | "<=";
        };
        DashboardCatalogInput: {
            source_binding: components["schemas"]["DashboardSourceBinding"];
            /** @enum {string} */
            signal: "metrics" | "logs" | "traces";
            /** @enum {string} */
            kind: "metrics" | "values" | "fields";
            metric?: string;
            field?: string;
            search?: string;
            selected_values: string[];
            filters: components["schemas"]["DashboardCatalogFilter"][];
            resource_ids: string[];
            /** Format: date-time */
            from: string;
            /** Format: date-time */
            to: string;
            limit: number;
            cursor?: string;
        };
        DashboardMetricDescriptor: {
            name: string;
            type: string;
            unit: string;
            labels: string[];
        };
        DashboardCatalogField: {
            name: string;
            type: string;
        };
        DashboardCatalogResult: {
            metrics: components["schemas"]["DashboardMetricDescriptor"][];
            fields: components["schemas"]["DashboardCatalogField"][];
            values: string[];
            complete: boolean;
            meta: {
                [key: string]: unknown;
            };
            selected_exists: {
                [key: string]: boolean;
            };
            has_more: boolean;
            next_cursor?: string;
        };
        DashboardDrilldownInput: {
            context_token: string;
            panel_id: string;
            drilldown_id: string;
            values: {
                [key: string]: string;
            };
            expand_authorized_resources: boolean;
        };
        DashboardDrilldownExecution: {
            /** Format: uuid */
            dashboard_id: string;
            /** Format: uuid */
            revision_id: string;
            /** Format: uuid */
            parent_execution_id: string;
            panel_id: string;
            drilldown_id: string;
            /** @enum {string} */
            scope_policy: "inherit" | "authorized_trace";
            /** Format: date-time */
            from: string;
            /** Format: date-time */
            to: string;
            resources: components["schemas"]["DashboardResourceScope"][];
            sources: components["schemas"]["DashboardResolvedSource"][];
            result: components["schemas"]["DashboardTargetExecution"];
            context_token: string;
            /** Format: date-time */
            context_expires_at: string;
            execution_hash: string;
        };
        DashboardGenerateDrilldownsInput: {
            /** Format: int64 */
            expected_version: number;
            panel_id: string;
            signal_sources: {
                [key: string]: components["schemas"]["DashboardSourceBinding"];
            };
        };
        DashboardGeneratedDrilldowns: {
            draft: components["schemas"]["DashboardDraft"];
            added: string[];
            issues: components["schemas"]["DashboardIssue"][];
        };
        DashboardBindingView: {
            /** Format: uuid */
            id: string;
            /** Format: int64 */
            version: number;
            /** @enum {string} */
            resource_type: "host" | "kubernetes_cluster";
            /** Format: uuid */
            resource_id: string;
            resource_name: string;
            /** Format: int64 */
            resource_version: number;
            dashboard: components["schemas"]["DashboardItem"];
        };
        ResourceDashboardBindings: {
            /** @enum {string} */
            resource_type: "host" | "kubernetes_cluster";
            /** Format: uuid */
            resource_id: string;
            resource_name: string;
            /** Format: int64 */
            resource_version: number;
            items: components["schemas"]["DashboardBindingView"][];
        };
        DashboardBindingInput: {
            /** @enum {string} */
            operation: "attach" | "detach";
            /** Format: uuid */
            dashboard_id: string;
            /** Format: int64 */
            expected_dashboard_version: number;
            /** Format: int64 */
            expected_resource_version: number;
            /** Format: uuid */
            binding_id?: string;
            /** Format: int64 */
            expected_binding_version?: number;
        };
        DashboardConvertPanelInput: {
            panel: components["schemas"]["DashboardPanel"];
            /** @enum {string} */
            mode: "builder" | "dsl";
        };
        DashboardConvertedPanel: {
            converted: boolean;
            panel: components["schemas"]["DashboardPanel"];
            issues: components["schemas"]["DashboardIssue"][];
        };
        DashboardQueryDrilldownInput: {
            /** Format: uuid */
            parent_job_id: string;
            panel_id: string;
            drilldown_id: string;
            values: {
                [key: string]: string;
            };
            expand_authorized_resources: boolean;
        };
        DashboardQueryJobInput: {
            /** Format: uuid */
            dashboard_id: string;
            /** Format: uuid */
            run_id?: string;
            parameters: components["schemas"]["DashboardExecutionInput"];
            /** @description Continue a sealed parent result with its published revision and effective parameters. parameters must have no overrides. */
            drilldown?: components["schemas"]["DashboardQueryDrilldownInput"];
        };
        DashboardQueryJobView: {
            /** Format: uuid */
            id: string;
            /** Format: uuid */
            dashboard_id: string;
            /** Format: uuid */
            revision_id: string;
            /** @enum {string} */
            status: "queued" | "fetching" | "materialized" | "delivering" | "complete" | "partial" | "cancelled" | "failed";
            /** Format: int64 */
            version: number;
            error_code?: string;
            manifest?: {
                [key: string]: unknown;
            };
            files: {
                /** Format: uuid */
                id: string;
                panel_id: string;
                target_id: string;
                /** @enum {string} */
                kind: "data" | "manifest";
                /** Format: int64 */
                bytes: number;
                sha256: string;
                source_ref: string;
                path: string;
                /** Format: uuid */
                workspace_id?: string;
                /** Format: uuid */
                workspace_file_id?: string;
            }[];
        };
        DashboardQueryResumeInput: {
            /** Format: int64 */
            expected_version: number;
        };
        DashboardConditionPatch: {
            reset_all?: boolean;
            reset_time?: boolean;
            reset_resources?: boolean;
            time?: components["schemas"]["DashboardTimeRange"];
            resources?: {
                all: boolean;
                ids: string[];
            };
            variables?: {
                [key: string]: components["schemas"]["DashboardSelection"] | null;
            };
            local_values?: {
                [key: string]: {
                    [key: string]: components["schemas"]["DashboardSelection"] | null;
                };
            };
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
