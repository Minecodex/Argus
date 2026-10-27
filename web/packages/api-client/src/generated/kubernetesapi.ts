export interface paths {
    "/enterprise/kubernetes-clusters/{id}/helm/releases": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        /** List safe Helm release metadata in an explicitly selected authorized Namespace */
        get: operations["listKubernetesHelmReleases"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/kubernetes-clusters/{id}/helm/releases/{releaseName}/history": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
                releaseName: string;
            };
            cookie?: never;
        };
        /** List safe revision metadata for a Helm release */
        get: operations["listKubernetesHelmHistory"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/kubernetes-clusters/{id}/helm/actions/preview": {
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
        /** Render, authorize and dry-run Helm objects before preparing an immutable approval */
        post: operations["previewKubernetesHelm"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/kubernetes-clusters/{id}/stream-sessions": {
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
        /** Create a UID-bound terminal or TCP forwarding session */
        post: operations["createKubernetesStreamSession"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/kubernetes-clusters/{id}/stream-sessions/{sessionId}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
                sessionId: string;
            };
            cookie?: never;
        };
        /** Read current Kubernetes session state */
        get: operations["getKubernetesStreamSession"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/kubernetes-clusters/{id}/stream-sessions/{sessionId}/close": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
                sessionId: string;
            };
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** Close a Kubernetes terminal or forwarding session */
        post: operations["closeKubernetesStreamSession"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/kubernetes-clusters/{id}/stream-sessions/{sessionId}/ticket": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
                sessionId: string;
            };
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** Replace the pending one-use Kubernetes stream ticket */
        post: operations["issueKubernetesStreamTicket"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/kubernetes-clusters/{id}/log-sessions/{sessionId}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
                sessionId: string;
            };
            cookie?: never;
        };
        /** Check the current authorization of the owning Pod log session */
        get: operations["getKubernetesLogSession"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/kubernetes-clusters/{id}/log-sessions": {
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
        /** Create a UID-bound Pod log session and a one-use attach ticket */
        post: operations["createKubernetesLogSession"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/kubernetes-clusters/{id}/log-sessions/{sessionId}/attach": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
                sessionId: string;
            };
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** Consume the short-lived ticket in the owning HTTP session */
        post: operations["attachKubernetesLogSession"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/kubernetes-clusters/{id}/log-sessions/{sessionId}/read": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
                sessionId: string;
            };
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** Read a bounded authorized log increment with a signed continuity cursor */
        post: operations["readKubernetesLogSession"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/kubernetes-clusters/{id}/log-sessions/{sessionId}/close": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
                sessionId: string;
            };
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** Close the owning Pod log session */
        post: operations["closeKubernetesLogSession"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/kubernetes-clusters/{id}/operations/{operationId}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
                operationId: string;
            };
            cookie?: never;
        };
        /** Read an authorized operation without its private execution plan */
        get: operations["getKubernetesOperation"];
        put?: never;
        /** Stop further batches of an authorized node drain; in-flight requests may finish and the node stays cordoned */
        post: operations["cancelKubernetesDrain"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/kubernetes-clusters/{id}/operations": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        /** List operations in the current authorized resource scope */
        get: operations["listKubernetesOperations"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/kubernetes-clusters/{id}/inventory/actions/preview-mutation": {
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
        /** Dry-run an authorized object change and prepare a confirmation */
        post: operations["previewKubernetesMutation"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/kubernetes-clusters/{id}/inventory/watch": {
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
        /** Watch the current page with frozen UID scope and bounded batches */
        post: operations["watchKubernetesInventory"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/kubernetes-clusters/{id}/inventory/types": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        /** List object types available in the current access scope */
        get: operations["listKubernetesInventoryTypes"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/kubernetes-clusters/{id}/inventory/scope": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        /** Read current Cluster and Namespace access */
        get: operations["getKubernetesInventoryScope"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/kubernetes-clusters/{id}/inventory/object": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        /** Read a projected object with frozen UID */
        get: operations["getKubernetesInventoryObject"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/kubernetes-clusters/{id}/inventory": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        /** List authorized Kubernetes objects with server pagination */
        get: operations["listKubernetesInventory"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/kubernetes-clusters": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** List explicitly authorized Kubernetes clusters. */
        get: operations["listKubernetesClusters"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/kubernetes-clusters/{id}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: components["parameters"]["ResourceId"];
            };
            cookie?: never;
        };
        /** Get an explicitly authorized Kubernetes cluster. */
        get: operations["getKubernetesCluster"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/kubernetes-clusters/actions/preview-create": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** Freeze a Kubernetes cluster creation plan. */
        post: operations["previewCreateKubernetesCluster"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/kubernetes-clusters/{id}/actions/preview-update": {
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
        /** Freeze a Kubernetes cluster update. */
        post: operations["previewUpdateKubernetesCluster"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/kubernetes-clusters/{id}/actions/preview-delete": {
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
        /** Freeze a Kubernetes cluster logical deletion. */
        post: operations["previewDeleteKubernetesCluster"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/kubernetes-clusters/connection-tests": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** Start a bounded Kubernetes connection test. */
        post: operations["createKubernetesConnectionTest"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/kubernetes-clusters/{id}/resources": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: components["parameters"]["ResourceId"];
            };
            cookie?: never;
        };
        /** List bounded Kubernetes resources after Kubernetes cluster authorization checks. */
        get: operations["listKubernetesResources"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/kubernetes-clusters/{id}/pod-logs": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: components["parameters"]["ResourceId"];
            };
            cookie?: never;
        };
        /** Read at most one MiB of Pod logs. */
        get: operations["getKubernetesPodLogs"];
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
        KubernetesStreamInput: {
            /** Format: uuid */
            namespace_id: string;
            /** @enum {string} */
            type: "pod" | "service";
            name: string;
            uid: string;
            container?: string;
            /** @enum {string} */
            mode: "exec" | "port_forward";
            /** @enum {string} */
            shell?: "sh" | "bash" | "powershell" | "cmd";
            port?: number;
            reason: string;
        };
        KubernetesStreamSession: {
            /** Format: uuid */
            id: string;
            /** Format: uuid */
            cluster_id: string;
            /** Format: uuid */
            namespace_id: string;
            namespace: string;
            namespace_uid: string;
            pod: string;
            pod_uid: string;
            container: string;
            restart_count: number;
            /** @enum {string} */
            status: "pending" | "connecting" | "active" | "closing" | "closed" | "invalidated" | "connection_lost";
            /** Format: int64 */
            version: number;
            /** Format: date-time */
            expires_at: string;
            /** @enum {string} */
            mode: "exec" | "port_forward";
            /** @enum {string} */
            shell?: "sh" | "bash" | "powershell" | "cmd";
            port?: number;
            service?: string;
            reason?: string;
            /** Format: int64 */
            bytes_in: number;
            /** Format: int64 */
            bytes_out: number;
        };
        KubernetesStreamTicket: {
            session: components["schemas"]["KubernetesStreamSession"];
            ticket: string;
            /** Format: date-time */
            ticket_expires_at: string;
            /** @enum {string} */
            protocol_version: "argus.kubernetes_stream/v1";
            /** Format: uri */
            websocket_url: string;
        };
        KubernetesCluster: {
            /** Format: uuid */
            id: string;
            /** Format: uuid */
            readonly enterprise_id: string;
            name: string;
            /** Format: uri */
            api_server: string;
            connection_mode: components["schemas"]["KubernetesConnectionMode"];
            /** Format: uuid */
            bastion_scope_id?: string;
            /** Format: uuid */
            connector_id?: string;
            /** Format: uuid */
            credential_id?: string;
            default_namespace?: string;
            environment: components["schemas"]["Environment"];
            labels: components["schemas"]["Labels"];
            /** Format: int64 */
            labels_version: number;
            /** Format: int64 */
            resource_version: number;
            /** @enum {string} */
            connection_status: "pending_connector" | "connected" | "degraded" | "disconnected";
            kubernetes_version?: string;
            /** Format: date-time */
            readonly statistics_observed_at?: string;
            node_count?: number;
            ready_node_count?: number;
            /** @enum {string} */
            status: "active" | "disabled" | "deleted";
            /** Format: date-time */
            created_at: string;
            /** Format: date-time */
            updated_at: string;
        };
        KubernetesClusterPage: {
            items: components["schemas"]["KubernetesCluster"][];
            page: components["schemas"]["CursorPage"];
        };
        KubernetesPreviewCreate: {
            name: string;
            /** Format: uri */
            api_server: string;
            connection_mode: components["schemas"]["KubernetesConnectionMode"];
            /** Format: uuid */
            bastion_scope_id?: string;
            /** Format: uuid */
            credential_id?: string;
            default_namespace?: string;
            environment: components["schemas"]["Environment"];
            labels: components["schemas"]["UserLabels"];
            /** Format: uuid */
            connection_test_id?: string;
            /** @description 仅 in_cluster：目标集群中已存在的 Connector 镜像拉取 Secret */
            connector_image_pull_secrets?: string[];
        };
        KubernetesPreviewUpdate: {
            name?: string;
            /** Format: uri */
            api_server?: string;
            connection_mode?: components["schemas"]["KubernetesConnectionMode"];
            /** Format: uuid */
            bastion_scope_id?: string;
            /** Format: uuid */
            credential_id?: string;
            default_namespace?: string;
            environment?: components["schemas"]["Environment"];
            labels?: components["schemas"]["UserLabels"];
            /** Format: uuid */
            connection_test_id?: string;
            /** Format: int64 */
            expected_version: number;
        };
        KubernetesResource: {
            /** Format: uuid */
            cluster_id: string;
            /** @enum {string} */
            resource_type: "namespace" | "node" | "pod" | "deployment" | "statefulset" | "daemonset" | "service";
            namespace?: string;
            name: string;
            labels: components["schemas"]["Labels"];
            summary: {
                [key: string]: unknown;
            };
        };
        KubernetesResourcePage: {
            items: components["schemas"]["KubernetesResource"][];
            page: components["schemas"]["CursorPage"];
        };
        PodLogs: {
            /** Format: uuid */
            cluster_id: string;
            namespace: string;
            pod: string;
            container?: string;
            content: string;
            truncated: boolean;
            bytes: number;
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
        KubernetesHelmRelease: {
            name: string;
            namespace: string;
            revision: number;
            status: string;
            chart: string;
            chart_version: string;
            /** Format: date-time */
            updated_at: string;
        };
        KubernetesHelmReleases: {
            items: components["schemas"]["KubernetesHelmRelease"][];
        };
        IdempotencyKey: string;
        KubernetesHelmChartSource: {
            /** @enum {string} */
            kind: "repository" | "oci";
            /** @description Repository base URL or oci://registry/path/chart; no user info, query or fragment */
            url: string;
            chart?: string;
            /** @description Exact semantic Chart version */
            version: string;
            /** @description Optional expected archive SHA256 for repository or manifest SHA256 for OCI */
            digest?: string;
            /** @description Explicitly permit HTTP for this source; TLS redirects/downgrades remain rejected */
            plain_http?: boolean;
            /** @description Explicit trusted cross-origin OCI token service URL, without query or fragment */
            auth_url?: string;
        };
        KubernetesHelmSourceCredentials: {
            username?: string;
            password?: string;
            token?: string;
            /** @description Additional CA certificates only; no private keys or disabled TLS validation */
            ca_pem?: string;
        };
        KubernetesHelmPreviewInput: {
            /** Format: uuid */
            namespace_id: string;
            /** @enum {string} */
            operation: "install" | "upgrade" | "rollback" | "uninstall";
            name: string;
            expected_revision: number;
            rollback_revision?: number;
            timeout_seconds: number;
            reason: string;
            /**
             * Format: byte
             * @description Protected packaged .tgz chart; never a URL or shell command
             */
            chart_archive?: string;
            source?: components["schemas"]["KubernetesHelmChartSource"];
            source_credentials?: components["schemas"]["KubernetesHelmSourceCredentials"];
            /** @description Protected YAML/JSON values; not included in public previews or model context */
            values?: string;
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
        KubernetesLogSession: {
            /** Format: uuid */
            id: string;
            /** Format: uuid */
            cluster_id: string;
            /** Format: uuid */
            namespace_id: string;
            namespace: string;
            namespace_uid: string;
            pod: string;
            pod_uid: string;
            container: string;
            restart_count: number;
            previous: boolean;
            /** @enum {string} */
            status: "pending" | "active" | "closed" | "invalidated";
            /** Format: int64 */
            version: number;
            /** Format: date-time */
            expires_at: string;
        };
        KubernetesLogSessionInput: {
            /** Format: uuid */
            namespace_id: string;
            pod: string;
            pod_uid: string;
            container: string;
            previous: boolean;
            tail_lines: number;
            since_seconds: number;
        };
        KubernetesLogSessionTicket: {
            session: components["schemas"]["KubernetesLogSession"];
            ticket: string;
            /** Format: date-time */
            ticket_expires_at: string;
        };
        KubernetesLogSessionAttachInput: {
            ticket: string;
        };
        KubernetesLogSessionReadInput: {
            cursor?: string;
        };
        KubernetesLogSessionBatch: {
            content: string;
            next_cursor: string;
            /** Format: date-time */
            observed_at: string;
            truncated: boolean;
            pending_line: boolean;
            redacted: boolean;
            complete: boolean;
        };
        KubernetesDrainVolume: {
            name: string;
            kind: string;
            local: boolean;
            claim?: string;
            claim_uid?: string;
            volume?: string;
            volume_uid?: string;
        };
        KubernetesDrainBudget: {
            name: string;
            uid: string;
            /** Format: int64 */
            disruptions_allowed: number;
        };
        KubernetesDrainPod: {
            namespace: string;
            namespace_uid: string;
            name: string;
            uid: string;
            reason?: string;
            /** @enum {string} */
            state: "pending" | "gone" | "terminating" | "eviction_requested" | "blocked" | "skipped";
            volumes: components["schemas"]["KubernetesDrainVolume"][];
            budgets: components["schemas"]["KubernetesDrainBudget"][];
        };
        KubernetesDrainProgress: {
            /** @enum {string} */
            phase: "draining" | "blocked" | "complete";
            pods: components["schemas"]["KubernetesDrainPod"][];
        };
        KubernetesOperation: {
            /** Format: uuid */
            id: string;
            /** Format: uuid */
            cluster_id: string;
            /** Format: uuid */
            namespace_id?: string;
            namespace_uid: string;
            /** @enum {string} */
            status: "awaiting_confirmation" | "queued" | "running" | "observing" | "succeeded" | "failed" | "cancelled" | "result_unknown";
            operation: string;
            type: string;
            name: string;
            uid: string;
            error_code: string;
            /** Format: int64 */
            version: number;
            result: {
                drain?: components["schemas"]["KubernetesDrainProgress"];
            } & {
                [key: string]: unknown;
            };
            /** Format: uuid */
            pending_action_id?: string;
            /** Format: date-time */
            created_at: string;
            /** Format: date-time */
            updated_at: string;
            /** Format: date-time */
            started_at?: string;
            /** Format: date-time */
            finished_at?: string;
        };
        KubernetesDrainCancelInput: {
            /** Format: int64 */
            expected_version: number;
        };
        KubernetesOperationPage: {
            items: components["schemas"]["KubernetesOperation"][];
            next_cursor?: string;
        };
        KubernetesRollbackReference: {
            name: string;
            uid: string;
            resource_version: string;
            /** Format: int64 */
            revision: number;
        };
        KubernetesObjectVersionReference: {
            name: string;
            uid: string;
            resource_version: string;
        };
        KubernetesMutationInput: {
            /** Format: uuid */
            namespace_id?: string;
            /** @enum {string} */
            operation: "create" | "patch" | "delete" | "scale" | "restart" | "rollback" | "cordon" | "uncordon" | "drain";
            type: string;
            name: string;
            uid?: string;
            expected_resource_version?: string;
            object?: {
                [key: string]: unknown;
            };
            /** Format: int64 */
            replicas?: number;
            rollback?: components["schemas"]["KubernetesRollbackReference"];
            cronjob_source?: components["schemas"]["KubernetesObjectVersionReference"];
            /** @description Protected Secret values; excluded from public manifests, action plans and Agent inputs. */
            secret_values?: {
                [key: string]: string;
            };
        };
        KubernetesInventoryWatchInput: {
            token: string;
        };
        KubernetesInventoryDefinition: {
            key: string;
            group: string;
            version: string;
            resource: string;
            kind: string;
            namespaced: boolean;
            category: string;
            manage_permission: string;
        };
        KubernetesInventoryObject: {
            definition: components["schemas"]["KubernetesInventoryDefinition"];
            uid: string;
            namespace: string;
            namespace_uid?: string;
            name: string;
            resource_version: string;
            /** Format: int64 */
            generation: number;
            labels: {
                [key: string]: string;
            };
            owners: {
                apiVersion: string;
                kind: string;
                name: string;
                uid: string;
                controller?: boolean;
                blockOwnerDeletion?: boolean;
            }[];
            /** Format: date-time */
            created_at: string;
            /** Format: date-time */
            observed_at: string;
            redacted: boolean;
            data: {
                [key: string]: unknown;
            };
        };
        KubernetesInventoryWatchEvent: {
            /** @enum {string} */
            type: "ADDED" | "MODIFIED" | "DELETED";
            object: components["schemas"]["KubernetesInventoryObject"];
        };
        KubernetesInventoryWatchResult: {
            events: components["schemas"]["KubernetesInventoryWatchEvent"][];
            watch_token: string;
            scope_signature: string;
            /** Format: date-time */
            observed_at: string;
            more: boolean;
        };
        KubernetesInventoryTypes: {
            items: components["schemas"]["KubernetesInventoryDefinition"][];
            complete: boolean;
            unavailable_reason?: string;
        };
        KubernetesNamespaceReference: {
            /** Format: uuid */
            id: string;
            name: string;
            uid: string;
            /** Format: int64 */
            version: number;
        };
        KubernetesInventoryScope: {
            /** Format: uuid */
            cluster_id: string;
            cluster_name: string;
            /** Format: int64 */
            cluster_version: number;
            cluster_wide: boolean;
            namespaces: components["schemas"]["KubernetesNamespaceReference"][];
        };
        KubernetesInventoryPage: {
            items: components["schemas"]["KubernetesInventoryObject"][];
            next_cursor?: string;
            complete: boolean;
            /** Format: date-time */
            observed_at: string;
            scope_signature: string;
            namespace_references: {
                [key: string]: string;
            };
            watch_token?: string;
        };
        /** @enum {string} */
        KubernetesConnectionMode: "via_bastion" | "direct" | "in_cluster";
        /** @enum {string} */
        Environment: "development" | "staging" | "production";
        UserLabelKey: string;
        SystemLabelKey: string;
        LabelValue: string;
        Labels: {
            [key: string]: components["schemas"]["LabelValue"];
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
        UserLabels: {
            [key: string]: components["schemas"]["LabelValue"];
        };
        ResourcePreviewDelete: {
            /** Format: int64 */
            expected_version: number;
        };
        KubernetesConnectionTestCreate: {
            /** Format: uri */
            api_server: string;
            /** @enum {string} */
            connection_mode: "via_bastion" | "direct";
            /** Format: uuid */
            bastion_scope_id?: string;
            /** Format: uuid */
            credential_id?: string;
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
        /** @description Stable Argus API error. */
        "responses-Error": {
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
        Cursor: string;
        Limit: number;
        ResourceId: string;
        "parameters-CsrfToken": string;
    };
    requestBodies: never;
    headers: never;
    pathItems: never;
}
export type $defs = Record<string, never>;
export interface operations {
    listKubernetesHelmReleases: {
        parameters: {
            query: {
                namespace_id: string;
            };
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Release metadata without chart, values, manifests or hook output */
            200: {
                headers: {
                    /** @description Protected enterprise response is never stored by shared or browser caches */
                    "Cache-Control": "no-store";
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["KubernetesHelmReleases"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    listKubernetesHelmHistory: {
        parameters: {
            query: {
                namespace_id: string;
            };
            header?: never;
            path: {
                id: string;
                releaseName: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Revision metadata */
            200: {
                headers: {
                    /** @description Protected enterprise response is never stored by shared or browser caches */
                    "Cache-Control": "no-store";
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["KubernetesHelmReleases"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    previewKubernetesHelm: {
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
                "application/json": components["schemas"]["KubernetesHelmPreviewInput"];
            };
        };
        responses: {
            /** @description Protected Helm PendingAction */
            201: {
                headers: {
                    /** @description Protected enterprise response is never stored by shared or browser caches */
                    "Cache-Control": "no-store";
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["pending-action-public.schema"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    createKubernetesStreamSession: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["KubernetesStreamInput"];
            };
        };
        responses: {
            /** @description Create a UID-bound terminal or TCP forwarding session */
            201: {
                headers: {
                    "Cache-Control": "no-store";
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["KubernetesStreamTicket"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    getKubernetesStreamSession: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
                sessionId: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Read current Kubernetes session state */
            200: {
                headers: {
                    "Cache-Control": "no-store";
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["KubernetesStreamSession"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    closeKubernetesStreamSession: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                id: string;
                sessionId: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Close a Kubernetes terminal or forwarding session */
            200: {
                headers: {
                    "Cache-Control": "no-store";
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["KubernetesStreamSession"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    issueKubernetesStreamTicket: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                id: string;
                sessionId: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Replace the pending one-use Kubernetes stream ticket */
            200: {
                headers: {
                    "Cache-Control": "no-store";
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["KubernetesStreamTicket"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    getKubernetesLogSession: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
                sessionId: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Close the owning Pod log session */
            200: {
                headers: {
                    "Cache-Control": "no-store";
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["KubernetesLogSession"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    createKubernetesLogSession: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["KubernetesLogSessionInput"];
            };
        };
        responses: {
            /** @description Create a UID-bound Pod log session and a one-use attach ticket */
            201: {
                headers: {
                    "Cache-Control": "no-store";
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["KubernetesLogSessionTicket"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    attachKubernetesLogSession: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                id: string;
                sessionId: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["KubernetesLogSessionAttachInput"];
            };
        };
        responses: {
            /** @description Consume the short-lived ticket in the owning HTTP session */
            200: {
                headers: {
                    "Cache-Control": "no-store";
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["KubernetesLogSession"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    readKubernetesLogSession: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                id: string;
                sessionId: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["KubernetesLogSessionReadInput"];
            };
        };
        responses: {
            /** @description Read a bounded authorized log increment with a signed continuity cursor */
            200: {
                headers: {
                    "Cache-Control": "no-store";
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["KubernetesLogSessionBatch"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    closeKubernetesLogSession: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                id: string;
                sessionId: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Close the owning Pod log session */
            200: {
                headers: {
                    "Cache-Control": "no-store";
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["KubernetesLogSession"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    getKubernetesOperation: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
                operationId: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Authorized Kubernetes operation */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["KubernetesOperation"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    cancelKubernetesDrain: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                id: string;
                operationId: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["KubernetesDrainCancelInput"];
            };
        };
        responses: {
            /** @description Cancelled node drain */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["KubernetesOperation"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    listKubernetesOperations: {
        parameters: {
            query?: {
                limit?: number;
                cursor?: string;
            };
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Authorized Kubernetes operation */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["KubernetesOperationPage"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    previewKubernetesMutation: {
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
                "application/json": components["schemas"]["KubernetesMutationInput"];
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
    watchKubernetesInventory: {
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
        requestBody: {
            content: {
                "application/json": components["schemas"]["KubernetesInventoryWatchInput"];
            };
        };
        responses: {
            /** @description Bounded events and next authorized resume token */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["KubernetesInventoryWatchResult"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    listKubernetesInventoryTypes: {
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
            /** @description Authorized UID-bound Kubernetes observation */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["KubernetesInventoryTypes"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    getKubernetesInventoryScope: {
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
            /** @description Authorized UID-bound Kubernetes observation */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["KubernetesInventoryScope"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    getKubernetesInventoryObject: {
        parameters: {
            query: {
                type: string;
                name: string;
                uid: string;
                namespace_id?: string;
            };
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Authorized UID-bound Kubernetes observation */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["KubernetesInventoryObject"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    listKubernetesInventory: {
        parameters: {
            query: {
                type: string;
                namespace_mode?: "all_authorized" | "explicit";
                namespace_ids?: string[];
                name?: string;
                labels?: string;
                limit?: number;
                cursor?: string;
            };
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Authorized UID-bound Kubernetes observation */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["KubernetesInventoryPage"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    listKubernetesClusters: {
        parameters: {
            query?: {
                cursor?: components["parameters"]["Cursor"];
                limit?: components["parameters"]["Limit"];
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Cluster page. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["KubernetesClusterPage"];
                };
            };
            default: components["responses"]["responses-Error"];
        };
    };
    getKubernetesCluster: {
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
            /** @description Kubernetes cluster. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["KubernetesCluster"];
                };
            };
            default: components["responses"]["responses-Error"];
        };
    };
    previewCreateKubernetesCluster: {
        parameters: {
            query?: never;
            header: {
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
                "X-CSRF-Token": components["parameters"]["parameters-CsrfToken"];
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["KubernetesPreviewCreate"];
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
            default: components["responses"]["responses-Error"];
        };
    };
    previewUpdateKubernetesCluster: {
        parameters: {
            query?: never;
            header: {
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
                "X-CSRF-Token": components["parameters"]["parameters-CsrfToken"];
            };
            path: {
                id: components["parameters"]["ResourceId"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["KubernetesPreviewUpdate"];
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
            default: components["responses"]["responses-Error"];
        };
    };
    previewDeleteKubernetesCluster: {
        parameters: {
            query?: never;
            header: {
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
                "X-CSRF-Token": components["parameters"]["parameters-CsrfToken"];
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
            default: components["responses"]["responses-Error"];
        };
    };
    createKubernetesConnectionTest: {
        parameters: {
            query?: never;
            header: {
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
                "X-CSRF-Token": components["parameters"]["parameters-CsrfToken"];
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["KubernetesConnectionTestCreate"];
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
            default: components["responses"]["responses-Error"];
        };
    };
    listKubernetesResources: {
        parameters: {
            query: {
                resource_type: "namespace" | "node" | "pod" | "deployment" | "statefulset" | "daemonset" | "service";
                namespace?: string;
                query?: string;
                cursor?: components["parameters"]["Cursor"];
                limit?: components["parameters"]["Limit"];
            };
            header?: never;
            path: {
                id: components["parameters"]["ResourceId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Kubernetes resource page. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["KubernetesResourcePage"];
                };
            };
            default: components["responses"]["responses-Error"];
        };
    };
    getKubernetesPodLogs: {
        parameters: {
            query: {
                namespace: string;
                pod: string;
                container?: string;
                tail_lines?: number;
            };
            header?: never;
            path: {
                id: components["parameters"]["ResourceId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Bounded Pod logs. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["PodLogs"];
                };
            };
            default: components["responses"]["responses-Error"];
        };
    };
}
