const fields = [
  "status",
  "connection_status",
  "severity",
  "level",
  "environment",
  "namespace",
  "resource_type",
  "service_name",
  "signal",
  "type",
];
const sorts = [
  "name",
  "title",
  "hostname",
  "id",
  "status",
  "severity",
  "namespace",
  "environment",
  "updated_at",
  "created_at",
  "started_at",
  "timestamp",
];
const scalar = (value: unknown) =>
  typeof value === "string" ||
  typeof value === "number" ||
  typeof value === "boolean"
    ? String(value)
    : "";
const searchable = (value: unknown, depth = 0): string => {
  if (depth > 5) return "";
  if (Array.isArray(value))
    return value.map((item) => searchable(item, depth + 1)).join(" ");
  if (value && typeof value === "object")
    return Object.entries(value)
      .filter(([key]) => !key.startsWith("_"))
      .map(([, item]) => searchable(item, depth + 1))
      .join(" ");
  return scalar(value);
};
export function filterStoredRows(
  rows: unknown[],
  parameters: Record<string, unknown>,
) {
  const search = parameters.search ?? "",
    sort = parameters.sort ?? "",
    direction = parameters.direction ?? "asc";
  if (
    typeof search !== "string" ||
    search.length > 256 ||
    typeof sort !== "string" ||
    (sort && !sorts.includes(sort)) ||
    !["asc", "desc"].includes(String(direction))
  )
    throw new Error("PRESENTATION_READ_INVALID");
  const filters = parameters.filters ?? {};
  if (
    !filters ||
    typeof filters !== "object" ||
    Array.isArray(filters) ||
    Object.entries(filters).some(
      ([key, value]) =>
        !fields.includes(key) ||
        typeof value !== "string" ||
        value.length > 256,
    )
  )
    throw new Error("PRESENTATION_READ_INVALID");
  const objects = rows as Record<string, unknown>[];
  const facets: Record<string, string[]> = {};
  for (const key of fields) {
    const values = [
      ...new Set(
        objects
          .map((row) => scalar(row?.[key]))
          .filter((value) => value && value.length <= 256),
      ),
    ].sort();
    if (values.length && values.length <= 50) facets[key] = values;
  }
  const items = objects.filter(
    (row) =>
      (!search ||
        searchable(row).toLowerCase().includes(search.trim().toLowerCase())) &&
      Object.entries(filters).every(
        ([key, value]) => !value || scalar(row?.[key]) === value,
      ),
  );
  if (sort)
    items.sort((a, b) => {
      if (typeof a?.[sort] === "number" && typeof b?.[sort] === "number")
        return (
          (Number(a[sort]) - Number(b[sort])) * (direction === "desc" ? -1 : 1)
        );
      const left = scalar(a?.[sort]),
        right = scalar(b?.[sort]);
      return (
        (left < right ? -1 : left > right ? 1 : 0) *
        (direction === "desc" ? -1 : 1)
      );
    });
  return { items, facets };
}
