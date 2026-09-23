// Package schemas embeds authoritative runtime contracts.
package schemas

import _ "embed"

//go:embed agent/tool-discovery.schema.json
var ToolDiscovery []byte

//go:embed agent/tool-manifest.schema.json
var ToolManifest []byte
