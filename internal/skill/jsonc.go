package skill

import (
	"bytes"
	"encoding/json"
	"io"
)

// decodeJSONC parses JSON that may contain `//` line comments into v.
func decodeJSONC(data []byte, v any) error {
	return json.NewDecoder(newStripComments(data)).Decode(v)
}

// newStripComments returns a reader over data with `//` line comments removed,
// so skill.json may be lightly documented. It is string-aware: a `//` inside a
// JSON string literal is preserved. Only line comments are supported.
func newStripComments(data []byte) io.Reader {
	var out bytes.Buffer
	inString := false
	escaped := false
	for i := 0; i < len(data); i++ {
		c := data[i]
		if inString {
			out.WriteByte(c)
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		if c == '"' {
			inString = true
			out.WriteByte(c)
			continue
		}
		if c == '/' && i+1 < len(data) && data[i+1] == '/' {
			// skip to end of line, but keep the newline for line numbers
			for i < len(data) && data[i] != '\n' {
				i++
			}
			if i < len(data) {
				out.WriteByte('\n')
			}
			continue
		}
		out.WriteByte(c)
	}
	return &out
}
