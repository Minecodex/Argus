package logcollection

import "regexp"

// ReservedRuntimePathPattern also guards the native receiver's resolved path.
// A log declaration must never become a platform identity/credential reader.
const ReservedRuntimePathPattern = `(?i)^(?:/(?:etc|var/lib)/argus(?:-otelcol|-connector|-bastion)?(?:/|$)|/(?:var/)?run/(?:secrets|argus[^/]*)(?:/|$)|[a-z]:[\\/]+ProgramData[\\/]+Argus(?:[\\/]|$))`

// Kubernetes-managed container files retain Pod/Owner and application scope
// through the dedicated k8slog entry. Generic host-file rules cannot own them.
const KubernetesContainerPathPattern = `^/var/log/(?:pods|containers)(?:/|$)`
const HostFileLogPathPattern = ReservedRuntimePathPattern + `|` + KubernetesContainerPathPattern

var kubernetesContainerPath = regexp.MustCompile(KubernetesContainerPathPattern)
var reservedRuntimePath = regexp.MustCompile(ReservedRuntimePathPattern)

func IsKubernetesContainerPath(value string) bool { return kubernetesContainerPath.MatchString(value) }

func KubernetesContainerExclusions() []string {
	return []string{"/var/log/pods/**", "/var/log/containers/**"}
}

func IsReservedRuntimePath(value string) bool { return reservedRuntimePath.MatchString(value) }

func ReservedRuntimeExclusions(windows bool) []string {
	if windows {
		return []string{"C:/ProgramData/Argus/**"}
	}
	return []string{"/etc/argus/**", "/etc/argus-otelcol/**", "/etc/argus-connector/**", "/etc/argus-bastion/**", "/var/lib/argus/**", "/var/lib/argus-otelcol/**", "/var/lib/argus-connector/**", "/var/lib/argus-bastion/**", "/run/secrets/**", "/var/run/secrets/**", "/run/argus*/**", "/var/run/argus*/**"}
}
