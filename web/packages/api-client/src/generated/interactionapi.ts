export interface paths {
    "/conversations/{conversation_id}/interactions": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                conversation_id: string;
            };
            cookie?: never;
        };
        /** List current question, latest drafts and confirmed plans */
        get: operations["listConversationInteractions"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/conversations/{conversation_id}/interactions/{interaction_id}": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                conversation_id: string;
                interaction_id: string;
            };
            cookie?: never;
        };
        /** Read an owned question or guidance plan */
        get: operations["getConversationInteraction"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/conversations/{conversation_id}/interactions/{interaction_id}/respond": {
        parameters: {
            query?: never;
            header?: never;
            path: {
                conversation_id: string;
                interaction_id: string;
            };
            cookie?: never;
        };
        get?: never;
        put?: never;
        /** Answer, skip, revise or confirm the exact interaction version */
        post: operations["respondConversationInteraction"];
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
        InteractionOption: {
            label: string;
            description: string;
        };
        InteractionQuestion: {
            id: string;
            header: string;
            question: string;
            options: components["schemas"]["InteractionOption"][];
        };
        UserInputQuestions: {
            questions: components["schemas"]["InteractionQuestion"][];
        };
        GuidancePlanStep: {
            description: string;
            code?: string;
            language?: string;
            target?: string;
        };
        GuidancePlan: {
            title: string;
            description?: string;
            steps?: components["schemas"]["GuidancePlanStep"][];
            /** Format: uuid */
            supersedes_id?: string;
            markdown?: string;
            document_markdown?: string;
            commands?: components["schemas"]["PlanCommand"][];
        };
        InteractionContent: {
            title?: string;
            description?: string;
            steps?: components["schemas"]["GuidancePlanStep"][];
            /** Format: uuid */
            supersedes_id?: string;
            questions?: components["schemas"]["InteractionQuestion"][];
            markdown?: string;
            document_markdown?: string;
            commands?: components["schemas"]["PlanCommand"][];
        };
        InteractionAnswer: {
            selected_option?: string;
            text?: string;
            skipped?: boolean;
        };
        InteractionResponse: {
            /** Format: int64 */
            version: number;
            content_hash: string;
            /** @enum {string} */
            action: "answer" | "skip" | "confirm" | "revise";
            answers?: {
                [key: string]: components["schemas"]["InteractionAnswer"];
            };
            feedback?: string;
        };
        ConversationInteraction: {
            /** Format: uuid */
            id: string;
            /** Format: uuid */
            conversation_id: string;
            /** Format: uuid */
            run_id: string;
            /** Format: uuid */
            family_id: string;
            /** @enum {string} */
            kind: "plan" | "user_input";
            revision: number;
            /** @enum {string} */
            status: "pending" | "answered" | "skipped" | "superseded" | "confirmed" | "cancelled";
            /** Format: int64 */
            version: number;
            content_hash: string;
            content: components["schemas"]["InteractionContent"];
            response?: components["schemas"]["InteractionResponse"];
            /** Format: uuid */
            implementation_run_id?: string;
            implementation_status?: string;
            /** Format: date-time */
            created_at: string;
            /** Format: date-time */
            updated_at: string;
        };
        ConversationInteractionPage: {
            items: components["schemas"]["ConversationInteraction"][];
            /** Format: uuid */
            active_run_id?: string;
            active_run_status?: string;
        };
        PlanCommand: {
            /** Format: uuid */
            approved_step_id: string;
            block_id: string;
            target: string;
            identity: string;
            interpreter: string;
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
    listConversationInteractions: {
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
            /** @description Durable conversation interaction state */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ConversationInteractionPage"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    getConversationInteraction: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                conversation_id: string;
                interaction_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Current interaction */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ConversationInteraction"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    respondConversationInteraction: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CsrfToken"];
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
            };
            path: {
                conversation_id: string;
                interaction_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["InteractionResponse"];
            };
        };
        responses: {
            /** @description Resolved interaction and implementation reference */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ConversationInteraction"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
}
