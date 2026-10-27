package logcollection

import "testing"

func TestLogRulesCannotCollectPlatformRuntimeCredentials(t *testing.T) {
	for _, path := range []string{"/etc/argus-otelcol/enrollment-token", "/var/lib/argus-otelcol/identity/client-key.pem", "/var/lib/argus-connector/collector/private.json", "/var/run/argus-bootstrap/identity/client-key.pem", "/var/run/secrets/kubernetes.io/serviceaccount/token", `C:\ProgramData\Argus\Collector\identity\client-key.pem`, `d:/programdata/ARGUS/Connector/identity.json`} {
		if !IsReservedRuntimePath(path) {
			t.Fatalf("runtime path not protected: %s", path)
		}
	}
	for _, path := range []string{"/var/log/app.log", "/srv/orders/logs/events.log", `C:\inetpub\logs\LogFiles\W3SVC1\access.log`} {
		if IsReservedRuntimePath(path) {
			t.Fatalf("ordinary log path rejected: %s", path)
		}
	}
	if _, err := Normalize("filelog", "linux_amd64", []byte(`{"scope":"system","include":["/var/lib/argus-otelcol/identity/*.pem"]}`)); err == nil {
		t.Fatal("explicit credential file rule accepted")
	}
}
