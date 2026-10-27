package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strings"
)

func plan8BackendTarget(key, value string) (string, string, error) {
	var host, port, containerPort string
	if key == "ARGUS_MONITORING_TEST_DATABASE_URL" {
		u, e := url.Parse(value)
		if e != nil || u == nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Path != "/argus_plan8" {
			return "", "", errors.New("Plan8 requires the isolated argus_plan8 database")
		}
		host, port, containerPort = u.Hostname(), u.Port(), "5432/tcp"
	} else if key == "ARGUS_PLAN8_REDIS_TEST_URL" {
		u, e := url.Parse(value)
		if e != nil || u == nil || (u.Scheme != "redis" && u.Scheme != "rediss") || u.Path != "/0" {
			return "", "", errors.New("Plan8 requires the explicit isolated Redis database zero")
		}
		host, port, containerPort = u.Hostname(), u.Port(), "6379/tcp"
	} else if key == "ARGUS_CLICKHOUSE_TEST_ADDRESS" {
		var e error
		host, port, e = net.SplitHostPort(value)
		if e != nil {
			return "", "", errors.New("Plan8 requires an explicit ClickHouse address and port")
		}
		containerPort = "9000/tcp"
	} else {
		return "", "", errors.New("unknown Plan8 backend input")
	}
	if (host != "127.0.0.1" && host != "localhost" && host != "::1") || port == "" {
		return "", "", errors.New("Plan8 component backends must use an explicitly published local test port")
	}
	return containerPort, port, nil
}

func (a *App) plan8OwnedBackend(ctx context.Context, key, value string) error {
	containerPort, port, err := plan8BackendTarget(key, value)
	if err != nil {
		return err
	}
	names, err := a.runner.Output(ctx, nil, "docker", "ps", "--format", "{{.Names}}")
	if err != nil {
		return errors.New("cannot verify owned Plan8 backend containers")
	}
	for _, name := range strings.Fields(names) {
		if !strings.HasPrefix(name, "argus-plan8-") {
			continue
		}
		raw, e := a.runner.Output(ctx, nil, "docker", "inspect", name)
		if e != nil {
			continue
		}
		var rows []struct {
			Config          struct{ Labels map[string]string }
			NetworkSettings struct {
				Ports map[string][]struct{ HostIp, HostPort string }
			}
		}
		if json.Unmarshal([]byte(raw), &rows) != nil || len(rows) != 1 {
			continue
		}
		labels := rows[0].Config.Labels
		owner := labels["argus.owner"]
		if owner == "" {
			owner = labels["io.argus.plan8.owner"]
		}
		if !strings.HasPrefix(owner, "plan8-") {
			continue
		}
		for _, binding := range rows[0].NetworkSettings.Ports[containerPort] {
			if binding.HostPort == port && (binding.HostIp == "127.0.0.1" || binding.HostIp == "::1") {
				return nil
			}
		}
	}
	return errors.New("backend port is not bound to an owned, loopback-only argus-plan8 container")
}
