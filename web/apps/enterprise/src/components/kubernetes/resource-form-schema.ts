import type { StructuredField } from "@argus/ui";
type Translate = (key: string) => string;
export function resourceFormSchema(
  kind: string,
  t: Translate,
): StructuredField[] {
  const field = (
    key: string,
    label: string,
    kind: StructuredField["kind"] = "text",
    fields?: StructuredField[],
  ): StructuredField => ({
    key,
    label: t("kubernetesResources." + label),
    kind,
    fields,
  });
  const raw = (
    key: string,
    kind: StructuredField["kind"] = "text",
    fields?: StructuredField[],
  ): StructuredField => ({
    key,
    label: t("kubernetesResources.schema." + key),
    kind,
    fields,
  });
  const strings = (key: string): StructuredField => ({
    ...raw(key, "list"),
    itemKind: "text",
  });
  const quantity = [
    field("cpu", "cpuRequest"),
    field("memory", "memoryRequest"),
  ];
  const resources = raw("resources", "object", [
    raw("requests", "object", quantity),
    raw("limits", "object", [
      field("cpu", "cpuLimit"),
      field("memory", "memoryLimit"),
    ]),
  ]);
  const env = raw("env", "list", [
    raw("name"),
    raw("value"),
    raw("valueFrom", "object", [
      raw("configMapKeyRef", "object", [raw("name"), raw("key")]),
      raw("secretKeyRef", "object", [raw("name"), raw("key")]),
    ]),
  ]);
  const containerPorts = raw("ports", "list", [
    raw("name"),
    raw("containerPort", "number"),
    {
      ...raw("protocol", "select"),
      options: ["TCP", "UDP", "SCTP"].map((value) => ({ value, label: value })),
    },
  ]);
  const containers = field("containers", "containers", "list", [
    raw("name"),
    { ...field("image", "image"), required: true },
    resources,
    env,
    strings("command"),
    strings("args"),
    containerPorts,
  ]);
  const podFields = [
    containers,
    raw("initContainers", "list", [
      raw("name"),
      field("image", "image"),
      resources,
    ]),
    raw("nodeSelector", "map"),
    raw("serviceAccountName"),
    raw("automountServiceAccountToken", "boolean"),
  ];
  const selector = [
    raw("matchLabels", "map"),
    raw("matchExpressions", "list", [
      raw("key"),
      {
        ...raw("operator", "select"),
        options: ["In", "NotIn", "Exists", "DoesNotExist"].map((value) => ({
          value,
          label: value,
        })),
      },
      strings("values"),
    ]),
  ];
  const podTemplate = raw("template", "object", [
    raw("metadata", "object", [
      field("labels", "labels", "map"),
      { ...field("annotations", "annotations", "map"), multiline: true },
    ]),
    raw("spec", "object", podFields),
  ]);
  const ports = [
    raw("name"),
    raw("port", "number"),
    raw("targetPort", "int_or_string"),
    {
      ...raw("protocol", "select"),
      options: ["TCP", "UDP", "SCTP"].map((value) => ({ value, label: value })),
    },
    raw("nodePort", "number"),
  ];
  const subject = [raw("kind"), raw("name"), raw("namespace"), raw("apiGroup")];
  const metadata = raw("metadata", "object", [
    raw("name"),
    field("labels", "labels", "map"),
    { ...field("annotations", "annotations", "map"), multiline: true },
  ]);
  let spec: StructuredField[] = [];
  switch (kind) {
    case "pod":
      spec = podFields;
      break;
    case "deployment":
    case "statefulset":
    case "daemonset":
    case "replicaset":
      spec = [
        ...(kind !== "daemonset"
          ? [field("replicas", "fields.desired_replicas", "number")]
          : []),
        raw("selector", "object", selector),
        podTemplate,
      ];
      if (kind === "deployment")
        spec.push(
          raw("strategy", "object", [
            {
              ...raw("type", "select"),
              options: ["RollingUpdate", "Recreate"].map((value) => ({
                value,
                label: value,
              })),
            },
            raw("rollingUpdate", "object", [
              raw("maxSurge", "int_or_string"),
              raw("maxUnavailable", "int_or_string"),
            ]),
          ]),
        );
      if (kind === "statefulset")
        spec.push(
          raw("serviceName"),
          raw("updateStrategy", "object", [
            {
              ...raw("type", "select"),
              options: ["RollingUpdate", "OnDelete"].map((value) => ({
                value,
                label: value,
              })),
            },
            raw("rollingUpdate", "object", [raw("partition", "number")]),
          ]),
        );
      break;
    case "job":
      spec = [
        raw("suspend", "boolean"),
        raw("parallelism", "number"),
        raw("completions", "number"),
        raw("backoffLimit", "number"),
        raw("activeDeadlineSeconds", "number"),
        podTemplate,
      ];
      break;
    case "cronjob":
      spec = [
        field("schedule", "fields.schedule"),
        field("timeZone", "fields.timezone"),
        {
          ...field("concurrencyPolicy", "fields.concurrency", "select"),
          options: ["Allow", "Forbid", "Replace"].map((value) => ({
            value,
            label: value,
          })),
        },
        raw("suspend", "boolean"),
        raw("successfulJobsHistoryLimit", "number"),
        raw("failedJobsHistoryLimit", "number"),
        raw("jobTemplate", "object", [
          raw("spec", "object", [raw("backoffLimit", "number"), podTemplate]),
        ]),
      ];
      break;
    case "service":
      spec = [
        {
          ...field("type", "fields.service_type", "select"),
          options: [
            "ClusterIP",
            "NodePort",
            "LoadBalancer",
            "ExternalName",
          ].map((value) => ({ value, label: value })),
        },
        field("selector", "selector", "map"),
        raw("externalName"),
        field("ports", "fields.ports", "list", ports),
      ];
      break;
    case "ingress":
      spec = [
        field("ingressClassName", "fields.ingress_class"),
        raw("rules", "list", [
          raw("host"),
          raw("http", "object", [
            raw("paths", "list", [
              raw("path"),
              {
                ...raw("pathType", "select"),
                options: ["Prefix", "Exact", "ImplementationSpecific"].map(
                  (value) => ({ value, label: value }),
                ),
              },
              raw("backend", "object", [
                raw("service", "object", [
                  raw("name"),
                  raw("port", "object", [raw("name"), raw("number", "number")]),
                ]),
              ]),
            ]),
          ]),
        ]),
        raw("tls", "list", [strings("hosts"), raw("secretName")]),
      ];
      break;
    case "ingressclass":
      spec = [field("controller", "fields.controller")];
      break;
    case "hpa":
      spec = [
        field("minReplicas", "fields.min_replicas", "number"),
        field("maxReplicas", "fields.max_replicas", "number"),
        raw("scaleTargetRef", "object", [
          raw("apiVersion"),
          raw("kind"),
          raw("name"),
        ]),
        raw("metrics", "list", [
          raw("type"),
          raw("resource", "object", [
            raw("name"),
            raw("target", "object", [
              raw("type"),
              raw("averageUtilization", "number"),
              raw("averageValue"),
              raw("value"),
            ]),
          ]),
        ]),
      ];
      break;
    case "pdb":
      spec = [
        raw("minAvailable", "int_or_string"),
        raw("maxUnavailable", "int_or_string"),
        raw("selector", "object", selector),
      ];
      break;
    case "networkpolicy": {
      const peers = [
        raw("podSelector", "object", selector),
        raw("namespaceSelector", "object", selector),
        raw("ipBlock", "object", [raw("cidr"), strings("except")]),
      ];
      const policyPorts = [
        raw("protocol"),
        raw("port", "int_or_string"),
        raw("endPort", "number"),
      ];
      spec = [
        raw("podSelector", "object", selector),
        strings("policyTypes"),
        raw("ingress", "list", [
          raw("from", "list", peers),
          raw("ports", "list", policyPorts),
        ]),
        raw("egress", "list", [
          raw("to", "list", peers),
          raw("ports", "list", policyPorts),
        ]),
      ];
      break;
    }
    case "pvc":
      spec = [
        field("storageClassName", "fields.storage_class"),
        strings("accessModes"),
        raw("resources", "object", [
          raw("requests", "object", [field("storage", "targetCapacity")]),
        ]),
      ];
      break;
    case "resourcequota":
      spec = [raw("hard", "map"), strings("scopes")];
      break;
    case "limitrange":
      spec = [
        raw("limits", "list", [
          raw("type"),
          raw("min", "map"),
          raw("max", "map"),
          raw("default", "map"),
          raw("defaultRequest", "map"),
        ]),
      ];
      break;
    case "namespace":
      return [metadata];
    case "configmap":
      return [metadata, { ...field("data", "data", "map"), multiline: true }];
    case "secret":
      return [metadata, field("type", "fields.configuration_type")];
    case "role":
    case "clusterrole":
      return [
        metadata,
        raw("rules", "list", [
          strings("apiGroups"),
          strings("resources"),
          strings("verbs"),
          strings("resourceNames"),
          strings("nonResourceURLs"),
        ]),
      ];
    case "rolebinding":
    case "clusterrolebinding":
      return [
        metadata,
        raw("roleRef", "object", [raw("apiGroup"), raw("kind"), raw("name")]),
        raw("subjects", "list", subject),
      ];
    case "serviceaccount":
      return [
        metadata,
        raw("automountServiceAccountToken", "boolean"),
        raw("imagePullSecrets", "list", [raw("name")]),
      ];
    default:
      return [metadata];
  }
  return [metadata, raw("spec", "object", spec)];
}
