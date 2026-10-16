package schemas

import _ "embed"

//go:embed alerts/alerts.schema.json
var AlertContracts []byte

//go:embed alerts/result.html
var AlertResultTemplate string
