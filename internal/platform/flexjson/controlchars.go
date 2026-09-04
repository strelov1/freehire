package flexjson

// jsonControlCharEscapes maps a raw control byte to the two-byte escape json.Unmarshal
// accepts in its place; a byte absent here (rare outside \n/\r/\t) falls back to \u00XX.
var jsonControlCharEscapes = map[byte]string{'\n': `\n`, '\r': `\r`, '\t': `\t`}

const hexDigits = "0123456789abcdef"

// SanitizeControlChars escapes raw control bytes (0x00-0x1F) found INSIDE a JSON string
// literal. Some sites template-render their embedded JSON by interpolating raw HTML into
// a string without escaping its newlines, which is invalid JSON — Go's decoder rejects it
// ("invalid character '\n' in string literal") where a lenient decoder (e.g. Python's
// json.loads(strict=False)) would accept it; live-verified on cryptocurrencyjobs.co.
// Control bytes outside a string (insignificant whitespace between tokens) are left
// untouched.
//
// This is the same tolerance the scalar types above provide, applied one layer earlier:
// there the decode survives a field of the wrong type, here it survives a byte no decoder
// should have been handed. Both exist because encoding/json abandons the whole document on
// the first thing it dislikes, so a single stray byte otherwise discards a whole record.
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
