import { observationObject, observationRows } from "./observation-data";

export type TraceObservation = {
  row: Record<string, unknown>;
  key: string;
  depth: number;
  start: number;
  duration: number;
};
const identity = (row: Record<string, unknown>) =>
  JSON.stringify([row.sourceId, row.resourceId, row.spanId]);
// Resolve only unambiguous identities. Vendor service names are display labels, never join keys.
export function traceObservations(
  envelope: Record<string, unknown>,
): TraceObservation[] {
  const spans = observationRows(envelope.spans);
  const nodes = new Map(spans.map((row) => [identity(row), row]));
  const parents = new Map<string, string>();
  const edges = observationRows(envelope.edges);
  for (const row of spans) {
    const links = edges.filter(
      (edge) =>
        edge.childSpanId === row.spanId &&
        (edge.childSourceId === undefined ||
          edge.childSourceId === row.sourceId) &&
        (edge.childResourceId === undefined ||
          edge.childResourceId === row.resourceId) &&
        !edge.missingParent &&
        !edge.cycle,
    );
    const possible = links.length
      ? spans.filter((parent) =>
          links.some(
            (edge) =>
              edge.parentSpanId === parent.spanId &&
              (edge.parentSourceId === undefined
                ? parent.sourceId === row.sourceId
                : edge.parentSourceId === parent.sourceId) &&
              (edge.parentResourceId === undefined
                ? parent.resourceId === row.resourceId
                : edge.parentResourceId === parent.resourceId),
          ),
        )
      : spans.filter(
          (parent) =>
            parent.spanId === row.parentSpanId &&
            parent.sourceId === row.sourceId &&
            parent.resourceId === row.resourceId,
        );
    if (possible.length === 1 && possible[0] !== row)
      parents.set(identity(row), identity(possible[0]!));
  }
  const visited = new Set<string>(),
    result: TraceObservation[] = [];
  const sorted = [...nodes.keys()].sort(
    (a, b) =>
      Date.parse(String(nodes.get(a)!.startTime)) -
      Date.parse(String(nodes.get(b)!.startTime)),
  );
  const children = new Map<string, string[]>();
  for (const key of sorted) {
    const parent = parents.get(key);
    if (parent) {
      const items = children.get(parent) ?? [];
      items.push(key);
      children.set(parent, items);
    }
  }
  const append = (key: string, depth: number) => {
    const stack = [{ key, depth }];
    while (stack.length) {
      const node = stack.pop()!;
      if (visited.has(node.key)) continue;
      visited.add(node.key);
      const row = nodes.get(node.key)!;
      result.push({
        row,
        ...node,
        start: Date.parse(String(row.startTime)),
        duration: Number(row.duration),
      });
      for (const child of [...(children.get(node.key) ?? [])].reverse())
        stack.push({ key: child, depth: node.depth + 1 });
    }
  };
  for (const key of sorted) if (!parents.has(key)) append(key, 0);
  // Cycles or inconsistent fragments remain visible without inventing a hierarchy.
  for (const key of sorted) append(key, 0);
  return result;
}

export function decodedObservation(value: unknown): unknown {
  if (typeof value !== "string") return value;
  try {
    return JSON.parse(value) as unknown;
  } catch {
    return value;
  }
}

export function traceQuality(
  envelope: Record<string, unknown>,
): Record<string, unknown> {
  return Object.fromEntries(
    Object.entries(envelope).filter(
      ([key, value]) =>
        [
          "completeness",
          "missingParentCount",
          "ambiguousParentCount",
          "excludedSpanCount",
          "rootPresent",
          "rootCount",
        ].includes(key) && !observationObject(value),
    ),
  );
}
