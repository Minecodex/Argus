// Package hostinventory observes the local machine. It never changes system
// configuration and reports missing capabilities instead of invented values.
package hostinventory

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
)

const SchemaVersion = "argus.host_inventory/v1"
const MaxBytes = 128 << 10

type Snapshot struct {
	SchemaVersion        string     `json:"schema_version"`
	CollectedAt          time.Time  `json:"collected_at"`
	Hostname             string     `json:"hostname"`
	OS                   string     `json:"os"`
	OSVersion            string     `json:"os_version"`
	KernelVersion        string     `json:"kernel_version"`
	Architecture         string     `json:"architecture"`
	MachineID            string     `json:"machine_id"`
	HardwareUUID         string     `json:"hardware_uuid"`
	VirtualizationSystem string     `json:"virtualization_system"`
	VirtualizationRole   string     `json:"virtualization_role"`
	Scope                string     `json:"scope"`
	BootTime             *time.Time `json:"boot_time,omitempty"`
	CPU                  *CPU       `json:"cpu,omitempty"`
	Memory               *Memory    `json:"memory,omitempty"`
	Disks                []Disk     `json:"disks"`
	Networks             []Network  `json:"networks"`
	GPUs                 []GPU      `json:"gpus"`
	Cloud                Cloud      `json:"cloud"`
	Issues               []Issue    `json:"issues"`
}
type CPU struct {
	Model         string `json:"model"`
	LogicalCores  int    `json:"logical_cores"`
	PhysicalCores int    `json:"physical_cores"`
}
type Memory struct {
	TotalBytes uint64 `json:"total_bytes"`
	SwapBytes  uint64 `json:"swap_bytes"`
}
type Disk struct {
	Device     string `json:"device"`
	Mountpoint string `json:"mountpoint"`
	Filesystem string `json:"filesystem"`
	TotalBytes uint64 `json:"total_bytes"`
	UsedBytes  uint64 `json:"used_bytes"`
	FreeBytes  uint64 `json:"free_bytes"`
}
type Network struct {
	Name      string   `json:"name"`
	MAC       string   `json:"mac"`
	MTU       int      `json:"mtu"`
	Addresses []string `json:"addresses"`
	State     string   `json:"state"`
}
type GPU struct {
	Name        string `json:"name"`
	Vendor      string `json:"vendor"`
	DeviceID    string `json:"device_id"`
	MemoryBytes uint64 `json:"memory_bytes"`
}
type Cloud struct {
	Status     string `json:"status"`
	Provider   string `json:"provider"`
	InstanceID string `json:"instance_id"`
	Region     string `json:"region"`
	Zone       string `json:"zone"`
}
type Issue struct {
	Section string `json:"section"`
	Code    string `json:"code"`
}

