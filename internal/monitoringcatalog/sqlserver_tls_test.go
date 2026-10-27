package monitoringcatalog

import (
	"net/url"
	"testing"
)

func TestSQLServerTLSModeDoesNotSilentlyTrustCertificates(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		dsn := sqlServerDSN(TargetIdentity{Endpoint: "database:1433"}, Config{TLS: TLS{Enabled: enabled}}, Credentials{Username: "monitor", Password: "a:@!?/$"})
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatal(err)
		}
		password, _ := u.User.Password()
		if password != "a:@!?/$" {
			t.Fatal("driver credentials lost URI escaping")
		}
		want := "disable"
		if enabled {
			want = "true"
			if u.Query().Get("TrustServerCertificate") != "false" {
				t.Fatal("TLS connection may bypass server verification")
			}
		}
		if u.Query().Get("encrypt") != want {
			t.Fatal("native connection does not honor explicit TLS mode")
		}
	}
}
