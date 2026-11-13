import type {
  KubernetesResourceReference,
  KubernetesInventoryObject,
} from "@argus/api-client";
import { asRecord } from "./resource-patch";
export function relatedResourceTypes(type: string): string[] | undefined {
  const groups: Record<string, string[]> = {
    controllerrevision: [
      "controllerrevision",
      "statefulset",
      "daemonset",
      "pod",
    ],
    statefulset: [
      "statefulset",
      "controllerrevision",
      "pod",
      "pvc",
      "pv",
      "service",
    ],
    daemonset: ["daemonset", "controllerrevision", "pod", "node"],
    role: ["role", "rolebinding", "serviceaccount"],
    clusterrole: [
      "clusterrole",
      "clusterrolebinding",
      "rolebinding",
      "serviceaccount",
    ],
    rolebinding: ["rolebinding", "role", "clusterrole", "serviceaccount"],
    clusterrolebinding: ["clusterrolebinding", "clusterrole", "serviceaccount"],
    serviceaccount: [
      "serviceaccount",
      "pod",
      "rolebinding",
      "clusterrolebinding",
      "role",
      "clusterrole",
    ],
    namespace: [
      "pod",
      "deployment",
      "statefulset",
      "daemonset",
      "job",
      "cronjob",
      "pvc",
      "resourcequota",
      "limitrange",
    ],
  };
  return groups[type];
}
export function visibleReferenceNames(
  object: KubernetesInventoryObject,
): string[] {
  const names = object.owners.map((owner) => owner.kind + "/" + owner.name);
  const role = asRecord(object.data.roleRef);
  if (role.name) names.push(String(role.kind) + "/" + String(role.name));
  for (const raw of Array.isArray(object.data.subjects)
    ? object.data.subjects
    : []) {
    const subject = asRecord(raw);
    if (subject.name)
      names.push(
        String(subject.kind) +
          "/" +
          (subject.namespace ? String(subject.namespace) + "/" : "") +
          String(subject.name),
      );
  }
  return names;
}
export function ownerMatches(
  reference: KubernetesResourceReference,
  object: KubernetesInventoryObject,
) {
  return object.owners.some((owner) => owner.uid === reference.uid);
}
