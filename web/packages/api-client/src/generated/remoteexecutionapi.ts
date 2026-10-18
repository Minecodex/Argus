export interface paths {
    "/conversations/{conversation_id}/remote-executions/{execution_id}/input-files/{file_id}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                conversation_id: string;
                execution_id: string;
                file_id: string;
            };
            cookie?: never;
        };
        /** Review the actual immutable input bytes covered by confirmation */
        get: operations["readRemoteExecutionInput"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/conversations/{conversation_id}/remote-executions": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                conversation_id: string;
            };
            cookie?: never;
        };
        /** List own conversation remote execution requests */
        get: operations["listRemoteExecutions"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/conversations/{conversation_id}/remote-executions/{execution_id}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                conversation_id: string;
                execution_id: string;
            };
            cookie?: never;
        };
        /** Read an authorized remote execution */
        get: operations["getRemoteExecution"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/conversations/{conversation_id}/remote-executions/{execution_id}/stop": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                conversation_id: string;
                execution_id: string;
            };
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** Explicitly stop own approved remote execution */
        post: operations["stopRemoteExecution"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/conversations/{conversation_id}/remote-executions/{execution_id}/feedback": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                conversation_id: string;
                execution_id: string;
            };
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** Supersede a pending plan and ask the Agent for a revised preview */
        post: operations["reviseRemotePlan"];
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
        RemoteExecution: {
            /** Format: uuid */
            id: string;
            action_ref: string;
            content_hash: string;
            /** @enum {string} */
            status: "awaiting_confirmation" | "ready" | "running" | "succeeded" | "failed" | "partial_failed" | "result_unknown" | "paused_for_recovery" | "stopping" | "cancelled" | "superseded" | "expired";
            stop_requested: boolean;
            confirmation_required?: boolean;
            /** @enum {string} */
            stop_status?: "" | "requested" | "confirmed" | "unknown";
            revision: number;
            spec: components["schemas"]["RemotePlan"];
            results: components["schemas"]["RemoteStepResult"][];
            error_code?: string;
            /** Format: date-time */
            updated_at: string;
        };
        RemoteExecutionPage: {
            items: components["schemas"]["RemoteExecution"][];
        };
        RemotePlanFeedback: {
            feedback: string;
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
        RemoteTarget: {
            name?: string;
            /** Format: int64 */
            resource_version?: number;
            /** @enum {string} */
            resource_type: "host" | "kubernetes_cluster";
            /** Format: uuid */
            resource_id: string;
            namespace?: string;
            pod?: string;
            pod_uid?: string;
            container?: string;
        };
        RemoteIdentity: {
            /** Format: uuid */
            id: string;
            username: string;
            sid?: string;
            /** Format: int64 */
            version: number;
            /** @enum {string} */
            platform: "linux" | "windows" | "kubernetes";
        };
        RemoteStep: {
            /** Format: uuid */
            id: string;
            description: string;
            target: components["schemas"]["RemoteTarget"];
            /** @enum {string} */
            interpreter: "bash" | "powershell" | "kubectl" | "pod_bash";
            script: string;
            timeout_seconds: number;
            /** Format: uuid */
            session_id: string;
            /** Format: uuid */
            session_generation: string;
            /** Format: int64 */
            context_revision: number;
            /** @description Last observed location at plan preparation; earlier steps in this plan may change it. */
            known_directory?: string;
            /** @description Last observed interpreter version; absent before approved initialization. */
            interpreter_version?: string;
            /** Format: uuid */
            previous_request_id?: string;
            previous_sequence?: number;
            identity: components["schemas"]["RemoteIdentity"];
        };
        RemoteFile: {
            /** Format: uuid */
            id?: string;
            content_preview?: string;
            /** Format: uuid */
            workspace_id?: string;
            path: string;
            /** Format: int64 */
            byte_size: number;
            content_hash: string;
            source_ref?: string;
        };
        RemotePlan: {
            /** Format: uuid */
            conversation_id?: string;
            /** Format: uuid */
            run_id?: string;
            /** @constant */
            schema_version: "argus.remote_execution/v1";
            /** Format: uuid */
            id: string;
            /** @enum {string} */
            kind: "connect" | "plan";
            title: string;
            description: string;
            steps: components["schemas"]["RemoteStep"][];
            inputs: components["schemas"]["RemoteFile"][];
            /** Format: uuid */
            supersedes_id?: string;
            /** Format: uuid */
            recovery_of_id?: string;
        };
        RemoteStepResult: {
            sequence: number;
            status: string;
            exit_code: number;
            stdout: string;
            stderr: string;
            truncated: boolean;
            directory: string;
            session_lost: boolean;
            error_code?: string;
            output_files: components["schemas"]["RemoteFile"][];
            outputs_collected?: boolean;
            terminated?: boolean;
            /** Format: date-time */
            observed_at: string;
        };
        IdempotencyKey: string;
    };
    responses: {
        /** @description Stable API error. */
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
    readRemoteExecutionInput: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                conversation_id: string;
                execution_id: string;
                file_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Frozen input file */
            200: {
                headers: {
                    "Content-Disposition": string;
                    "X-Content-SHA256": string;
                    [name: string]: unknown;
                };
                content: {
                    "application/octet-stream": string;
                };
            };
            default: components["responses"]["Error"];
        };
    };
    listRemoteExecutions: {
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
            /** @description Authoritative previews and execution state */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["RemoteExecutionPage"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    getRemoteExecution: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                conversation_id: string;
                execution_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Current remote execution */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["RemoteExecution"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    stopRemoteExecution: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CsrfToken"];
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
            };
            path: {
                conversation_id: string;
                execution_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Stop intent and current authoritative state */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["RemoteExecution"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    reviseRemotePlan: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CsrfToken"];
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
            };
            path: {
                conversation_id: string;
                execution_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["RemotePlanFeedback"];
            };
        };
        responses: {
            /** @description Revision requested; no execution authorized */
            202: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["RemoteExecution"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
}
