package worker

import (
	"github.com/kakj-go/Argus/internal/config"
	"github.com/kakj-go/Argus/internal/remoteexecution"
)

func queueConcurrency(queue string, settings config.RuntimeWorkers) int {
	switch queue {
	case remoteexecution.Queue:
		return settings.RemoteExecution
	case remoteexecution.ControlQueue:
		return settings.RemoteControl
	default:
		return 1
	}
}
