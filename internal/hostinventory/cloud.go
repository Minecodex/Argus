package hostinventory

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// Metadata requests are local, fixed, bounded and proxy-free. No credentials,
// user-data, tags, arbitrary URLs or redirects are collected.
var metadataClient = &http.Client{Timeout: 650 * time.Millisecond, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

func metadata(ctx context.Context, method, url string, headers map[string]string) string {
	request, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return ""
	}
	for k, v := range headers {
		request.Header.Set(k, v)
	}
	response, err := metadataClient.Do(request)
	if err != nil {
		return ""
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ""
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 16385))
	if err != nil || len(raw) > 16384 {
		return ""
	}
	return strings.TrimSpace(string(raw))
}
func collectCloud(ctx context.Context) Cloud {
	token := metadata(ctx, "PUT", "http://169.254.169.254/latest/api/token", map[string]string{"X-aws-ec2-metadata-token-ttl-seconds": "60"})
	if token != "" {
		raw := metadata(ctx, "GET", "http://169.254.169.254/latest/dynamic/instance-identity/document", map[string]string{"X-aws-ec2-metadata-token": token})
		var identity struct {
			InstanceID string `json:"instanceId"`
			Region     string `json:"region"`
			Zone       string `json:"availabilityZone"`
		}
		if json.Unmarshal([]byte(raw), &identity) == nil && identity.InstanceID != "" {
			return Cloud{"available", "aws", identity.InstanceID, identity.Region, identity.Zone}
		}
	}
	raw := metadata(ctx, "GET", "http://169.254.169.254/metadata/instance/compute?api-version=2021-02-01", map[string]string{"Metadata": "true"})
	var azure struct {
		VMID     string `json:"vmId"`
		Location string `json:"location"`
		Zone     string `json:"zone"`
	}
	if json.Unmarshal([]byte(raw), &azure) == nil && azure.VMID != "" {
		return Cloud{"available", "azure", azure.VMID, azure.Location, azure.Zone}
	}
	headers := map[string]string{"Metadata-Flavor": "Google"}
	id := metadata(ctx, "GET", "http://169.254.169.254/computeMetadata/v1/instance/id", headers)
	if id != "" && strings.Trim(id, "0123456789") == "" {
		zone := metadata(ctx, "GET", "http://169.254.169.254/computeMetadata/v1/instance/zone", headers)
		parts := strings.Split(zone, "/")
		zone = parts[len(parts)-1]
		region := ""
		if split := strings.LastIndex(zone, "-"); split > 0 {
			region = zone[:split]
		}
		return Cloud{"available", "gcp", id, region, zone}
	}
	// Alibaba ECS has a separate fixed metadata endpoint and IMDSv2 token.
	token = metadata(ctx, "PUT", "http://100.100.100.200/latest/api/token", map[string]string{"X-aliyun-ecs-metadata-token-ttl-seconds": "60"})
	if token != "" {
		headers = map[string]string{"X-aliyun-ecs-metadata-token": token}
		id = metadata(ctx, "GET", "http://100.100.100.200/latest/meta-data/instance-id", headers)
		if strings.HasPrefix(id, "i-") {
			return Cloud{"available", "alibaba", id, metadata(ctx, "GET", "http://100.100.100.200/latest/meta-data/region-id", headers), metadata(ctx, "GET", "http://100.100.100.200/latest/meta-data/zone-id", headers)}
		}
	}
	return Cloud{Status: "not_detected"}
}
