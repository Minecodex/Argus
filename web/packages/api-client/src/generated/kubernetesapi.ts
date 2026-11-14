import type { operations } from "./kubernetesapi_operations.js";
export type { operations };

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
    "/enterprise/kubernetes-clusters/{id}/inventory/related": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        /** Read a UID-bound object relation or event page within the current authorization scope */
        get: operations["getKubernetesRelatedResources"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/kubernetes-clusters/{id}/inventory/snapshot": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        /** Read a bounded authorized resource snapshot and real resource relations */
        get: operations["getKubernetesResourceSnapshot"];
        put?: never;
        post?: never;
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
    "/enterprise/kubernetes-clusters/{id}/inventory/metrics/prepare": {
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
        /** Prepare a bounded current-page metric context */
        post: operations["prepareKubernetesListMetrics"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/kubernetes-clusters/{id}/inventory/metrics/read": {
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
        /** Read one metric independently for the current resource page */
        post: operations["readKubernetesListMetrics"];
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
            /** @description Initial lookback in seconds. Zero reads the latest retained tail without a time filter; byte and row limits still apply. */
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
        KubernetesResourceSummary: {
            /** @enum {string} */
            state: "unknown" | "healthy" | "configured" | "progressing" | "pending" | "warning" | "failed" | "completed" | "paused" | "scaled_down" | "terminating";
            reason?: string;
            fields: {
                [key: string]: string;
            };
        };
        KubernetesResourceAction: {
            action: string;
            /** @enum {string} */
            state: "requires_check" | "unavailable";
            reason?: string;
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
            summary?: components["schemas"]["KubernetesResourceSummary"];
            actions?: components["schemas"]["KubernetesResourceAction"][];
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
        KubernetesSnapshotResource: {
            type: string;
            name: string;
            uid: string;
            namespace: string;
            /** Format: uuid */
            namespace_id?: string;
            namespace_uid?: string;
            summary: components["schemas"]["KubernetesResourceSummary"];
            actions: components["schemas"]["KubernetesResourceAction"][];
            /** Format: date-time */
            created_at: string;
        };
        KubernetesRelatedPage: {
            items: components["schemas"]["KubernetesSnapshotResource"][];
            next_cursor?: string;
            complete: boolean;
            errors: {
                [key: string]: string;
            };
            /** Format: date-time */
            observed_at: string;
        };
        KubernetesResourceRelation: {
            from: string;
            to: string;
            /** @enum {string} */
            kind: "owns" | "selects" | "schedules" | "mounts" | "references" | "binds" | "uses" | "scales" | "routes";
        };
        KubernetesResourceReference: {
            type: string;
            name: string;
            uid: string;
            namespace: string;
            /** Format: uuid */
            namespace_id?: string;
            namespace_uid?: string;
        };
        KubernetesResourceSnapshot: {
            /** Format: uuid */
            cluster_id: string;
            cluster_wide: boolean;
            resources: components["schemas"]["KubernetesSnapshotResource"][];
            relations: components["schemas"]["KubernetesResourceRelation"][];
            counts: {
                [key: string]: {
                    [key: string]: number;
                };
            };
            complete: boolean;
            errors: {
                [key: string]: string;
            };
            /** Format: date-time */
            observed_at: string;
            scope_signature: string;
            focus_uid?: string;
            focus_candidates?: components["schemas"]["KubernetesResourceReference"][];
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
        KubernetesMetricSelection: {
            /** @enum {string} */
            mode: "explicit" | "all_authorized";
            ids?: string[];
        };
        KubernetesMetricList: {
            type: string;
            namespaces: components["schemas"]["KubernetesMetricSelection"];
            state?: string;
            name?: string;
            labels?: string;
            limit?: number;
            cursor?: string;
        };
        KubernetesMetricPrepareInput: {
            list: components["schemas"]["KubernetesMetricList"];
            uids: string[];
        };
        KubernetesMetricContext: {
            token: string;
            /** Format: date-time */
            observed_at: string;
            /** Format: date-time */
            expires_at: string;
            complete: boolean;
        };
        KubernetesMetricReadInput: {
            token: string;
            /** @enum {string} */
            metric: "cpu" | "memory" | "storage";
        };
        KubernetesMetricPoint: {
            /** Format: date-time */
            time: string;
            value: number | null;
            request?: number | null;
            limit?: number | null;
            capacity?: number | null;
        };
        KubernetesMetricTrend: {
            /** @enum {string} */
            kind: "cpu" | "memory" | "ephemeral" | "local_filesystem" | "volume";
            /** @enum {string} */
            unit: "cores" | "bytes";
            /** @enum {string} */
            state: "ready" | "no_data" | "stale" | "partial" | "not_configured" | "forbidden" | "query_failed" | "identity_changed" | "unsupported";
            reason?: string;
            /** @enum {string} */
            basis: "usage" | "request" | "limit" | "capacity";
            points: components["schemas"]["KubernetesMetricPoint"][];
            current_request?: number | null;
            current_limit?: number | null;
            current_capacity?: number | null;
            allocatable?: number | null;
            partial_configuration: boolean;
        };
        KubernetesMetricEntry: {
            uid: string;
            trends: components["schemas"]["KubernetesMetricTrend"][];
        };
        KubernetesMetricResult: {
            items: components["schemas"]["KubernetesMetricEntry"][];
            /** Format: date-time */
            observed_at: string;
            /** Format: date-time */
            from: string;
            /** Format: date-time */
            to: string;
            complete: boolean;
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
