package promqlengine

import "strings"

// Native Array parameters use ClickHouse's Escaped value format. Values travel
// outside SQL text and never become identifiers, predicates, or parser-sized IN
// lists. Authorization and deterministic source precedence remain in the
// separate binary external tables.
func arrayParameter(values []string) string {
	var out strings.Builder
	out.WriteByte('[')
	for i, value := range values {
		if i > 0 {
			out.WriteByte(',')
		}
		out.WriteByte('\'')
		for _, ch := range value {
			switch ch {
			case '\\', '\'':
				out.WriteByte('\\')
				out.WriteRune(ch)
			case '\n':
				out.WriteString(`\n`)
			case '\r':
				out.WriteString(`\r`)
			case '\t':
				out.WriteString(`\t`)
			case 0:
				out.WriteString(`\0`)
			default:
				out.WriteRune(ch)
			}
		}
		out.WriteByte('\'')
	}
	out.WriteByte(']')
	return out.String()
}
