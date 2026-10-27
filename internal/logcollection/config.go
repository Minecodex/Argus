// Package logcollection describes bounded log rules independently of storage,
// authorization and the Collector lifecycle transport.
package logcollection

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

const Version = "argus.log_rule/v1"

var ErrInvalid = errors.New("invalid log collection rule")

var (
	unitName      = regexp.MustCompile(`^[A-Za-z0-9_.@:-]{1,200}$`)
	channelName   = regexp.MustCompile(`^[A-Za-z0-9_./ -]{1,200}$`)
	containerName = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$`)
	windowsDrive  = regexp.MustCompile(`^[A-Za-z]:/`)
)

type Config struct {
	Scope          string    `json:"scope"`
	Include        []string  `json:"include,omitempty"`
	Exclude        []string  `json:"exclude,omitempty"`
	StartAt        string    `json:"start_at"`
	Encoding       string    `json:"encoding"`
	Format         string    `json:"format"`
	MultilineStart string    `json:"multiline_start,omitempty"`
	MaxLogBytes    int       `json:"max_log_bytes"`
	Units          []string  `json:"units,omitempty"`
	Channel        string    `json:"channel,omitempty"`
	Workload       *Workload `json:"workload,omitempty"`
}
type Workload struct {
	NamespaceID  uuid.UUID `json:"namespace_id"`
	Namespace    string    `json:"namespace,omitempty"`
	NamespaceUID string    `json:"namespace_uid,omitempty"`
	Type         string    `json:"type"`
	Name         string    `json:"name"`
	UID          string    `json:"uid"`
	Containers   []string  `json:"containers"`
	// Ancestors are filled by the control plane from live UID owner references.
	Ancestors []string `json:"ancestors,omitempty"`
}

func IsIntegration(key string) bool {
	return slices.Contains([]string{"filelog", "journald", "eventlog", "k8slog"}, key)
}
func Normalize(key, platform string, raw []byte) (Config, error) {
	var c Config
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if len(raw) > 64<<10 || decoder.Decode(&c) != nil || decoder.Decode(new(any)) != io.EOF || !IsIntegration(key) {
		return c, ErrInvalid
	}
	if c.Scope != "business" && c.Scope != "system" && c.Scope != "component" && c.Scope != "resource" {
		return c, ErrInvalid
	}
	if c.StartAt == "" {
		c.StartAt = "end"
	}
	if c.StartAt != "end" && c.StartAt != "beginning" {
		return c, ErrInvalid
	}
	if c.Encoding == "" {
		c.Encoding = "utf-8"
	}
	if !slices.Contains([]string{"utf-8", "utf-16le", "utf-16be", "ascii"}, c.Encoding) {
		return c, ErrInvalid
	}
	if c.Format == "" {
		c.Format = "text"
	}
	if c.Format != "text" && c.Format != "json" {
		return c, ErrInvalid
	}
	if c.MaxLogBytes == 0 {
		c.MaxLogBytes = 256 << 10
	}
	if c.MaxLogBytes < 1024 || c.MaxLogBytes > 1<<20 {
		return c, ErrInvalid
	}
	if len(c.MultilineStart) > 1024 {
		return c, ErrInvalid
	}
	if c.MultilineStart != "" {
		r, err := regexp.Compile(c.MultilineStart)
		if err != nil || r.MatchString("") {
			return c, ErrInvalid
		}
	}
	windows := platform == "windows_amd64"
	if !windows && !slices.Contains([]string{"linux_amd64", "linux_arm64"}, platform) {
		return c, ErrInvalid
	}
	switch key {
	case "filelog":
		if len(c.Include) == 0 || len(c.Include) > 16 || len(c.Exclude) > 16 || c.Workload != nil || len(c.Units) > 0 || c.Channel != "" {
			return c, ErrInvalid
		}
		for _, patterns := range [][]string{c.Include, c.Exclude} {
			for i, value := range patterns {
				normalized, err := NormalizePattern(value, windows)
				if err != nil {
					return c, err
				}
				patterns[i] = normalized
			}
			slices.Sort(patterns)
		}
		c.Include = slices.Compact(c.Include)
		c.Exclude = slices.Compact(c.Exclude)
		for _, path := range c.Include {
			if IsReservedRuntimePath(path) || !windows && IsKubernetesContainerPath(path) {
				return c, ErrInvalid
			}
		}
	case "journald":
		if windows || c.Workload != nil || len(c.Include) > 0 || len(c.Exclude) > 0 || c.Channel != "" || len(c.Units) > 64 || c.Scope == "business" && len(c.Units) == 0 || c.MultilineStart != "" || c.Format != "text" {
			return c, ErrInvalid
		}
		for i, unit := range c.Units {
			if !unitName.MatchString(unit) {
				return c, ErrInvalid
			}
			// Normalize journalctl's implicit service suffix before checking claims.
			if !slices.ContainsFunc([]string{".service", ".socket", ".device", ".mount", ".automount", ".swap", ".target", ".path", ".timer", ".slice", ".scope"}, func(suffix string) bool { return strings.HasSuffix(unit, suffix) }) {
				unit += ".service"
			}
			if len(unit) > 200 {
				return c, ErrInvalid
			}
			c.Units[i] = unit
		}
		slices.Sort(c.Units)
		c.Units = slices.Compact(c.Units)
	case "eventlog":
		if !windows || c.Workload != nil || len(c.Include) > 0 || len(c.Exclude) > 0 || len(c.Units) > 0 || !channelName.MatchString(c.Channel) || c.MultilineStart != "" || c.Format != "text" {
			return c, ErrInvalid
		}
	case "k8slog":
		w := c.Workload
		if windows || w == nil || w.NamespaceID == uuid.Nil || w.UID == "" || w.Name == "" || len(w.UID) > 128 || len(w.Name) > 253 || len(c.Include) > 0 || len(c.Exclude) > 0 || len(c.Units) > 0 || c.Channel != "" || len(w.Containers) == 0 || len(w.Containers) > 32 || !slices.Contains([]string{"pod", "deployment", "replicaset", "daemonset", "statefulset", "job", "cronjob"}, w.Type) {
			return c, ErrInvalid
		}
		for _, name := range w.Containers {
			if !containerName.MatchString(name) {
				return c, ErrInvalid
			}
		}
		slices.Sort(w.Containers)
		w.Containers = slices.Compact(w.Containers)
	}
	return c, nil
}

func NormalizePattern(value string, windows bool) (string, error) {
	if value == "" || utf8.RuneCountInString(value) > 1024 || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\x00\r\n[]{}") {
		return "", ErrInvalid
	}
	if windows {
		value = strings.ReplaceAll(value, `\`, "/")
		if !windowsDrive.MatchString(value) {
			return "", ErrInvalid
		}
		value = strings.ToUpper(value[:1]) + value[1:]
	} else if !strings.HasPrefix(value, "/") || strings.Contains(value, `\`) {
		return "", ErrInvalid
	}
	for _, part := range strings.Split(value, "/") {
		if part == ".." {
			return "", ErrInvalid
		}
	}
	if strings.Contains(value, "***") {
		return "", ErrInvalid
	}
	return path.Clean(value), nil
}
