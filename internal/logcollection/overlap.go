package logcollection

import (
	"context"
	"errors"
	"regexp"
	"regexp/syntax"
	"sort"
	"strconv"
	"strings"
)

var ErrOverlapBudget = errors.New("log rule overlap analysis budget exceeded")

func FileOverlapForPlatform(ctx context.Context, a, b Config, windows bool) (bool, error) {
	if windows {
		fold := func(values []string) []string {
			out := make([]string, len(values))
			for i, value := range values {
				out[i] = strings.ToLower(value)
			}
			return out
		}
		a.Include = fold(a.Include)
		a.Exclude = fold(a.Exclude)
		b.Include = fold(b.Include)
		b.Exclude = fold(b.Exclude)
	}
	return FileOverlap(ctx, a, b)
}

// FileOverlap checks language intersection, including both rules' exclusions.
// The alphabet is partitioned by literal runes and path separators; every
// other rune has the same transitions in our restricted glob grammar.
func FileOverlap(ctx context.Context, a, b Config) (bool, error) {
	for _, left := range a.Include {
		for _, right := range b.Include {
			patterns := append([]string{left, right}, a.Exclude...)
			patterns = append(patterns, b.Exclude...)
			match, err := patternIntersection(ctx, patterns)
			if err != nil || match {
				return match, err
			}
		}
	}
	return false, nil
}
func globExpression(pattern string) string {
	var out strings.Builder
	chars := []rune(pattern)
	for i := 0; i < len(chars); i++ {
		switch chars[i] {
		case '*':
			if i+1 < len(chars) && chars[i+1] == '*' {
				i++
				if i+1 < len(chars) && chars[i+1] == '/' {
					i++
					out.WriteString(`(?:[^/]+/)*`)
				} else {
					out.WriteString(`(?s:.*)`)
				}
			} else {
				out.WriteString(`[^/]*`)
			}
		case '?':
			out.WriteString(`[^/]`)
		default:
			out.WriteString(regexp.QuoteMeta(string(chars[i])))
		}
	}
	return out.String()
}
func closure(program *syntax.Prog, initial []uint32) []uint32 {
	seen := map[uint32]bool{}
	pending := append([]uint32{}, initial...)
	out := []uint32{}
	for len(pending) > 0 {
		id := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if seen[id] {
			continue
		}
		seen[id] = true
		instruction := program.Inst[id]
		switch instruction.Op {
		case syntax.InstAlt, syntax.InstAltMatch:
			pending = append(pending, instruction.Out, instruction.Arg)
		case syntax.InstNop, syntax.InstCapture:
			pending = append(pending, instruction.Out)
		case syntax.InstFail:
		default:
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
func advance(program *syntax.Prog, states []uint32, char rune) []uint32 {
	next := []uint32{}
	for _, id := range states {
		instruction := program.Inst[id]
		matched := false
		switch instruction.Op {
		case syntax.InstRune, syntax.InstRune1:
			matched = instruction.MatchRune(char)
		case syntax.InstRuneAny:
			matched = true
		case syntax.InstRuneAnyNotNL:
			matched = char != '\n'
		}
		if matched {
			next = append(next, instruction.Out)
		}
	}
	return closure(program, next)
}
func accepting(program *syntax.Prog, states []uint32) bool {
	for _, id := range states {
		if program.Inst[id].Op == syntax.InstMatch {
			return true
		}
	}
	return false
}
func productKey(states [][]uint32) string {
	var key strings.Builder
	for _, set := range states {
		for _, id := range set {
			key.WriteString(strconv.FormatUint(uint64(id), 10))
			key.WriteByte(',')
		}
		key.WriteByte(';')
	}
	return key.String()
}
func patternIntersection(ctx context.Context, patterns []string) (bool, error) {
	programs := make([]*syntax.Prog, len(patterns))
	alphabet := map[rune]bool{'/': true}
	start := make([][]uint32, len(patterns))
	for index, pattern := range patterns {
		parsed, err := syntax.Parse(globExpression(pattern), syntax.Perl)
		if err != nil {
			return false, ErrInvalid
		}
		program, err := syntax.Compile(parsed.Simplify())
		if err != nil {
			return false, ErrInvalid
		}
		programs[index] = program
		start[index] = closure(program, []uint32{uint32(program.Start)})
		for _, char := range pattern {
			if char != '*' && char != '?' {
				alphabet[char] = true
			}
		}
	}
	for char := 'a'; ; char++ {
		if !alphabet[char] && char != '/' {
			alphabet[char] = true
			break
		}
	}
	chars := make([]rune, 0, len(alphabet))
	for char := range alphabet {
		chars = append(chars, char)
	}
	sort.Slice(chars, func(i, j int) bool { return chars[i] < chars[j] })
	queue := [][][]uint32{start}
	seen := map[string]bool{productKey(start): true}
	steps := 0
	for cursor := 0; cursor < len(queue); cursor++ {
		if cursor%32 == 0 {
			if err := ctx.Err(); err != nil {
				return false, err
			}
		}
		states := queue[cursor]
		if accepting(programs[0], states[0]) && accepting(programs[1], states[1]) {
			excluded := false
			for i := 2; i < len(programs); i++ {
				excluded = excluded || accepting(programs[i], states[i])
			}
			if !excluded {
				return true, nil
			}
		}
		for _, char := range chars {
			steps++
			if steps > 200000 || len(seen) > 20000 {
				return false, ErrOverlapBudget
			}
			next := make([][]uint32, len(programs))
			next[0] = advance(programs[0], states[0], char)
			if len(next[0]) == 0 {
				continue
			}
			next[1] = advance(programs[1], states[1], char)
			if len(next[1]) == 0 {
				continue
			}
			for i := 2; i < len(programs); i++ {
				next[i] = advance(programs[i], states[i], char)
			}
			key := productKey(next)
			if !seen[key] {
				seen[key] = true
				queue = append(queue, next)
			}
		}
	}
	return false, nil
}

func WorkloadOverlap(a, b Workload) bool {
	if a.NamespaceID != b.NamespaceID || a.NamespaceUID != b.NamespaceUID {
		return false
	}
	related := a.UID == b.UID
	for _, uid := range a.Ancestors {
		related = related || uid == b.UID
	}
	for _, uid := range b.Ancestors {
		related = related || uid == a.UID
	}
	if !related {
		return false
	}
	for _, a := range a.Containers {
		for _, b := range b.Containers {
			if a == b {
				return true
			}
		}
	}
	return false
}
