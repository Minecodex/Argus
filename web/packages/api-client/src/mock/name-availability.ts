import { ApiError } from "../transport/errors";
import type { MockDb } from "./store";

function normalizeResourceName(name: string): string {
  const normalized = name.trim();
  if (!normalized || Array.from(normalized).length > 128) {
    throw new ApiError(
      {
        code: "INVALID_ARGUMENT",
        message_key: "errors.common.invalid_argument",
        params: { field: "name" },
        request_id: `mock-request-${Date.now()}`,
        retryable: false,
      },
      400,
    );
  }
  return normalized;
}

export function resourceNameAvailable(
  db: MockDb,
  enterpriseId: string,
  name: string,
  includeBastions = false,
): boolean {
  const normalized = normalizeResourceName(name).toLowerCase();
  const conflicts = (entry: {
    enterpriseId: string;
    name: string;
    status?: string;
  }) =>
    entry.enterpriseId === enterpriseId &&
    entry.status !== "deleted" &&
    entry.name.toLowerCase() === normalized;
  return (
    !db.hosts.some(conflicts) &&
    (!includeBastions || !db.bastionScopes.some(conflicts))
  );
}

export function requireAvailableResourceName(
  db: MockDb,
  enterpriseId: string,
  name: string,
  includeBastions = false,
): string {
  const normalized = normalizeResourceName(name);
  if (resourceNameAvailable(db, enterpriseId, normalized, includeBastions))
    return normalized;
  throw new ApiError(
    {
      code: "RESOURCE_NAME_CONFLICT",
      message_key: "errors.common.resource_name_conflict",
      request_id: `mock-request-${Date.now()}`,
      retryable: false,
    },
    409,
  );
}
