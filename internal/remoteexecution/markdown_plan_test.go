package remoteexecution

import "testing"

func TestMarkdownCommandsUseExplicitVisibleASTFences(t *testing.T) {
	valid := "# Repair\n\n```bash\nprintf example\n```\n\n```bash argus:backup\nif test -f status.conf; then\n  cp status.conf backup.conf\nfi\n```\n\n~~~powershell argus:verify\nGet-Content $env:TEMP/status.conf\n~~~\n"
	blocks, err := commandBlocks(valid)
	if err != nil || len(blocks) != 2 || blocks[0].id != "backup" || blocks[0].script != "if test -f status.conf; then\n  cp status.conf backup.conf\nfi\n" || blocks[1].language != "powershell" {
		t.Fatalf("wrong extraction: %#v %v", blocks, err)
	}
	for name, source := range map[string]string{
		"hidden":        "<!--\n```bash argus:hidden\necho hidden\n```\n-->\n",
		"outer example": "````markdown\n```bash argus:hidden\necho hidden\n```\n````\n",
	} {
		t.Run(name, func(t *testing.T) {
			blocks, err := commandBlocks(source)
			if err != nil || len(blocks) != 0 {
				t.Fatalf("example became executable: %#v %v", blocks, err)
			}
		})
	}
	for name, source := range map[string]string{
		"unclosed":            "```bash argus:unsafe\necho missing-close\n",
		"duplicate":           "```bash argus:a\necho 1\n```\n```bash argus:a\necho 2\n```\n",
		"quoted":              "> ```bash argus:quoted\n> echo quoted\n> ```\n",
		"unknown interpreter": "```python argus:a\nprint(1)\n```\n",
		"extra attributes":    "```bash argus:a hidden\necho 1\n```\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := commandBlocks(source); err == nil {
				t.Fatal("accepted ambiguous executable block")
			}
		})
	}
}
