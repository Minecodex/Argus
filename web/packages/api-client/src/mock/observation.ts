import type {
  ObservationDiscovery,
  ObservationSource,
} from "../observation";
import type { MockContext } from "./context";
import { mockDashboardScopeDirectory } from "./dashboard-scope-directory";
import { mockDashboardCatalog } from "./dashboard-catalog";
import { resolveObservationResources } from "./observation-access";
import { dashboardFailure as fail } from "./dashboard-state";

export function createObservationDiscovery(
  ctx: MockContext,
): ObservationDiscovery {
  return {
    resources: mockDashboardScopeDirectory(ctx, true),
    async catalog(input, signal) {
      signal?.throwIfAborted();
      const refs = resolveObservationResources(ctx, input.scope);
      const result = {
        metrics: [],
        sources: [],
        fields: [],
        values: [],
        has_more: false,
        complete: true,
        selected_exists: {},
        warnings: ["MOCK_DATA"],
      } as Awaited<ReturnType<ObservationDiscovery["catalog"]>>;
      if (!refs.length) return result;
      const types = [
        ...new Set(
          refs.flatMap((ref) =>
            input.signal !== "metrics"
              ? ["otlp"]
              : ref.type === "host"
                ? ["hostmetrics"]
                : ref.type.startsWith("kubernetes")
                  ? ["kubeletstats"]
                  : ref.type === "monitoring_target"
                    ? ["redis"]
                    : ["otlp"],
          ),
        ),
      ];
      const sources: ObservationSource[] = types
        .filter(
          (source) =>
            !input.scope.source_type || source === input.scope.source_type,
        )
        .map((source_type) => ({
          source_type,
          capability_version: "v1",
          keys: [],
          integration_keys: [source_type],
        }));
      if (input.kind === "sources") {
        result.sources = sources;
        return result;
      }
      if (!sources.length) return result;
      const { cursor, ...rest } = input;
      const context = JSON.stringify([
        ctx.enterpriseId(),
        ctx.actor().id,
        ctx.db.dataAuthorizationVersions,
        rest,
      ]);
      let backendCursor: string | undefined;
      if (cursor) {
        try {
          const c = JSON.parse(decodeURIComponent(cursor));
          if (c.context !== context) fail("QUERY_INVALID", 400);
          backendCursor = c.cursor;
        } catch {
          fail("QUERY_INVALID", 400);
        }
      }
      const data = mockDashboardCatalog({
        signal: input.signal,
        kind: input.kind,
        source_binding: {
          source_type: input.scope.source_type || sources[0]!.source_type,
          capability_version: "v1",
        },
        resource_ids: refs.map((r) => r.id),
        metric: input.metric,
        field: input.field,
        search: input.search,
        cursor: backendCursor,
        from: input.from,
        to: input.to,
        selected_values: input.selected_values ?? [],
        filters: input.filters ?? [],
        limit: input.limit,
      });
      result.fields = data.fields;
      result.values = data.values;
      result.selected_exists = data.selected_exists ?? {};
      result.complete = data.complete;
      result.metrics = data.metrics.map((m) => ({
        ...m,
        source_type: input.scope.source_type || sources[0]!.source_type,
        source_keys: [],
        description: "",
        observed: true,
        recipes: [
          {
            id: `mock/${m.name}`,
            version: 1,
            title: m.name,
            description: "",
            expression: m.name,
            unit: m.unit,
            source_type: input.scope.source_type || sources[0]!.source_type,
            integration_key: "mock",
            metrics: [m.name],
          },
        ],
      }));
      result.has_more = data.has_more;
      if (data.next_cursor)
        result.next_cursor = encodeURIComponent(
          JSON.stringify({ context, cursor: data.next_cursor }),
        );
      await ctx.pause();
      signal?.throwIfAborted();
      return result;
    },
  };
}
