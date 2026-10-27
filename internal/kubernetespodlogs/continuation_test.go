package kubernetespodlogs

import (
	"bytes"
	"context"
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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func testQuery() Query {
	return Query{APIServer: "https://cluster.example.test", ContinuityKey: bytes.Repeat([]byte{7}, 32), Pin: Pin{Namespace: "orders", NamespaceUID: "namespace-uid", Pod: "api", PodUID: "pod-uid", Container: "main", ContainerID: "containerd://running", RestartCount: 1, PreviousContainerID: "containerd://old"}, Since: time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC), TailLines: 100}
}
func TestLogContinuationRetainsDuplicatesAndSecondPrecision(t *testing.T) {
	q := testQuery()
	q.TailLines = 1
	first := []byte("2026-10-19T10:00:01.123Z repeated\n2026-10-19T10:00:01.123Z repeated\n")
	batch, err := Continue(first, q, time.Now())
	if err != nil || batch.Content != "2026-10-19T10:00:01.123Z repeated\n" || batch.Next.Count != 2 {
		t.Fatalf("initial tail: %+v %v", batch, err)
	}
	q.Resume = &batch.Next
	next := append(bytes.Clone(first), []byte("2026-10-19T10:00:01.123Z repeated\n2026-10-19T10:00:02.001Z next\n")...)
	batch, err = Continue(next, q, time.Now())
	if err != nil || strings.Count(batch.Content, "repeated") != 1 || !strings.Contains(batch.Content, "next") || batch.Next.Count != 1 || batch.Next.Second.Second() != 2 {
		t.Fatalf("same-time legitimate duplicates lost: %+v %v", batch, err)
	}
	q.Resume = &batch.Next
	batch, err = Continue([]byte("2026-10-19T10:00:02.001Z next\n"), q, time.Now())
	if err != nil || batch.Content != "" {
		t.Fatalf("empty incremental read: %+v %v", batch, err)
	}
}
func TestLogContinuationRejectsRotatedOrReorderedPrefix(t *testing.T) {
	q := testQuery()
	initial, err := Continue([]byte("2026-10-19T10:00:01Z original\n"), q, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	q.Resume = &initial.Next
	for _, raw := range []string{"2026-10-19T10:00:01Z replaced\n", "2026-10-19T10:00:02Z skipped\n", ""} {
		if batch, e := Continue([]byte(raw), q, time.Now()); !errors.Is(e, ErrGap) || batch.Content != "" {
			t.Fatalf("continuity failure exposed content: %v", e)
		}
	}
	q.Resume = nil
	if _, err = Continue([]byte("2026-10-19T10:00:02Z newer\n2026-10-19T10:00:01Z older\n"), q, time.Now()); !errors.Is(err, ErrGap) {
		t.Fatal("clock reversal accepted")
	}
}
func TestLogContinuationWaitsForPartialLineAndMarksBudget(t *testing.T) {
	q := testQuery()
	first, err := Continue([]byte("2026-10-19T10:00:01Z full\n2026-10-19T10:00:01Z part"), q, time.Now())
	if err != nil || !first.PendingLine || strings.Contains(first.Content, "part") || first.Next.Count != 1 {
		t.Fatalf("partial line emitted: %+v %v", first, err)
	}
	q.Resume = &first.Next
	next, err := Continue([]byte("2026-10-19T10:00:01Z full\n2026-10-19T10:00:01Z partial finished\n"), q, time.Now())
	if err != nil || !strings.Contains(next.Content, "partial finished") {
		t.Fatal(err)
	}
	q.Resume = nil
	raw := bytes.Repeat([]byte("2026-10-19T10:00:01Z log\n"), MaxBytes/20)
	batch, err := Continue(raw, q, time.Now())
	if err != nil || !batch.Truncated {
		t.Fatalf("budget did not stop follow: %v", err)
	}
}
func TestPinnedLogReadRejectsReplacementAfterNativeResponse(t *testing.T) {
	q := testQuery()
	replaced := false
	reads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/namespaces/orders":
			_ = json.NewEncoder(w).Encode(corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "orders", UID: "namespace-uid"}})
		case "/api/v1/namespaces/orders/pods/api":
			uid := types.UID("pod-uid")
			if replaced {
				uid = "foreign-pod"
			}
			_ = json.NewEncoder(w).Encode(corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "orders", UID: uid}, Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{Name: "main", ContainerID: "containerd://running", RestartCount: 1, LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ContainerID: "containerd://old"}}}}}})
		case "/api/v1/namespaces/orders/pods/api/log":
			reads++
			replaced = true
			_, _ = fmt.Fprint(w, "2026-10-19T10:00:01Z foreign-private-log\n")
		default:
			t.Errorf("unexpected API path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	api, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := Read(context.Background(), api, q)
	if !errors.Is(err, kubernetesinventory.ErrIdentityChanged) || batch.Content != "" || reads != 1 {
		t.Fatalf("replacement log disclosure: %+v %v", batch, err)
	}
	if _, err = Read(context.Background(), api, q); !errors.Is(err, kubernetesinventory.ErrIdentityChanged) || reads != 1 {
		t.Fatal("changed UID read logs before validation")
	}
}
