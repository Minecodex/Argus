import type { CollectionClaim } from "@argus/api-client";

export function activeCollectorProfileIds(
  claims: CollectionClaim[],
  collectorId: string | undefined,
  now = Date.now(),
): string[] {
  if (!collectorId) return [];
  return [
    ...new Set(
      claims
        .filter(
          (claim) =>
            claim.collector_id === collectorId &&
            claim.status === "active" &&
            (!claim.expires_at || Date.parse(claim.expires_at) > now),
        )
        .map((claim) => claim.profile_id)
        .filter((id): id is string => Boolean(id)),
    ),
  ];
}
