package kql

import (
	"strconv"
	"strings"
)

func parseLogfmt(input string) map[string]string {
	result := map[string]string{}
	for input != "" {
		input = strings.TrimLeft(input, " \t\r\n")
		end := strings.IndexByte(input, '=')
		if end < 1 {
			break
		}
		key := input[:end]
		if strings.ContainsAny(key, " \t\r\n\"") {
			break
		}
		input = input[end+1:]
		if strings.HasPrefix(input, "\"") {
			i := 1
			for i < len(input) {
				if input[i] == '\\' {
					i += 2
					continue
				}
				if input[i] == '"' {
					break
				}
				i++
			}
			if i >= len(input) {
				break
			}
			value, err := strconv.Unquote(input[:i+1])
			if err != nil {
				break
			}
			result[key] = value
			input = input[i+1:]
		} else {
			i := strings.IndexAny(input, " \t\r\n")
			if i < 0 {
				i = len(input)
			}
			result[key] = input[:i]
			input = input[i:]
		}
	}
	return result
}
