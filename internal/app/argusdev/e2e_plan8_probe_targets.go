package argusdev

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type plan8ProbeFixture struct {
	Target   plan8ExtraTarget
	Identity map[string]any
}

func (a *App) preparePlan8ProbeTargets(ctx context.Context, env *E2EEnvironment, host *plan6HostLoop) (rows []plan8ProbeFixture, failure error) {
	if host == nil || host.Docker == nil || !filepath.IsAbs(env.WorkDir) || env.State == nil || env.State.FixtureImages["remote-runtime"] == "" {
		return rows, errors.New("owned probe target runtime missing")
	}
	network := ""
	for _, n := range host.Docker.Networks {
		if n.Internal {
			network = n.Name
		}
	}
	if network == "" {
		return rows, errors.New("owned probe customer network missing")
	}
	owner := "plan8-probes-" + env.Options.RunID
	save := func() error {
		public := []map[string]any{}
		for _, r := range rows {
			public = append(public, map[string]any{"owner": r.Target.owner, "name": r.Target.name, "id": r.Target.id, "integration": r.Target.engine, "image_id": r.Target.imageID, "runtime_version": r.Target.version, "identity": r.Identity})
		}
		body, _ := json.MarshalIndent(map[string]any{"network": network, "containers": public}, "", "  ")
		return writePrivate(filepath.Join(env.Options.Artifacts, "plan8-probe-targets-ownership.json"), body)
	}
	script := `import http.server, ssl, threading
class H(http.server.BaseHTTPRequestHandler):
 def do_GET(self):
  self.send_response(200); self.end_headers(); self.wfile.write(b'owned-plan8-probe')
 def log_message(self,*a): pass
httpd=http.server.ThreadingHTTPServer(('0.0.0.0',8080),H)
tlsd=http.server.ThreadingHTTPServer(('0.0.0.0',8443),H)
context=ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
context.minimum_version=ssl.TLSVersion.TLSv1_2
context.load_cert_chain('/server.pem','/server-key.pem')
tlsd.socket=context.wrap_socket(tlsd.socket,server_side=True)
threading.Thread(target=httpd.serve_forever,daemon=True).start()
tlsd.serve_forever()
`
	scriptPath := filepath.Join(env.WorkDir, "plan8-probe-server.py")
	if err := os.WriteFile(scriptPath, []byte(script), 0600); err != nil {
		return rows, err
	}
	for _, engine := range []string{"http", "tcp", "tls", "dns", "icmp"} {
		row := plan8ExtraTarget{plan8DatabaseFixture: plan8DatabaseFixture{owner: owner, name: kubernetesNameForDev(owner + "-" + engine), engine: engine}}
		image := env.State.FixtureImages["remote-runtime"]
		// The fixture registry has separate host and cluster addresses. Docker
		// runs on the host; reusing the Kubernetes pull address can attempt an
		// HTTPS pull from a local HTTP registry despite an existing local image.
		if local, ok := localE2ERegistryImage(image); ok {
			image = local
		}
		args := []string{"run", "--detach", "--name", row.name, "--label", "io.argus.e2e.run=" + owner, "--network", network, "--memory=96m", "--cpus=.5", "--cap-drop=ALL", "--user", "0"}
		identity := map[string]any{}
		if engine == "dns" {
			image = "coredns/coredns:1.12.4"
			corefile := filepath.Join(env.WorkDir, "plan8-probe-Corefile")
			if err := os.WriteFile(corefile, []byte(".:1053 {\n hosts {\n  192.0.2.42 reachable.plan8.test\n  ttl 1\n }\n}\n"), 0600); err != nil {
				return rows, err
			}
			args = append(args, "--cap-add=NET_BIND_SERVICE", "--mount", "type=bind,source="+corefile+",target=/Corefile,readonly", image, "-conf", "/Corefile")
			row.endpoint, row.metric = row.name+":1053", "dnscheck_status"
			identity["endpoint"] = row.endpoint
			identity["hostnames"] = []string{"reachable.plan8.test"}
			row.config = map[string]any{"interval": "5s", "timeout": "3s", "dns_network": "udp", "record_type": "A"}
		} else if engine == "icmp" {
			image = "alpine:3.21"
			args = append(args, image, "sleep", "600")
			row.metric = "ping_loss_ratio"
			row.config = map[string]any{"interval": "5s", "timeout": "3s", "ping_count": 2}
		} else {
			public, key, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				return rows, err
			}
			template := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: row.name}, DNSNames: []string{row.name, "localhost"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(90 * 24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
			der, err := x509.CreateCertificate(rand.Reader, template, template, public, key)
			if err != nil {
				return rows, err
			}
			private, err := x509.MarshalPKCS8PrivateKey(key)
			if err != nil {
				return rows, err
			}
			certPath, keyPath := filepath.Join(env.WorkDir, "plan8-probe-"+engine+".pem"), filepath.Join(env.WorkDir, "plan8-probe-"+engine+"-key.pem")
			if err = os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
				return rows, err
			}
			if err = os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private}), 0600); err != nil {
				return rows, err
			}
			args = append(args, "--mount", "type=bind,source="+scriptPath+",target=/server.py,readonly", "--mount", "type=bind,source="+certPath+",target=/server.pem,readonly", "--mount", "type=bind,source="+keyPath+",target=/server-key.pem,readonly", "--entrypoint", "python3", image, "-u", "/server.py")
			row.endpoint, row.metric = "http://"+row.name+":8080", "httpcheck_duration"
			if engine == "tcp" {
				row.endpoint, row.metric = row.name+":8080", "tcpcheck_status"
			}
			if engine == "tls" {
				row.endpoint, row.metric = row.name+":8443", "tlscheck_time_left"
			}
			identity["endpoint"] = row.endpoint
		}
		rows = append(rows, plan8ProbeFixture{Target: row, Identity: identity})
		if err := save(); err != nil {
			return rows, err
		}
		var outputID, startupError bytes.Buffer
		err := a.runner.RunIO(ctx, nil, nil, &outputID, &startupError, "docker", args...)
		if err != nil {
			_ = writePrivate(filepath.Join(env.Options.Artifacts, "plan8-probe-"+engine+"-start-error.log"), redactDiagnostic(startupError.Bytes()))
			return rows, fmt.Errorf("owned native %s probe target start failed; diagnostic recorded", engine)
		}
		row.id = strings.TrimSpace(outputID.String())
		rows[len(rows)-1].Target = row
		if err = save(); err != nil {
			return rows, err
		}
		if engine == "icmp" {
			raw, err := a.runner.Output(ctx, nil, "docker", "inspect", row.id)
			if err != nil {
				return rows, errors.New("actual ICMP peer identity missing")
			}
			var inspected []struct {
				NetworkSettings struct {
					Networks map[string]struct{ IPAddress string }
				}
			}
			if json.Unmarshal([]byte(raw), &inspected) != nil || len(inspected) != 1 {
				return rows, errors.New("actual ICMP peer network invalid")
			}
			row.endpoint = inspected[0].NetworkSettings.Networks[network].IPAddress
			if row.endpoint == "" {
				return rows, errors.New("actual ICMP peer IP missing")
			}
			identity["endpoint"] = row.endpoint
		}
		var output, stderr bytes.Buffer
		if engine == "dns" {
			err = a.runner.RunIO(ctx, nil, nil, &output, &stderr, "docker", "exec", row.id, "/coredns", "-version")
		} else if engine == "icmp" {
			err = a.runner.RunIO(ctx, nil, nil, &output, &stderr, "docker", "exec", row.id, "cat", "/etc/alpine-release")
		} else {
			err = a.runner.RunIO(ctx, nil, nil, &output, &stderr, "docker", "exec", row.id, "python3", "--version")
		}
		if err != nil || output.Len()+stderr.Len() == 0 || output.Len()+stderr.Len() > 512 {
			state, _ := a.runner.Output(ctx, nil, "docker", "inspect", "--format", "{{.State.Status}}|{{.State.ExitCode}}", row.id)
			var captured, errorsCaptured bytes.Buffer
			_ = a.runner.RunIO(ctx, nil, nil, &captured, &errorsCaptured, "docker", "logs", "--tail", "8", row.id)
			logs := captured.String() + errorsCaptured.String()
			return rows, fmt.Errorf("actual %s probe runtime unavailable state=%s logs=%s", engine, strings.TrimSpace(state), logs)
		}
		row.version = strings.TrimSpace(output.String() + stderr.String())
		imageID, err := a.runner.Output(ctx, nil, "docker", "inspect", "--format", "{{.Image}}", row.id)
		if err != nil || !strings.HasPrefix(strings.TrimSpace(imageID), "sha256:") {
			return rows, errors.New("actual probe image identity missing")
		}
		row.imageID = strings.TrimSpace(imageID)
		row.identity = identity
		rows[len(rows)-1] = plan8ProbeFixture{Target: row, Identity: identity}
		if err = save(); err != nil {
			return rows, err
		}
	}
	return rows, nil
}
