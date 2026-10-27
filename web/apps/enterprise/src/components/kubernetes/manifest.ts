// Server-owned identity stays in the frozen request context, outside editable
// YAML. Merge-patch preserves omitted fields; redacted values stay markers.
export function editableKubernetesManifest(
  data: Record<string, unknown>,
): Record<string, unknown> {
  const value = structuredClone(data);
  delete value.status;
  const metadata = value.metadata as Record<string, unknown> | undefined;
  if (metadata)
    for (const key of [
      "uid",
      "resourceVersion",
      "generation",
      "creationTimestamp",
      "managedFields",
      "selfLink",
    ])
      delete metadata[key];
  return value;
}
