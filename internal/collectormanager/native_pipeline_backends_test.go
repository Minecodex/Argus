package collectormanager_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/redis"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/scram"
)

type pipelineDocker struct {
	t                *testing.T
	owner, directory string
}

func newPipelineDocker(t *testing.T) *pipelineDocker {
	t.Helper()
	d := &pipelineDocker{t: t, owner: "plan8-pipeline-" + uuid.NewString()[:8], directory: t.TempDir()}
	d.must("network", "create", "--label", "argus.owner="+d.owner, d.owner)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		label, err := exec.CommandContext(ctx, "docker", "network", "inspect", "--format", `{{index .Labels "argus.owner"}}`, d.owner).Output()
		if err == nil && strings.TrimSpace(string(label)) == d.owner {
			_ = exec.CommandContext(ctx, "docker", "network", "rm", d.owner).Run()
		}
	})
	return d
}
func (d *pipelineDocker) command(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(d.t.Context(), 90*time.Second)
	defer cancel()
	raw, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	return strings.TrimSpace(string(raw)), err
}
func (d *pipelineDocker) must(args ...string) string {
	d.t.Helper()
	output, err := d.command(args...)
	if err != nil {
		d.t.Fatalf("owned Docker operation: %v: %s", err, output)
	}
	return output
}
func (d *pipelineDocker) file(name string, value []byte) string {
	d.t.Helper()
	path := filepath.Join(d.directory, name)
	if err := os.WriteFile(path, value, 0600); err != nil {
		d.t.Fatal(err)
	}
	return path
}
func (d *pipelineDocker) start(name string, args ...string) string {
	d.t.Helper()
	name = d.owner + "-" + name
	d.t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		label, err := exec.CommandContext(ctx, "docker", "inspect", "--format", `{{index .Config.Labels "argus.owner"}}`, name).Output()
		if err == nil && strings.TrimSpace(string(label)) == d.owner {
			_ = exec.CommandContext(ctx, "docker", "rm", "-f", "-v", name).Run()
		}
	})
	d.must(append([]string{"run", "-d", "--name", name, "--label", "argus.owner=" + d.owner, "--network", d.owner}, args...)...)
	return name
}
func (d *pipelineDocker) port(name, internal string) string {
	d.t.Helper()
	return d.must("port", name, internal+"/tcp")
}

