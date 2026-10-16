//go:build windows

package hostinventory

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
)

func readText(_ string) string { return "" }
func hardwareUUID(ctx context.Context) string {
	output, err := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "(Get-CimInstance Win32_ComputerSystemProduct).UUID").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
func collectGPUs(ctx context.Context) ([]GPU, error) {
	output, err := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", `ConvertTo-Json -Compress -InputObject @(Get-CimInstance Win32_VideoController | Select-Object Name,PNPDeviceID,AdapterRAM,AdapterCompatibility)`).Output()
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Name, PNPDeviceID, AdapterCompatibility string
		AdapterRAM                              uint64
	}
	if err = json.Unmarshal(output, &rows); err != nil {
		return nil, err
	}
	gpus := []GPU{}
	for _, row := range rows {
		if len(gpus) >= 128 {
			break
		}
		gpus = append(gpus, GPU{row.Name, row.AdapterCompatibility, row.PNPDeviceID, row.AdapterRAM})
	}
	return gpus, nil
}
