package flexjson

// jsonControlCharEscapes maps a raw control byte to the two-byte escape json.Unmarshal
// accepts in its place; a byte absent here (rare outside \n/\r/\t) falls back to \u00XX.
var jsonControlCharEscapes = map[byte]string{'\n': `\n`, '\r': `\r`, '\t': `\t`}

const hexDigits = "0123456789abcdef"

// SanitizeControlChars escapes raw control bytes (0x00-0x1F) found inside a JSON string literal.
// Some sites interpolate raw HTML into embedded JSON without escaping newlines, which Go's decoder
// rejects. Control bytes outside a string (whitespace between tokens) are left untouched.
//
// It is the byte-level counterpart of the scalar types: encoding/json abandons the whole document
// on the first thing it dislikes.
func SanitizeControlChars(raw []byte) []byte {
	out := make([]byte, 0, len(raw))
	inString, escaped := false, false
	for _, b := range raw {
		switch {
		case !inString:
			if b == '"' {
				inString = true
			}
			out = append(out, b)
		case escaped:
			escaped = false
			out = append(out, b)
		case b == '\\':
			escaped = true
			out = append(out, b)
		case b == '"':
			inString = false
			out = append(out, b)
		case b < 0x20:
			if esc, ok := jsonControlCharEscapes[b]; ok {
				out = append(out, esc...)
			} else {
				out = append(out, '\\', 'u', '0', '0', hexDigits[b>>4], hexDigits[b&0xf])
			}
		default:
			out = append(out, b)
		}
	}
	return out
}
