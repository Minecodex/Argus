import { dump, JSON_SCHEMA, load, YAMLException } from "js-yaml";

export type DocumentFormat = "yaml" | "json";
export type StructuredDocumentValue = {
  format: DocumentFormat;
  source: string;
};
export class StructuredDocumentError extends Error {
  constructor(
    public code: "syntax" | "limit" | "shape",
    public line?: number,
    public column?: number,
  ) {
    super(`DOCUMENT_${code.toUpperCase()}`);
  }
}

const maxBytes = 1 << 20;
function boundedObject(value: unknown): Record<string, unknown> {
  let nodes = 0;
  const ancestors = new Set<object>();
  function copy(value: unknown, depth: number): unknown {
    if (++nodes > 20000 || depth > 64)
      throw new StructuredDocumentError("limit");
    if (
      value === null ||
      typeof value === "string" ||
      typeof value === "boolean"
    )
      return value;
    if (typeof value === "number") {
      if (
        !Number.isFinite(value) ||
        (Number.isInteger(value) && !Number.isSafeInteger(value))
      )
        throw new StructuredDocumentError("shape");
      return value;
    }
    if (typeof value !== "object" || !value)
      throw new StructuredDocumentError("shape");
    if (ancestors.has(value)) throw new StructuredDocumentError("limit");
    if (
      !Array.isArray(value) &&
      Object.getPrototypeOf(value) !== Object.prototype &&
      Object.getPrototypeOf(value) !== null
    )
      throw new StructuredDocumentError("shape");
    ancestors.add(value);
    let result: unknown;
    if (Array.isArray(value))
      result = value.map((item) => copy(item, depth + 1));
    else {
      const object: Record<string, unknown> = Object.create(null);
      for (const [key, item] of Object.entries(value))
        object[key] = copy(item, depth + 1);
      result = object;
    }
    ancestors.delete(value);
    return result;
  }
  const result = copy(value, 0);
  if (!result || Array.isArray(result) || typeof result !== "object")
    throw new StructuredDocumentError("shape");
  if (new TextEncoder().encode(JSON.stringify(result)).length > maxBytes)
    throw new StructuredDocumentError("limit");
  return result as Record<string, unknown>;
}

// Only JSON-compatible values cross the API boundary. Duplicate keys, custom
// custom YAML types, multiple documents, cycles and excessive references are rejected.
// Error objects never retain parser snippets, original values or raw input.
export function parseStructuredDocument(
  source: string,
  format: DocumentFormat,
): Record<string, unknown> {
  if (new TextEncoder().encode(source).length > maxBytes)
    throw new StructuredDocumentError("limit");
  try {
    if (format === "json") JSON.parse(source);
    const options = {
      schema: JSON_SCHEMA,
      json: false,
      maxDepth: 64,
      maxTotalMergeKeys: 1024,
    };
    return boundedObject(load(source, options));
  } catch (error) {
    if (error instanceof StructuredDocumentError) throw error;
    if (error instanceof YAMLException)
      throw new StructuredDocumentError(
        "syntax",
        error.mark?.line === undefined ? undefined : error.mark.line + 1,
        error.mark?.column === undefined ? undefined : error.mark.column + 1,
      );
    throw new StructuredDocumentError("syntax");
  }
}

export function formatStructuredDocument(
  value: Record<string, unknown>,
  format: DocumentFormat,
): string {
  const object = boundedObject(value);
  return format === "json"
    ? JSON.stringify(object, null, 2)
    : dump(object, {
        schema: JSON_SCHEMA,
        noRefs: true,
        lineWidth: -1,
        quotingType: '"',
      });
}
