export interface paths {
    "/enterprise/mcp-connections": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /** listMCPConnections. */
        get: operations["listMCPConnections"];
        put?: never;
        /** createMCPConnection. */
        post: operations["createMCPConnection"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/mcp-connections/{id}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        /** getMCPConnection. */
        get: operations["getMCPConnection"];
        /** updateMCPConnection. */
        put: operations["updateMCPConnection"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/mcp-connections/{id}/test": {
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
        /** testMCPConnection. */
        post: operations["testMCPConnection"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/mcp-connections/{id}/state": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        get?: never;
        /** setMCPConnectionState. */
        put: operations["setMCPConnectionState"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/enterprise/mcp-connections/{id}/members": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
            };
            cookie?: never;
        };
        get?: never;
        /** setMCPConnectionMembers. */
        put: operations["setMCPConnectionMembers"];
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
        MCPConnection: {
            /** Format: uuid */
            id: string;
            name: string;
            /** @enum {string} */
            transport: "streamable_http";
            /** Format: uri */
            endpoint: string;
            /** @enum {string} */
            auth_type: "none" | "bearer" | "basic";
            /** @enum {string} */
            status: "enabled" | "disabled";
            /** @enum {string} */
            health_status: "unknown" | "healthy" | "unhealthy";
            /** Format: int64 */
            version: number;
            revision: number;
            tool_count: number;
            member_ids: string[];
            last_error_code?: string;
            /** Format: date-time */
            created_at: string;
            /** Format: date-time */
            updated_at: string;
        };
        MCPConnectionPage: {
            items: components["schemas"]["MCPConnection"][];
        };
        IdempotencyKey: string;
        MCPConnectionWrite: {
            name: string;
            /** Format: uri */
            endpoint: string;
            /** @enum {string} */
            auth_type: "none" | "bearer" | "basic";
            credential_value?: string;
            /** Format: int64 */
            expected_version?: number;
            member_ids: string[];
        };
        MCPConnectionState: {
            /** @enum {string} */
            status: "enabled" | "disabled";
            /** Format: int64 */
            expected_version: number;
        };
        MCPConnectionMembers: {
            member_ids: string[];
            /** Format: int64 */
            expected_version: number;
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
    listMCPConnections: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
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
                    "application/json": components["schemas"]["MCPConnectionPage"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    createMCPConnection: {
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
                "application/json": components["schemas"]["MCPConnectionWrite"];
            };
        };
        responses: {
            /** @description Operation result. */
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["MCPConnection"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    getMCPConnection: {
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
            /** @description Operation result. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["MCPConnection"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    updateMCPConnection: {
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
                "application/json": components["schemas"]["MCPConnectionWrite"];
            };
        };
        responses: {
            /** @description Operation result. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["MCPConnection"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    testMCPConnection: {
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
        requestBody?: never;
        responses: {
            /** @description Operation result. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["MCPConnection"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    setMCPConnectionState: {
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
                "application/json": components["schemas"]["MCPConnectionState"];
            };
        };
        responses: {
            /** @description Operation result. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["MCPConnection"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    setMCPConnectionMembers: {
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
                "application/json": components["schemas"]["MCPConnectionMembers"];
            };
        };
        responses: {
            /** @description Operation result. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["MCPConnection"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
}
