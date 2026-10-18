package config

import (
	"fmt"
	"os"
	"strconv"
)

type RuntimeWorkers struct {
	RemoteExecution int
	RemoteControl   int
}

func LoadRuntimeWorkers() (RuntimeWorkers, error) {
	read := func(name string, fallback, maximum int) (int, error) {
		value := os.Getenv(name)
		if value == "" {
			return fallback, nil
		}
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 || n > maximum {
			return 0, fmt.Errorf("%s must be between 1 and %d", name, maximum)
		}
		return n, nil
	}
	execution, err := read("ARGUS_REMOTE_EXECUTION_WORKERS", 8, 64)
	if err != nil {
		return RuntimeWorkers{}, err
	}
	control, err := read("ARGUS_REMOTE_CONTROL_WORKERS", 2, 16)
	return RuntimeWorkers{RemoteExecution: execution, RemoteControl: control}, err
}
