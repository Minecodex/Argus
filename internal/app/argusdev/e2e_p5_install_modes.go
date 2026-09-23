package argusdev

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

func (a *App) verifyP5LiteInstallation(ctx context.Context, env *E2EEnvironment) error {
	_, err := env.Kube.Client.AppsV1().Deployments(env.SandboxNS).Get(ctx, "opensandbox-server", metav1.GetOptions{})
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("P5 agent-lite requires OpenSandbox deployment to be absent: %v", err)
	}
	_, err = env.Kube.Client.CoreV1().Services(env.SandboxNS).Get(ctx, "opensandbox-server", metav1.GetOptions{})
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("P5 agent-lite requires OpenSandbox service to be absent: %v", err)
	}
	config, err := os.ReadFile(env.ConfigPath)
	if err != nil {
		return err
	}
	if err := writePrivate(filepath.Join(env.Options.Artifacts, "agent-lite-install-config.yaml"), config); err != nil {
		return err
	}
	conversation, err := a.p5Conversation(ctx, env, "lite-file-io")
	if err != nil {
		return err
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	content := []byte("files survive without a compute backend\n")
	upload, err := client.JSON(ctx, "p5-lite-upload-create", "enterprise", http.MethodPost, "/conversations/"+conversation+"/workspace/uploads", 201, map[string]any{"name": "before-compute.txt", "byte_size": len(content)}, enterpriseHeaders(env, "p5-lite-upload-create"))
	if err != nil {
		return err
	}
	uploadID, err := stringField(upload, "id")
	if err != nil {
		return err
	}
	data, _, err := p5FileHTTP(ctx, client, env, http.MethodPut, "/conversations/"+conversation+"/workspace/uploads/"+uploadID+"/content", content, 201, "")
	if err != nil {
		return err
	}
	var file map[string]any
	if err := json.Unmarshal(data, &file); err != nil {
		return err
	}
	fileID, err := stringField(file, "id")
	if err != nil {
		return err
	}
	data, _, err = p5FileHTTP(ctx, client, env, http.MethodGet, "/conversations/"+conversation+"/workspace/files/"+fileID+"/content", nil, 200, "")
	if err != nil {
		return err
	}
	if !bytes.Equal(data, content) {
		return fmt.Errorf("P5 agent-lite file IO returned different content")
	}
	if err := a.p5DeleteWorkspace(ctx, env, conversation); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(a.stdout, "P5 agent-lite installation and file IO passed without OpenSandbox")
	return nil
}

func (a *App) enableP5Sandbox(ctx context.Context, env *E2EEnvironment) error {
	config, err := os.ReadFile(env.ConfigPath)
	if err != nil {
		return err
	}
	updated, err := p5SandboxInstallConfig(config, env.ReleaseID)
	if err != nil {
		return err
	}
	if err := writePrivate(env.ConfigPath, updated); err != nil {
		return err
	}
	if err := writePrivate(filepath.Join(env.Options.Artifacts, "agent-sandbox-install-config.yaml"), updated); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(a.stdout, "P5 enabling OpenSandbox on the same persistent installation")
	defer env.clearArtifactSigningPrivateKey()
	if err := a.invokeArgusctl(ctx, env, "install", "--config", env.ConfigPath); err != nil {
		return err
	}
	return a.patchArtifactTrust(ctx, env, suiteFixtureFeatures(env.Options.Suite))
}

func p5SandboxInstallConfig(config []byte, releaseID string) ([]byte, error) {
	var document map[string]any
	if err := yaml.Unmarshal(config, &document); err != nil {
		return nil, err
	}
	spec := nestedMap(document, "spec")
	if releaseID == "" || spec["releaseId"] != releaseID {
		return nil, fmt.Errorf("P5 install configuration belongs to another release")
	}
	sandbox := nestedMap(spec, "openSandbox")
	if sandbox["enabled"] != false {
		return nil, fmt.Errorf("P5 must start with OpenSandbox explicitly disabled")
	}
	sandbox["enabled"] = true
	return yaml.Marshal(document)
}
