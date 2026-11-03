package argusdev

import (
	"path/filepath"
	"regexp"
	"strings"
)

var plan11LiteralCD = regexp.MustCompile(`(?:^|[;&\n])[ \t]*cd[ \t]+(?:'([^'\r\n]+)'|"([^"\r\n]+)"|([^ \t\r\n;&|]+))[ \t]*(?:&&|;|\r?\n)`)

// Match literal references in simple cwd segments only; do not execute or
// evaluate arbitrary shell substitutions. Result identity and arithmetic are
// independently verified, and callers still require a complete successful
// process. A later cd ends the previous directory's relative-reference scope.
func plan11WorkspaceFileReference(command, file string) bool {
	if !strings.HasPrefix(file, "/workspace/") {
		return false
	}
	mentions := func(segment, name string) bool {
		return regexp.MustCompile(`(?:^|[\s'"=(,:])` + regexp.QuoteMeta(name) + `(?:$|[\s'"),;|&])`).MatchString(segment)
	}
	if mentions(command, file) {
		return true
	}
	cwd, position := "/workspace", 0
	segmentMentions := func(segment string) bool {
		if cwd == "" {
			return false
		}
		prefix := strings.TrimSuffix(cwd, "/") + "/"
		return strings.HasPrefix(file, prefix) && mentions(segment, strings.TrimPrefix(file, prefix))
	}
	for cursor := 0; cursor < len(command); {
		match := plan11LiteralCD.FindStringSubmatchIndex(command[cursor:])
		if match == nil {
			break
		}
		for i := range match {
			if match[i] >= 0 {
				match[i] += cursor
			}
		}
		// Keep the final separator available to recognize an adjacent cd.
		// It is not part of any command argument in the previous segment.
		cursor = match[1] - 1
		if segmentMentions(command[position:max(position, match[0])]) {
			return true
		}
		directory := ""
		for group := 1; group <= 3; group++ {
			if match[2*group] >= 0 {
				directory = command[match[2*group]:match[2*group+1]]
				break
			}
		}
		if strings.ContainsAny(directory, "$`\\") {
			cwd = ""
		} else if strings.HasPrefix(directory, "/") {
			cwd = filepath.ToSlash(filepath.Clean(directory))
		} else if cwd != "" {
			cwd = filepath.ToSlash(filepath.Clean(filepath.Join(cwd, directory)))
		}
		if cwd != "/workspace" && !strings.HasPrefix(cwd, "/workspace/") {
			cwd = ""
		}
		position = match[1]
	}
	return segmentMentions(command[position:])
}