func (d *pipelineDocker) kafka() (*kgo.Client, *kgo.Client, *kgo.Client) {
	d.t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		d.t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	password := uuid.NewString()
	properties := fmt.Sprintf(`process.roles=broker,controller
node.id=1
controller.listener.names=CONTROLLER
controller.quorum.bootstrap.servers=127.0.0.1:19093
listeners=INTERNAL://0.0.0.0:19092,EXTERNAL://0.0.0.0:9092,CONTROLLER://0.0.0.0:19093
advertised.listeners=INTERNAL://127.0.0.1:19092,EXTERNAL://127.0.0.1:%d
listener.security.protocol.map=INTERNAL:PLAINTEXT,EXTERNAL:SASL_PLAINTEXT,CONTROLLER:PLAINTEXT
inter.broker.listener.name=INTERNAL
sasl.enabled.mechanisms=SCRAM-SHA-512
listener.name.external.scram-sha-512.sasl.jaas.config=org.apache.kafka.common.security.scram.ScramLoginModule required;
log.dirs=/tmp/kafka-data
num.partitions=1
offsets.topic.replication.factor=1
transaction.state.log.replication.factor=1
transaction.state.log.min.isr=1
group.initial.rebalance.delay.ms=0
log.segment.bytes=1048576
log.retention.bytes=16777216
log.retention.ms=3600000
`, port)
	config := d.file("kafka.properties", []byte(properties))
	cluster := uuid.New()
	// A valid Kafka UUID can begin with '-'. Keep that boundary in every
	// actual fixture startup; the option value must not become a CLI flag.
	cluster[0] = 0xf8
	// Only the loopback-published listener accepts test clients. The internal
	// listener is confined to the owned Docker network for broker bootstrap.
	name := d.start("kafka", "--memory=1024m", "--cpus=1", "-p", fmt.Sprintf("127.0.0.1:%d:9092", port), "-e", "KAFKA_HEAP_OPTS=-Xms128m -Xmx256m", "-e", "ARGUS_TEST_CLUSTER="+base64.RawURLEncoding.EncodeToString(cluster[:]), "-e", "ARGUS_TEST_SCRAM=SCRAM-SHA-512=[name=argus-pipeline,password="+password+"]", "--mount", "type=bind,source="+config+",target=/config.properties,readonly", "--entrypoint", "/bin/sh", "apache/kafka:4.2.1", "-ec", `/opt/kafka/bin/kafka-storage.sh format --standalone --ignore-formatted --cluster-id="$ARGUS_TEST_CLUSTER" -c /config.properties --add-scram "$ARGUS_TEST_SCRAM" >/tmp/format.log; exec /opt/kafka/bin/kafka-server-start.sh /config.properties`)
	address := fmt.Sprintf("127.0.0.1:%d", port)
	producer, err := telemetry.NewKafkaProducer([]string{address}, "argus-pipeline", password)
	if err != nil {
		d.t.Fatal(err)
	}
	d.t.Cleanup(producer.Close)
	deadline := time.Now().Add(75 * time.Second)
	for {
		ctx, cancel := context.WithTimeout(d.t.Context(), 2*time.Second)
		err = producer.Ping(ctx)
		cancel()
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			logs, _ := d.command("logs", "--tail", "35", name)
			d.t.Fatalf("SASL Kafka readiness: %v %s", err, strings.ReplaceAll(logs, password, "[REDACTED]"))
		}
		time.Sleep(time.Second)
	}
	for _, topic := range []string{"otlp-logs", "otlp-metrics", "otlp-traces", "otlp-logs-dlq", "otlp-metrics-dlq", "otlp-traces-dlq"} {
		d.must("exec", name, "/opt/kafka/bin/kafka-topics.sh", "--bootstrap-server", "127.0.0.1:19092", "--create", "--if-not-exists", "--topic", topic, "--partitions", "1", "--replication-factor", "1")
	}
	consumer, err := telemetry.NewKafkaConsumer([]string{address}, d.owner, "argus-pipeline", password)
	if err != nil {
		d.t.Fatal(err)
	}
	d.t.Cleanup(consumer.Close)
	probe, err := kgo.NewClient(kgo.SeedBrokers(address), kgo.SASL(scram.Auth{User: "argus-pipeline", Pass: password}.AsSha512Mechanism()), kgo.ConsumePartitions(map[string]map[int32]kgo.Offset{"otlp-logs": {0: kgo.NewOffset().AtStart()}, "otlp-traces": {0: kgo.NewOffset().AtStart()}, "otlp-metrics": {0: kgo.NewOffset().AtStart()}}))
	if err != nil {
		d.t.Fatal(err)
	}
	d.t.Cleanup(probe.Close)
	return producer, consumer, probe
}

func (d *pipelineDocker) redis() *redis.Client {
	d.t.Helper()
	password := uuid.NewString()
	name := d.start("redis", "--memory=64m", "--cpus=.5", "-p", "127.0.0.1::6379", "redis:7.4-alpine", "redis-server", "--save", "", "--appendonly", "no", "--requirepass", password)
	address := d.port(name, "6379")
	client, err := redis.Open(d.t.Context(), "redis://:"+password+"@"+address+"/0")
	if client == nil {
		d.t.Fatal(err)
	}
	d.t.Cleanup(func() { _ = client.Close() })
	deadline := time.Now().Add(10 * time.Second)
	for err != nil && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		err = client.Ready(d.t.Context())
	}
	if err != nil {
		d.t.Fatal(err)
	}
	return client
}
