package kubernetesinventory

import (
	"encoding/json"
	corev1 "k8s.io/api/core/v1"
	resourcehelper "k8s.io/component-helpers/resource"
	"strconv"
)

func podRequestSummary(raw json.RawMessage, fields map[string]string) {
	var pod corev1.Pod
	if json.Unmarshal(raw, &pod) != nil {
		return
	}
	requests := resourcehelper.PodRequests(&pod, resourcehelper.PodResourcesOptions{})
	fields["cpu_request_millicores"] = strconv.FormatInt(requests.Cpu().MilliValue(), 10)
	fields["memory_request_bytes"] = strconv.FormatInt(requests.Memory().Value(), 10)
}
