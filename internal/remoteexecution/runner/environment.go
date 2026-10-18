package runner

import (
	"os"
	"strings"
)

// The execution account supplies OS permissions, not the Connector process's
// secrets. Start interpreters with ordinary OS settings and explicit exchange
// variables; credentials and service configuration stay in the supervisor.
func commandEnvironment(kubernetes bool) []string {
	allowed := map[string]bool{}
	for _, key := range strings.Fields("PATH HOME USER LOGNAME SHELL LANG TZ TMP TEMP TMPDIR SYSTEMROOT WINDIR SYSTEMDRIVE USERPROFILE USERNAME USERDOMAIN HOMEDRIVE HOMEPATH COMSPEC PATHEXT COMPUTERNAME PROGRAMFILES PROGRAMFILES(X86) PROGRAMW6432 COMMONPROGRAMFILES PSModulePath") {
		allowed[strings.ToUpper(key)] = true
	}
	if kubernetes {
		allowed["KUBECONFIG"] = true
		allowed["ARGUS_KUBECTL_PROXY"] = true
	}
	result := []string{}
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		upper := strings.ToUpper(key)
		if ok && (allowed[upper] || strings.HasPrefix(upper, "LC_")) {
			result = append(result, entry)
		}
	}
	return result
}
