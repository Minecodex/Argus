package configbundle

import (
	"encoding/json"
	"errors"
	"github.com/kakj-go/Argus/internal/builtincollection"
)

const KubeletCAFile = "kubelet-ca.pem"
const KubeletCAPath = "/etc/argus-otelcol/" + KubeletCAFile

// KubernetesPublicFiles materializes only the fixed node public CA. Native
// kubelet ServiceAccount auth reads ca_file, rather than configtls ca_pem.
func KubernetesPublicFiles(raw []byte) (map[string]string, error) {
	var bundle Bundle
	if json.Unmarshal(raw, &bundle) != nil || bundle.SchemaVersion != SchemaVersion || bundle.CollectorID == "" {
		return nil, errors.New("invalid Collector configuration bundle")
	}
	files := map[string]string{}
	for _, collection := range bundle.Collections {
		if !collection.Enabled || collection.Integration != "kubernetes_node_metrics" {
			continue
		}
		value, err := builtincollection.ParseFor(collection.Integration, collection.Config)
		if err != nil {
			return nil, err
		}
		if value.ComponentTLS != nil && value.ComponentTLS.CAPEM != "" {
			files[KubeletCAFile] = value.ComponentTLS.CAPEM
		}
	}
	return files, nil
}
