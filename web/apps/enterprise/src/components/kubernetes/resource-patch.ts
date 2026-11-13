export type JsonRecord = Record<string, unknown>;
export function asRecord(value: unknown): JsonRecord {
  return value !== null && typeof value === "object" && !Array.isArray(value)
    ? (value as JsonRecord)
    : {};
}
const same = (a: unknown, b: unknown) =>
  JSON.stringify(a) === JSON.stringify(b);
export function resourcePatch(
  before: JsonRecord,
  after: JsonRecord,
): JsonRecord {
  const patch: JsonRecord = {};
  for (const key of new Set([...Object.keys(before), ...Object.keys(after)])) {
    if (same(before[key], after[key])) continue;
    if (!(key in after)) {
      patch[key] = null;
      continue;
    }
    if (
      after[key] !== null &&
      typeof after[key] === "object" &&
      !Array.isArray(after[key])
    )
      patch[key] = resourcePatch(asRecord(before[key]), asRecord(after[key]));
    else patch[key] = after[key];
  }
  return patch;
}
export class ResourceRebaseConflict extends Error {
  constructor(public path: string) {
    super(path);
  }
}
// No write is issued here. A conflicting edit requires the user's explicit
// choice before adopting a new resourceVersion and producing a new preview.
export function rebaseResource(
  before: unknown,
  draft: unknown,
  current: unknown,
  keepMine = false,
  path = "$",
): unknown {
  if (same(before, draft)) return structuredClone(current);
  if (same(draft, current)) return structuredClone(current);
  if (before !== undefined && current === undefined && !keepMine)
    throw new ResourceRebaseConflict(path);
  if (Array.isArray(before) && Array.isArray(draft) && Array.isArray(current)) {
    const named = (rows: unknown[]) =>
      rows.every((row) => typeof asRecord(row).name === "string") &&
      new Set(rows.map((row) => asRecord(row).name)).size === rows.length;
    if (named(before) && named(draft) && named(current)) {
      const old = new Map(before.map((v) => [asRecord(v).name, v])),
        next = new Map(draft.map((v) => [asRecord(v).name, v]));
      const result: unknown[] = [];
      for (const item of current) {
        const name = asRecord(item).name;
        if (old.has(name) && !next.has(name)) {
          if (!same(old.get(name), item) && !keepMine)
            throw new ResourceRebaseConflict(path + "." + String(name));
          continue;
        }
        result.push(
          next.has(name)
            ? rebaseResource(
                old.get(name),
                next.get(name),
                item,
                keepMine,
                path + "." + String(name),
              )
            : structuredClone(item),
        );
      }
      for (const item of draft) {
        const name = asRecord(item).name;
        if (!current.some((v) => asRecord(v).name === name)) {
          if (old.has(name) && !same(old.get(name), item) && !keepMine)
            throw new ResourceRebaseConflict(path + "." + String(name));
          if (!old.has(name) || !same(old.get(name), item))
            result.push(structuredClone(item));
        }
      }
      return result;
    }
  }
  if (draft !== null && typeof draft === "object" && !Array.isArray(draft)) {
    const base = asRecord(before),
      edited = asRecord(draft),
      fresh = { ...asRecord(current) };
    for (const key of new Set([...Object.keys(base), ...Object.keys(edited)])) {
      if (same(base[key], edited[key])) continue;
      if (!(key in edited)) {
        if (!same(base[key], fresh[key]) && !keepMine)
          throw new ResourceRebaseConflict(path + "." + key);
        delete fresh[key];
      } else
        fresh[key] = rebaseResource(
          base[key],
          edited[key],
          fresh[key],
          keepMine,
          path + "." + key,
        );
    }
    return fresh;
  }
  if (!same(before, current) && !keepMine)
    throw new ResourceRebaseConflict(path);
  return structuredClone(draft);
}

/** RFC 7396 merge-patch: omitted fields remain unchanged; null deletes. */
export function applyResourcePatch(
  base: JsonRecord,
  patch: JsonRecord,
): JsonRecord {
  const result = structuredClone(base);
  for (const [key, value] of Object.entries(patch)) {
    if (value === null) delete result[key];
    else if (value && typeof value === "object" && !Array.isArray(value))
      result[key] = applyResourcePatch(asRecord(result[key]), asRecord(value));
    else result[key] = structuredClone(value);
  }
  return result;
}
