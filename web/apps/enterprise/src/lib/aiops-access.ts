import { useMyPermissions } from "./permissions";

/** Navigation and settings tabs share the same capability boundary. */
export function useAiopsAccess() {
  const permissions = useMyPermissions();
  const has = (permission: string) =>
    permissions.has("*") || permissions.has(permission);
  const canViewModels =
    has("model.manage") || has("model.quota.manage") || has("model.usage.read");
  const canManageMCP = has("mcp_connection.manage");
  return {
    canViewModels,
    canManageMCP,
    canAccessAiops: canViewModels || canManageMCP,
  };
}
