package collectormanager

import (
	"context"
	"github.com/google/uuid"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestNativeRedisDedicatedReceiverUsesMetadataOnlyACL(t *testing.T) {
	f := newNativeResourceFixture(t)
	password := "P8-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	config := "bind 0.0.0.0\nprotected-mode no\nappendonly no\nsave \"\"\nuser default off\nuser monitor reset on >" + password + " -@all +info +ping +hello +cluster|info +client|setinfo\n"
	path := f.write("redis-readonly.conf", []byte(config))
	container := f.start("redis-readonly", "--memory=64m", "--cpus=.5", "--mount", "type=bind,source="+path+",target=/redis.conf,readonly", "redis:7.4-alpine", "redis-server", "/redis.conf")
	call := func(arguments ...string) (string, error) {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		args := append([]string{"exec", "--env", "REDISCLI_AUTH", container, "redis-cli", "--user", "monitor", "--no-auth-warning", "-e"}, arguments...)
		cmd := exec.CommandContext(ctx, "docker", args...)
		cmd.Env = append(os.Environ(), "REDISCLI_AUTH="+password)
		result, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(result)), err
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		if result, err := call("PING"); err == nil && result == "PONG" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("owned metadata-only Redis was not ready")
		}
		time.Sleep(200 * time.Millisecond)
	}
	for _, arguments := range [][]string{{"SET", "forbidden", "value"}, {"GET", "forbidden"}, {"CONFIG", "GET", "requirepass"}} {
		result, err := call(arguments...)
		if err == nil || !strings.Contains(result, "NOPERM") {
			t.Fatal("monitoring ACL exposed business values or configuration")
		}
	}
	f.add("redis", f.owner+"-redis-readonly:6379", "", map[string]string{"username": "monitor", "password": password})
	collector := f.run(1)
	metrics := f.wait(collector, 1)[f.collections[0].TargetID]
	for _, name := range []string{"redis.clients.connected", "redis.memory.used", "redis.uptime"} {
		if !slices.Contains(metrics, name) {
			t.Fatal("dedicated receiver did not collect through metadata-only ACL", name)
		}
	}
	t.Log("actual native Redis receiver collected INFO/CLUSTER metadata; GET, SET and CONFIG secrets were denied")
}
