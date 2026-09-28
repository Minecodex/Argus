package resource

import "errors"

// ActionValidationCode exposes only explicitly supported public domain codes.
// Arbitrary worker errors and error strings must never become API error codes.
// Callers must still enforce action ownership and current authorization first.
func ActionValidationCode(err error) string {
	var failure interface{ ActionValidationCode() string }
	if errors.As(err, &failure) {
		switch failure.ActionValidationCode() {
		case "DASHBOARD_VERSION_CONFLICT":
			return failure.ActionValidationCode()
		}
	}
	return ""
}
