package collectormanager

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestNativeRabbitMQAndZooKeeperTargets(t *testing.T) {
	f := newNativeResourceFixture(t)
	password := "plan8-private-statistics"
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	rabbit := f.start("rabbitmq", "--memory=512m", "--cpus=1", "-p", fmt.Sprintf("127.0.0.1:%d:15672", port), "-e", "RABBITMQ_DEFAULT_USER=admin", "-e", "RABBITMQ_DEFAULT_PASS="+password, "-e", "RABBITMQ_SERVER_ADDITIONAL_ERL_ARGS=+S 2:2 +A 4", "rabbitmq:4-management-alpine")
	zoo := f.start("zookeeper", "--memory=384m", "--cpus=1", "-e", "JVMFLAGS=-Xms64m -Xmx128m", "-e", "ZOO_4LW_COMMANDS_WHITELIST=mntr,ruok,stat", "zookeeper:3.9")
	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	lastResponse := ""
	request := func(user, method, path, body string) int {
		t.Helper()
		r, e := http.NewRequest(method, url+path, strings.NewReader(body))
		if e != nil {
			t.Fatal(e)
		}
		r.SetBasicAuth(user, password)
		r.Header.Set("Content-Type", "application/json")
		client := &http.Client{Timeout: 3 * time.Second}
		response, e := client.Do(r)
		if e != nil {
			return 0
		}
		defer response.Body.Close()
		text, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		lastResponse = string(text)
		return response.StatusCode
	}
	deadline := time.Now().Add(50 * time.Second)
	for request("admin", "GET", "/api/overview", "") != 200 {
		if time.Now().After(deadline) {
			logs, _ := f.command("", "logs", "--tail", "10", rabbit)
			t.Fatalf("owned RabbitMQ management not ready: %s", logs)
		}
		time.Sleep(500 * time.Millisecond)
	}
	f.must("", "exec", rabbit, "rabbitmqctl", "add_user", "monitor", password)
	f.must("", "exec", rabbit, "rabbitmqctl", "set_user_tags", "monitor", "monitoring")
	f.must("", "exec", rabbit, "rabbitmqctl", "set_permissions", "-p", "/", "monitor", "^$", "^$", "^$")
	if request("monitor", "GET", "/api/whoami", "") != 200 || request("monitor", "GET", "/api/nodes", "") != 200 {
		t.Fatal("monitor identity did not authenticate with read-only management access")
	}
	if status := request("admin", "PUT", "/api/queues/%2F/plan8-owned", `{"durable":true,"auto_delete":false,"arguments":{}}`); status != 201 && status != 204 {
		t.Fatalf("owned queue fixture: %d %s", status, lastResponse)
	}
	if status := request("monitor", "PUT", "/api/queues/%2F/should-not-create", `{"durable":true,"auto_delete":false,"arguments":{}}`); status != 401 && status != 403 {
		t.Fatalf("monitor account unexpectedly wrote a queue: %d", status)
	}
	f.add("rabbitmq", "http://"+f.owner+"-rabbitmq:15672", "", map[string]string{"username": "monitor", "password": password})
	f.add("zookeeper", f.owner+"-zookeeper:2181", "", nil)
	first := f.run(1)
	samples := f.wait(first, 2)
	time.Sleep(6 * time.Second)
	for i, names := range [][]string{{"rabbitmq.message.current", "rabbitmq.consumer.count", "rabbitmq.node.mem_used", "rabbitmq.node.disk_free", "rabbitmq.node.fd_used", "rabbitmq.node.uptime"}, {"zookeeper.connection.active", "zookeeper.latency.avg", "zookeeper.packet.count", "zookeeper.znode.count", "zookeeper.watch.count", "zookeeper.request.active"}} {
		for _, name := range names {
			if !slices.Contains(samples[f.collections[i].TargetID], name) {
				t.Fatalf("native %s omitted %s: %v", f.collections[i].Integration, name, samples[f.collections[i].TargetID])
			}
		}
	}
	f.save(os.Getenv("ARGUS_BROKER_COORDINATION_METRIC_EVIDENCE"), 1)
	f.must("", "stop", "--time", "1", first)
	f.capture.reset()
	f.credentials[f.collections[0].ID] = map[string]string{"username": "monitor", "password": "wrong-private-statistics"}
	second := f.run(2)
	f.wait(second, 1)
	time.Sleep(3 * time.Second)
	if f.capture.snapshot()[f.collections[0].TargetID] != nil {
		t.Fatal("bad RabbitMQ credentials yielded target metrics")
	}
	f.must("", "stop", "--time", "1", second)
	f.capture.reset()
	f.credentials[f.collections[0].ID] = map[string]string{"username": "monitor", "password": password}
	f.collections[1].Enabled = false
	f.collections[1].Revision = 2
	third := f.run(3)
	f.wait(third, 1)
	if f.capture.snapshot()[f.collections[1].TargetID] != nil || f.capture.snapshot()[f.collections[0].TargetID] == nil {
		t.Fatal("ZooKeeper disable or RabbitMQ recovery failed")
	}
	f.must("", "stop", "--time", "1", third)
	f.capture.reset()
	f.collections[0].Enabled = false
	f.collections[0].Revision = 2
	fourth := f.run(4)
	time.Sleep(2 * time.Second)
	if len(f.capture.snapshot()) != 0 || f.must("", "inspect", "--format", "{{.State.Running}}", fourth) != "true" {
		t.Fatal("all-off coordination runtime incorrect")
	}
	t.Logf("actual RabbitMQ monitoring-only management identity, node/queue metrics, auth refusal/recovery and ZooKeeper mntr independent lifecycle; owned ZooKeeper=%s", zoo)
}
