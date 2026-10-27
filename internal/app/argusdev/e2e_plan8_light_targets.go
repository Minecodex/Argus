package argusdev

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (a *App) preparePlan8LightTargets(ctx context.Context, env *E2EEnvironment, host *plan6HostLoop) (rows []plan8ExtraTarget, failure error) {
	if host == nil || host.Docker == nil || !filepath.IsAbs(env.WorkDir) {
		return rows, errors.New("owned target environment missing")
	}
	network := ""
	for _, n := range host.Docker.Networks {
		if n.Internal {
			network = n.Name
		}
	}
	if network == "" {
		return rows, errors.New("owned customer network missing")
	}
	owner := "plan8-light-" + env.Options.RunID
	save := func() error {
		public := []map[string]string{}
		for _, r := range rows {
			public = append(public, map[string]string{"owner": r.owner, "name": r.name, "id": r.id, "integration": r.engine, "image_id": r.imageID, "runtime_version": r.version})
		}
		body, _ := json.MarshalIndent(map[string]any{"network": network, "containers": public}, "", "  ")
		return writePrivate(filepath.Join(env.Options.Artifacts, "plan8-light-targets-ownership.json"), body)
	}
	apacheName := kubernetesNameForDev(owner + "-apache")
	for _, engine := range []string{"memcached", "apache", "haproxy", "zookeeper"} {
		row := plan8ExtraTarget{plan8DatabaseFixture: plan8DatabaseFixture{owner: owner, name: kubernetesNameForDev(owner + "-" + engine), engine: engine}}
		image, port, memory := "memcached:1.6-alpine", "11211", "64m"
		options := []string{}
		command := []string{}
		configPath, containerPath, configBody := "", "", ""
		switch engine {
		case "memcached":
			row.endpoint, row.metric = row.name+":11211", "memcached_bytes"
			command = []string{"memcached", "-m", "16", "-c", "32", "-t", "1"}
		case "apache":
			image, port, memory = "httpd:2.4-alpine", "8080", "96m"
			row.endpoint, row.metric = "http://"+row.name+":8080/server-status?auto", "apache_uptime"
			base, err := a.runner.Output(ctx, nil, "docker", "run", "--rm", "--label", "io.argus.e2e.run="+owner, "--entrypoint", "cat", image, "/usr/local/apache2/conf/httpd.conf")
			if err != nil || !strings.Contains(base, "LoadModule status_module modules/mod_status.so") {
				return rows, errors.New("Apache native status prerequisite missing")
			}
			configBody = strings.Replace(base, "Listen 80", "Listen 8080", 1) + "\nServerName localhost\nExtendedStatus On\n<Location /server-status>\nSetHandler server-status\nRequire all granted\n</Location>\n"
			containerPath = "/usr/local/apache2/conf/httpd.conf"
		case "haproxy":
			image, port, memory = "haproxy:3.2-alpine", "8080", "96m"
			row.endpoint, row.metric = "http://"+row.name+":8080/stats", "haproxy_sessions_count"
			row.password = "P8-" + strings.ReplaceAll(uuid.NewString(), "-", "")
			configBody = "global\n  maxconn 64\ndefaults\n  mode http\n  timeout connect 2s\n  timeout client 5s\n  timeout server 5s\nfrontend public\n  bind *:8081\n  default_backend applications\nbackend applications\n  server apache " + apacheName + ":8080 check\nlisten stats\n  bind *:8080\n  mode http\n  stats enable\n  stats uri /stats\n  stats auth monitor:" + row.password + "\n"
			containerPath = "/usr/local/etc/haproxy/haproxy.cfg"
			options = append(options, "--user", "0")
		case "zookeeper":
			image, port, memory = "zookeeper:3.9", "2181", "384m"
			row.endpoint, row.metric = row.name+":2181", "zookeeper_connection_active"
			options = append(options, "--env", "JVMFLAGS=-Xms64m -Xmx128m", "--env", "ZOO_4LW_COMMANDS_WHITELIST=mntr,ruok,stat")
		}
		if configBody != "" {
			configPath = filepath.Join(env.WorkDir, "plan8-light-"+engine+".conf")
			if err := os.WriteFile(configPath, []byte(configBody), 0600); err != nil {
				return rows, err
			}
			options = append(options, "--mount", "type=bind,source="+configPath+",target="+containerPath+",readonly")
		}
		args := []string{"run", "--detach", "--name", row.name, "--label", "io.argus.e2e.run=" + owner, "--network", network, "--memory=" + memory, "--cpus=.5"}
		args = append(args, options...)
		args = append(args, image)
		args = append(args, command...)
		rows = append(rows, row)
		if err := save(); err != nil {
			return rows, err
		}
		id, err := a.runner.Output(ctx, nil, "docker", args...)
		if err != nil {
			return rows, fmt.Errorf("owned %s target start failed", engine)
		}
		row.id = strings.TrimSpace(id)
		rows[len(rows)-1] = row
		if err = save(); err != nil {
			return rows, err
		}
		probe := func() (string, error) {
			if engine == "apache" {
				return a.runner.Output(ctx, nil, "docker", "exec", row.id, "wget", "-q", "-O", "-", "http://127.0.0.1:8080/server-status?auto")
			}
			if engine == "zookeeper" {
				return a.runner.Output(ctx, nil, "docker", "exec", row.id, "bash", "-c", `exec 3<>/dev/tcp/127.0.0.1/2181; printf stat >&3; IFS= read -r -t 2 line <&3; printf '%s\n' "$line"`)
			}
			message := "version\r\n"
			targetPort := port
			if engine == "haproxy" {
				targetPort = "8080"
				message = "GET /stats HTTP/1.0\r\nHost: localhost\r\nAuthorization: Basic " + base64.StdEncoding.EncodeToString([]byte("monitor:"+row.password)) + "\r\nConnection: close\r\n\r\n"
			}
			var output bytes.Buffer
			err := a.runner.RunIO(ctx, nil, strings.NewReader(message), &output, io.Discard, "docker", "exec", "-i", row.id, "sh", "-c", "exec nc -w 2 127.0.0.1 "+targetPort)
			if engine == "haproxy" && !strings.Contains(output.String(), "200 OK") {
				return "", errors.New("native HAProxy authenticated status unavailable")
			}
			return output.String(), err
		}
		deadline := time.Now().Add(60 * time.Second)
		var response string
		for {
			response, err = probe()
			valid := err == nil && len(response) > 0
			if engine == "memcached" {
				valid = valid && strings.Contains(response, "VERSION ")
			}
			if engine == "zookeeper" {
				valid = valid && strings.Contains(strings.ToLower(response), "zookeeper version")
			}
			if valid {
				break
			}
			if time.Now().After(deadline) {
				return rows, fmt.Errorf("owned %s native endpoint timed out", engine)
			}
			if err = waitContext(ctx, 250*time.Millisecond); err != nil {
				return rows, err
			}
		}
		versionText := response
		if engine == "apache" || engine == "haproxy" {
			program, flag := "httpd", "-v"
			if engine == "haproxy" {
				program = "haproxy"
			}
			var stdout, stderr bytes.Buffer
			if err = a.runner.RunIO(ctx, nil, nil, &stdout, &stderr, "docker", "exec", row.id, program, flag); err != nil {
				return rows, errors.New("target executable version unavailable")
			}
			versionText = stdout.String() + "\n" + stderr.String()
		}
		pattern := regexp.MustCompile(`(?i)(?:version[ :]+|apache/)([0-9]+\.[0-9]+(?:\.[0-9]+)?[^\s,]*)`)
		match := pattern.FindStringSubmatch(versionText)
		if len(match) != 2 {
			return rows, fmt.Errorf("owned %s runtime version missing", engine)
		}
		row.version = match[1]
		if row.password != "" && strings.Contains(row.version, row.password) {
			return rows, errors.New("private material in version observation")
		}
		imageID, err := a.runner.Output(ctx, nil, "docker", "inspect", "--format", "{{.Image}}", row.id)
		if err != nil || !strings.HasPrefix(strings.TrimSpace(imageID), "sha256:") {
			return rows, errors.New("target image identity missing")
		}
		row.imageID = strings.TrimSpace(imageID)
		rows[len(rows)-1] = row
		if err = save(); err != nil {
			return rows, err
		}
	}
	return rows, nil
}
