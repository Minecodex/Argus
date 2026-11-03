package workspace

import (
	"mime"
	"path"
)

// A friendly delivery title need not contain a file extension. Preserve an
// explicit content type, then a recognized display extension, then classify
// the actual Workspace path. Neither renaming nor classification changes bytes.
func publicationMediaType(filePath, name, explicit string) string {
	if explicit != "" {
		return explicit
	}
	for _, candidate := range []string{name, filePath} {
		if media := importedMediaType(candidate, nil); media != "application/octet-stream" {
			return media
		}
		if media := mime.TypeByExtension(path.Ext(candidate)); media != "" && media != "application/octet-stream" {
			return media
		}
	}
	return "application/octet-stream"
}
