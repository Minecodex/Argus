package worker

import (
	"testing"

	"github.com/kakj-go/Argus/internal/config"
	"github.com/kakj-go/Argus/internal/remoteexecution"
)

func TestRemoteQueuesUseIndependentCapacityWithoutParallelizingAgentTurns(t *testing.T) {
	settings := config.RuntimeWorkers{RemoteExecution: 8, RemoteControl: 2}
	for queue, expected := range map[string]int{remoteexecution.Queue: 8, remoteexecution.ControlQueue: 2, PoolAgent: 1, PoolCompaction: 1, PoolAction: 1, PoolSandbox: 1} {
		if got := queueConcurrency(queue, settings); got != expected {
			t.Fatalf("%s concurrency=%d want=%d", queue, got, expected)
		}
	}
}
