export interface paths {
    "/enterprise/hosts/name-availability": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** Check a creation name using host.manage permission without disclosing the occupying resource. */
        get: operations["checkHostNameAvailability"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/hosts": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** List explicitly authorized Hosts. */
        get: operations["listHosts"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/hosts/{id}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: components["parameters"]["ResourceId"];
            };
            cookie?: never;
        };
        /** Get an explicitly authorized Host. */
        get: operations["getHost"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/hosts/actions/preview-create": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** Freeze a validated Host creation plan. */
        post: operations["previewCreateHost"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/hosts/{id}/actions/preview-retry": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: components["parameters"]["ResourceId"];
            };
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** Preview a new SSH installation attempt for a failed unregistered Host. */
        post: operations["previewRetryHost"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/hosts/{id}/actions/preview-update": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: components["parameters"]["ResourceId"];
            };
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** Freeze a Host metadata, labels, or path update. */
        post: operations["previewUpdateHost"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/hosts/{id}/actions/preview-delete": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: components["parameters"]["ResourceId"];
            };
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** Freeze a Host logical deletion plan. */
        post: operations["previewDeleteHost"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/hosts/connection-tests": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** Start a bounded Host connection test. */
        post: operations["createHostConnectionTest"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/host-onboarding-operations/{id}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: components["parameters"]["ResourceId"];
            };
            cookie?: never;
        };
        /** Get a durable Host Connector onboarding operation. */
        get: operations["getHostOnboardingOperation"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/host-removals/connection-defaults": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** Read SSH account and credential references from the current installation. */
        get: operations["getHostRemovalConnectionDefaults"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/host-removals/actions/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** Freeze a fenced Host or Bastion removal plan. */
        post: operations["previewHostRemoval"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/host-removal-operations/{id}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: components["parameters"]["ResourceId"];
            };
            cookie?: never;
        };
        /** Get a durable Host or Bastion removal operation. */
        get: operations["getHostRemovalOperation"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/host-removal-operations/{id}/actions/retry": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: components["parameters"]["ResourceId"];
            };
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** Resume a failed or cleanup-unknown removal from its first unverified step. */
        post: operations["retryHostRemovalOperation"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/host-removal-operations/{id}/actions/regenerate-command": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: components["parameters"]["ResourceId"];
            };
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** Rotate the one-time token and return a replacement command for the same removal operation. */
        post: operations["regenerateHostRemovalCommand"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/host-removal/bootstrap-script": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** Claim a one-time removal token and download the target-specific strict-TLS uninstaller. */
        get: operations["getHostRemovalBootstrapScript"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/host-removal/receipt": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** Submit an operation-scoped local cleanup receipt after the Connector has stopped. */
        post: operations["submitHostRemovalReceipt"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/hosts/{id}/windows-rdp/actions/preview-enable": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: components["parameters"]["ResourceId"];
            };
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** Preview the registry, NLA, service, and firewall changes required to enable RDP. */
        post: operations["previewEnableHostWindowsRDP"];
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
        Host: {
            /** Format: uuid */
            id: string;
            /** Format: uuid */
            readonly enterprise_id: string;
            name: string;
            hostname?: string;
            /** @description Connector 注册前可以为空，注册后由可信 enrollment 回填 */
            address: string;
            port: number;
            /** @enum {string} */
            platform: "linux" | "windows";
            /**
             * @description 连接测试探测的目标架构(uname -m 归一化),Linux 连接测试必须成功识别架构,Collector 安装按此选择签名产物
             * @enum {string}
             */
            readonly architecture?: "amd64" | "arm64";
            role: components["schemas"]["HostRole"];
            control_path: components["schemas"]["HostControlPath"];
            /** Format: uuid */
            bastion_scope_id?: string;
            /** Format: uuid */
            connector_id?: string;
            readonly runtime?: components["schemas"]["HostRuntimeObservation"];
            environment: components["schemas"]["Environment"];
            labels: components["schemas"]["Labels"];
            /** Format: int64 */
            labels_version: number;
            /** Format: int64 */
            resource_version: number;
            /** @enum {string} */
            connection_status: "online" | "offline" | "onboarding" | "degraded" | "unknown";
            pinned_host_key?: string;
            /** Format: date-time */
            last_seen_at?: string;
            /** @enum {string} */
            status: "active" | "disabled" | "draining" | "uninstalling" | "uninstalled" | "removal_failed" | "cleanup_unknown" | "deleted";
            /** Format: int64 */
            readonly removal_generation?: number;
            /** @enum {string} */
            readonly local_cleanup?: "verified" | "pending" | "unknown";
            /** Format: uuid */
            readonly removal_operation_id?: string;
            onboarding: components["schemas"]["OnboardingProjection"];
            /** Format: date-time */
            created_at: string;
            /** Format: date-time */
            updated_at: string;
        };
        HostPage: {
            items: components["schemas"]["Host"][];
            page: components["schemas"]["CursorPage"];
        };
        HostPreviewCreate: {
            name: string;
            hostname?: string;
            /** @description manual 不填写；ssh 必填 */
            address?: string;
            /** @description manual 为 0；ssh 必填 */
            port?: number;
            /** @enum {string} */
            platform: "linux" | "windows";
            /** @constant */
            role: "managed_host";
            control_path: components["schemas"]["HostControlPath"];
            install_method: components["schemas"]["HostInstallMethod"];
            ssh_path: components["schemas"]["HostSSHPath"];
            /** Format: uuid */
            bastion_scope_id?: string;
            /**
             * Format: uuid
             * @description ssh 模式必填
             */
            credential_id?: string;
            /** @description ssh 模式必填 */
            username?: string;
            /**
             * @description manual 由用户选择；ssh 由连接测试探测
             * @enum {string}
             */
            architecture?: "amd64" | "arm64";
            environment: components["schemas"]["Environment"];
            labels: components["schemas"]["UserLabels"];
            /**
             * Format: uuid
             * @description ssh 模式必填
             */
            connection_test_id?: string;
        };
        HostPreviewUpdate: {
            name?: string;
            hostname?: string;
            environment?: components["schemas"]["Environment"];
            labels?: components["schemas"]["UserLabels"];
            /** Format: int64 */
            expected_version: number;
        };
        ConnectionTest: {
            /** Format: uuid */
            id: string;
            /** Format: uuid */
            readonly enterprise_id: string;
            /** @enum {string} */
            target_type: "host" | "kubernetes_cluster";
            /** Format: uuid */
            resource_id?: string;
            /** @enum {string} */
            path: "connector" | "direct" | "in_cluster";
            /** @enum {string} */
            status: "queued" | "running" | "succeeded" | "failed" | "result_unknown" | "expired";
            checks: {
                name: string;
                /** @enum {string} */
                status: "passed" | "failed" | "skipped";
                detail?: string;
            }[];
            latency_ms?: number;
            resolved_ips?: string[];
            host_key_fingerprint?: string;
            remote_version?: string;
            /** @enum {string} */
            platform?: "linux" | "windows";
            /** @enum {string} */
            architecture?: "amd64" | "arm64";
            distribution_version?: string;
            /** @enum {string} */
            service_manager?: "systemd" | "windows_scm";
            privileged?: boolean;
            /** Format: int64 */
            free_disk_bytes?: number;
            error_code?: string;
            /** Format: date-time */
            expires_at: string;
            /** Format: date-time */
            created_at: string;
            /** Format: date-time */
            updated_at: string;
        };
        HostOnboardingOperation: {
            /** Format: uuid */
            id: string;
            /** Format: uuid */
            host_id: string;
            /** Format: uuid */
            connector_id: string;
            /** Format: uuid */
            retry_of?: string;
            /** Format: uuid */
            release_version_id?: string;
            /** Format: uuid */
            connection_test_id?: string;
            /** @enum {string} */
            install_method: "manual" | "ssh";
            /** @enum {string} */
            ssh_path: "none" | "direct_executor" | "bastion_connector";
            /** @enum {string} */
            target_platform: "linux_amd64" | "linux_arm64" | "windows_amd64";
            control_path: components["schemas"]["HostControlPath"];
            /** Format: uuid */
            bastion_scope_id?: string;
            /** @enum {string} */
            stage: "queued" | "probing" | "transferring" | "installing" | "enrolling" | "waiting_online" | "completed";
            /** @enum {string} */
            status: "queued" | "running" | "succeeded" | "failed" | "result_unknown" | "expired" | "cancelled";
            attempts: number;
            max_attempts: number;
            /** Format: date-time */
            connector_online_at?: string;
            error_code?: string;
            events: components["schemas"]["HostOnboardingOperationEvent"][];
            /** Format: date-time */
            completed_at?: string;
            /** Format: date-time */
            expires_at: string;
            /** Format: date-time */
            created_at: string;
            /** Format: date-time */
            updated_at: string;
        };
        HostRemovalConnectionDefaults: {
            username: string;
            /** @enum {string} */
            status: "available" | "credential_unavailable" | "unavailable" | "not_applicable";
            /** Format: uuid */
            credential_id?: string;
            credential_name?: string;
        };
        HostRemovalPreview: {
            target_type: components["schemas"]["HostRemovalTargetType"];
            /** Format: uuid */
            target_id: string;
            /** Format: int64 */
            expected_version: number;
            mode: components["schemas"]["HostRemovalMode"];
            /** Format: uuid */
            connection_test_id?: string;
            /** Format: uuid */
            credential_id?: string;
            /** @description mode=forget 时必须与资源当前名称完全一致 */
            confirmation_name?: string;
        };
        HostRemovalOperation: {
            /** Format: uuid */
            id: string;
            target_type: components["schemas"]["HostRemovalTargetType"];
            /** Format: uuid */
            target_id: string;
            /** Format: uuid */
            connector_id: string;
            mode: components["schemas"]["HostRemovalMode"];
            /** @enum {string} */
            delivery_method: "manual" | "ssh" | "server_only";
            ssh_path: components["schemas"]["HostSSHPath"];
            /** @enum {string} */
            target_platform: "linux_amd64" | "linux_arm64" | "windows_amd64";
            status: components["schemas"]["HostRemovalStatus"];
            stage: components["schemas"]["HostRemovalStage"];
            attempt: number;
            max_attempts: number;
            /** @enum {string} */
            local_cleanup: "pending" | "verified" | "unknown";
            error_code?: string;
            events: components["schemas"]["HostRemovalOperationEvent"][];
            /** Format: date-time */
            expires_at: string;
            /** Format: date-time */
            completed_at?: string;
            /** Format: date-time */
            created_at: string;
            /** Format: date-time */
            updated_at: string;
        };
        HostRemovalInstruction: {
            /** Format: uuid */
            operation_id: string;
            /** @enum {string} */
            platform: "linux" | "windows";
            /** @enum {string} */
            shell: "posix_sh" | "powershell";
            /** @constant */
            privilege: "system";
            command: string;
            /** Format: date-time */
            expires_at: string;
        };
        HostRemovalReceipt: {
            /** Format: uuid */
            operation_id: string;
            /** Format: int64 */
            removal_generation: number;
            /** Format: uuid */
            connector_id: string;
            stage: components["schemas"]["HostRemovalStage"];
            /** @constant */
            local_cleanup: "verified";
            result_hash: string;
            evidence: components["schemas"]["HostRemovalCleanupEvidence"];
            error_code?: string;
        };
        HostRuntimeObservation: {
            /** @enum {string} */
            platform: "linux" | "windows";
            /** @enum {string} */
            openssh_status: "available" | "unavailable" | "unknown";
            /** @enum {string} */
            rdp_status: "enabled" | "disabled" | "unavailable" | "unknown";
            rdp_nla_enabled: boolean;
            rdp_firewall_enabled: boolean;
            rdp_service_running: boolean;
            /** Format: date-time */
            observed_at: string;
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
        ResourceNameAvailability: {
            /** @description Whether the name can currently be used for creation in the authenticated enterprise. This does not reserve the name. */
            available: boolean;
        };
        /** @enum {string} */
        HostControlPath: "direct" | "bastion_relay" | "executor_tunnel";
        /** @enum {string} */
        HostRole: "managed_host" | "bastion";
        /** @enum {string} */
        Environment: "development" | "staging" | "production";
        UserLabelKey: string;
        SystemLabelKey: string;
        LabelValue: string;
        Labels: {
            [key: string]: components["schemas"]["LabelValue"];
        };
        /** @enum {string} */
        HostInstallMethod: "manual" | "ssh";
        /** @enum {string} */
        HostSSHPath: "none" | "direct_executor" | "bastion_connector";
        OnboardingProjection: {
            /** @enum {string} */
            state: "command_available" | "command_consumed" | "command_expired" | "awaiting_approval" | "installing" | "install_failed" | "registered";
            pending_action_ref?: string;
            /** Format: uuid */
            execution_id?: string;
            /** Format: uuid */
            operation_id?: string;
            error_code?: string;
            readonly install_method?: components["schemas"]["HostInstallMethod"];
            readonly ssh_path?: components["schemas"]["HostSSHPath"];
            /** Format: date-time */
            updated_at: string;
        };
        PartialMetadata: {
            partial: boolean;
            reasons: ("authorization_filtered" | "budget_truncated" | "source_timeout" | "source_unavailable")[];
        };
        CursorPage: {
            next_cursor: string | null;
            has_more: boolean;
            partial: components["schemas"]["PartialMetadata"];
        };
        IdempotencyKey: string;
        UserLabels: {
            [key: string]: components["schemas"]["LabelValue"];
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
        ResourcePreviewDelete: {
            /** Format: int64 */
            expected_version: number;
        };
        HostConnectionTestCreate: {
            address: string;
            port: number;
            /** @enum {string} */
            platform: "linux" | "windows";
            /** @enum {string} */
            ssh_path: "direct_executor" | "bastion_connector";
            /**
             * @description Validate the target callback route for SSH installation; omit for an SSH-only connection test such as removal.
             * @enum {string}
             */
            onboarding_control_path?: "direct" | "bastion_relay" | "executor_tunnel";
            /** Format: uuid */
            bastion_scope_id?: string;
            /** Format: uuid */
            credential_id: string;
            username: string;
        };
        HostOnboardingOperationEvent: {
            /** Format: uuid */
            id: string;
            /** @enum {string} */
            stage: "queued" | "probing" | "transferring" | "installing" | "enrolling" | "waiting_online" | "completed";
            /** @enum {string} */
            status: "started" | "succeeded" | "failed" | "retrying";
            error_code?: string;
            /** Format: date-time */
            occurred_at: string;
        };
        /** @enum {string} */
        HostRemovalTargetType: "managed_host" | "bastion_scope";
        /** @enum {string} */
        HostRemovalMode: "uninstall" | "forget";
        /** @enum {string} */
        HostRemovalStatus: "queued" | "running" | "awaiting_manual_execution" | "succeeded" | "failed" | "cleanup_unknown";
        /** @enum {string} */
        HostRemovalStage: "queued" | "draining" | "terminating_sessions" | "awaiting_manual_execution" | "uninstalling_workloads" | "stopping_relay" | "uninstalling_connector" | "verifying_cleanup" | "revoking_identities" | "completed";
        HostRemovalOperationEvent: {
            /** Format: uuid */
            id: string;
            /** Format: int64 */
            sequence: number;
            stage: components["schemas"]["HostRemovalStage"];
            /** @enum {string} */
            status: "started" | "succeeded" | "failed" | "unknown" | "resumed";
            error_code?: string;
            /** Format: date-time */
            occurred_at: string;
        };
        HostRemovalCleanupEvidence: {
            /** @constant */
            schema_version: "argus.host_cleanup_evidence/v1";
            /** Format: uuid */
            operation_id: string;
            /** Format: uuid */
            connector_id: string;
            /** Format: int64 */
            removal_generation: number;
            /** @enum {string} */
            platform: "linux" | "windows";
            collector_service_absent: boolean;
            collector_process_absent: boolean;
            collector_files_absent: boolean;
            connector_service_absent: boolean;
            connector_process_absent: boolean;
            connector_files_absent: boolean;
            connector_user_absent: boolean;
            relay_ports_released: boolean;
            /** @enum {string} */
            rdp_config_status: "not_applicable" | "restored" | "drifted";
            /** Format: date-time */
            observed_at: string;
        };
    };
    responses: {
        /** @description Stable Argus API error. */
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
        Cursor: string;
        Limit: number;
        ResourceId: string;
        IdempotencyKey: components["schemas"]["IdempotencyKey"];
        CsrfToken: string;
    };
    requestBodies: never;
    headers: never;
    pathItems: never;
}
export type $defs = Record<string, never>;
export interface operations {
    checkHostNameAvailability: {
        parameters: {
            query: {
                name: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Current name availability. Deleted records do not occupy a name; the name is not reserved. */
            200: {
                headers: {
                    "Cache-Control"?: string;
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResourceNameAvailability"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    listHosts: {
        parameters: {
            query?: {
                query?: string;
                control_path?: components["schemas"]["HostControlPath"];
                bastion_scope_id?: string;
                labels?: string;
                cursor?: components["parameters"]["Cursor"];
                limit?: components["parameters"]["Limit"];
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Host page. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["HostPage"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    getHost: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: components["parameters"]["ResourceId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Host. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Host"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    previewCreateHost: {
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
                "application/json": components["schemas"]["HostPreviewCreate"];
            };
        };
        responses: {
            /** @description Pending Action. */
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
    previewRetryHost: {
        parameters: {
            query?: never;
            header: {
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
                "X-CSRF-Token": components["parameters"]["CsrfToken"];
            };
            path: {
                id: components["parameters"]["ResourceId"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["HostPreviewCreate"];
            };
        };
        responses: {
            /** @description Installation retry preview. */
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
    previewUpdateHost: {
        parameters: {
            query?: never;
            header: {
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
                "X-CSRF-Token": components["parameters"]["CsrfToken"];
            };
            path: {
                id: components["parameters"]["ResourceId"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["HostPreviewUpdate"];
            };
        };
        responses: {
            /** @description Pending Action. */
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
    previewDeleteHost: {
        parameters: {
            query?: never;
            header: {
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
                "X-CSRF-Token": components["parameters"]["CsrfToken"];
            };
            path: {
                id: components["parameters"]["ResourceId"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ResourcePreviewDelete"];
            };
        };
        responses: {
            /** @description Pending Action. */
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
    createHostConnectionTest: {
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
                "application/json": components["schemas"]["HostConnectionTestCreate"];
            };
        };
        responses: {
            /** @description Connection Test. */
            202: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ConnectionTest"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    getHostOnboardingOperation: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: components["parameters"]["ResourceId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Host onboarding operation. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["HostOnboardingOperation"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    getHostRemovalConnectionDefaults: {
        parameters: {
            query: {
                target_type: components["schemas"]["HostRemovalTargetType"];
                target_id: string;
                expected_version: number;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Non-secret SSH defaults. A fresh connection test is still required. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["HostRemovalConnectionDefaults"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    previewHostRemoval: {
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
                "application/json": components["schemas"]["HostRemovalPreview"];
            };
        };
        responses: {
            /** @description Pending Action. */
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
    getHostRemovalOperation: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: components["parameters"]["ResourceId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Removal operation. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["HostRemovalOperation"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    retryHostRemovalOperation: {
        parameters: {
            query?: never;
            header: {
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
                "X-CSRF-Token": components["parameters"]["CsrfToken"];
            };
            path: {
                id: components["parameters"]["ResourceId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Removal operation queued. */
            202: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["HostRemovalOperation"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    regenerateHostRemovalCommand: {
        parameters: {
            query?: never;
            header: {
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
                "X-CSRF-Token": components["parameters"]["CsrfToken"];
            };
            path: {
                id: components["parameters"]["ResourceId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description One-time removal instruction. */
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["HostRemovalInstruction"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    getHostRemovalBootstrapScript: {
        parameters: {
            query: {
                operation_id: string;
            };
            header: {
                "X-Argus-Removal-Token": string;
            };
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Removal script. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "text/x-shellscript": string;
                    "text/x-powershell": string;
                };
            };
            default: components["responses"]["Error"];
        };
    };
    submitHostRemovalReceipt: {
        parameters: {
            query?: never;
            header: {
                "X-Argus-Removal-Token": string;
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["HostRemovalReceipt"];
            };
        };
        responses: {
            /** @description Reconciled removal operation. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["HostRemovalOperation"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    previewEnableHostWindowsRDP: {
        parameters: {
            query?: never;
            header: {
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
                "X-CSRF-Token": components["parameters"]["CsrfToken"];
            };
            path: {
                id: components["parameters"]["ResourceId"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ResourcePreviewDelete"];
            };
        };
        responses: {
            /** @description Pending Action. */
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
}
