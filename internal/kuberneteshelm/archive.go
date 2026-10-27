package kuberneteshelm

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"path"
	"strings"

	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	"sigs.k8s.io/yaml"
)

func loadChart(raw []byte) (*chart.Chart, error) {
	if len(raw) == 0 || len(raw) > MaxArchiveBytes {
		return nil, ErrBudget
	}
	total, files := int64(0), 0
	if e := checkArchive(raw, 0, &total, &files); e != nil {
		return nil, e
	}
	ch, e := loader.LoadArchive(bytes.NewReader(raw))
	if e != nil {
		return nil, ErrInvalid
	}
	if ch.Metadata == nil || ch.Metadata.Name == "" || ch.Metadata.Type != "" && ch.Metadata.Type != "application" {
		return nil, ErrInvalid
	}
	return ch, nil
}
func checkArchive(raw []byte, depth int, total *int64, files *int) error {
	if depth > 8 {
		return ErrBudget
	}
	z, e := gzip.NewReader(bytes.NewReader(raw))
	if e != nil {
		return ErrInvalid
	}
	defer z.Close()
	reader := tar.NewReader(io.LimitReader(z, MaxExpandedBytes+1))
	seen := map[string]bool{}
	for {
		header, e := reader.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return ErrInvalid
		}
		*files++
		name := strings.ReplaceAll(header.Name, "\\", "/")
		clean := path.Clean(name)
		if *files > 1000 || header.Size < 0 || header.Size > MaxExpandedBytes || clean == "." || clean != strings.TrimSuffix(name, "/") || strings.HasPrefix(clean, "/") || strings.HasPrefix(clean, "../") || strings.Contains(clean, ":") || header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeDir || seen[clean] {
			return ErrInvalid
		}
		seen[clean] = true
		*total += header.Size
		if *total > MaxExpandedBytes {
			return ErrBudget
		}
		if header.Typeflag == tar.TypeReg && strings.Contains(clean, "/charts/") && strings.HasSuffix(clean, ".tgz") {
			content, e := io.ReadAll(reader)
			if e != nil {
				return ErrInvalid
			}
			if e = checkArchive(content, depth+1, total, files); e != nil {
				return e
			}
			continue
		}
		if _, e = io.Copy(io.Discard, reader); e != nil {
			return ErrInvalid
		}
	}
	return nil
}
func parseValues(raw []byte) (map[string]any, error) {
	if len(raw) > MaxValuesBytes {
		return nil, ErrBudget
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return map[string]any{}, nil
	}
	// JSON also accepts YAML via the public parser; duplicate keys are rejected.
	encoded, e := yaml.YAMLToJSONStrict(raw)
	if e != nil {
		return nil, ErrInvalid
	}
	var values map[string]any
	if json.Unmarshal(encoded, &values) != nil || values == nil {
		return nil, ErrInvalid
	}
	return values, nil
}
