/** Generated from internal/kubernetesinventory. DO NOT EDIT. */
export const kubernetesInventoryTypes = [
  {
    "key": "namespace",
    "group": "",
    "version": "v1",
    "resource": "namespaces",
    "kind": "Namespace",
    "namespaced": false,
    "category": "cluster",
    "manage_permission": "kubernetes.namespace.manage"
  },
  {
    "key": "node",
    "group": "",
    "version": "v1",
    "resource": "nodes",
    "kind": "Node",
    "namespaced": false,
    "category": "cluster",
    "manage_permission": "kubernetes.node.manage"
  },
  {
    "key": "pod",
    "group": "",
    "version": "v1",
    "resource": "pods",
    "kind": "Pod",
    "namespaced": true,
    "category": "workload",
    "manage_permission": "kubernetes.workload.manage"
  },
  {
    "key": "deployment",
    "group": "apps",
    "version": "v1",
    "resource": "deployments",
    "kind": "Deployment",
    "namespaced": true,
    "category": "workload",
    "manage_permission": "kubernetes.workload.manage"
  },
  {
    "key": "statefulset",
    "group": "apps",
    "version": "v1",
    "resource": "statefulsets",
    "kind": "StatefulSet",
    "namespaced": true,
    "category": "workload",
    "manage_permission": "kubernetes.workload.manage"
  },
  {
    "key": "daemonset",
    "group": "apps",
    "version": "v1",
    "resource": "daemonsets",
    "kind": "DaemonSet",
    "namespaced": true,
    "category": "workload",
    "manage_permission": "kubernetes.workload.manage"
  },
  {
    "key": "replicaset",
    "group": "apps",
    "version": "v1",
    "resource": "replicasets",
    "kind": "ReplicaSet",
    "namespaced": true,
    "category": "workload",
    "manage_permission": "kubernetes.workload.manage"
  },
  {
    "key": "controllerrevision",
    "group": "apps",
    "version": "v1",
    "resource": "controllerrevisions",
    "kind": "ControllerRevision",
    "namespaced": true,
    "category": "workload",
    "manage_permission": "kubernetes.workload.manage"
  },
  {
    "key": "job",
    "group": "batch",
    "version": "v1",
    "resource": "jobs",
    "kind": "Job",
    "namespaced": true,
    "category": "workload",
    "manage_permission": "kubernetes.workload.manage"
  },
  {
    "key": "cronjob",
    "group": "batch",
    "version": "v1",
    "resource": "cronjobs",
    "kind": "CronJob",
    "namespaced": true,
    "category": "workload",
    "manage_permission": "kubernetes.workload.manage"
  },
  {
    "key": "hpa",
    "group": "autoscaling",
    "version": "v2",
    "resource": "horizontalpodautoscalers",
    "kind": "HorizontalPodAutoscaler",
    "namespaced": true,
    "category": "autoscaling",
    "manage_permission": "kubernetes.workload.manage"
  },
  {
    "key": "pdb",
    "group": "policy",
    "version": "v1",
    "resource": "poddisruptionbudgets",
    "kind": "PodDisruptionBudget",
    "namespaced": true,
    "category": "workload",
    "manage_permission": "kubernetes.workload.manage"
  },
  {
    "key": "service",
    "group": "",
    "version": "v1",
    "resource": "services",
    "kind": "Service",
    "namespaced": true,
    "category": "network",
    "manage_permission": "kubernetes.network.manage"
  },
  {
    "key": "endpointslice",
    "group": "discovery.k8s.io",
    "version": "v1",
    "resource": "endpointslices",
    "kind": "EndpointSlice",
    "namespaced": true,
    "category": "network",
    "manage_permission": "kubernetes.network.manage"
  },
  {
    "key": "ingress",
    "group": "networking.k8s.io",
    "version": "v1",
    "resource": "ingresses",
    "kind": "Ingress",
    "namespaced": true,
    "category": "network",
    "manage_permission": "kubernetes.network.manage"
  },
  {
    "key": "ingressclass",
    "group": "networking.k8s.io",
    "version": "v1",
    "resource": "ingressclasses",
    "kind": "IngressClass",
    "namespaced": false,
    "category": "network",
    "manage_permission": "kubernetes.network.manage"
  },
  {
    "key": "networkpolicy",
    "group": "networking.k8s.io",
    "version": "v1",
    "resource": "networkpolicies",
    "kind": "NetworkPolicy",
    "namespaced": true,
    "category": "network",
    "manage_permission": "kubernetes.network.manage"
  },
  {
    "key": "configmap",
    "group": "",
    "version": "v1",
    "resource": "configmaps",
    "kind": "ConfigMap",
    "namespaced": true,
    "category": "configuration",
    "manage_permission": "kubernetes.configuration.manage"
  },
  {
    "key": "secret",
    "group": "",
    "version": "v1",
    "resource": "secrets",
    "kind": "Secret",
    "namespaced": true,
    "category": "configuration",
    "manage_permission": "kubernetes.configuration.manage"
  },
  {
    "key": "serviceaccount",
    "group": "",
    "version": "v1",
    "resource": "serviceaccounts",
    "kind": "ServiceAccount",
    "namespaced": true,
    "category": "rbac",
    "manage_permission": "kubernetes.rbac.manage"
  },
  {
    "key": "role",
    "group": "rbac.authorization.k8s.io",
    "version": "v1",
    "resource": "roles",
    "kind": "Role",
    "namespaced": true,
    "category": "rbac",
    "manage_permission": "kubernetes.rbac.manage"
  },
  {
    "key": "rolebinding",
    "group": "rbac.authorization.k8s.io",
    "version": "v1",
    "resource": "rolebindings",
    "kind": "RoleBinding",
    "namespaced": true,
    "category": "rbac",
    "manage_permission": "kubernetes.rbac.manage"
  },
  {
    "key": "clusterrole",
    "group": "rbac.authorization.k8s.io",
    "version": "v1",
    "resource": "clusterroles",
    "kind": "ClusterRole",
    "namespaced": false,
    "category": "rbac",
    "manage_permission": "kubernetes.rbac.manage"
  },
  {
    "key": "clusterrolebinding",
    "group": "rbac.authorization.k8s.io",
    "version": "v1",
    "resource": "clusterrolebindings",
    "kind": "ClusterRoleBinding",
    "namespaced": false,
    "category": "rbac",
    "manage_permission": "kubernetes.rbac.manage"
  },
  {
    "key": "pvc",
    "group": "",
    "version": "v1",
    "resource": "persistentvolumeclaims",
    "kind": "PersistentVolumeClaim",
    "namespaced": true,
    "category": "storage",
    "manage_permission": "kubernetes.storage.manage"
  },
  {
    "key": "pv",
    "group": "",
    "version": "v1",
    "resource": "persistentvolumes",
    "kind": "PersistentVolume",
    "namespaced": false,
    "category": "storage",
    "manage_permission": "kubernetes.storage.manage"
  },
  {
    "key": "storageclass",
    "group": "storage.k8s.io",
    "version": "v1",
    "resource": "storageclasses",
    "kind": "StorageClass",
    "namespaced": false,
    "category": "storage",
    "manage_permission": "kubernetes.storage.manage"
  },
  {
    "key": "pdb",
    "group": "policy",
    "version": "v1",
    "resource": "poddisruptionbudgets",
    "kind": "PodDisruptionBudget",
    "namespaced": true,
    "category": "workload",
    "manage_permission": "kubernetes.workload.manage"
  },
  {
    "key": "resourcequota",
    "group": "",
    "version": "v1",
    "resource": "resourcequotas",
    "kind": "ResourceQuota",
    "namespaced": true,
    "category": "configuration",
    "manage_permission": "kubernetes.configuration.manage"
  },
  {
    "key": "limitrange",
    "group": "",
    "version": "v1",
    "resource": "limitranges",
    "kind": "LimitRange",
    "namespaced": true,
    "category": "configuration",
    "manage_permission": "kubernetes.configuration.manage"
  },
  {
    "key": "event",
    "group": "",
    "version": "v1",
    "resource": "events",
    "kind": "Event",
    "namespaced": true,
    "category": "events",
    "manage_permission": ""
  },
  {
    "key": "crd",
    "group": "apiextensions.k8s.io",
    "version": "v1",
    "resource": "customresourcedefinitions",
    "kind": "CustomResourceDefinition",
    "namespaced": false,
    "category": "extension",
    "manage_permission": "kubernetes.crd.manage"
  }
] as const;
