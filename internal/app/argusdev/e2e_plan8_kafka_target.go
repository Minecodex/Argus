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
	"strings"
	"time"

	"github.com/google/uuid"
)

func (a *App) preparePlan8KafkaTarget(ctx context.Context, env *E2EEnvironment, host *plan6HostLoop) (row plan8ExtraTarget, failure error) {
	if host == nil || host.Docker == nil || !filepath.IsAbs(env.WorkDir) {
		return row, errors.New("owned Kafka environment missing")
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
	row.plan8DatabaseFixture = plan8DatabaseFixture{owner: "plan8-kafka-" + env.Options.RunID, name: kubernetesNameForDev("p8-kafka-" + env.Options.RunID), engine: "kafka", password: "MonitorP8" + strings.ReplaceAll(uuid.NewString(), "-", "")}
	row.endpoint, row.metric = row.name+":9092", "kafka_brokers"
	row.config = map[string]any{"interval": "5s", "timeout": "5s", "sasl_mechanism": "PLAIN", "topic_match": "^plan8-owned$", "group_match": "^plan8-consumers$"}
	properties := fmt.Sprintf(`process.roles=broker,controller
node.id=1
controller.listener.names=CONTROLLER
controller.quorum.bootstrap.servers=127.0.0.1:19093
listeners=INTERNAL://127.0.0.1:19092,EXTERNAL://0.0.0.0:9092,CONTROLLER://127.0.0.1:19093
advertised.listeners=INTERNAL://127.0.0.1:19092,EXTERNAL://%s:9092
listener.security.protocol.map=INTERNAL:PLAINTEXT,EXTERNAL:SASL_PLAINTEXT,CONTROLLER:PLAINTEXT
inter.broker.listener.name=INTERNAL
sasl.enabled.mechanisms=PLAIN
listener.name.external.plain.sasl.jaas.config=org.apache.kafka.common.security.plain.PlainLoginModule required user_monitor="%s";
authorizer.class.name=org.apache.kafka.metadata.authorizer.StandardAuthorizer
super.users=User:ANONYMOUS
allow.everyone.if.no.acl.found=false
log.dirs=/tmp/kafka-data
num.partitions=2
offsets.topic.replication.factor=1
transaction.state.log.replication.factor=1
transaction.state.log.min.isr=1
group.initial.rebalance.delay.ms=0
log.segment.bytes=1048576
log.retention.bytes=16777216
`, row.name, row.password)
	serverPath := filepath.Join(env.WorkDir, "plan8-kafka-server.properties")
	clientPath := filepath.Join(env.WorkDir, "plan8-kafka-monitor.properties")
	if err := os.WriteFile(serverPath, []byte(properties), 0600); err != nil {
		return row, err
	}
	if err := os.WriteFile(clientPath, []byte("security.protocol=SASL_PLAINTEXT\nsasl.mechanism=PLAIN\nsasl.jaas.config=org.apache.kafka.common.security.plain.PlainLoginModule required username=\"monitor\" password=\""+row.password+"\";\nrequest.timeout.ms=5000\ndefault.api.timeout.ms=5000\n"), 0600); err != nil {
		return row, err
	}
	clusterID := uuid.New()
	privateEnv := filepath.Join(env.WorkDir, "plan8-kafka.env")
	if err := os.WriteFile(privateEnv, []byte("KAFKA_HEAP_OPTS=-Xms128m -Xmx256m\nARGUS_TEST_CLUSTER="+base64.RawURLEncoding.EncodeToString(clusterID[:])+"\n"), 0600); err != nil {
		return row, err
	}
	save := func() error {
		body, _ := json.MarshalIndent(map[string]any{"owner": row.owner, "network": network, "name": row.name, "id": row.id, "engine": row.engine, "image_id": row.imageID, "runtime_version": row.version, "admin_listener": "container_loopback_only", "monitor_listener": "sasl_plain_read_only_metadata"}, "", "  ")
		return writePrivate(filepath.Join(env.Options.Artifacts, "plan8-kafka-ownership.json"), body)
	}
	if err := save(); err != nil {
		return row, err
	}
	id, err := a.runner.Output(ctx, nil, "docker", "run", "--detach", "--name", row.name, "--label", "io.argus.e2e.run="+row.owner, "--network", network, "--memory=768m", "--cpus=1", "--env-file", privateEnv, "--mount", "type=bind,source="+serverPath+",target=/config.properties,readonly", "--mount", "type=bind,source="+clientPath+",target=/monitor.properties,readonly", "--entrypoint", "/bin/sh", "apache/kafka:4.2.1", "-ec", `/opt/kafka/bin/kafka-storage.sh format --standalone --ignore-formatted --cluster-id="$ARGUS_TEST_CLUSTER" -c /config.properties >/tmp/format.log; exec /opt/kafka/bin/kafka-server-start.sh /config.properties`)
	if err != nil {
		return row, errors.New("owned Kafka start failed")
	}
	row.id = strings.TrimSpace(id)
	if err = save(); err != nil {
		return row, err
	}
	cli := func(script string, arguments []string, input io.Reader, output io.Writer) error {
		args := append([]string{"exec", "-i", row.id, "/opt/kafka/bin/" + script, "--bootstrap-server", "127.0.0.1:19092"}, arguments...)
		return a.runner.RunIO(ctx, nil, input, output, io.Discard, "docker", args...)
	}
	deadline := time.Now().Add(90 * time.Second)
	for cli("kafka-topics.sh", []string{"--list"}, nil, io.Discard) != nil {
		if time.Now().After(deadline) {
			return row, errors.New("owned Kafka readiness timed out")
		}
		if err = waitContext(ctx, 500*time.Millisecond); err != nil {
			return row, err
		}
	}
	operations := []struct {
		script    string
		arguments []string
	}{
		{"kafka-topics.sh", []string{"--create", "--topic", "plan8-owned", "--partitions", "2", "--replication-factor", "1"}},
		{"kafka-acls.sh", []string{"--add", "--allow-principal", "User:monitor", "--operation", "Describe", "--cluster"}},
		{"kafka-acls.sh", []string{"--add", "--allow-principal", "User:monitor", "--operation", "Describe", "--operation", "DescribeConfigs", "--topic", "plan8-owned"}},
		{"kafka-acls.sh", []string{"--add", "--allow-principal", "User:monitor", "--operation", "Describe", "--group", "plan8-consumers"}},
	}
	for _, operation := range operations {
		if err = cli(operation.script, operation.arguments, nil, io.Discard); err != nil {
			return row, errors.New("Kafka actual topic or monitoring ACL setup failed")
		}
	}
	if err = cli("kafka-console-producer.sh", []string{"--topic", "plan8-owned"}, strings.NewReader("one\ntwo\nthree\nfour\n"), io.Discard); err != nil {
		return row, errors.New("Kafka actual message workload failed")
	}
	if err = cli("kafka-consumer-groups.sh", []string{"--reset-offsets", "--group", "plan8-consumers", "--topic", "plan8-owned", "--to-offset", "0", "--execute"}, nil, io.Discard); err != nil {
		return row, errors.New("Kafka actual consumer offsets unavailable")
	}
	external := []string{"exec", "-i", row.id, "/opt/kafka/bin/kafka-topics.sh", "--bootstrap-server", row.endpoint, "--command-config", "/monitor.properties"}
	if err = a.runner.RunIO(ctx, nil, nil, io.Discard, io.Discard, "docker", append(external, "--describe", "--topic", "plan8-owned")...); err != nil {
		return row, errors.New("Kafka authenticated metadata read failed")
	}
	var denied, deniedError bytes.Buffer
	if err = a.runner.RunIO(ctx, nil, nil, &denied, &deniedError, "docker", append(external, "--create", "--topic", "plan8-forbidden-write", "--partitions", "1", "--replication-factor", "1")...); err == nil || !strings.Contains(denied.String()+deniedError.String(), "Authorization") {
		return row, errors.New("Kafka metadata identity did not explicitly reject topic creation")
	}
	var listed bytes.Buffer
	if err = cli("kafka-topics.sh", []string{"--list"}, nil, &listed); err != nil || strings.Contains(listed.String(), "plan8-forbidden-write") {
		return row, errors.New("Kafka rejected write had an unexpected outcome")
	}
	version, err := a.runner.Output(ctx, nil, "docker", "exec", row.id, "/opt/kafka/bin/kafka-topics.sh", "--version")
	if err != nil {
		return row, errors.New("Kafka actual executable version unavailable")
	}
	row.version = strings.TrimSpace(version)
	if row.version == "" || len(row.version) > 200 || strings.Contains(row.version, row.password) {
		return row, errors.New("Kafka runtime version invalid")
	}
	image, err := a.runner.Output(ctx, nil, "docker", "inspect", "--format", "{{.Image}}", row.id)
	if err != nil || !strings.HasPrefix(strings.TrimSpace(image), "sha256:") {
		return row, errors.New("Kafka immutable image identity missing")
	}
	row.imageID = strings.TrimSpace(image)
	return row, save()
}
