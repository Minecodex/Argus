//go:build windows

package connector

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

func configureWindowsRDP(ctx context.Context, request *connectorv1.HostWindowsRDPConfigure) (*connectorv1.HostWindowsRDPConfigureResult, error) {
	if request == nil || request.GetHostId() == "" || !request.GetEnable() || !request.GetEnforceNla() || !request.GetEnableFirewall() {
		return nil, errors.New("invalid Windows RDP configuration request")
	}
	terminalServer, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Terminal Server`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return nil, err
	}
	beforeDeny, _, err := terminalServer.GetIntegerValue("fDenyTSConnections")
	if err != nil {
		_ = terminalServer.Close()
		return nil, err
	}
	err = terminalServer.SetDWordValue("fDenyTSConnections", 0)
	_ = terminalServer.Close()
	if err != nil {
		return nil, err
	}
	rdpTCP, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Terminal Server\WinStations\RDP-Tcp`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return nil, err
	}
	beforeNLA, _, err := rdpTCP.GetIntegerValue("UserAuthentication")
	if err != nil {
		_ = rdpTCP.Close()
		return nil, err
	}
	err = rdpTCP.SetDWordValue("UserAuthentication", 1)
	_ = rdpTCP.Close()
	if err != nil {
		return nil, err
	}
	firewallBeforeOutput, err := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
		`Get-NetFirewallRule -Name 'RemoteDesktop-UserMode-In-TCP','RemoteDesktop-UserMode-In-UDP' | Select-Object Name,@{N='EnabledValue';E={$_.Enabled.ToString()}} | ConvertTo-Json -Compress`).CombinedOutput()
	if err != nil {
		return nil, errors.New("read Windows RDP firewall rules: " + strings.TrimSpace(string(firewallBeforeOutput)))
	}
	var firewallBefore []struct {
		Name    string `json:"Name"`
		Enabled string `json:"EnabledValue"`
	}
	if err = json.Unmarshal(firewallBeforeOutput, &firewallBefore); err != nil || len(firewallBefore) != 2 {
		return nil, errors.New("parse Windows RDP firewall state")
	}
	output, err := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
		`Enable-NetFirewallRule -Name 'RemoteDesktop-UserMode-In-TCP','RemoteDesktop-UserMode-In-UDP' -ErrorAction Stop`).CombinedOutput()
	if err != nil {
		return nil, errors.New("enable Windows RDP firewall rules: " + strings.TrimSpace(string(output)))
	}
	manager, err := mgr.Connect()
	if err != nil {
		return nil, err
	}
	defer manager.Disconnect()
	service, err := manager.OpenService("TermService")
	if err != nil {
		return nil, err
	}
	defer service.Close()
	beforeService, err := service.Query()
	if err != nil {
		return nil, err
	}
	if err = service.Start(); err != nil {
		status, queryErr := service.Query()
		if queryErr != nil || status.State != svc.Running {
			return nil, err
		}
	}
	observation := observeHostRuntime()
	if observation.GetRdpStatus() != "enabled" || !observation.GetRdpNlaEnabled() || !observation.GetRdpFirewallEnabled() || !observation.GetRdpServiceRunning() {
		return nil, errors.New("Windows RDP did not converge")
	}
	before := map[string]any{"f_deny_ts_connections": beforeDeny, "user_authentication": beforeNLA,
		"firewall_tcp_enabled": firewallRuleEnabled(firewallBefore, "RemoteDesktop-UserMode-In-TCP"),
		"firewall_udp_enabled": firewallRuleEnabled(firewallBefore, "RemoteDesktop-UserMode-In-UDP"), "term_service_running": beforeService.State == svc.Running}
	applied := map[string]any{"f_deny_ts_connections": uint64(0), "user_authentication": uint64(1),
		"firewall_tcp_enabled": true, "firewall_udp_enabled": true, "term_service_running": true}
	beforeJSON, _ := json.Marshal(before)
	appliedJSON, _ := json.Marshal(applied)
	return &connectorv1.HostWindowsRDPConfigureResult{HostId: request.GetHostId(), RdpStatus: observation.GetRdpStatus(), NlaEnabled: true,
		FirewallEnabled: true, ServiceRunning: true, BeforeStateJson: beforeJSON, AppliedStateJson: appliedJSON}, nil
}

func firewallRuleEnabled(values []struct {
	Name    string `json:"Name"`
	Enabled string `json:"EnabledValue"`
}, name string) bool {
	for _, value := range values {
		if value.Name == name {
			return strings.EqualFold(value.Enabled, "True")
		}
	}
	return false
}