func Collect(parent context.Context) Snapshot {
	ctx, cancel := context.WithTimeout(parent, 8*time.Second)
	defer cancel()
	s := Snapshot{SchemaVersion: SchemaVersion, CollectedAt: time.Now().UTC(), Architecture: runtime.GOARCH, Scope: "machine", Disks: []Disk{}, Networks: []Network{}, GPUs: []GPU{}, Issues: []Issue{}, Cloud: Cloud{Status: "not_detected"}}
	issue := func(section string) { s.Issues = append(s.Issues, Issue{Section: section, Code: "unavailable"}) }
	if info, err := host.InfoWithContext(ctx); err == nil {
		s.Hostname, s.OS, s.OSVersion, s.KernelVersion, s.MachineID = info.Hostname, info.Platform, info.PlatformVersion, info.KernelVersion, info.HostID
		s.VirtualizationSystem, s.VirtualizationRole = info.VirtualizationSystem, info.VirtualizationRole
		if info.BootTime > 0 {
			b := time.Unix(int64(info.BootTime), 0).UTC()
			s.BootTime = &b
		}
	} else {
		issue("system")
	}
	if _, err := os.Stat("/.dockerenv"); err == nil {
		s.Scope = "container"
	}
	if _, err := os.Stat("/run/.containerenv"); err == nil {
		s.Scope = "container"
	}
	if s.VirtualizationSystem == "docker" || s.VirtualizationSystem == "podman" || s.VirtualizationSystem == "lxc" {
		s.Scope = "container"
	}
	s.HardwareUUID = hardwareUUID(ctx)
	logical, e1 := cpu.CountsWithContext(ctx, true)
	physical, e2 := cpu.CountsWithContext(ctx, false)
	info, e3 := cpu.InfoWithContext(ctx)
	if e1 == nil && logical > 0 {
		s.CPU = &CPU{LogicalCores: logical}
		if e2 == nil {
			s.CPU.PhysicalCores = physical
		}
		if e3 == nil && len(info) > 0 {
			s.CPU.Model = info[0].ModelName
		}
	} else {
		issue("cpu")
	}
	if v, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		s.Memory = &Memory{TotalBytes: v.Total}
		if swap, err := mem.SwapMemoryWithContext(ctx); err == nil {
			s.Memory.SwapBytes = swap.Total
		} else {
			issue("swap")
		}
	} else {
		issue("memory")
	}
	partitions, err := disk.PartitionsWithContext(ctx, true)
	if err != nil {
		issue("disks")
	}
	seen := map[string]bool{}
	for _, p := range partitions {
		if len(s.Disks) >= 128 {
			s.Issues = append(s.Issues, Issue{Section: "disks", Code: "truncated"})
			break
		}
		if seen[p.Mountpoint] || pseudoFilesystem(p.Fstype) {
			continue
		}
		seen[p.Mountpoint] = true
		usage, err := disk.UsageWithContext(ctx, p.Mountpoint)
		if err != nil {
			issue("disks")
			continue
		}
		if usage.Total == 0 {
			continue
		}
		s.Disks = append(s.Disks, Disk{p.Device, p.Mountpoint, p.Fstype, usage.Total, usage.Used, usage.Free})
	}
	sort.Slice(s.Disks, func(i, j int) bool { return s.Disks[i].Mountpoint < s.Disks[j].Mountpoint })
	interfaces, err := net.Interfaces()
	if err != nil {
		issue("networks")
	}
	for _, n := range interfaces {
		if len(s.Networks) >= 128 {
			s.Issues = append(s.Issues, Issue{Section: "networks", Code: "truncated"})
			break
		}
		addresses := []string{}
		values, err := n.Addrs()
		if err != nil {
			issue("networks")
		}
		for _, a := range values {
			if len(addresses) < 32 {
				addresses = append(addresses, a.String())
			}
		}
		sort.Strings(addresses)
		state := "down"
		if n.Flags&net.FlagUp != 0 {
			state = "up"
		}
		s.Networks = append(s.Networks, Network{n.Name, n.HardwareAddr.String(), n.MTU, addresses, state})
	}
	sort.Slice(s.Networks, func(i, j int) bool { return s.Networks[i].Name < s.Networks[j].Name })
	if gpus, err := collectGPUs(ctx); err == nil {
		s.GPUs = gpus
	} else {
		issue("gpus")
	}
	if s.Scope != "container" {
		s.Cloud = collectCloud(ctx)
	}
	// Empty identity fields are meaningful, and must never become generated IDs.
	if s.MachineID == "" {
		issue("machine_id")
	}
	if s.HardwareUUID == "" {
		issue("hardware_uuid")
	}
	return s
}
func pseudoFilesystem(fs string) bool {
	return strings.Contains(" proc sysfs devtmpfs devpts tmpfs cgroup cgroup2 securityfs debugfs tracefs configfs pstore mqueue hugetlbfs nsfs ", " "+fs+" ")
}

func Decode(raw []byte) (*Snapshot, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if len(raw) > MaxBytes {
		return nil, errors.New("inventory too large")
	}
	var s Snapshot
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	if s.SchemaVersion != SchemaVersion || s.CollectedAt.IsZero() || s.CollectedAt.After(time.Now().Add(time.Minute)) || (s.Scope != "machine" && s.Scope != "container") || len(s.Disks) > 128 || len(s.Networks) > 128 || len(s.GPUs) > 128 || s.Disks == nil || s.Networks == nil || s.GPUs == nil || s.Issues == nil || (s.Cloud.Status != "available" && s.Cloud.Status != "not_detected") {
		return nil, errors.New("invalid inventory")
	}
	return &s, nil
}
