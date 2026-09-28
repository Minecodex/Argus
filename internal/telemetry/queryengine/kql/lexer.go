package kql

import (
	"fmt"
	"strings"
	"unicode"
)

func lex(input string) ([]string, error) {
	if len(input) > 65536 {
		return nil, fmt.Errorf("KQL expression too long")
	}
	var tokens []string
	depth := 0
	for i := 0; i < len(input); {
		if unicode.IsSpace(rune(input[i])) {
			i++
			continue
		}
		start := i
		switch input[i] {
		case '"':
			i++
			closed := false
			for i < len(input) {
				if input[i] == '\\' {
					i += 2
					continue
				}
				if input[i] == '"' {
					i++
					closed = true
					break
				}
				i++
			}
			if !closed {
				return nil, fmt.Errorf("unterminated quote")
			}
		case '(', ')':
			if input[i] == '(' {
				depth++
			} else {
				depth--
			}
			if depth < 0 || depth > 64 {
				return nil, fmt.Errorf("invalid expression depth")
			}
			i++
		case ':':
			i++
		case '=', '!', '<', '>':
			i++
			if i < len(input) && input[i] == '=' {
				i++
			}
		default:
			for i < len(input) && !unicode.IsSpace(rune(input[i])) && !strings.ContainsRune("()=!:<>\"", rune(input[i])) {
				i++
			}
		}
		tokens = append(tokens, input[start:i])
		if len(tokens) > 4096 {
			return nil, fmt.Errorf("too many expression tokens")
		}
	}
	if depth != 0 {
		return nil, fmt.Errorf("unbalanced expression")
	}
	return tokens, nil
}
