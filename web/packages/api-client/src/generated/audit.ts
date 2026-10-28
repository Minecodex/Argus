export interface paths {
    "/{audience}/audit-events": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                audience: components["parameters"]["Audience"];
            };
            cookie?: never;
        };
        /** listAuditEvents. */
        get: operations["listAuditEvents"];
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
        AuditEvent: {
            /** Format: uuid */
            id: string;
            /** @enum {string} */
            domain: "platform" | "enterprise";
            /** Format: uuid */
            enterprise_id?: string;
            /** @enum {string} */
            actor_type: "platform_user" | "enterprise_user" | "service_account" | "connector" | "direct_executor" | "remote_access_gateway" | "system";
            actor_id: string;
            actor_display_name?: string;
            actor_username?: string;
            operation?: components["schemas"]["AuditOperation"];
            action: string;
            resource_type?: string;
            resource_id?: string;
            resource_display_name?: string;
            /** @enum {string} */
            result: "success" | "failure" | "denied";
            details: {
                [key: string]: unknown;
            };
            previous_hash: string;
            event_hash: string;
            /** Format: date-time */
            created_at: string;
        };
        AuditEventPage: {
            items: components["schemas"]["AuditEvent"][];
            page: components["schemas"]["CursorPage"];
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
        /** @description Current operation projection. Separate from immutable event facts and never a source of execution authority. */
        AuditOperation: {
            action_ref: string;
            action_type: string;
            action_status: string;
            /** Format: uuid */
            execution_id?: string;
            execution_status?: string;
            error_code?: string;
            resource_type?: string;
            /** Format: uuid */
            resource_id?: string;
            resource_name?: string;
            /** Format: date-time */
            created_at: string;
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
        Audience: "platform" | "enterprise";
        Cursor: string;
        Limit: number;
    };
    requestBodies: never;
    headers: never;
    pathItems: never;
}
export type $defs = Record<string, never>;
export interface operations {
    listAuditEvents: {
        parameters: {
            query?: {
                cursor?: components["parameters"]["Cursor"];
                limit?: components["parameters"]["Limit"];
                /** @description Exact server-owned action reference; restricted to the same authorized audit scope. */
                operation_ref?: string;
                action?: string;
                actor_id?: string;
                resource_type?: string;
                resource_id?: string;
                result?: "success" | "failure" | "denied";
                /** @description Case-insensitive search in event codes, identifiers and sanitized details. */
                query?: string;
                from?: string;
                to?: string;
            };
            header?: never;
            path: {
                audience: components["parameters"]["Audience"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Audit events for the authenticated domain. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["AuditEventPage"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
}
