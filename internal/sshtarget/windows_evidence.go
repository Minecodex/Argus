package sshtarget

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

func parseWindowsEvidence(output []byte) (Evidence, error) {
	var raw struct {
		IdentitySID         string `json:"identity_sid"`
		Architecture        string `json:"architecture"`
		DistributionVersion string `json:"distribution_version"`
		ServiceManager      string `json:"service_manager"`
		ProductType         int    `json:"product_type"`
		BuildNumber         int    `json:"build_number"`
		Privileged          bool   `json:"privileged"`
		FreeDiskBytes       uint64 `json:"free_disk_bytes"`
	}
	if json.Unmarshal(output, &raw) != nil {
		return Evidence{}, errors.New("invalid Windows probe evidence")
	}
	supported := raw.ProductType == 1 && raw.BuildNumber >= 22000 || (raw.ProductType == 2 || raw.ProductType == 3) && raw.BuildNumber >= 17763
	if !supported {
		return Evidence{}, errors.New("Windows 11 or Windows Server 2019 or newer is required")
	}
	evidence := Evidence{Platform: "windows", Architecture: strings.ToLower(raw.Architecture), DistributionVersion: raw.DistributionVersion,
		ServiceManager: raw.ServiceManager, Privileged: raw.Privileged, FreeDiskBytes: raw.FreeDiskBytes, IdentitySID: raw.IdentitySID,
		WindowsProductType: raw.ProductType, WindowsBuildNumber: raw.BuildNumber}
	switch evidence.Architecture {
	case "x64", "amd64":
		evidence.Architecture = "amd64"
	default:
		return Evidence{}, fmt.Errorf("unsupported Windows architecture %q", evidence.Architecture)
	}
	return validateEvidence(evidence)
}
