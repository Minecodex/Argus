package collectormanager

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNativeKafkaReadOnlyMetadataAndConsumerOffsets(t *testing.T) {
	f := newNativeResourceFixture(t)
	password := "plan8-${env:HOME}-$" + uuid.NewString()
	properties := fmt.Sprintf(`process.roles=broker,controller
node.id=1
controller.listener.names=CONTROLLER
controller.quorum.bootstrap.servers=127.0.0.1:19093
listeners=INTERNAL://0.0.0.0:19092,EXTERNAL://0.0.0.0:9092,CONTROLLER://0.0.0.0:19093
advertised.listeners=INTERNAL://127.0.0.1:19092,EXTERNAL://%s-kafka:9092
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
`, f.owner, password)
	serverConfig := f.write("kafka.properties", []byte(properties))
	clientConfig := f.write("monitor.properties", []byte("security.protocol=SASL_PLAINTEXT\nsasl.mechanism=PLAIN\nsasl.jaas.config=org.apache.kafka.common.security.plain.PlainLoginModule required username=\"monitor\" password=\""+password+"\";\nrequest.timeout.ms=5000\ndefault.api.timeout.ms=5000\n"))
	cluster := uuid.New()
	cluster[0] = 0xf8 // Exercise a valid leading '-' in --cluster-id=value.
	kafka := f.start("kafka", "--memory=768m", "--cpus=1", "-e", "KAFKA_HEAP_OPTS=-Xms128m -Xmx256m", "-e", "ARGUS_TEST_CLUSTER="+base64.RawURLEncoding.EncodeToString(cluster[:]), "--mount", "type=bind,source="+serverConfig+",target=/config.properties,readonly", "--mount", "type=bind,source="+clientConfig+",target=/monitor.properties,readonly", "--entrypoint", "/bin/sh", "apache/kafka:4.2.1", "-ec", `/opt/kafka/bin/kafka-storage.sh format --standalone --ignore-formatted --cluster-id="$ARGUS_TEST_CLUSTER" -c /config.properties >/tmp/format.log; exec /opt/kafka/bin/kafka-server-start.sh /config.properties`)
	deadline := time.Now().Add(60 * time.Second)
	for {
		if _, err := f.command("", "exec", kafka, "/opt/kafka/bin/kafka-topics.sh", "--bootstrap-server", "127.0.0.1:19092", "--list"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			logs, _ := f.command("", "logs", "--tail", "15", kafka)
			t.Fatal("owned Kafka readiness: " + strings.ReplaceAll(logs, password, "[REDACTED]"))
		}
		time.Sleep(300 * time.Millisecond)
	}
	cli := func(script string, args ...string) string {
		t.Helper()
		return f.must("", append([]string{"exec", kafka, "/opt/kafka/bin/" + script, "--bootstrap-server", "127.0.0.1:19092"}, args...)...)
	}
	cli("kafka-topics.sh", "--create", "--topic", "plan8-owned", "--partitions", "2", "--replication-factor", "1")
	cli("kafka-acls.sh", "--add", "--allow-principal", "User:monitor", "--operation", "Describe", "--cluster")
	cli("kafka-acls.sh", "--add", "--allow-principal", "User:monitor", "--operation", "Describe", "--operation", "DescribeConfigs", "--topic", "plan8-owned")
	cli("kafka-acls.sh", "--add", "--allow-principal", "User:monitor", "--operation", "Describe", "--group", "plan8-consumers")
	f.must("one\ntwo\nthree\nfour\n", "exec", "-i", kafka, "/opt/kafka/bin/kafka-console-producer.sh", "--bootstrap-server", "127.0.0.1:19092", "--topic", "plan8-owned")
	cli("kafka-consumer-groups.sh", "--reset-offsets", "--group", "plan8-consumers", "--topic", "plan8-owned", "--to-offset", "0", "--execute")
	external := f.owner + "-kafka:9092"
	// Establish authenticated, authorized read before checking a valid write
	// denial. No fixture record or Collector metric is injected into the sink.
	f.must("", "exec", kafka, "/opt/kafka/bin/kafka-topics.sh", "--bootstrap-server", external, "--command-config", "/monitor.properties", "--describe", "--topic", "plan8-owned")
	if _, err := f.command("", "exec", kafka, "/opt/kafka/bin/kafka-topics.sh", "--bootstrap-server", external, "--command-config", "/monitor.properties", "--create", "--topic", "plan8-forbidden-write", "--partitions", "1", "--replication-factor", "1"); err == nil {
		t.Fatal("metadata-only monitor created a topic")
	}
	if strings.Contains(cli("kafka-topics.sh", "--list"), "plan8-forbidden-write") {
		t.Fatal("denied topic creation had a side effect")
	}
	f.add("kafka", external, "", map[string]string{"username": "monitor", "password": password})
	f.collections[0].Config = json.RawMessage(`{"interval":"3s","timeout":"3s","sasl_mechanism":"PLAIN","topic_match":"^plan8-owned$","group_match":"^plan8-consumers$"}`)
	first := f.run(1)
	deadline = time.Now().Add(40 * time.Second)
	wanted := []string{"kafka.brokers", "kafka.topic.partitions", "kafka.partition.current_offset", "kafka.partition.replicas", "kafka.partition.replicas_in_sync", "kafka.consumer_group.offset", "kafka.consumer_group.lag", "kafka.consumer_group.members"}
	for {
		samples := f.capture.snapshot()[f.collections[0].TargetID]
		complete := true
		for _, name := range wanted {
			complete = complete && slices.Contains(samples, name)
		}
		if complete {
			break
		}
		if time.Now().After(deadline) {
			logs, _ := f.command("", "logs", "--tail", "15", first)
			t.Fatalf("actual Kafka metrics incomplete: %v %s", samples, strings.ReplaceAll(logs, password, "[REDACTED]"))
		}
		time.Sleep(300 * time.Millisecond)
	}
	f.waitValue(0, "kafka.brokers", nil, 1)
	f.waitValue(0, "kafka.topic.partitions", map[string]string{"topic": "plan8-owned"}, 2)
	time.Sleep(5 * time.Second)
	f.save(os.Getenv("ARGUS_KAFKA_METRIC_EVIDENCE"), 1)
	for _, payload := range f.capture.payloads() {
		if strings.Contains(string(payload), password) {
			t.Fatal("monitor password leaked into Kafka telemetry")
		}
	}
	f.must("", "stop", "--time", "1", first)
	f.capture.reset()
	f.credentials[f.collections[0].ID] = map[string]string{"username": "monitor", "password": "incorrect-native-password"}
	denied := f.run(2)
	time.Sleep(9 * time.Second)
	if len(f.capture.snapshot()) != 0 {
		t.Fatal("wrong Kafka SASL credentials fabricated target data")
	}
	f.must("", "stop", "--time", "1", denied)
	f.capture.reset()
	f.credentials[f.collections[0].ID] = map[string]string{"username": "monitor", "password": password}
	recovered := f.run(3)
	f.wait(recovered, 1)
	f.waitValue(0, "kafka.brokers", nil, 1)
	f.must("", "stop", "--time", "1", recovered)
	f.capture.reset()
	f.collections[0].Enabled, f.collections[0].Revision = false, 2
	allOff := f.run(4)
	time.Sleep(3 * time.Second)
	if len(f.capture.snapshot()) != 0 || f.must("", "inspect", "--format", "{{.State.Running}}", allOff) != "true" {
		t.Fatal("all-off Kafka runtime incorrect")
	}
	t.Log("actual Kafka 4.2.1 KRaft metadata-only SASL/PLAIN monitor, two partitions/committed consumer offsets, valid write denial, literal credentials, auth failure/recovery and independent lifecycle; no JVM/JMX collection")
}
