package kubernetesworkbench

import (
	"encoding/json"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
)

type objectReference struct{ Type, Name, Namespace, UID string }

func record(value any) map[string]any { result, _ := value.(map[string]any); return result }
func objectReferences(object kubernetesinventory.Object) []objectReference {
	var data map[string]any
	if json.Unmarshal(object.Data, &data) != nil {
		return nil
	}
	refs := []objectReference{}
	spec := record(data["spec"])
	add := func(kind string, name any, namespace string) {
		if value, ok := name.(string); ok && value != "" {
			refs = append(refs, objectReference{Type: kind, Name: value, Namespace: namespace})
		}
	}
	switch object.Definition.Key {
	case "statefulset":
		add("service", spec["serviceName"], object.Namespace)
	case "ingress":
		add("ingressclass", spec["ingressClassName"], "")
		for _, raw := range slice(spec["tls"]) {
			add("secret", record(raw)["secretName"], object.Namespace)
		}
	case "serviceaccount":
		for _, key := range []string{"secrets", "imagePullSecrets"} {
			for _, raw := range slice(data[key]) {
				add("secret", record(raw)["name"], object.Namespace)
			}
		}
	case "pv":
		add("storageclass", spec["storageClassName"], "")
	}
	podSpec := spec
	switch object.Definition.Key {
	case "pod":
	case "deployment", "statefulset", "daemonset", "replicaset", "job":
		podSpec = record(record(spec["template"])["spec"])
	case "cronjob":
		podSpec = record(record(record(record(spec["jobTemplate"])["spec"])["template"])["spec"])
	default:
		return refs
	}
	add("serviceaccount", podSpec["serviceAccountName"], object.Namespace)
	for _, raw := range slice(podSpec["imagePullSecrets"]) {
		add("secret", record(raw)["name"], object.Namespace)
	}
	for _, group := range []string{"containers", "initContainers", "ephemeralContainers"} {
		for _, raw := range slice(podSpec[group]) {
			container := record(raw)
			for _, key := range []string{"envFrom", "env"} {
				for _, entry := range slice(container[key]) {
					value := record(entry)
					if key == "env" {
						value = record(value["valueFrom"])
					}
					for _, ref := range []struct{ key, kind string }{{"configMapRef", "configmap"}, {"configMapKeyRef", "configmap"}, {"secretRef", "secret"}, {"secretKeyRef", "secret"}} {
						add(ref.kind, record(value[ref.key])["name"], object.Namespace)
					}
				}
			}
		}
	}
	for _, raw := range slice(podSpec["volumes"]) {
		volume := record(raw)
		add("pvc", record(volume["persistentVolumeClaim"])["claimName"], object.Namespace)
		add("configmap", record(volume["configMap"])["name"], object.Namespace)
		add("secret", record(volume["secret"])["secretName"], object.Namespace)
		for _, source := range slice(record(volume["projected"])["sources"]) {
			for _, ref := range []struct{ key, kind string }{{"configMap", "configmap"}, {"secret", "secret"}} {
				add(ref.kind, record(record(source)[ref.key])["name"], object.Namespace)
			}
		}
		add("secret", record(record(volume["csi"])["nodePublishSecretRef"])["name"], object.Namespace)
		add("secret", record(record(volume["flexVolume"])["secretRef"])["name"], object.Namespace)
	}
	return refs
}
func slice(value any) []any { result, _ := value.([]any); return result }
