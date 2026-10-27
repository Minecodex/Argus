package kubernetesinventory

import "slices"

var definitions = []Definition{
	{"namespace", "", "v1", "namespaces", "Namespace", false, "cluster", "kubernetes.namespace.manage"},
	{"node", "", "v1", "nodes", "Node", false, "cluster", "kubernetes.node.manage"},
	{"pod", "", "v1", "pods", "Pod", true, "workload", "kubernetes.workload.manage"},
	{"deployment", "apps", "v1", "deployments", "Deployment", true, "workload", "kubernetes.workload.manage"},
	{"statefulset", "apps", "v1", "statefulsets", "StatefulSet", true, "workload", "kubernetes.workload.manage"},
	{"daemonset", "apps", "v1", "daemonsets", "DaemonSet", true, "workload", "kubernetes.workload.manage"},
	{"replicaset", "apps", "v1", "replicasets", "ReplicaSet", true, "workload", "kubernetes.workload.manage"},
	{"controllerrevision", "apps", "v1", "controllerrevisions", "ControllerRevision", true, "workload", "kubernetes.workload.manage"},
	{"job", "batch", "v1", "jobs", "Job", true, "workload", "kubernetes.workload.manage"},
	{"cronjob", "batch", "v1", "cronjobs", "CronJob", true, "workload", "kubernetes.workload.manage"},
	{"hpa", "autoscaling", "v2", "horizontalpodautoscalers", "HorizontalPodAutoscaler", true, "autoscaling", "kubernetes.workload.manage"},
	{"pdb", "policy", "v1", "poddisruptionbudgets", "PodDisruptionBudget", true, "workload", "kubernetes.workload.manage"},
	{"service", "", "v1", "services", "Service", true, "network", "kubernetes.network.manage"},
	{"endpointslice", "discovery.k8s.io", "v1", "endpointslices", "EndpointSlice", true, "network", "kubernetes.network.manage"},
	{"ingress", "networking.k8s.io", "v1", "ingresses", "Ingress", true, "network", "kubernetes.network.manage"},
	{"ingressclass", "networking.k8s.io", "v1", "ingressclasses", "IngressClass", false, "network", "kubernetes.network.manage"},
	{"networkpolicy", "networking.k8s.io", "v1", "networkpolicies", "NetworkPolicy", true, "network", "kubernetes.network.manage"},
	{"configmap", "", "v1", "configmaps", "ConfigMap", true, "configuration", "kubernetes.configuration.manage"},
	{"secret", "", "v1", "secrets", "Secret", true, "configuration", "kubernetes.configuration.manage"},
	{"serviceaccount", "", "v1", "serviceaccounts", "ServiceAccount", true, "rbac", "kubernetes.rbac.manage"},
	{"role", "rbac.authorization.k8s.io", "v1", "roles", "Role", true, "rbac", "kubernetes.rbac.manage"},
	{"rolebinding", "rbac.authorization.k8s.io", "v1", "rolebindings", "RoleBinding", true, "rbac", "kubernetes.rbac.manage"},
	{"clusterrole", "rbac.authorization.k8s.io", "v1", "clusterroles", "ClusterRole", false, "rbac", "kubernetes.rbac.manage"},
	{"clusterrolebinding", "rbac.authorization.k8s.io", "v1", "clusterrolebindings", "ClusterRoleBinding", false, "rbac", "kubernetes.rbac.manage"},
	{"pvc", "", "v1", "persistentvolumeclaims", "PersistentVolumeClaim", true, "storage", "kubernetes.storage.manage"},
	{"pv", "", "v1", "persistentvolumes", "PersistentVolume", false, "storage", "kubernetes.storage.manage"},
	{"storageclass", "storage.k8s.io", "v1", "storageclasses", "StorageClass", false, "storage", "kubernetes.storage.manage"},
	{"pdb", "policy", "v1", "poddisruptionbudgets", "PodDisruptionBudget", true, "workload", "kubernetes.workload.manage"},
	{"resourcequota", "", "v1", "resourcequotas", "ResourceQuota", true, "configuration", "kubernetes.configuration.manage"},
	{"limitrange", "", "v1", "limitranges", "LimitRange", true, "configuration", "kubernetes.configuration.manage"},
	{"event", "", "v1", "events", "Event", true, "events", ""},
	{"crd", "apiextensions.k8s.io", "v1", "customresourcedefinitions", "CustomResourceDefinition", false, "extension", "kubernetes.crd.manage"},
}

func Definitions() []Definition { return slices.Clone(definitions) }
func Lookup(key string) (Definition, bool) {
	for _, d := range definitions {
		if d.Key == key {
			return d, true
		}
	}
	return Definition{}, false
}
