package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/kakj-go/Argus/internal/connector"
	"github.com/kakj-go/Argus/internal/resource"
)

func TestNameAvailabilityContractQueries(t *testing.T) {
	for _, path := range []string{"/enterprise/hosts/name-availability", "/enterprise/bastion-scopes/name-availability"} {
		for _, tc := range []struct {
			name, query string
			status      int
		}{
			{"valid", "?name=Kakj", http.StatusNoContent},
			{"unicode", "?name=" + url.QueryEscape(strings.Repeat("机", 128)), http.StatusNoContent},
			{"missing", "", http.StatusBadRequest},
			{"empty", "?name=", http.StatusBadRequest},
			{"too long", "?name=" + strings.Repeat("a", 129), http.StatusBadRequest},
		} {
			t.Run(path+"/"+tc.name, func(t *testing.T) {
				handler := openAPIRequestValidationMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusNoContent)
				}))
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1"+path+tc.query, nil))
				if response.Code != tc.status {
					t.Fatalf("status=%d, want=%d; response=%s", response.Code, tc.status, response.Body.String())
				}
			})
		}
	}
}

func TestCreationNameErrorsAreSafeAndActionable(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		code   string
		status int
	}{
		{"duplicate host", resource.ErrResourceNameConflict, "RESOURCE_NAME_CONFLICT", http.StatusConflict},
		{"invalid name", resource.ErrInvalidResourceName, "INVALID_ARGUMENT", http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := hostErrorBase(context.Background(), tc.err)
			if body.Code != tc.code || resourceStatus(tc.err) != tc.status {
				t.Fatalf("host name error=%s/%d, want=%s/%d", body.Code, resourceStatus(tc.err), tc.code, tc.status)
			}
			if body.Retryable == nil || *body.Retryable {
				t.Fatal("a name validation failure must not be retried unchanged")
			}
			if tc.err == resource.ErrInvalidResourceName {
				encoded, _ := json.Marshal(body.Params)
				if !strings.Contains(string(encoded), `"field":"name"`) {
					t.Fatalf("invalid name must target its field: %s", encoded)
				}
				bastion := connectorError(context.Background(), tc.err)
				if bastion.Code != tc.code || connectorStatus(tc.err) != tc.status {
					t.Fatalf("bastion name error=%s/%d", bastion.Code, connectorStatus(tc.err))
				}
			}
		})
	}
	conflict := connectorError(context.Background(), connector.ErrBastionNameConflict)
	if conflict.Code != "RESOURCE_NAME_CONFLICT" || conflict.Retryable == nil || *conflict.Retryable {
		t.Fatalf("bastion conflict must be permanent: %#v", conflict)
	}
}

func TestNameAvailabilityRoutesRequireAuthentication(t *testing.T) {
	router := NewRouterWithOptions(RouterOptions{Host: &HostHandler{}, Connector: &ConnectorHandler{}})
	for _, path := range []string{"/enterprise/hosts/name-availability", "/enterprise/bastion-scopes/name-availability"} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api/v1"+path+"?name=Kakj", nil)
			router.ServeHTTP(response, request)
			if response.Code != http.StatusForbidden {
				t.Fatalf("anonymous check reached domain query: %d %s", response.Code, response.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if _, disclosed := body["available"]; disclosed {
				t.Fatal("anonymous check disclosed name availability")
			}
		})
	}
}
