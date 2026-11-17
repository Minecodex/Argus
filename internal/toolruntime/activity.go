package toolruntime

// Activity is an immutable, provider-authored UI projection. It never enters the
// model result or grants file access. External tool output cannot author it.
type Activity struct {
	Schema      string      `json:"schema"`
	Kind        string      `json:"kind"`
	Path        string      `json:"path,omitempty"`
	WorkspaceID string      `json:"workspace_id,omitempty"`
	Change      *FileChange `json:"change,omitempty"`
}

type FileChange struct {
	Operation  string     `json:"operation"`
	BeforeHash string     `json:"before_hash,omitempty"`
	AfterHash  string     `json:"after_hash,omitempty"`
	Added      *int       `json:"added,omitempty"`
	Removed    *int       `json:"removed,omitempty"`
	Complete   bool       `json:"complete"`
	Reason     string     `json:"reason,omitempty"`
	Lines      []DiffLine `json:"lines,omitempty"`
}

type DiffLine struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}
