//go:build !windows

package hostinventory

import (
	"context"
	"encoding/csv"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func hardwareUUID(_ context.Context) string { return readText("/sys/class/dmi/id/product_uuid") }
func readText(path string) string {
	data, err := os.ReadFile(path)
	if err != nil || len(data) > 4096 {
		return ""
	}
	return strings.TrimSpace(string(data))
}
func collectGPUs(ctx context.Context) ([]GPU, error) {
	gpus := []GPU{}
	if binary, err := exec.LookPath("nvidia-smi"); err == nil {
		output, err := exec.CommandContext(ctx, binary, "--query-gpu=name,uuid,memory.total", "--format=csv,noheader,nounits").Output()
		if err == nil && len(output) <= MaxBytes {
			rows, _ := csv.NewReader(strings.NewReader(string(output))).ReadAll()
			for _, row := range rows {
				if len(row) != 3 || len(gpus) >= 128 {
					continue
				}
				size, _ := strconv.ParseUint(strings.TrimSpace(row[2]), 10, 64)
				gpus = append(gpus, GPU{strings.TrimSpace(row[0]), "NVIDIA", strings.TrimSpace(row[1]), size * 1024 * 1024})
			}
			return gpus, nil
		}
	}
	paths, err := filepath.Glob("/sys/bus/pci/devices/*")
	if err != nil {
		return gpus, err
	}
	if _, err = os.Stat("/sys/bus/pci/devices"); err != nil {
		return gpus, err
	}
	for _, path := range paths {
		if len(gpus) >= 128 {
			break
		}
		class := readText(filepath.Join(path, "class"))
		if !strings.HasPrefix(class, "0x03") {
			continue
		}
		vendor := readText(filepath.Join(path, "vendor"))
		device := readText(filepath.Join(path, "device"))
		gpus = append(gpus, GPU{Name: vendor + ":" + device, Vendor: vendor, DeviceID: filepath.Base(path)})
	}
	return gpus, nil
}
