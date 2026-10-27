package argusdev

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPlan8ActualProbeTargetVersionsAndCleanup(t *testing.T) {
	image := os.Getenv("ARGUS_PLAN8_PROBE_RUNTIME_IMAGE")
	if os.Getenv("ARGUS_PLAN8_EXTRA_FIXTURES") != "1" || image == "" {
		t.Skip("explicit owned probe runtime required")
	}
	root, _ := filepath.Abs(filepath.Join("..", "..", ".."))
	a := App{root: root, runner: Runner{Dir: root, Stdout: io.Discard, Stderr: io.Discard}}
	owner := "plan8-probe-fixture-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	network, err := a.runner.Output(t.Context(), nil, "docker", "network", "create", "--internal", "--label", "argus.owner="+owner, owner)
	if err != nil {
		t.Fatal(err)
	}
	network = strings.TrimSpace(network)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		actual, e := a.runner.Output(ctx, nil, "docker", "network", "inspect", "--format", `{{index .Labels "argus.owner"}}`, network)
		if e != nil || strings.TrimSpace(actual) != owner {
			t.Error("owned probe network changed")
			return
		}
		if e = a.runner.RunIO(ctx, nil, nil, io.Discard, io.Discard, "docker", "network", "rm", network); e != nil {
			t.Error(e)
		}
	})
	env := &E2EEnvironment{Root: root, WorkDir: t.TempDir(), Options: E2EOptions{RunID: owner, Artifacts: t.TempDir()}, State: NewScenarioState(owner)}
	env.State.FixtureImages["remote-runtime"] = image
	host := &plan6HostLoop{Docker: &plan6DockerFixture{}}
	raw, _ := json.Marshal(map[string]any{"networks": []map[string]any{{"name": owner, "id": network, "internal": true}}})
	if err = json.Unmarshal(raw, host.Docker); err != nil {
		t.Fatal(err)
	}
	rows, err := a.preparePlan8ProbeTargets(t.Context(), env, host)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		targets := []plan8DatabaseFixture{}
		for _, row := range rows {
			targets = append(targets, row.Target.plan8DatabaseFixture)
		}
		if e := a.cleanupPlan8Databases(ctx, targets); e != nil {
			t.Error(e)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 5 {
		t.Fatal("actual probe target set incomplete")
	}
	for _, row := range rows {
		if row.Target.version == "" || !strings.HasPrefix(row.Target.imageID, "sha256:") {
			t.Fatal("actual probe target identity missing")
		}
		if row.Target.engine == "http" || row.Target.engine == "tcp" || row.Target.engine == "tls" {
			script := `import urllib.request,socket,ssl; assert urllib.request.urlopen('http://127.0.0.1:8080').status==200; s=ssl._create_unverified_context().wrap_socket(socket.create_connection(('127.0.0.1',8443)),server_hostname='localhost'); assert s.getpeercert(binary_form=True); s.close()`
			deadline := time.Now().Add(15 * time.Second)
			for {
				e := a.runner.RunIO(t.Context(), nil, nil, io.Discard, io.Discard, "docker", "exec", row.Target.id, "python3", "-c", script)
				if e == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("actual probe HTTP/TLS listener did not become available")
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
	}
	script := `import socket,struct; host='reachable.plan8.test'; labels=b''.join(bytes([len(x)])+x.encode() for x in host.split('.'))+b'\0'; q=struct.pack('!HHHHHH',13,256,1,0,0,0)+labels+struct.pack('!HH',1,1); s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM); s.settimeout(3); s.sendto(q,('` + rows[3].Target.name + `',1053)); r,_=s.recvfrom(4096); assert struct.unpack('!H',r[6:8])[0]>=1 and socket.inet_aton('192.0.2.42') in r`
	if err = a.runner.RunIO(t.Context(), nil, nil, io.Discard, io.Discard, "docker", "exec", rows[0].Target.id, "python3", "-c", script); err != nil {
		t.Fatal("actual DNS response did not match the owned zone")
	}
	body, err := os.ReadFile(filepath.Join(env.Options.Artifacts, "plan8-probe-targets-ownership.json"))
	if err != nil {
		t.Fatal(err)
	}
	if output := os.Getenv("ARGUS_PLAN8_PROBE_VERSION_EVIDENCE"); output != "" {
		if err = os.WriteFile(output, body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Log("actual HTTP/TCP/TLS listeners, certificate, DNS answer, peer IP and runtime identities observed; full Collector admission and query gates remain separate")
}
