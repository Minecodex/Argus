package argusdev

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// Actual API/IO/object-storage publication using a local scripted provider.
// This is file protocol acceptance, never paid-model or telemetry evidence.
func (a *App) verifyP5PublicationMedia(ctx context.Context, env *E2EEnvironment) (failure error) {
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	convo, err := a.p5Conversation(ctx, env, "Publication media regression "+env.Options.RunID)
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		defer cancel()
		failure = errors.Join(failure, a.p5DeleteWorkspace(cleanup, env, convo))
	}()
	body := []byte("# Publication protocol regression\n\nActual uploaded Markdown bytes; no monitoring values.\n")
	upload, err := client.JSON(ctx, "p5-media-upload-create", "enterprise", http.MethodPost, "/conversations/"+convo+"/workspace/uploads", 201,
		map[string]any{"name": "source-report.md", "byte_size": len(body)}, enterpriseHeaders(env, p5RequestKey("p5-media-upload", convo)))
	if err != nil {
		return err
	}
	id, err := stringField(upload, "id")
	if err != nil {
		return err
	}
	raw, _, err := p5FileHTTP(ctx, client, env, http.MethodPut, "/conversations/"+convo+"/workspace/uploads/"+id+"/content", body, 201, "")
	if err != nil {
		return err
	}
	var file struct{ ID, Path string }
	if json.Unmarshal(raw, &file) != nil || file.ID == "" || file.Path == "" {
		return errors.New("publication media upload identity missing")
	}
	title := "Friendly investigation report (no extension)"
	run, err := a.p5Run(ctx, env, convo, "publication-media", []p5ToolStep{
		{"tool.search", map[string]any{"category": "workflow"}},
		{"tool.describe", map[string]any{"category": "workflow", "name": "publish_file"}},
		{"tool.invoke", map[string]any{"category": "workflow", "name": "publish_file", "arguments": map[string]any{"path": strings.TrimPrefix(file.Path, "/workspace/"), "name": title}}},
	}, []string{file.ID})
	if err != nil {
		return err
	}
	value, err := a.postgresQuery(ctx, env, "SELECT json_build_object('id',id,'name',name,'media',media_type,'hash',content_hash,'bytes',byte_size) FROM file_deliveries WHERE run_id='"+run+"'")
	if err != nil {
		return err
	}
	var delivery struct {
		ID, Name, Media, Hash string
		Bytes                 int
	}
	if json.Unmarshal([]byte(value), &delivery) != nil || delivery.Name != title || delivery.Bytes != len(body) || delivery.Hash != fmt.Sprintf("%x", sha256.Sum256(body)) {
		return errors.New("publication metadata changed original file bytes or display title")
	}
	download, headers, err := p5FileHTTP(ctx, client, env, http.MethodGet, "/conversations/"+convo+"/deliveries/"+delivery.ID+"/content", nil, 200, "")
	if err != nil {
		return err
	}
	contentType, _, err := mime.ParseMediaType(headers.Get("Content-Type"))
	mediaType, _, mediaErr := mime.ParseMediaType(delivery.Media)
	if err != nil || mediaErr != nil || contentType != "application/octet-stream" || mediaType != "text/markdown" || !bytes.Equal(download, body) {
		return fmt.Errorf("publication media mismatch: download=%q metadata=%q bytes_equal=%t", contentType, delivery.Media, bytes.Equal(download, body))
	}
	if headers.Get("X-Content-Type-Options") != "nosniff" || !strings.HasPrefix(headers.Get("Content-Disposition"), "attachment;") || headers.Get("ETag") != `"sha256:`+delivery.Hash+`"` {
		return errors.New("publication download lost attachment/nosniff/hash contract")
	}
	proof, _ := json.MarshalIndent(map[string]any{"passed": true, "scope": "actual uploaded file -> local scripted model native Search/Describe/Publish -> object store -> authorized HTTP download", "file_media_type": mediaType, "download_transport_media_type": contentType, "attachment_and_nosniff": true, "etag_matches_content_hash": true, "display_name": title, "bytes": len(body), "sha256": delivery.Hash, "external_model_business_calls": 0, "whole_plan_complete": false}, "", "  ")
	return writePrivate(filepath.Join(env.Options.Artifacts, "p5-publication-media.json"), proof)
}
