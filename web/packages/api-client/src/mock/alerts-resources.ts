import type { BaseContext } from "./context";
import { createSeedHost } from "./seed-helpers";

/** Alert fixtures refer to real mock inventory objects, through the same ACL. */
export function ensureAlertResources(ctx: BaseContext, at: string) {
  const enterpriseId = ctx.enterpriseId();
  const existing = ctx.db.hosts.find(
    (row) => row.enterpriseId === enterpriseId,
  );
  const owner = ctx.db.dataAuthorizationGrants?.find(
    (grant) =>
      grant.active &&
      grant.subject_type === "user" &&
      grant.resource_id === existing?.id,
  )?.subject_id;
  const grant = (
    resource_type: "host" | "kubernetes_cluster",
    resource_id: string,
  ) => {
    if (owner)
      (ctx.db.dataAuthorizationGrants ??= []).push({
        subject_type: "user",
        subject_id: owner,
        resource_type,
        resource_id,
        active: true,
      });
  };
  const host = createSeedHost(() => at);
  for (const [index, name] of [
    "web-prod-01",
    "web-prod-02",
    "db-prod-01",
  ].entries()) {
    if (
      ctx.db.hosts.some(
        (row) => row.enterpriseId === enterpriseId && row.name === name,
      )
    )
      continue;
    ctx.db.hosts.push(
      host({
        id: `alert-${enterpriseId}-${name}`,
        enterpriseId,
        name,
        address: `192.0.2.${index + 101}`,
        collectorStatus: "converged",
        labels: {
          env: "production",
          service: index === 2 ? "database" : "checkout",
        },
      }),
    );
    grant("host", `alert-${enterpriseId}-${name}`);
  }
  if (
    !ctx.db.clusters.some(
      (row) =>
        row.enterpriseId === enterpriseId && row.name === "k8s-production",
    )
  ) {
    ctx.db.clusters.push({
      id: `alert-${enterpriseId}-k8s-production`,
      enterpriseId,
      name: "k8s-production",
      apiServer: "https://alerts-cluster.example.invalid",
      connectionMode: "in_cluster",
      credentialRef: "",
      version: "1.33.0",
      environment: "production",
      labels: { env: "production" },
      connectionStatus: "connected",
      nodeCount: 3,
      readyNodeCount: 3,
      createdAt: at,
      updatedAt: at,
      resourceVersion: 1,
    });
    grant("kubernetes_cluster", `alert-${enterpriseId}-k8s-production`);
  }
}
