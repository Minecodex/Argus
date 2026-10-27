import { z } from "zod";
import { parseStructuredDocument } from "@argus/ui";

export const kubernetesMutationFormSchema = z
  .object({
    operation: z.enum([
      "create",
      "patch",
      "delete",
      "scale",
      "restart",
      "rollback",
      "cordon",
      "uncordon",
      "drain",
    ]),
    manifest: z.object({
      format: z.enum(["yaml", "json"]),
      source: z.string(),
    }),
    replicas: z.string(),
  })
  .superRefine((value, context) => {
    if (value.operation === "create" || value.operation === "patch")
      try {
        parseStructuredDocument(value.manifest.source, value.manifest.format);
      } catch {
        context.addIssue({
          code: "custom",
          path: ["manifest", "source"],
          message: "kubernetesWorkbench.mutation.invalidManifest",
        });
      }
    if (
      value.operation === "scale" &&
      (!/^\d+$/.test(value.replicas) || Number(value.replicas) > 10000)
    )
      context.addIssue({
        code: "custom",
        path: ["replicas"],
        message: "kubernetesWorkbench.mutation.invalidReplicas",
      });
  });
