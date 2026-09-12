//go:build windows

package connector

import (
	"context"
	"os/exec"
	"strings"
	"time"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func observeHostRuntime() *connectorv1.HostRuntimeObservation {
	observation := &connectorv1.HostRuntimeObservation{Platform: "windows", OpensshStatus: "unavailable", RdpStatus: "unknown", ObservedAt: timestamppb.Now()}
	manager, err := mgr.Connect()
	if err == nil {
		defer manager.Disconnect()
		if service, openErr := manager.OpenService("sshd"); openErr == nil {
			if status, queryErr := service.Query(); queryErr == nil && status.State == svc.Running {
				observation.OpensshStatus = "available"
			}
			_ = service.Close()
		}
		if service, openErr := manager.OpenService("TermService"); openErr == nil {
			if status, queryErr := service.Query(); queryErr == nil && status.State == svc.Running {
				observation.RdpServiceRunning = true
			}
			_ = service.Close()
		}
	}
	if key, openErr := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Terminal Server`, registry.QUERY_VALUE); openErr == nil {
		if deny, _, readErr := key.GetIntegerValue("fDenyTSConnections"); readErr == nil {
			if deny == 0 {
				observation.RdpStatus = "enabled"
			} else {
				observation.RdpStatus = "disabled"
			}
		}
		_ = key.Close()
	}
	if key, openErr := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Terminal Server\WinStations\RDP-Tcp`, registry.QUERY_VALUE); openErr == nil {
		if nla, _, readErr := key.GetIntegerValue("UserAuthentication"); readErr == nil {
			observation.RdpNlaEnabled = nla == 1
		}
		_ = key.Close()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	output, commandErr := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
		`$r=Get-NetFirewallRule -Name 'RemoteDesktop-UserMode-In-TCP' -ErrorAction SilentlyContinue; if($r.Enabled -eq 'True'){Write-Output enabled}`).CombinedOutput()
	observation.RdpFirewallEnabled = commandErr == nil && strings.Contains(strings.ToLower(string(output)), "enabled")
	if observation.RdpStatus == "enabled" && !observation.RdpServiceRunning {
		observation.RdpStatus = "unavailable"
	}
	return observation
}
