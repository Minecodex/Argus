package collectormanager

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNativePostgresAndHTTPNetworkCertificateProbes(t *testing.T) {
	f := newNativeResourceFixture(t)
	password := "plan8-literal-${env:HOME}-$native"
	postgres := f.start("postgres", "--memory=256m", "--cpus=1", "-e", "POSTGRES_PASSWORD="+password, "postgres:17", "-c", "shared_buffers=32MB", "-c", "max_connections=30")
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, err := f.command("SELECT 1;", "exec", "-i", "-e", "PGPASSWORD="+password, postgres, "psql", "-h", "127.0.0.1", "-U", "postgres"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("owned PostgreSQL did not become ready")
		}
		time.Sleep(300 * time.Millisecond)
	}
	f.must("CREATE ROLE monitor LOGIN PASSWORD '"+password+"'; GRANT pg_monitor TO monitor; CREATE TABLE plan8_samples(id integer); INSERT INTO plan8_samples SELECT generate_series(1,20); ANALYZE plan8_samples;", "exec", "-i", "-e", "PGPASSWORD="+password, postgres, "psql", "-h", "127.0.0.1", "-U", "postgres", "-v", "ON_ERROR_STOP=1")
	for _, sql := range []string{"CREATE TABLE unexpected_monitor_write(id integer);", "SELECT * FROM plan8_samples;"} {
		if _, err := f.command(sql, "exec", "-i", "-e", "PGPASSWORD="+password, postgres, "psql", "-h", "127.0.0.1", "-U", "monitor", "-d", "postgres", "-v", "ON_ERROR_STOP=1"); err == nil {
			t.Fatal("read-only pg_monitor identity accessed business rows or created a table")
		}
	}
	var httpStatus atomic.Int64
	httpStatus.Store(200)
	httpListener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(int(httpStatus.Load()))
		_, _ = w.Write([]byte("plan8 owned probe"))
	}))
	httpServer.Listener = httpListener
	httpServer.Start()
	t.Cleanup(httpServer.Close)
	tlsListener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	tlsServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	tlsServer.Listener = tlsListener
	tlsServer.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
	tlsServer.StartTLS()
	t.Cleanup(tlsServer.Close)
	f.add("postgresql", f.owner+"-postgres:5432", "postgres", map[string]string{"username": "monitor", "password": password})
	f.add("http", fmt.Sprintf("http://host.docker.internal:%d", httpListener.Addr().(*net.TCPAddr).Port), "", nil)
	f.add("tcp", fmt.Sprintf("host.docker.internal:%d", httpListener.Addr().(*net.TCPAddr).Port), "", nil)
	f.add("tls", fmt.Sprintf("host.docker.internal:%d", tlsListener.Addr().(*net.TCPAddr).Port), "", nil)
	first := f.run(1)
	samples := f.wait(first, 4)
	time.Sleep(5 * time.Second)
	wanted := [][]string{{"postgresql.backends", "postgresql.db_size", "postgresql.commits", "postgresql.rollbacks", "postgresql.blocks_read", "postgresql.connection.max"}, {"httpcheck.duration", "httpcheck.status"}, {"tcpcheck.duration", "tcpcheck.status"}, {"tlscheck.time_left"}}
	for i, collection := range f.collections {
		for _, name := range wanted[i] {
			if !slices.Contains(samples[collection.TargetID], name) {
				t.Fatalf("native %s omitted %s", collection.Integration, name)
			}
		}
	}
	waitValue := func(index int, name string, attrs map[string]string, expected float64) {
		t.Helper()
		deadline := time.Now().Add(12 * time.Second)
		for time.Now().Before(deadline) {
			if value, ok := f.capture.latestValue(f.collections[index].TargetID, name, attrs); ok && value == expected {
				return
			}
			time.Sleep(200 * time.Millisecond)
		}
		t.Fatalf("native %s %s did not reach actual value %v", f.collections[index].Integration, name, expected)
	}
	waitValue(1, "httpcheck.status", map[string]string{"http.status_class": "2xx"}, 1)
	waitValue(2, "tcpcheck.status", nil, 1)
	if value, ok := f.capture.latestValue(f.collections[3].TargetID, "tlscheck.time_left", nil); !ok || value-tlsServer.Certificate().NotAfter.Sub(time.Now()).Seconds() > 10 || value-tlsServer.Certificate().NotAfter.Sub(time.Now()).Seconds() < -10 {
		t.Fatal("certificate metric does not match actual certificate expiry")
	}
	// Self-signed certificate expiry is real certificate metadata. This receiver
	// is not a trust/CA validation probe; no TLS trust success is claimed here.
	f.save(os.Getenv("ARGUS_POSTGRES_PROBE_METRIC_EVIDENCE"), 1)
	for _, request := range f.capture.payloads() {
		if strings.Contains(string(request), password) {
			t.Fatal("native receiver leaked monitoring password")
		}
	}
	httpStatus.Store(503)
	waitValue(1, "httpcheck.status", map[string]string{"http.status_class": "5xx"}, 1)
	httpServer.Close()
	waitValue(2, "tcpcheck.status", nil, 0)
	deadline = time.Now().Add(10 * time.Second)
	for {
		snapshot := f.capture.snapshot()
		if slices.Contains(snapshot[f.collections[1].TargetID], "httpcheck.error") && slices.Contains(snapshot[f.collections[2].TargetID], "tcpcheck.error") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("actual connection errors missing native error metrics")
		}
		time.Sleep(200 * time.Millisecond)
	}
	// Restore the actual endpoint at the same identity before auth/lifecycle checks.
	restored, err := net.Listen("tcp", httpListener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	httpStatus.Store(200)
	recovery := httptest.NewUnstartedServer(httpServer.Config.Handler)
	recovery.Listener = restored
	recovery.Start()
	t.Cleanup(recovery.Close)
	waitValue(2, "tcpcheck.status", nil, 1)
	waitValue(1, "httpcheck.status", map[string]string{"http.status_class": "2xx"}, 1)
	f.must("", "stop", "--time", "1", first)
	f.capture.reset()
	f.credentials[f.collections[0].ID] = map[string]string{"username": "monitor", "password": "wrong-private-password"}
	second := f.run(2)
	f.wait(second, 3)
	time.Sleep(3 * time.Second)
	if f.capture.snapshot()[f.collections[0].TargetID] != nil {
		t.Fatal("wrong PostgreSQL password still yielded target metrics")
	}
	f.must("", "stop", "--time", "1", second)
	f.capture.reset()
	f.credentials[f.collections[0].ID] = map[string]string{"username": "monitor", "password": password}
	f.collections[1].Enabled = false
	f.collections[1].Revision = 2
	third := f.run(3)
	f.wait(third, 3)
	if f.capture.snapshot()[f.collections[1].TargetID] != nil || f.capture.snapshot()[f.collections[0].TargetID] == nil {
		t.Fatal("independent HTTP disable or PostgreSQL auth recovery failed")
	}
	f.must("", "stop", "--time", "1", third)
	f.capture.reset()
	for i := range f.collections {
		f.collections[i].Enabled = false
		f.collections[i].Revision++
	}
	fourth := f.run(4)
	time.Sleep(3 * time.Second)
	if len(f.capture.snapshot()) != 0 || f.must("", "inspect", "--format", "{{.State.Running}}", fourth) != "true" {
		logs, _ := f.command("", "logs", "--tail", "12", fourth)
		t.Fatalf("all-off lost runtime or retained target data: %s", logs)
	}
	t.Log("actual PostgreSQL 17 pg_monitor and native HTTP/TCP/certificate expiry: four targets, literal credentials, auth failure/recovery, independent disable and all-off runtime; no CA trust claim for certificate probe")
}
