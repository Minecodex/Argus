import fs from "node:fs/promises";
import path from "node:path";
import { isDeepStrictEqual } from "node:util";
import { createHash } from "node:crypto";

const base = path.resolve(process.argv[2]);
const read = async (name) =>
  JSON.parse(
    (await fs.readFile(path.join(base, name), "utf8")).replace(/^\uFEFF/, ""),
  );
const requireFact = (condition, message) => {
  if (!condition) throw new Error(message);
};
const key = (value) =>
  [value.kind, value.metadata.namespace, value.metadata.name].join("/");
const images = await read("images.json");
const report = { passed: false, images, checks: [] };
const record = (name, facts) =>
  report.checks.push({ name, passed: true, ...facts });
try {
  const before = await read("business-before.json"),
    after = await read("business-after.json");
  for (const [entity, ids] of Object.entries(before)) {
    requireFact(
      (ids ?? []).every((id) => (after[entity] ?? []).includes(id)),
      `Original ${entity} records missing`,
    );
    record(`business/${entity}`, {
      before: (ids ?? []).length,
      after: (after[entity] ?? []).length,
      original_ids_preserved: true,
    });
  }
  const pvcAfter = new Map(
    (await read("pvc-after.json")).items.map((value) => [key(value), value]),
  );
  const pvcBefore = (await read("pvc-before.json")).items.filter((value) =>
    ["argus-system", "argus-sandbox"].includes(value.metadata.namespace),
  );
  for (const pvc of pvcBefore) {
    const current = pvcAfter.get(key(pvc));
    requireFact(
      current?.metadata.uid === pvc.metadata.uid &&
        current.spec.volumeName === pvc.spec.volumeName &&
        current.status.phase === "Bound",
      `PVC continuity failed ${key(pvc)}`,
    );
  }
  record("pvc/argus", {
    count: pvcBefore.length,
    uid_and_volume_preserved: true,
    all_bound: true,
  });
  const workloadsBefore = (await read("all-workloads-before.json")).items;
  const inventory = await read("platform-resource-inventory.json");
  const concurrentRollout = await read("concurrent-rollout-proof.json").catch(
    (error) => {
      if (error.code === "ENOENT") return undefined;
      throw error;
    },
  );
  const externalRollouts = [];
  const externalUpgrades = [];
  const concurrentUpgrade = await read(
    "concurrent-helm-upgrade-proof.json",
  ).catch((error) => {
    if (error.code === "ENOENT") return undefined;
    throw error;
  });
  const canonical = (value) =>
    Array.isArray(value)
      ? value.map(canonical)
      : value && typeof value === "object"
        ? Object.fromEntries(
            Object.keys(value)
              .sort()
              .map((name) => [name, canonical(value[name])]),
          )
        : value;
  const specHash = (value) =>
    createHash("sha256")
      .update(JSON.stringify(canonical(value)))
      .digest("hex");
  const withoutRestart = (spec) => {
    const copy = structuredClone(spec);
    const annotations = copy.template?.metadata?.annotations;
    if (annotations) {
      delete annotations["kubectl.kubernetes.io/restartedAt"];
      if (!Object.keys(annotations).length)
        delete copy.template.metadata.annotations;
    }
    return copy;
  };
  const workloadsAfter = new Map(
    (await read("all-workloads-after.json")).items.map((value) => [
      key(value),
      value,
    ]),
  );
  for (const old of workloadsBefore) {
    const current = workloadsAfter.get(key(old));
    requireFact(
      current?.metadata.uid === old.metadata.uid &&
        current.spec.replicas === old.spec.replicas,
      `Workload identity/replicas changed ${key(old)}`,
    );
    const managed = old.metadata.namespace === "argus-system";
    if (!managed && !isDeepStrictEqual(current.spec, old.spec)) {
      const outsideRelease = !inventory.some(
        (item) =>
          item.kind === old.kind &&
          item.namespace === old.metadata.namespace &&
          item.name === old.metadata.name,
      );
      const explainedUpgrade =
        concurrentUpgrade?.key === key(old) &&
        concurrentUpgrade.uid === old.metadata.uid &&
        concurrentUpgrade.release ===
          current.metadata.annotations?.["meta.helm.sh/release-name"] &&
        concurrentUpgrade.namespace === old.metadata.namespace &&
        outsideRelease &&
        concurrentUpgrade.before_spec_hash === specHash(old.spec) &&
        concurrentUpgrade.after_spec_hash === specHash(current.spec);
      if (explainedUpgrade) {
        externalUpgrades.push({
          workload: key(old),
          release: concurrentUpgrade.release,
          previous_revision: concurrentUpgrade.previous_revision,
          current_revision: concurrentUpgrade.current_revision,
          changed_fields: concurrentUpgrade.changed_fields,
          preserved_as_published_by_its_owner: true,
        });
      } else {
        // This observed, independently owned rollout occurred during acceptance.
        // Pin its exact identity and annotation; no other specification drift is accepted.
        const stamp =
          current.spec.template?.metadata?.annotations?.[
            "kubectl.kubernetes.io/restartedAt"
          ];
        requireFact(
          concurrentRollout?.key === key(old) &&
            concurrentRollout.uid === old.metadata.uid &&
            concurrentRollout.annotation === stamp &&
            concurrentRollout.helm_release ===
              current.metadata.annotations?.["meta.helm.sh/release-name"] &&
            concurrentRollout.helm_release !== "argus-platform" &&
            concurrentRollout.field_managers.some(
              (item) => item.manager === "kubectl-rollout",
            ) &&
            outsideRelease &&
            isDeepStrictEqual(
              withoutRestart(current.spec),
              withoutRestart(old.spec),
            ),
          `Unrelated workload specification changed ${key(old)}`,
        );
        externalRollouts.push({
          workload: key(old),
          annotation: stamp,
          field_manager: "kubectl-rollout",
          helm_release: concurrentRollout.helm_release,
          all_other_configuration_preserved: true,
        });
      }
    }
    if (
      ["argus-system", "argus-sandbox", "argus-ingress"].includes(
        old.metadata.namespace,
      )
    ) {
      requireFact(
        current.status.observedGeneration >= current.metadata.generation &&
          current.status.readyReplicas === current.spec.replicas,
        `Workload not ready ${key(old)}`,
      );
    }
    for (const container of old.spec.template.spec.containers) {
      const family = container.image.includes("/argus/argus-backend:")
        ? "backend"
        : container.image.includes("/argus/argus-web:")
          ? "web"
          : null;
      if (managed && family)
        requireFact(
          current.spec.template.spec.containers.find(
            (value) => value.name === container.name,
          )?.image === images[family],
          `Release image missing ${key(old)}`,
        );
    }
  }
  record("workloads", {
    count: workloadsBefore.length,
    original_ids_replicas_preserved: true,
    unrelated_changes_preserved: true,
    concurrent_rollout_annotations: externalRollouts,
    concurrent_helm_upgrades: externalUpgrades,
    argus_ready: true,
  });
  const secretsAfter = new Map(
    (await read("secret-uids-after.json")).map((value) => [
      value.name,
      value.uid,
    ]),
  );
  const secretsBefore = await read("secret-uids-before.json");
  for (const item of secretsBefore)
    requireFact(
      secretsAfter.get(item.name) === item.uid,
      `Secret identity changed ${item.name}`,
    );
  record("secrets", {
    count: secretsBefore.length,
    original_uids_preserved: true,
  });
  const merge = (target, patch) => {
    for (const [name, value] of Object.entries(patch)) {
      if (value && typeof value === "object" && !Array.isArray(value))
        merge((target[name] ??= {}), value);
      else target[name] = value;
    }
    return target;
  };
  const expected = merge(
    await read("private/platform-values-before.json"),
    await read("platform-image-overrides.json"),
  );
  requireFact(
    isDeepStrictEqual(
      expected,
      await read("private/platform-values-after.json"),
    ),
    "Unexpected Helm values change",
  );
  record("helm/configuration", { only_declared_image_overrides_changed: true });
  const allowed = new Map();
  for (const reference of Object.values(images)) {
    const [repositoryTag, digest] = reference.split("@");
    const repository = repositoryTag
      .replace(/^host\.docker\.internal:5001\//, "")
      .split(":")[0];
    const tag = repositoryTag.slice(repositoryTag.lastIndexOf(":") + 1);
    const response = await fetch(
      `http://127.0.0.1:5001/v2/${repository}/manifests/${tag}`,
      {
        headers: {
          Accept:
            "application/vnd.oci.image.index.v1+json,application/vnd.docker.distribution.manifest.list.v2+json",
        },
      },
    );
    requireFact(
      response.ok && response.headers.get("docker-content-digest") === digest,
      "Registry digest changed",
    );
    const index = await response.json(),
      platform = index.manifests.find(
        (value) =>
          value.platform?.os === "linux" &&
          value.platform?.architecture === "amd64",
      );
    requireFact(platform, "No linux/amd64 manifest");
    allowed.set(reference, [digest, platform.digest]);
  }
  let running = 0;
  for (const pod of (await read("pods-after.json")).items) {
    if (pod.status.phase !== "Running") continue;
    for (const container of pod.spec.containers) {
      if (!allowed.has(container.image)) continue;
      const status = pod.status.containerStatuses?.find(
        (value) => value.name === container.name,
      );
      requireFact(
        status?.ready &&
          allowed
            .get(container.image)
            .some((digest) => status.imageID.endsWith("@" + digest)),
        "Actual running image ID differs",
      );
      running++;
    }
  }
  requireFact(running === 9, "Expected eight backend and one web containers");
  record("images", {
    pinned_manifests: allowed.size,
    running_containers: running,
    actual_image_ids_checked: true,
  });
  const verify = await read("verify/verify.json");
  requireFact(
    verify.passed && verify.checks.every((value) => value.status === "pass"),
    "Formal health verification failed",
  );
  record("health", {
    checks_passed: verify.checks.length,
    total: verify.checks.length,
    local_profile_degradations: verify.degradations,
  });
  report.passed = true;
} finally {
  await fs.writeFile(
    path.join(base, "release-verification.json"),
    JSON.stringify(report, null, 2) + "\n",
  );
}
console.log(JSON.stringify(report, null, 2));
