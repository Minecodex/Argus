import type { components } from "./dashboardapi.js";

export interface operations {
    listDashboards: {
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
                    "application/json": components["schemas"]["DashboardItem"][];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    getDashboard: {
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
                    "application/json": components["schemas"]["DashboardDetail"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    listDashboardRevisions: {
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
                    "application/json": components["schemas"]["DashboardRevision"][];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    listDashboardDrafts: {
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
                    "application/json": components["schemas"]["DashboardDraft"][];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    createDashboardDraft: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CsrfToken"];
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["DashboardDraftInput"];
            };
        };
        responses: {
            /** @description Operation result. */
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["DashboardDraft"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    getDashboardDraft: {
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
                    "application/json": components["schemas"]["DashboardDraft"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    discardDashboardDraft: {
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
                "application/json": components["schemas"]["DashboardVersionInput"];
            };
        };
        responses: {
            /** @description Operation completed. */
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            default: components["responses"]["Error"];
        };
    };
    saveDashboardDraft: {
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
                "application/json": components["schemas"]["DashboardDraftInput"];
            };
        };
        responses: {
            /** @description Operation result. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["DashboardDraft"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    previewDashboardPublication: {
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
                "application/json": components["schemas"]["DashboardVersionInput"];
            };
        };
        responses: {
            /** @description Operation result. */
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
    rebaseDashboardDraft: {
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
                "application/json": components["schemas"]["DashboardRebaseInput"];
            };
        };
        responses: {
            /** @description Operation result. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["DashboardDraft"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    validateDashboardSpec: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CsrfToken"];
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["DashboardSpec"];
            };
        };
        responses: {
            /** @description Operation result. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["DashboardValidationReport"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    listDashboardFolders: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Folders. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["DashboardFolder"][];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    previewDashboardFolder: {
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
                "application/json": components["schemas"]["DashboardLifecycleInput"];
            };
        };
        responses: {
            /** @description Operation result. */
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
    previewDashboardLifecycle: {
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
                "application/json": components["schemas"]["DashboardLifecycleInput"];
            };
        };
        responses: {
            /** @description Operation result. */
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
    executeDashboard: {
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
                "application/json": components["schemas"]["DashboardExecutionInput"];
            };
        };
        responses: {
            /** @description Operation result. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["DashboardExecution"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    sampleDashboardDraft: {
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
                "application/json": components["schemas"]["DashboardVersionInput"];
            };
        };
        responses: {
            /** @description Operation result. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["DashboardDraftSample"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    queryDashboardCatalog: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["DashboardCatalogInput"];
            };
        };
        responses: {
            /** @description Operation result. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["DashboardCatalogResult"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    executeDashboardDrilldown: {
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
                "application/json": components["schemas"]["DashboardDrilldownInput"];
            };
        };
        responses: {
            /** @description Operation result. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["DashboardDrilldownExecution"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    generateDashboardDrilldowns: {
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
                "application/json": components["schemas"]["DashboardGenerateDrilldownsInput"];
            };
        };
        responses: {
            /** @description Operation result. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["DashboardGeneratedDrilldowns"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    listHostDashboardBindings: {
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
                    "application/json": components["schemas"]["ResourceDashboardBindings"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    previewHostDashboardBinding: {
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
                "application/json": components["schemas"]["DashboardBindingInput"];
            };
        };
        responses: {
            /** @description Operation result. */
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
    listClusterDashboardBindings: {
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
                    "application/json": components["schemas"]["ResourceDashboardBindings"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    previewClusterDashboardBinding: {
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
                "application/json": components["schemas"]["DashboardBindingInput"];
            };
        };
        responses: {
            /** @description Operation result. */
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
    listDashboardBindings: {
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
                    "application/json": components["schemas"]["DashboardBindingView"][];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    convertDashboardPanel: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CsrfToken"];
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["DashboardConvertPanelInput"];
            };
        };
        responses: {
            /** @description Atomic conversion result. Unsupported conversion returns the original panel. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["DashboardConvertedPanel"];
                };
            };
            default: components["responses"]["Error"];
        };
    };
    startDashboardQueryJob: {
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
                "application/json": components["schemas"]["DashboardQueryJobInput"];
            };
        };
        responses: {
            /** @description Persistent query state. Materialized files retain their original attempt and source. */
            202: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["DashboardQueryJobView"];
                };
            };
            403: components["responses"]["Error"];
            409: components["responses"]["Error"];
            default: components["responses"]["Error"];
        };
    };
    getDashboardQueryJob: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                id: string;
                job_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Persistent query state. Materialized files retain their original attempt and source. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["DashboardQueryJobView"];
                };
            };
            403: components["responses"]["Error"];
            409: components["responses"]["Error"];
            default: components["responses"]["Error"];
        };
    };
    cancelDashboardQueryJob: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CsrfToken"];
            };
            path: {
                id: string;
                job_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Persistent query state. Materialized files retain their original attempt and source. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["DashboardQueryJobView"];
                };
            };
            403: components["responses"]["Error"];
            409: components["responses"]["Error"];
            default: components["responses"]["Error"];
        };
    };
    resumeDashboardQueryJob: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": components["parameters"]["CsrfToken"];
            };
            path: {
                id: string;
                job_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["DashboardQueryResumeInput"];
            };
        };
        responses: {
            /** @description Persistent query state. Materialized files retain their original attempt and source. */
            202: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["DashboardQueryJobView"];
                };
            };
            403: components["responses"]["Error"];
            409: components["responses"]["Error"];
            default: components["responses"]["Error"];
        };
    };
}
