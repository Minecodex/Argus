export interface paths {
    "/conversations/{conversation_id}/tool-presentations/{tool_call_id}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                conversation_id: string;
                tool_call_id: string;
            };
            cookie?: never;
        };
        /** getToolPresentation. */
        get: operations["getToolPresentation"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/conversations/{conversation_id}/tool-presentations/{tool_call_id}/read": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                conversation_id: string;
                tool_call_id: string;
            };
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** Read a capability bound to this authorized presentation. */
        post: operations["readToolPresentation"];
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
        ToolPresentation: {
            /** Format: uuid */
            tool_call_id: string;
            /** @enum {string} */
            runtime: "argus-template/v1" | "argus-template/v2";
            read_operations?: string[];
            template_hash: string;
            template_source: string;
            detail_data: {
                [key: string]: unknown;
            };
            resource_refs?: {
                /** @enum {string} */
                type: "host" | "kubernetes_cluster" | "connector" | "dashboard";
                /** Format: uuid */
                id: string;
            }[];
            result_refs?: string[];
            /** @enum {string} */
            status: "ready" | "failed";
        };
        PresentationReadInput: {
            /** @enum {string} */
            operation: "resources.context" | "metric.context" | "alert.details" | "dashboard.result" | "result.page" | "runtime.status" | "result.detail";
            /** @description result.page reads sealed data only. Search, exact scalar filters and ordering apply before pagination to the entire bound collection; the cursor binds these conditions and the original presentation/attempt/authorization version. Nested sample slices do not accept filtering. Other operations retain their own parameter allowlists. */
            parameters: {
                search?: string;
                /** @description Allowed fields: status, connection_status, severity, level, environment, namespace, resource_type, service_name, signal, type. */
                filters?: {
                    [key: string]: string;
                };
                /** @enum {string} */
                sort?: "" | "name" | "title" | "hostname" | "id" | "status" | "severity" | "namespace" | "environment" | "updated_at" | "created_at" | "started_at" | "timestamp";
                /** @enum {string} */
                direction?: "asc" | "desc";
            } & {
                [key: string]: unknown;
            };
        };
        PresentationReadResult: {
            /** @enum {string} */
            status: "ready" | "pending";
            data: {
                [key: string]: unknown;
            };
            next_cursor?: string;
        };
    };
    responses: {
        /** @description Stable Argus error. */
        Error: {
            headers: {
                [name: string]: unknown;
            };
            content: {
                "application/json": components["schemas"]["ApiError"];
            };
        };
    };
    parameters: never;
    requestBodies: never;
    headers: never;
    pathItems: never;
}
export type $defs = Record<string, never>;
export interface operations {
    getToolPresentation: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                conversation_id: string;
                tool_call_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Operation result. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ToolPresentation"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    readToolPresentation: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                conversation_id: string;
                tool_call_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["PresentationReadInput"];
            };
        };
        responses: {
            /** @description Bound public presentation data. A result.page response includes matched total, original source_total, offset, complete and low-cardinality public facets. Counts describe this sealed result, not unqueried business data. Filters never execute a new business query. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["PresentationReadResult"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
}
