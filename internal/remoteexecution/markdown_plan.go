package remoteexecution

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

type CommandBinding struct {
	BlockID        string `json:"block_id"`
	Description    string `json:"description"`
	Target         Target `json:"target"`
	Interpreter    string `json:"interpreter"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
}
type CommandManifest struct {
	Version  string   `json:"version"`
	Blocks   []string `json:"blocks"`
	Spec     Spec     `json:"spec"`
	Markdown string   `json:"markdown"`
}
type commandBlock struct {
	id, language, script string
	start                int
}

var blockIdentifier = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

func EscapePlanText(value string) string {
	return strings.NewReplacer("\\", "\\\\", "`", "\\`", "*", "\\*", "_", "\\_", "[", "\\[", "]", "\\]", "<", "&lt;", ">", "&gt;", "#", "\\#", "\r", " ", "\n", " ").Replace(value)
}

// Only explicit, top-level AST code fences grant commands. Quoted examples,
// indented code and hidden HTML cannot introduce an executable step.
func commandBlocks(markdown string) ([]commandBlock, error) {
	source := []byte(markdown)
	document := goldmark.DefaultParser().Parse(text.NewReader(source))
	blocks := []commandBlock{}
	seen := map[string]bool{}
	err := ast.Walk(document, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		code, ok := n.(*ast.FencedCodeBlock)
		if !ok || code.Info == nil {
			return ast.WalkContinue, nil
		}
		info := strings.Fields(string(code.Info.Text(source)))
		if len(info) < 2 || !strings.HasPrefix(info[1], "argus:") {
			return ast.WalkContinue, nil
		}
		id := strings.TrimPrefix(info[1], "argus:")
		if len(info) != 2 || !blockIdentifier.MatchString(id) || seen[id] || n.Parent() != document {
			return ast.WalkStop, Error("REMOTE_PLAN_INVALID")
		}
		if info[0] != "bash" && info[0] != "powershell" {
			return ast.WalkStop, Error("REMOTE_PLAN_INVALID")
		}
		var script strings.Builder
		for i := 0; i < code.Lines().Len(); i++ {
			line := code.Lines().At(i)
			script.Write(line.Value(source))
		}
		if strings.TrimSpace(script.String()) == "" || script.Len() > MaxScriptBytes {
			return ast.WalkStop, Error("REMOTE_PLAN_INVALID")
		}
		start := bytes.LastIndexByte(source[:code.Info.Segment.Start], '\n') + 1
		// CommonMark accepts an unclosed fence to EOF. Consent requires closure.
		open := strings.TrimSpace(string(source[start:code.Info.Segment.Start]))
		closeFrom := code.Info.Segment.Stop
		if code.Lines().Len() > 0 {
			closeFrom = code.Lines().At(code.Lines().Len() - 1).Stop
		}
		rest := strings.TrimSpace(string(source[closeFrom:]))
		closing := strings.SplitN(rest, "\n", 2)[0]
		if len(open) < 3 || len(closing) < len(open) || strings.Trim(closing, string(open[0])) != "" {
			return ast.WalkStop, Error("REMOTE_PLAN_INVALID")
		}
		seen[id] = true
		blocks = append(blocks, commandBlock{id, info[0], script.String(), start})
		return ast.WalkContinue, nil
	})
	if err != nil || len(blocks) > MaxSteps {
		return nil, Error("REMOTE_PLAN_INVALID")
	}
	return blocks, nil
}

func (s Service) FreezePlanCommands(ctx context.Context, call toolruntime.Invocation, id uuid.UUID, title, markdown string, bindings []CommandBinding) (CommandManifest, error) {
	markdown = strings.ReplaceAll(markdown, "\r\n", "\n")
	if !strings.HasPrefix(strings.TrimSpace(markdown), "# ") {
		markdown = "# " + EscapePlanText(title) + "\n\n" + markdown
	}
	blocks, err := commandBlocks(markdown)
	if err != nil {
		return CommandManifest{}, err
	}
	if len(blocks) != len(bindings) {
		return CommandManifest{}, Error("REMOTE_PLAN_INVALID")
	}
	manifest := CommandManifest{Version: "argus.markdown_commands/v1", Blocks: []string{}, Markdown: markdown, Spec: Spec{SchemaVersion: SchemaVersion, ID: id, Kind: "plan", Title: title, Description: title, ConversationID: call.Principal.ConversationID, RunID: call.RunID, Steps: []Step{}, Inputs: []File{}}}
	if len(blocks) == 0 {
		return manifest, nil
	}
	p, err := s.principal(ctx, call.Principal.EnterpriseID, call.Principal.UserID, call.Principal.ConversationID)
	if err != nil {
		return manifest, err
	}
	byID := map[string]CommandBinding{}
	for _, binding := range bindings {
		if _, ok := byID[binding.BlockID]; ok {
			return manifest, Error("REMOTE_PLAN_INVALID")
		}
		byID[binding.BlockID] = binding
	}
	for _, block := range blocks {
		b, ok := byID[block.id]
		if !ok {
			return manifest, Error("REMOTE_PLAN_INVALID")
		}
		language := "bash"
		if b.Interpreter == "powershell" {
			language = "powershell"
		}
		if block.language != language {
			return manifest, Error("REMOTE_PLAN_INVALID")
		}
		manifest.Blocks = append(manifest.Blocks, block.id)
		manifest.Spec.Steps = append(manifest.Spec.Steps, Step{Description: b.Description, Target: b.Target, Interpreter: b.Interpreter, Script: block.script, TimeoutSeconds: b.TimeoutSeconds})
	}
	input := Input{Title: title, Description: title, Steps: manifest.Spec.Steps}
	if err = input.Validate("plan"); err != nil {
		return manifest, err
	}
	manifest.Spec.Steps = input.Steps
	manifest.Spec, err = s.freezeSpec(ctx, call, p, manifest.Spec)
	if err != nil {
		return manifest, err
	}
	var rendered strings.Builder
	position := 0
	for i, block := range blocks {
		step := manifest.Spec.Steps[i]
		rendered.WriteString(markdown[position:block.start])
		name := step.Target.Name
		if name == "" {
			name = step.Target.ResourceID.String()
		}
		for _, scope := range []string{step.Target.Namespace, step.Target.Pod, step.Target.Container} {
			if scope != "" {
				name += " / " + scope
			}
		}
		// Escaped platform-derived captions are part of the frozen document.
		fmt.Fprintf(&rendered, "\n**待执行 / Execute · %s · %s**\n\n", EscapePlanText(name), EscapePlanText(step.Identity.Username))
		position = block.start
	}
	rendered.WriteString(markdown[position:])
	manifest.Markdown = rendered.String()
	return manifest, nil
}
