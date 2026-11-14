import type { components } from "./kubernetesapi.js";

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
    getKubernetesRelatedResources: {
        parameters: {
            query: {
                type: string;
                name: string;
                uid: string;
                namespace_id?: string;
                view: "relations" | "events";
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
            /** @description Bounded native scan; next_cursor continues reading; complete reports read failures separately from remaining pages. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["KubernetesRelatedPage"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    getKubernetesResourceSnapshot: {
        parameters: {
            query?: {
                namespace_mode?: "all_authorized" | "explicit";
                namespace_ids?: string[];
                types?: string[];
                max_items?: number;
            };
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Authorized current snapshot; completeness is explicit. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["KubernetesResourceSnapshot"];
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
    prepareKubernetesListMetrics: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["KubernetesMetricPrepareInput"];
            };
        };
        responses: {
            /** @description Authorized current-page read result */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["KubernetesMetricContext"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    readKubernetesListMetrics: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["KubernetesMetricReadInput"];
            };
        };
        responses: {
            /** @description Authorized current-page read result */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["KubernetesMetricResult"];
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
                /** @description Unified resource state; attention matches failed, warning, pending or unknown. Filtering retains bounded server pagination. */
                state?: "healthy" | "configured" | "progressing" | "pending" | "warning" | "failed" | "completed" | "paused" | "scaled_down" | "terminating" | "unknown" | "attention";
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
