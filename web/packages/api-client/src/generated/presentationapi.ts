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
            runtime: "argus-template/v1";
            template_hash: string;
            template_source: string;
            detail_data: {
                [key: string]: unknown;
            };
            resource_refs?: {
                /** @enum {string} */
                type: "host" | "kubernetes_cluster" | "connector";
                /** Format: uuid */
                id: string;
            }[];
            result_refs?: string[];
            /** @enum {string} */
            status: "ready" | "failed";
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
}
