import type { DashboardDraft } from "@argus/api-client";
function equal(a: unknown, b: unknown) {
  return JSON.stringify(a) === JSON.stringify(b);
}
function object(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === "object" && !Array.isArray(value);
}
function merge(base: unknown, remote: unknown, local: unknown): unknown {
  if (equal(base, local)) return remote;
  if (equal(base, remote)) return local;
  if (
    Array.isArray(base) &&
    Array.isArray(remote) &&
    Array.isArray(local) &&
    [...base, ...remote, ...local].every(
      (v) => object(v) && typeof v.id === "string",
    )
  ) {
    const previous = new Map(base.map((v) => [v.id, v])),
      server = new Map(remote.map((v) => [v.id, v]));
    return [
      ...local.map((v) => merge(previous.get(v.id), server.get(v.id) ?? v, v)),
      ...remote.filter(
        (v) => !previous.has(v.id) && !local.some((item) => item.id === v.id),
      ),
    ];
  }
  if (object(base) && object(remote) && object(local))
    return Object.fromEntries(
      [...new Set([...Object.keys(remote), ...Object.keys(local)])].map(
        (key) => [key, merge(base[key], remote[key], local[key])],
      ),
    );
  return local;
}
/** Keep unsaved local fields while adopting the server's new revision and generated definitions. */
export function mergeDraftMutation(
  base: DashboardDraft,
  remote: DashboardDraft,
  local: DashboardDraft,
): DashboardDraft {
  return {
    ...remote,
    name: merge(base.name, remote.name, local.name) as string,
    description: merge(
      base.description,
      remote.description,
      local.description,
    ) as string,
    folder_id: merge(
      base.folder_id,
      remote.folder_id,
      local.folder_id,
    ) as DashboardDraft["folder_id"],
    proposed_bindings: merge(
      base.proposed_bindings,
      remote.proposed_bindings,
      local.proposed_bindings,
    ) as DashboardDraft["proposed_bindings"],
    spec: merge(base.spec, remote.spec, local.spec) as DashboardDraft["spec"],
  };
}
