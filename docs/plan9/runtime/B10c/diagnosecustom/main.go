// diagnosecustom performs only a server dry-run using the batch API identity.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type traceTransport struct{ base http.RoundTripper }

func (t traceTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(request)
	if err != nil {
		fmt.Println("transport", request.Method, request.URL.Path, err)
		return nil, err
	}
	fmt.Println("HTTP", request.Method, request.URL.Path, request.Header.Get("Content-Type"), response.StatusCode)
	if response.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 8192))
		response.Body.Close()
		response.Body = io.NopCloser(bytes.NewReader(body))
		fmt.Println("API_STATUS", string(body))
	}
	return response, nil
}

func main() {
	file := flag.String("kubeconfig", "", "Private batch platform kubeconfig")
	endpoint := flag.String("endpoint", "", "Local forwarded address with the same verified server CA")
	flag.Parse()
	config, err := clientcmd.BuildConfigFromFlags(*endpoint, *file)
	if err != nil {
		panic(err)
	}
	config.Timeout = 20 * time.Second
	transport, err := rest.TransportFor(config)
	if err != nil {
		panic(err)
	}
	api, err := dynamic.NewForConfigAndClient(config, &http.Client{Transport: traceTransport{transport}, Timeout: 20 * time.Second})
	if err != nil {
		panic(err)
	}
	ctx := context.Background()
	client := kubernetesinventory.Client{Dynamic: api}
	const namespace = "b10c-workloads"
	ns, err := api.Resource(schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}).Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		panic(err)
	}
	catalog, err := client.Types(ctx)
	if err != nil {
		panic(err)
	}
	key := ""
	for _, item := range catalog.Items {
		if item.Group == "b10c.argus.test" {
			key = item.Key
		}
	}
	if key == "" {
		panic("Owned custom definition missing")
	}
	object, _ := json.Marshal(map[string]any{"apiVersion": "b10c.argus.test/v1", "kind": "Plan9Check",
		"metadata": map[string]any{"name": "b10c-check", "namespace": namespace, "labels": map[string]string{"argus.plan9/run": "p9-b10c-20261113-01"}},
		"spec":     map[string]any{"message": "Created through the normal Argus workbench", "replicas": 1}})
	_, err = client.Mutate(ctx, kubernetesinventory.Mutation{ID: "diagnostic-dryrun-only", Operation: "create", ClusterWide: true,
		Query: kubernetesinventory.Query{Type: key, Name: "b10c-check", Namespace: namespace, NamespaceUID: string(ns.GetUID())}, Object: object}, true)
	if err != nil {
		fmt.Println("MUTATION_ERROR", err)
		os.Exit(1)
	}
	fmt.Println("MUTATION_DRYRUN_OK")
}
