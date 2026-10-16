const privateKey =
  /^(argus__token|api[_-]?key|password|secret|secret_value|private[_-]?key|authorization|cookie|csrf[_-]?token|access[_-]?token|refresh[_-]?token)$/i;
export function publicToolDetails(value: unknown, depth = 0): unknown {
  if (depth > 8) return "…";
  if (Array.isArray(value))
    return value.slice(0, 40).map((item) => publicToolDetails(item, depth + 1));
  if (value && typeof value === "object")
    return Object.fromEntries(
      Object.entries(value)
        .slice(0, 40)
        .map(([key, child]) => [
          key,
          privateKey.test(key)
            ? "[redacted]"
            : publicToolDetails(child, depth + 1),
        ]),
    );
  return typeof value === "string" && value.length > 2000
    ? value.slice(0, 2000) + "…"
    : value;
}
export function toolDetailsText(value: unknown): string {
  return (JSON.stringify(publicToolDetails(value), null, 2) ?? "").slice(
    0,
    12_000,
  );
}
