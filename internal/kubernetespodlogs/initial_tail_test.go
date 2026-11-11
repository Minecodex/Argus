package kubernetespodlogs

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func tailFixture(t *testing.T, logs http.HandlerFunc) kubernetes.Interface {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/namespaces/orders":
			_ = json.NewEncoder(w).Encode(corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "orders", UID: "namespace-uid"}})
		case "/api/v1/namespaces/orders/pods/api":
			_ = json.NewEncoder(w).Encode(corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "orders", UID: "pod-uid"}, Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{Name: "main", ContainerID: "containerd://running", RestartCount: 1, LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ContainerID: "containerd://old"}}}}}})
		case "/api/v1/namespaces/orders/pods/api/log":
			logs(w, r)
		default:
			t.Errorf("Unexpected endpoint %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	api, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	return api
}

func TestInitialNativeTailRetainsOldLogsAndWholeBoundary(t *testing.T) {
	q := testQuery()
	q.Since = time.Unix(0, 0).UTC()
	q.TailLines = 2
	line := "2026-10-19T10:00:01.123Z repeated\n"
	calls := 0
	api := tailFixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			if r.URL.Query().Get("tailLines") != "2" || r.URL.Query().Get("sinceTime") != "" {
				t.Error("Initial request did not use the native retained tail", r.URL.RawQuery)
			}
			_, _ = fmt.Fprint(w, strings.Repeat(line, 2))
		} else {
			if r.URL.Query().Get("tailLines") != "" || r.URL.Query().Get("sinceTime") != "2026-10-19T10:00:01Z" {
				t.Error("Boundary request changed", r.URL.RawQuery)
			}
			count := 6
			if calls == 3 {
				count = 7
			}
			_, _ = fmt.Fprint(w, strings.Repeat(line, count))
		}
	})
	first, err := Read(t.Context(), api, q)
	if err != nil || first.Content != strings.Repeat(line, 2) || first.Next.Count != 6 || calls != 2 {
		t.Fatalf("Retained tail or complete boundary lost: %+v %v calls=%d", first, err, calls)
	}
	q.Resume = &first.Next
	next, err := Read(t.Context(), api, q)
	if err != nil || next.Content != line || next.Next.Count != 7 {
		t.Fatalf("Duplicate line was lost or replayed: %+v %v", next, err)
	}
}

func TestInitialNativeTailRejectsChangedBoundary(t *testing.T) {
	q := testQuery()
	q.Since = time.Unix(0, 0).UTC()
	calls := 0
	api := tailFixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			_, _ = fmt.Fprint(w, "2026-10-19T10:00:01Z before rotation\n")
		} else {
			_, _ = fmt.Fprint(w, "2026-10-19T10:00:01Z after rotation\n")
		}
	})
	batch, err := Read(t.Context(), api, q)
	if !errors.Is(err, ErrGap) || batch.Content != "" {
		t.Fatalf("Rotation silently admitted: %+v %v", batch, err)
	}
}

func TestInitialNativeTailKeepsExplicitBoundaryBudget(t *testing.T) {
	q := testQuery()
	q.Since = time.Unix(0, 0).UTC()
	q.TailLines = 1
	line := []byte("2026-10-19T10:00:01Z busy line\n")
	calls := 0
	api := tailFixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			_, _ = w.Write(line)
		} else {
			_, _ = w.Write(bytes.Repeat(line, MaxBytes/len(line)+2))
		}
	})
	batch, err := Read(t.Context(), api, q)
	if err != nil || !batch.Truncated || batch.Content != string(line) {
		t.Fatalf("Boundary budget was hidden: %+v %v", batch, err)
	}
}

func TestInitialNativeTailStillRejectsAnInvalidIdentity(t *testing.T) {
	q := testQuery()
	q.Since = time.Unix(0, 0).UTC()
	q.Pin.PodUID = "foreign"
	api := tailFixture(t, func(w http.ResponseWriter, r *http.Request) { t.Error("Logs read before identity validation") })
	batch, err := Read(t.Context(), api, q)
	if !errors.Is(err, kubernetesinventory.ErrIdentityChanged) || batch.Content != "" {
		t.Fatal("Foreign identity read", err)
	}
}
