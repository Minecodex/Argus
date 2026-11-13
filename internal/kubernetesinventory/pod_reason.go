package kubernetesinventory

// Keep a container failure visible when another container is still starting.
// Equal severity preserves native order, so refreshes do not arbitrarily switch
// the displayed cause. Per-container conditions remain available in detail.
func podReasonPriority(reason string) int {
	switch reason {
	case "":
		return 0
	case "ContainerCreating", "PodInitializing":
		return 1
	case "CrashLoopBackOff", "ImagePullBackOff", "ErrImagePull", "CreateContainerConfigError", "CreateContainerError", "RunContainerError", "InvalidImageName", "Error", "OOMKilled":
		return 3
	default:
		return 2
	}
}
