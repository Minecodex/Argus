package argusctl

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestHelmConfigurationUsesMergedKubeconfigAndRejectsInvalidContext(t *testing.T) {
	t.Run("merged_context", func(t *testing.T) {
		dir := t.TempDir()
		files := []string{}
		for i, name := range []string{"first", "second"} {
			path := filepath.Join(dir, name+".yaml")
			config := clientcmdapi.NewConfig()
			server := "https://127.0.0.1:12345"
			if i == 1 {
				server = "https://127.0.0.1:12346"
			}
			config.Clusters[name] = &clientcmdapi.Cluster{Server: server}
			config.AuthInfos[name] = &clientcmdapi.AuthInfo{Token: name + "-fixture-only"}
			config.Contexts[name] = &clientcmdapi.Context{Cluster: name, AuthInfo: name}
			config.CurrentContext = name
			if err := clientcmd.WriteToFile(*config, path); err != nil {
				t.Fatal(err)
			}
			files = append(files, path)
		}
		value := files[0] + string(os.PathListSeparator) + files[1]
		t.Setenv("KUBECONFIG", value)
		manager := helmManager{contextName: "second", log: io.Discard}
		configuration, err := manager.configuration("argus-context-fixture")
		if err != nil {
			t.Fatal(err)
		}
		actual, err := configuration.RESTClientGetter.ToRESTConfig()
		if err != nil {
			t.Fatal("Helm treated a Kubernetes config search list as one filename", err)
		}
		if actual.Host != "https://127.0.0.1:12346" || actual.BearerToken != "second-fixture-only" {
			t.Fatal("Helm selected another context identity")
		}
		if os.Getenv("KUBECONFIG") != value {
			t.Fatal("Helm changed the ambient Kubernetes config list")
		}
	})
	t.Run("invalid_config", func(t *testing.T) {
		t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "missing.yaml"))
		if _, err := (helmManager{contextName: "missing", log: io.Discard}).configuration("argus-context-fixture"); err == nil {
			t.Fatal("invalid Kubernetes configuration reached the lazy Helm client")
		}
	})
}
