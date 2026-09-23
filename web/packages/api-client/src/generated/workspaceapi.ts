export interface paths {
    "/conversations/{conversation_id}/workspace": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                conversation_id: string;
            };
            cookie?: never;
        };
        /** getConversationWorkspace. */
        get: operations["getConversationWorkspace"];
        put?: never;
        post?: never;
        /** deleteConversationWorkspace. */
        delete: operations["deleteConversationWorkspace"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/conversations/{conversation_id}/workspace/files": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                conversation_id: string;
            };
            cookie?: never;
        };
        /** listWorkspaceFiles. */
        get: operations["listWorkspaceFiles"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/conversations/{conversation_id}/workspace/uploads": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                conversation_id: string;
            };
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** createWorkspaceUpload. */
        post: operations["createWorkspaceUpload"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/conversations/{conversation_id}/workspace/uploads/{upload_id}/content": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                conversation_id: string;
                upload_id: string;
            };
            cookie?: never;
        };
        get?: never;
        /** uploadWorkspaceContent. */
        put: operations["uploadWorkspaceContent"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/conversations/{conversation_id}/workspace/files/{file_id}/content": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                conversation_id: string;
                file_id: string;
            };
            cookie?: never;
        };
        /** downloadWorkspaceFile. */
        get: operations["downloadWorkspaceFile"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/conversations/{conversation_id}/deliveries/{delivery_id}/content": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                conversation_id: string;
                delivery_id: string;
            };
            cookie?: never;
        };
        /** downloadFileDelivery. */
        get: operations["downloadFileDelivery"];
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
        Workspace: {
            /** Format: uuid */
            id: string;
            /** Format: uuid */
            conversation_id: string;
            /** @enum {string} */
            status: "provisioning" | "ready" | "deleting" | "deleted" | "failed";
            /** Format: int64 */
            capacity_bytes: number;
            /** Format: int64 */
            version: number;
            /** Format: date-time */
            created_at: string;
        };
        IdempotencyKey: string;
        WorkspaceFile: {
            /** Format: uuid */
            id: string;
            name: string;
            path: string;
            /** Format: int64 */
            byte_size: number;
            content_hash: string;
            media_type: string;
            /** Format: date-time */
            created_at: string;
        };
        WorkspaceFilePage: {
            items: components["schemas"]["WorkspaceFile"][];
        };
        WorkspaceUploadCreate: {
            name: string;
            /** Format: int64 */
            byte_size: number;
        };
        WorkspaceUpload: {
            /** Format: uuid */
            id: string;
            name: string;
            /** Format: int64 */
            byte_size: number;
            /** @enum {string} */
            status: "pending" | "uploading" | "complete" | "failed" | "cancelled";
            /** Format: date-time */
            expires_at: string;
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
    getConversationWorkspace: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                conversation_id: string;
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
                    "application/json": components["schemas"]["Workspace"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    deleteConversationWorkspace: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CsrfToken"];
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
            };
            path: {
                conversation_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Operation result. */
            202: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Workspace"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    listWorkspaceFiles: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                conversation_id: string;
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
                    "application/json": components["schemas"]["WorkspaceFilePage"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    createWorkspaceUpload: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CsrfToken"];
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
            };
            path: {
                conversation_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["WorkspaceUploadCreate"];
            };
        };
        responses: {
            /** @description Operation result. */
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["WorkspaceUpload"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    uploadWorkspaceContent: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CsrfToken"];
            };
            path: {
                conversation_id: string;
                upload_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/octet-stream": string;
            };
        };
        responses: {
            /** @description Operation result. */
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["WorkspaceFile"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    downloadWorkspaceFile: {
        parameters: {
            query?: never;
            header?: {
                Range?: string;
            };
            path: {
                conversation_id: string;
                file_id: string;
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
                    "application/octet-stream": string;
                };
            };
            /** @description Partial file content. */
            206: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/octet-stream": string;
                };
            };
            default: components["responses"]["Error"];
        };
    };
    downloadFileDelivery: {
        parameters: {
            query?: never;
            header?: {
                Range?: string;
            };
            path: {
                conversation_id: string;
                delivery_id: string;
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
                    "application/octet-stream": string;
                };
            };
            /** @description Partial file content. */
            206: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/octet-stream": string;
                };
            };
            default: components["responses"]["Error"];
        };
    };
}
