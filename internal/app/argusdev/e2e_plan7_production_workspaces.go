package argusdev

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Exercise the normal file API after the production rollout. Fenced access
// replaces an old helper image while retaining the exact PVC and file bytes.
func (a *App) refreshPlan7ProductionWorkspaces(ctx context.Context, env *E2EEnvironment) error {
	if env.Options.Suite != "plan7" {
		return nil
	}
	query := fmt.Sprintf(`SELECT coalesce(json_agg(t),'[]') FROM (SELECT w.id,w.conversation_id,w.namespace,w.pvc_name FROM workspaces w JOIN conversations c ON c.id=w.conversation_id WHERE w.enterprise_id='%s' AND c.owner_user_id='%s' AND w.namespace='%s' AND w.status='ready' ORDER BY w.id) t`, env.State.Values["enterprise_id"], env.State.Values["admin_user_id"], env.SandboxNS)
	raw, err := a.postgresQuery(ctx, env, query)
	if err != nil {
		return err
	}
	var rows []struct {
		ID             uuid.UUID `json:"id"`
		ConversationID uuid.UUID `json:"conversation_id"`
		Namespace      string    `json:"namespace"`
		PVCName        string    `json:"pvc_name"`
	}
	if err = json.Unmarshal([]byte(raw), &rows); err != nil {
		return err
	}
	if len(rows) == 0 {
		return fmt.Errorf("Plan7 production rollout has no owned Workspace evidence")
	}
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	proof := []map[string]any{}
	for _, row := range rows {
		if row.Namespace != env.SandboxNS || row.ID == uuid.Nil || row.ConversationID == uuid.Nil {
			return fmt.Errorf("workspace rollout scope is invalid")
		}
		claim, err := env.Kube.Client.CoreV1().PersistentVolumeClaims(row.Namespace).Get(ctx, row.PVCName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if claim.Labels["argus.io/workspace-id"] != row.ID.String() {
			return fmt.Errorf("workspace rollout PVC ownership mismatch")
		}
		files, err := client.JSON(ctx, "p7-rollout-files", "enterprise", http.MethodGet, "/conversations/"+row.ConversationID.String()+"/workspace/files", 200, nil, enterpriseHeaders(env, ""))
		if err != nil {
			return err
		}
		prior := objectItems(files)
		body := []byte("Plan7 production file helper " + env.Options.RunID)
		upload, err := client.JSON(ctx, "p7-rollout-upload", "enterprise", http.MethodPost, "/conversations/"+row.ConversationID.String()+"/workspace/uploads", 201, map[string]any{"name": "runtime-image-proof.txt", "byte_size": len(body)}, enterpriseHeaders(env, "p7-rollout-"+row.ID.String()))
		if err != nil {
			return err
		}
		id, err := stringField(upload, "id")
		if err != nil {
			return err
		}
		if _, _, err = p5FileHTTP(ctx, client, env, http.MethodPut, "/conversations/"+row.ConversationID.String()+"/workspace/uploads/"+id+"/content", body, 201, ""); err != nil {
			return err
		}
		after, err := env.Kube.Client.CoreV1().PersistentVolumeClaims(row.Namespace).Get(ctx, row.PVCName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if after.UID != claim.UID || after.Spec.VolumeName != claim.Spec.VolumeName {
			return fmt.Errorf("workspace rollout replaced persistent data")
		}
		checked := ""
		if len(prior) > 0 {
			fileID, e := stringField(prior[0], "id")
			if e != nil {
				return e
			}
			expected, e := stringField(prior[0], "content_hash")
			if e != nil {
				return e
			}
			content, _, e := p5FileHTTP(ctx, client, env, http.MethodGet, "/conversations/"+row.ConversationID.String()+"/workspace/files/"+fileID+"/content", nil, 200, "")
			if e != nil {
				return e
			}
			checked = fmt.Sprintf("%x", sha256.Sum256(content))
			if checked != expected {
				return fmt.Errorf("workspace bytes changed across image rollout")
			}
		}
		proof = append(proof, map[string]any{"workspace_id": row.ID, "conversation_id": row.ConversationID, "pvc_uid": claim.UID, "persistent_volume": claim.Spec.VolumeName, "existing_file_hash_verified": checked, "prior_files": len(prior)})
	}
	data, _ := json.MarshalIndent(map[string]any{"run_id": env.Options.RunID, "workspaces": proof, "existing_volumes_preserved": true}, "", "  ")
	return writePrivate(filepath.Join(env.Options.Artifacts, "plan7-production-workspaces.json"), data)
}
