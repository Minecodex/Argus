export interface paths {
    "/applications": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** Search authorized single-environment applications */
        get: operations["listMonitoringApplications"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/applications/{id}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        /** Read an authorized application */
        get: operations["getMonitoringApplication"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/applications/services": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** Search services within authorized applications */
        get: operations["listMonitoringServices"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/monitoring/commands/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** Preview an immutable monitoring workspace change */
        post: operations["previewMonitoringCommand"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/monitoring/collections": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** Search and filter authorized collections before pagination */
        get: operations["listMonitoringCollections"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/monitoring/collections/{id}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        /** Read a collection and its observed state */
        get: operations["getMonitoringCollection"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/monitoring/targets": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** Search authorized targets */
        get: operations["listMonitoringTargets"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/monitoring/integrations": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** List integration eligibility and validation status */
        get: operations["listMonitoringIntegrations"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/monitoring/collectors": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** Search authorized executing Collectors */
        get: operations["listMonitoringCollectorChoices"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/applications/{id}/ingest-endpoints": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        /** List authorized application ingestion endpoints */
        get: operations["listApplicationIngestEndpoints"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/monitoring/ingest-endpoints/{id}/keys": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        /** List ingestion Key metadata without credentials */
        get: operations["listApplicationIngestKeys"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/monitoring/ingest-keys/{id}/reveal": {
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
        /** Reveal a newly issued Key once to its interactive creator */
        post: operations["revealApplicationIngestKey"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/monitoring/builtins": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** List fixed resource-owned collection instances and actual state */
        get: operations["listMonitoringBuiltins"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/monitoring/builtins/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** Preview an own-resource fixed collection change */
        post: operations["previewMonitoringBuiltin"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/monitoring/ingest-endpoints/{id}/connection": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        /** Read authorized SDK endpoint configuration and public TLS trust without a Key */
        get: operations["getMonitoringIngestConnection"];
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
        MonitoringApplication: {
            application_key: string;
            /** Format: date-time */
            created_at: string;
            description: string;
            environment: string;
            /** Format: uuid */
            id: string;
            name: string;
            owner_name: string;
            /** @enum {string} */
            status: "active" | "archived";
            /** Format: date-time */
            updated_at: string;
            /** Format: int64 */
            version: number;
        };
        MonitoringApplicationPage: {
            items: components["schemas"]["MonitoringApplication"][];
            next_cursor?: string;
        };
        MonitoringApplicationService: {
            /** Format: uuid */
            application_id: string;
            /** Format: date-time */
            first_seen_at?: string;
            /** Format: uuid */
            id: string;
            /** Format: date-time */
            last_seen_at?: string;
            runtime: string;
            service_name: string;
            service_namespace: string;
            /** @enum {string} */
            status: "active" | "archived";
            /** Format: int64 */
            version: number;
        };
        MonitoringServicePage: {
            items: components["schemas"]["MonitoringApplicationService"][];
            next_cursor?: string;
        };
        IdempotencyKey: string;
        MonitoringApplicationInput: {
            application_key?: string;
            description?: string;
            environment?: string;
            name: string;
            owner_name?: string;
        };
        MonitoringResourceRef: {
            /** Format: uuid */
            id: string;
            /** @enum {string} */
            kind: "host" | "kubernetes_cluster" | "monitoring_target";
        };
        MonitoringBindingInput: {
            /** Format: uuid */
            application_id: string;
            /** Format: uuid */
            service_id?: string;
            target: components["schemas"]["MonitoringResourceRef"];
        };
        MonitoringCollectionBindingInput: {
            /** Format: uuid */
            application_id: string;
            /** Format: uuid */
            service_id?: string;
        };
        MonitoringComponentTLS: {
            /** @description Public certificate PEM only; private keys and authentication material are rejected. */
            ca_pem?: string;
            server_name?: string;
        };
        /** @description Client-selected workload identity. Namespace identity and owner ancestry are resolved by the server and are never writable input. */
        MonitoringLogWorkloadInput: {
            containers: string[];
            name: string;
            /** Format: uuid */
            namespace_id: string;
            /** @enum {string} */
            type: "pod" | "deployment" | "replicaset" | "daemonset" | "statefulset" | "job" | "cronjob";
            uid: string;
        };
        MonitoringCollectionConfigInput: {
            allow_native_passwords?: boolean;
            auth_source?: string;
            channel?: string;
            component_tls?: components["schemas"]["MonitoringComponentTLS"];
            direct_connection?: boolean;
            /** @enum {string} */
            dns_network?: "udp" | "tcp" | "tcp-tls";
            /** @enum {string} */
            encoding?: "utf-8" | "utf-16le" | "utf-16be" | "ascii";
            exclude?: string[];
            /** @enum {string} */
            format?: "text" | "json";
            group_match?: string;
            include?: string[];
            indices?: string[];
            interval?: string;
            max_log_bytes?: number;
            /** @enum {string} */
            method?: "GET" | "HEAD" | "OPTIONS";
            multiline_start?: string;
            nodes?: string[];
            ping_count?: number;
            /** @enum {string} */
            record_type?: "A" | "AAAA" | "CNAME" | "MX" | "TXT" | "NS" | "SOA" | "SRV";
            replica_set?: string;
            /** @enum {string} */
            sasl_mechanism?: "PLAIN" | "SCRAM-SHA-256" | "SCRAM-SHA-512";
            /** @enum {string} */
            scope?: "business" | "system" | "component" | "resource";
            /** @enum {string} */
            start_at?: "beginning" | "end";
            timeout?: string;
            tls?: {
                enabled: boolean;
                server_name?: string;
            };
            topic_match?: string;
            units?: string[];
            workload?: components["schemas"]["MonitoringLogWorkloadInput"];
        };
        MonitoringCollectionResourceRef: {
            /** Format: uuid */
            id: string;
            /** @enum {string} */
            kind: "host" | "kubernetes_cluster" | "monitoring_target" | "application";
        };
        MonitoringTargetIdentity: {
            database?: string;
            endpoint: string;
            hostnames?: string[];
            members?: string[];
        };
        MonitoringCollectionInput: {
            /** Format: uuid */
            application_id?: string;
            bindings?: components["schemas"]["MonitoringCollectionBindingInput"][];
            /** Format: uuid */
            collector_id?: string;
            config?: components["schemas"]["MonitoringCollectionConfigInput"];
            /** @enum {string} */
            credential_update?: "keep" | "replace" | "remove";
            enabled?: boolean;
            integration?: string;
            name?: string;
            /** Format: uuid */
            service_id?: string;
            target?: components["schemas"]["MonitoringCollectionResourceRef"];
            target_identity?: components["schemas"]["MonitoringTargetIdentity"];
            target_name?: string;
        };
        MonitoringCredentialInput: {
            password?: string;
            username?: string;
        };
        MonitoringIngestEndpointInput: {
            /** Format: uuid */
            application_id?: string;
            /** Format: uuid */
            collector_id?: string;
            enabled?: boolean;
            name?: string;
            /** @enum {string} */
            protocol?: "otlp" | "jaeger" | "skywalking";
            service_allowlist?: string[];
            signals?: ("metrics" | "logs" | "traces")[];
        };
        MonitoringIngestKeyInput: {
            /** Format: uuid */
            endpoint_id?: string;
            /** Format: date-time */
            expires_at?: string;
        };
        MonitoringApplicationServiceInput: {
            /** Format: uuid */
            application_id: string;
            service_name: string;
            service_namespace?: string;
        };
        MonitoringMutation: {
            application?: components["schemas"]["MonitoringApplicationInput"];
            binding?: components["schemas"]["MonitoringBindingInput"];
            collection?: components["schemas"]["MonitoringCollectionInput"];
            credentials?: components["schemas"]["MonitoringCredentialInput"];
            endpoint?: components["schemas"]["MonitoringIngestEndpointInput"];
            /** Format: int64 */
            expected_version?: number;
            /** Format: uuid */
            id?: string;
            ingest_key?: components["schemas"]["MonitoringIngestKeyInput"];
            /** @enum {string} */
            kind: "application" | "binding" | "collection" | "endpoint" | "ingest_key" | "service";
            /** @enum {string} */
            operation: "create" | "update" | "archive" | "delete" | "set_enabled" | "revoke";
            service?: components["schemas"]["MonitoringApplicationServiceInput"];
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
        MonitoringLogWorkload: {
            readonly ancestors?: string[];
            containers: string[];
            name: string;
            readonly namespace?: string;
            /** Format: uuid */
            namespace_id: string;
            readonly namespace_uid?: string;
            /** @enum {string} */
            type: "pod" | "deployment" | "replicaset" | "daemonset" | "statefulset" | "job" | "cronjob";
            uid: string;
        };
        MonitoringCollectionConfig: {
            allow_native_passwords?: boolean;
            auth_source?: string;
            channel?: string;
            component_tls?: components["schemas"]["MonitoringComponentTLS"];
            direct_connection?: boolean;
            /** @enum {string} */
            dns_network?: "udp" | "tcp" | "tcp-tls";
            /** @enum {string} */
            encoding?: "utf-8" | "utf-16le" | "utf-16be" | "ascii";
            exclude?: string[];
            /** @enum {string} */
            format?: "text" | "json";
            group_match?: string;
            include?: string[];
            indices?: string[];
            interval?: string;
            max_log_bytes?: number;
            /** @enum {string} */
            method?: "GET" | "HEAD" | "OPTIONS";
            multiline_start?: string;
            nodes?: string[];
            ping_count?: number;
            /** @enum {string} */
            record_type?: "A" | "AAAA" | "CNAME" | "MX" | "TXT" | "NS" | "SOA" | "SRV";
            replica_set?: string;
            /** @enum {string} */
            sasl_mechanism?: "PLAIN" | "SCRAM-SHA-256" | "SCRAM-SHA-512";
            /** @enum {string} */
            scope?: "business" | "system" | "component" | "resource";
            /** @enum {string} */
            start_at?: "beginning" | "end";
            timeout?: string;
            tls?: {
                enabled: boolean;
                server_name?: string;
            };
            topic_match?: string;
            units?: string[];
            workload?: components["schemas"]["MonitoringLogWorkload"];
        };
        MonitoringCollection: {
            /** Format: uuid */
            application_id?: string;
            /** @enum {string} */
            apply_status: "pending" | "applying" | "effective" | "failed" | "unknown";
            /** Format: int64 */
            assignment_epoch: number;
            /** Format: uuid */
            collector_id: string;
            config: components["schemas"]["MonitoringCollectionConfig"];
            credential_configured: boolean;
            desired_enabled: boolean;
            /** Format: int64 */
            desired_revision: number;
            /**
             * Format: int64
             * @description Epoch of the verified execution assignment; zero before first successful configuration.
             */
            readonly effective_assignment_epoch?: number;
            /**
             * Format: uuid
             * @description Collector of the last verified effective assignment; may differ from the desired collector_id while moving or after a failed candidate.
             */
            readonly effective_collector_id?: string;
            /** Format: int64 */
            effective_revision: number;
            error_code: string;
            /** @enum {string} */
            freshness: "no_data" | "fresh" | "stale";
            /** @enum {string} */
            health: "unknown" | "healthy" | "degraded" | "failed" | "stopped";
            /** Format: uuid */
            id: string;
            integration: string;
            /** Format: date-time */
            last_attempt_at?: string;
            /** Format: date-time */
            last_sample_at?: string;
            name: string;
            /** Format: date-time */
            observed_at?: string;
            /** Format: uuid */
            service_id?: string;
            /** @enum {string} */
            status: "active" | "archived";
            target: components["schemas"]["MonitoringCollectionResourceRef"];
            /** Format: int64 */
            version: number;
        };
        MonitoringCollectionPage: {
            items: components["schemas"]["MonitoringCollection"][];
            next_cursor?: string;
        };
        MonitoringTarget: {
            engine: string;
            /** Format: uuid */
            id: string;
            identity: components["schemas"]["MonitoringTargetIdentity"];
            /** @enum {string} */
            kind: "database" | "cache" | "messaging" | "web" | "runtime" | "storage" | "probe";
            name: string;
            /** @enum {string} */
            status: "active" | "archived";
            /** Format: int64 */
            version: number;
        };
        MonitoringTargetPage: {
            items: components["schemas"]["MonitoringTarget"][];
            next_cursor?: string;
        };
        MonitoringIntegrationForm: {
            /** @enum {string} */
            authentication: "none" | "optional" | "required";
            database: boolean;
            database_required: boolean;
            /** @enum {string} */
            endpoint_kind: "host_port" | "http_url" | "host" | "local";
            hostnames: boolean;
            members: boolean;
            tls: boolean;
            tls_server_name: boolean;
        };
        MonitoringIntegration: {
            adapter_ready: boolean;
            component?: string;
            config_version: string;
            family: string;
            form?: components["schemas"]["MonitoringIntegrationForm"];
            key: string;
            module?: string;
            name: string;
            platforms: string[];
            reason?: string;
            /** @enum {string} */
            selection: "candidate" | "deferred_no_eligible_embedded_component" | "deferred_upstream_defect";
            signals: ("metrics" | "logs" | "traces")[];
            /** @enum {string} */
            validation: "not_run" | "not_applicable" | "passed" | "blocked";
        };
        MonitoringCollectorChoice: {
            collector_version: string;
            components: string[];
            distribution_version: string;
            /** Format: uuid */
            id: string;
            name: string;
            platform: string;
            resource: components["schemas"]["MonitoringResourceRef"];
            status: string;
            /** @enum {string} */
            support_status: "supported" | "validation_pending" | "retired";
            /** Format: int64 */
            version: number;
        };
        MonitoringCollectorChoicePage: {
            items: components["schemas"]["MonitoringCollectorChoice"][];
            next_cursor?: string;
        };
        MonitoringIngestEndpoint: {
            /** Format: uuid */
            application_id: string;
            /** Format: uuid */
            collector_id: string;
            enabled: boolean;
            /** Format: uuid */
            id: string;
            last_error_code: string;
            /** Format: date-time */
            last_received_at?: string;
            /** Format: date-time */
            last_rejected_at?: string;
            name: string;
            /** @enum {string} */
            protocol: "otlp" | "jaeger" | "skywalking";
            service_allowlist: string[];
            signals: ("metrics" | "logs" | "traces")[];
            /** Format: int64 */
            version: number;
        };
        MonitoringIngestEndpointPage: {
            items: components["schemas"]["MonitoringIngestEndpoint"][];
            next_cursor?: string;
        };
        MonitoringIngestKey: {
            /** Format: date-time */
            created_at: string;
            /** Format: uuid */
            endpoint_id: string;
            /** Format: date-time */
            expires_at?: string;
            /** Format: uuid */
            id: string;
            /** Format: date-time */
            last_used_at?: string;
            prefix: string;
            revealed: boolean;
            /** Format: date-time */
            revoked_at?: string;
            /** Format: int64 */
            version: number;
        };
        MonitoringIngestKeyPage: {
            items: components["schemas"]["MonitoringIngestKey"][];
            next_cursor?: string;
        };
        MonitoringIngestKeyReveal: {
            /** @description One-time interactive portal response. Never cache, persist, log, or pass to model tools. */
            key: string;
        };
        MonitoringBuiltinDefinition: {
            component: string;
            component_tls_supported: boolean;
            /** @enum {string} */
            key: "host_metrics" | "host_process_metrics" | "host_systemd_metrics" | "host_windows_service_metrics" | "collector_metrics" | "kubernetes_cluster_metrics" | "kubernetes_node_metrics" | "kubernetes_apiserver_metrics" | "kubernetes_coredns_metrics" | "kubernetes_scheduler_metrics" | "kubernetes_controller_metrics" | "kubernetes_etcd_metrics";
            legacy_profile: string;
            name: string;
            placement: string;
            platforms: string[];
            resource_types: ("host" | "kubernetes_cluster")[];
            source_type: string;
        };
        MonitoringBuiltin: {
            available: boolean;
            collection?: components["schemas"]["MonitoringCollection"];
            /** Format: uuid */
            collector_id?: string;
            definition: components["schemas"]["MonitoringBuiltinDefinition"];
            /** Format: uuid */
            id: string;
            reason?: string;
        };
        /** @description Certificate trust applies only to definitions advertising component_tls_supported. */
        MonitoringBuiltinConfig: {
            component_tls?: components["schemas"]["MonitoringComponentTLS"];
            interval?: string;
        };
        MonitoringBuiltinInput: {
            config?: components["schemas"]["MonitoringBuiltinConfig"];
            enabled: boolean;
            /** Format: int64 */
            expected_version: number;
            /** @enum {string} */
            key: "host_metrics" | "host_process_metrics" | "host_systemd_metrics" | "host_windows_service_metrics" | "collector_metrics" | "kubernetes_cluster_metrics" | "kubernetes_node_metrics" | "kubernetes_apiserver_metrics" | "kubernetes_coredns_metrics" | "kubernetes_scheduler_metrics" | "kubernetes_controller_metrics" | "kubernetes_etcd_metrics";
            target: {
                /** Format: uuid */
                id: string;
                /** @enum {string} */
                kind: "host" | "kubernetes_cluster";
            };
        };
        MonitoringIngestListener: {
            endpoint: string;
            local_endpoint: string;
            path: string;
            port: number;
            /** @enum {string} */
            transport: "grpc" | "http" | "thrift_http";
        };
        MonitoringIngestTLS: {
            ca_bundle_pem: string;
            ca_bundle_sha256: string;
            ca_fingerprints: string[];
            /** @enum {string} */
            minimum_version: "1.3";
            required: boolean;
            server_name: string;
            /** Format: int64 */
            trust_bundle_epoch: number;
        };
        MonitoringIngestConnection: {
            /** Format: uuid */
            application_id: string;
            /** @enum {string} */
            application_status: "active" | "archived";
            /** @enum {string} */
            authentication_header: "authorization";
            /** @enum {string} */
            authentication_scheme: "Bearer";
            /** Format: uuid */
            collector_id: string;
            collector_status: string;
            /** @enum {string} */
            configuration_status: "pending" | "effective" | "unknown" | "unavailable";
            desired_enabled: boolean;
            /** Format: uuid */
            endpoint_id: string;
            environment: string;
            last_error_code: string;
            /** Format: date-time */
            last_received_at?: string;
            /** Format: date-time */
            last_rejected_at?: string;
            listeners: components["schemas"]["MonitoringIngestListener"][];
            name: string;
            /** @enum {string} */
            network_prerequisite: "resolve_collector_dns_or_use_local_address_with_tls_server_name";
            /** @enum {string} */
            observation_state: "waiting" | "received" | "rejected";
            /** @enum {string} */
            protocol: "otlp" | "jaeger";
            service_name_required: boolean;
            signals: ("metrics" | "logs" | "traces")[];
            tls: components["schemas"]["MonitoringIngestTLS"];
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
        IdempotencyKey: components["schemas"]["IdempotencyKey"];
        CsrfToken: string;
    };
    requestBodies: never;
    headers: never;
    pathItems: never;
}
export type $defs = Record<string, never>;
export interface operations {
    listMonitoringApplications: {
        parameters: {
            query?: {
                search?: string;
                cursor?: string;
                limit?: number;
                status?: "active" | "archived";
                /** @description Exact application environment; omitted means all authorized environments. Combined with search before pagination. */
                environment?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Authorized monitoring workspace result */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["MonitoringApplicationPage"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    getMonitoringApplication: {
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
            /** @description Authorized monitoring workspace result */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["MonitoringApplication"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    listMonitoringServices: {
        parameters: {
            query?: {
                search?: string;
                cursor?: string;
                limit?: number;
                application_ids?: string[];
                /** @description explicit preserves an empty application_ids set; all_authorized enumerates current grants */
                application_mode?: "all_authorized" | "explicit";
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Authorized monitoring workspace result */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["MonitoringServicePage"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    previewMonitoringCommand: {
        parameters: {
            query?: never;
            header: {
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
                "X-CSRF-Token": components["parameters"]["CsrfToken"];
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["MonitoringMutation"];
            };
        };
        responses: {
            /** @description Existing PendingAction confirmation workflow */
            201: {
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
    listMonitoringCollections: {
        parameters: {
            query?: {
                /** @description Omit for both states; false explicitly selects disabled collections. */
                desired_enabled?: boolean;
                apply_status?: "pending" | "applying" | "effective" | "failed" | "unknown";
                health?: "unknown" | "healthy" | "degraded" | "failed" | "stopped";
                /** @description Uses the same three-interval / two-minute sample freshness threshold as collection rows. */
                freshness?: "no_data" | "fresh" | "stale";
                search?: string;
                cursor?: string;
                limit?: number;
                integration?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Authorized monitoring workspace result */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["MonitoringCollectionPage"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    getMonitoringCollection: {
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
            /** @description Authorized monitoring workspace result */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["MonitoringCollection"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    listMonitoringTargets: {
        parameters: {
            query?: {
                search?: string;
                cursor?: string;
                limit?: number;
                integration?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Authorized monitoring workspace result */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["MonitoringTargetPage"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    listMonitoringIntegrations: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Authorized monitoring workspace result */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["MonitoringIntegration"][];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    listMonitoringCollectorChoices: {
        parameters: {
            query?: {
                search?: string;
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Authorized monitoring workspace result */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["MonitoringCollectorChoicePage"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    listApplicationIngestEndpoints: {
        parameters: {
            query?: {
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Authorized application ingestion result */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["MonitoringIngestEndpointPage"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    listApplicationIngestKeys: {
        parameters: {
            query?: {
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Authorized application ingestion result */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["MonitoringIngestKeyPage"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    revealApplicationIngestKey: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CsrfToken"];
            };
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Authorized application ingestion result */
            200: {
                headers: {
                    "Cache-Control": "no-store";
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["MonitoringIngestKeyReveal"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    listMonitoringBuiltins: {
        parameters: {
            query: {
                target_type: "host" | "kubernetes_cluster";
                target_id: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Fixed collection definitions and observed state */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["MonitoringBuiltin"][];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    previewMonitoringBuiltin: {
        parameters: {
            query?: never;
            header: {
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
                "X-CSRF-Token": components["parameters"]["CsrfToken"];
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["MonitoringBuiltinInput"];
            };
        };
        responses: {
            /** @description Existing PendingAction confirmation workflow */
            201: {
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
    getMonitoringIngestConnection: {
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
            /** @description Application-scoped SDK connection metadata */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["MonitoringIngestConnection"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
}
