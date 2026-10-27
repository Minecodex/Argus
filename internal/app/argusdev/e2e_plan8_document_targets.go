package argusdev

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (a *App) plan8TargetCurlHTTP(ctx context.Context, target, port, method, path, user, password string, body any) (int, []byte, error) {
	config := "url = " + strconv.Quote("http://127.0.0.1:"+port+path) + "\nuser = " + strconv.Quote(user+":"+password) + "\nrequest = " + strconv.Quote(method) + "\nheader = \"Content-Type: application/json\"\ninclude\nsilent\nmax-time = 3\n"
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		config += "data = " + strconv.Quote(string(data)) + "\n"
	}
	var output bytes.Buffer
	if err := a.runner.RunIO(ctx, nil, strings.NewReader(config), &output, io.Discard, "docker", "exec", "-i", target, "curl", "--config", "-"); err != nil {
		return 0, nil, errors.New("owned target HTTP request unavailable")
	}
	response, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(output.Bytes())), nil)
	if err != nil {
		return 0, nil, errors.New("owned target HTTP response invalid")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 65537))
	if err != nil || len(data) > 65536 {
		return 0, nil, errors.New("owned target HTTP response incomplete")
	}
	return response.StatusCode, data, nil
}

func (a *App) preparePlan8DocumentTarget(ctx context.Context, env *E2EEnvironment, host *plan6HostLoop, engine string) (row plan8ExtraTarget, failure error) {
	if host == nil || host.Docker == nil || !filepath.IsAbs(env.WorkDir) || engine != "mongodb" && engine != "elasticsearch" {
		return row, errors.New("owned document target environment invalid")
	}
	network := ""
	for _, n := range host.Docker.Networks {
		if n.Internal {
			network = n.Name
		}
	}
	if network == "" {
		return row, errors.New("owned customer network missing")
	}
	row.plan8DatabaseFixture = plan8DatabaseFixture{owner: "plan8-" + engine + "-" + env.Options.RunID, name: kubernetesNameForDev("p8-" + engine + "-" + env.Options.RunID), engine: engine, password: "Monitor-P8-" + strings.ReplaceAll(uuid.NewString(), "-", "")}
	rootPassword := "Root-P8-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	image, memory, environment := "mongo:7.0.16", "768m", "MONGO_INITDB_ROOT_USERNAME=bootstrap\nMONGO_INITDB_ROOT_PASSWORD="+rootPassword+"\n"
	command := []string{"--wiredTigerCacheSizeGB", "0.25"}
	row.endpoint, row.metric = row.name+":27017", "mongodb_connection_count"
	row.config = map[string]any{"interval": "5s", "timeout": "5s", "auth_source": "admin", "direct_connection": true}
	if engine == "elasticsearch" {
		image, memory = "docker.elastic.co/elasticsearch/elasticsearch:8.17.0", "1536m"
		environment = "discovery.type=single-node\nxpack.security.enabled=true\nxpack.security.http.ssl.enabled=false\nxpack.security.transport.ssl.enabled=false\nES_JAVA_OPTS=-Xms512m -Xmx512m\nELASTIC_PASSWORD=" + rootPassword + "\n"
		command = nil
		row.endpoint, row.metric = "http://"+row.name+":9200", "elasticsearch_cluster_nodes"
		row.config = nil
	}
	private := filepath.Join(env.WorkDir, "plan8-"+engine+".env")
	if err := os.WriteFile(private, []byte(environment), 0600); err != nil {
		return row, err
	}
	save := func() error {
		body, _ := json.MarshalIndent(map[string]any{"owner": row.owner, "network": network, "name": row.name, "id": row.id, "engine": row.engine, "image_id": row.imageID, "runtime_version": row.version}, "", "  ")
		return writePrivate(filepath.Join(env.Options.Artifacts, "plan8-"+engine+"-ownership.json"), body)
	}
	if err := save(); err != nil {
		return row, err
	}
	args := []string{"run", "--detach", "--name", row.name, "--label", "io.argus.e2e.run=" + row.owner, "--network", network, "--memory=" + memory, "--cpus=2", "--env-file", private, image}
	args = append(args, command...)
	id, err := a.runner.Output(ctx, nil, "docker", args...)
	if err != nil {
		return row, errors.New("owned document target start failed")
	}
	row.id = strings.TrimSpace(id)
	if err = save(); err != nil {
		return row, err
	}
	if engine == "mongodb" {
		run := func(root bool, script string, output io.Writer) error {
			auth := `db.getSiblingDB("admin").auth("monitor",process.env.P8_MONITOR_PASSWORD);`
			if root {
				auth = `db.getSiblingDB("admin").auth("bootstrap",process.env.P8_ROOT_PASSWORD);`
			}
			return a.runner.RunIO(ctx, map[string]string{"P8_ROOT_PASSWORD": rootPassword, "P8_MONITOR_PASSWORD": row.password}, nil, output, io.Discard, "docker", "exec", "--env", "P8_ROOT_PASSWORD", "--env", "P8_MONITOR_PASSWORD", row.id, "mongosh", "--quiet", "--norc", "--eval", auth+script)
		}
		deadline := time.Now().Add(2 * time.Minute)
		for run(true, `if(db.adminCommand({ping:1}).ok!==1)quit(9);`, io.Discard) != nil {
			if time.Now().After(deadline) {
				return row, errors.New("owned MongoDB readiness timed out")
			}
			if err = waitContext(ctx, 500*time.Millisecond); err != nil {
				return row, err
			}
		}
		if err = run(true, `db.getSiblingDB("admin").createUser({user:"monitor",pwd:process.env.P8_MONITOR_PASSWORD,roles:[{role:"clusterMonitor",db:"admin"}]});db.getSiblingDB("plan8").samples.insertMany([{value:1},{value:2}]);`, io.Discard); err != nil {
			return row, errors.New("MongoDB monitoring role setup failed")
		}
		var version bytes.Buffer
		if err = run(false, `print(db.version());`, &version); err != nil {
			return row, errors.New("MongoDB monitoring login or version unavailable")
		}
		row.version = strings.TrimSpace(version.String())
		if err = run(false, `try{db.getSiblingDB("plan8").samples.insertOne({value:3});quit(9);}catch(e){if(e.code!==13)quit(8);}`, io.Discard); err != nil {
			return row, errors.New("MongoDB did not explicitly deny a monitor business write")
		}
	} else {
		call := func(user, password, method, path string, body any) (int, []byte, error) {
			return a.plan8TargetCurlHTTP(ctx, row.id, "9200", method, path, user, password, body)
		}
		deadline := time.Now().Add(2 * time.Minute)
		for {
			status, _, e := call("elastic", rootPassword, http.MethodGet, "/_cluster/health", nil)
			if e == nil && status == 200 {
				break
			}
			if time.Now().After(deadline) {
				return row, errors.New("owned Elasticsearch readiness timed out")
			}
			if err = waitContext(ctx, 500*time.Millisecond); err != nil {
				return row, err
			}
		}
		steps := []struct {
			method, path string
			body         any
		}{
			{http.MethodPut, "/_security/role/plan8_monitor", map[string]any{"cluster": []string{"monitor"}, "indices": []any{map[string]any{"names": []string{"*"}, "privileges": []string{"monitor"}}}}},
			{http.MethodPut, "/_security/user/monitor", map[string]any{"password": row.password, "roles": []string{"plan8_monitor"}}},
			{http.MethodPut, "/plan8", map[string]any{"settings": map[string]any{"number_of_shards": 1, "number_of_replicas": 0}}},
			{http.MethodPost, "/plan8/_doc?refresh=true", map[string]any{"value": 1}},
		}
		for _, step := range steps {
			status, _, e := call("elastic", rootPassword, step.method, step.path, step.body)
			if e != nil || status < 200 || status >= 300 {
				return row, errors.New("Elasticsearch read-only role or actual document setup failed")
			}
		}
		status, data, e := call("monitor", row.password, http.MethodGet, "/", nil)
		if e != nil || status != 200 {
			return row, errors.New("Elasticsearch monitoring login unavailable")
		}
		var version struct {
			Version struct {
				Number string `json:"number"`
			} `json:"version"`
		}
		if json.Unmarshal(data, &version) != nil {
			return row, errors.New("Elasticsearch version response invalid")
		}
		row.version = version.Version.Number
		status, _, e = call("monitor", row.password, http.MethodPut, "/forbidden", map[string]any{})
		if e != nil || status != 403 {
			return row, errors.New("Elasticsearch did not explicitly deny monitor index creation")
		}
		status, _, e = call("monitor", "invalid-"+row.password, http.MethodGet, "/_cluster/health", nil)
		if e != nil || status != 401 {
			return row, errors.New("Elasticsearch wrong password was not rejected")
		}
	}
	if row.version == "" || len(row.version) > 200 || strings.Contains(row.version, row.password) || strings.Contains(row.version, rootPassword) {
		return row, errors.New("owned document target version missing or invalid")
	}
	imageID, err := a.runner.Output(ctx, nil, "docker", "inspect", "--format", "{{.Image}}", row.id)
	if err != nil || !strings.HasPrefix(strings.TrimSpace(imageID), "sha256:") {
		return row, errors.New("document target image identity unavailable")
	}
	row.imageID = strings.TrimSpace(imageID)
	return row, save()
}
