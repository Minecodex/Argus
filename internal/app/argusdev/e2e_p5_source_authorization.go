package argusdev

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (a *App) verifyP5ImportedSourceRevocation(ctx context.Context, env *E2EEnvironment) error {
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	convo, err := a.p5Conversation(ctx, env, "imported source authorization")
	if err != nil {
		return err
	}
	run, err := a.p5Run(ctx, env, convo, "import-source-query", []p5ToolStep{{"tool.invoke", map[string]any{"category": "host", "name": "list", "arguments": map[string]any{}}}}, []string{})
	if err != nil {
		return err
	}
	ref, err := a.postgresQuery(ctx, env, "SELECT a.result_ref FROM artifacts a JOIN tool_results x ON x.artifact_id=a.id JOIN tool_calls t ON t.id=x.tool_call_id WHERE t.run_id='"+run+"' AND t.status='succeeded' AND t.tool_id='tool.invoke' LIMIT 1;")
	if err != nil {
		return err
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return fmt.Errorf("P5 import source result missing")
	}
	importRun, err := a.p5Run(ctx, env, convo, "import-source-write", []p5ToolStep{
		{"tool.invoke", map[string]any{"category": "workflow", "name": "import_result", "arguments": map[string]any{"result_ref": ref, "path": "source.json"}}},
		{"bash", map[string]any{"command": "cp /workspace/source.json /workspace/derived.json && test -s /workspace/derived.json"}},
		{"tool.invoke", map[string]any{"category": "workflow", "name": "publish_file", "arguments": map[string]any{"path": "derived.json"}}},
	}, []string{})
	if err != nil {
		return err
	}
	if err := a.waitPostgresValue(ctx, env, "SELECT count(*) FROM tool_calls t JOIN tool_results r ON r.tool_call_id=t.id WHERE t.run_id='"+importRun+"' AND t.tool_id='bash' AND t.status='succeeded' AND r.projection->'summary'->>'exit_code'='0';", "1", time.Minute); err != nil {
		return fmt.Errorf("P5 derived file copy did not succeed: %w", err)
	}
	delivery, err := a.postgresQuery(ctx, env, "SELECT id::text FROM file_deliveries WHERE run_id='"+importRun+"' AND conversation_id='"+convo+"' AND name='derived.json' AND cardinality(source_result_refs)=1;")
	if err != nil {
		return err
	}
	delivery = strings.TrimSpace(delivery)
	if delivery == "" {
		return fmt.Errorf("P5 derived publication or its provenance is missing")
	}
	file, err := a.postgresQuery(ctx, env, "SELECT id::text FROM workspace_files WHERE conversation_id='"+convo+"' AND path='source.json';")
	if err != nil {
		return err
	}
	file = strings.TrimSpace(file)
	if file == "" {
		return fmt.Errorf("P5 source import did not complete")
	}
	sourceBytes, _, err := p5FileHTTP(ctx, client, env, http.MethodGet, "/conversations/"+convo+"/workspace/files/"+file+"/content", nil, 200, "")
	if err != nil {
		return err
	}
	deliveryPath := "/conversations/" + convo + "/deliveries/" + delivery + "/content"
	published, _, err := p5FileHTTP(ctx, client, env, http.MethodGet, deliveryPath, nil, 200, "")
	if err != nil {
		return err
	}
	if len(sourceBytes) == 0 || !bytes.Equal(sourceBytes, published) {
		return fmt.Errorf("P5 derived publication differs from imported source bytes")
	}
	before, err := a.p5WorkspacePod(ctx, env, convo)
	if err != nil {
		return err
	}
	stateSQL := "SELECT id::text||':'||fence_token::text||':'||cardinality(source_result_refs)::text FROM workspaces WHERE conversation_id='" + convo + "' AND status='ready';"
	fence, err := a.postgresQuery(ctx, env, stateSQL)
	if err != nil {
		return err
	}
	if !strings.HasSuffix(strings.TrimSpace(fence), ":1") {
		return fmt.Errorf("P5 import provenance was not retained")
	}
	if _, err = a.postgresQuery(ctx, env, "UPDATE enterprise_users SET authorization_version=authorization_version+1 WHERE id='"+env.State.Values["admin_user_id"]+"' AND enterprise_id='"+env.State.Values["enterprise_id"]+"';"); err != nil {
		return err
	}
	if err = a.refreshEnterpriseLogin(ctx, env); err != nil {
		return err
	}
	if _, _, err = p5FileHTTP(ctx, client, env, http.MethodGet, "/conversations/"+convo+"/workspace/files/"+file+"/content", nil, 403, ""); err != nil {
		return err
	}
	if _, _, err = p5FileHTTP(ctx, client, env, http.MethodGet, deliveryPath, nil, 403, ""); err != nil {
		return err
	}
	for _, tool := range []string{"read", "bash"} {
		args := map[string]any{"path": "derived.json"}
		if tool == "bash" {
			// Force the second attempt through a cold workload path. A denied
			// directory must not create a replacement writer or delete its PVC.
			if err = env.Kube.Client.CoreV1().Pods(before.Namespace).Delete(ctx, before.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &before.UID}}); err != nil {
				return err
			}
			deadline := time.Now().Add(time.Minute)
			for {
				_, err := env.Kube.Client.CoreV1().Pods(before.Namespace).Get(ctx, before.Name, metav1.GetOptions{})
				if apierrors.IsNotFound(err) {
					break
				}
				if err != nil {
					return err
				}
				if time.Now().After(deadline) {
					return fmt.Errorf("P5 old Workspace Pod did not terminate")
				}
				if err := waitContext(ctx, time.Second); err != nil {
					return err
				}
			}
			args = map[string]any{"command": "cat /workspace/derived.json; printf unauthorized > /workspace/forbidden-write"}
		}
		denied, err := a.p5Run(ctx, env, convo, "revoked-source-"+tool, []p5ToolStep{{tool, args}}, []string{})
		if err != nil {
			return err
		}
		if err = a.waitPostgresValue(ctx, env, "SELECT count(*) FROM tool_calls WHERE run_id='"+denied+"' AND tool_id='"+tool+"' AND status='failed' AND error_code='TOOL_RESULT_FORBIDDEN';", "1", time.Minute); err != nil {
			return err
		}
	}
	after, err := a.postgresQuery(ctx, env, stateSQL)
	if err != nil {
		return err
	}
	if strings.TrimSpace(after) != strings.TrimSpace(fence) {
		return fmt.Errorf("P5 revoked tool acquired a writer lease or changed provenance")
	}
	if _, err = client.JSON(ctx, "p5-revoked-source-delete", "enterprise", http.MethodDelete, "/conversations/"+convo+"/workspace", 202, nil, enterpriseHeaders(env, "p5-revoked-source-delete")); err != nil {
		return err
	}
	if err = a.waitPostgresValue(ctx, env, "SELECT count(*) FROM workspaces WHERE conversation_id='"+convo+"' AND status<>'deleted';", "0", 3*time.Minute); err != nil {
		return err
	}
	data, _ := json.Marshal(map[string]any{"source_imported": true, "derived_file_created": true, "published_bytes_match_source": true, "delivery_download_denied": true, "source_revoked": true, "file_download_denied": true, "warm_read_denied": true, "cold_bash_denied": true, "writer_fence_unchanged": true, "explicit_delete_completed": true})
	return writePrivate(filepath.Join(env.Options.Artifacts, "p5-source-authorization.json"), append(data, '\n'))
}
