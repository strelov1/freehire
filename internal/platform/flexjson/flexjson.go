// Package flexjson makes encoding/json survive a document it would otherwise abandon.
//
// One premise runs through everything here: encoding/json aborts the WHOLE unmarshal on the
// first thing it dislikes, so a single wrong-typed field or a single stray byte silently
// discards an entire decoded record. The package answers that at two levels.
//
// The scalar types (Int, Int64, Float, Bool) handle a value of the wrong JSON type, which is
// what LLM output produces: models non-deterministically write a number as a string ("85"), a
// string as a number, a bool as "true"/1. They coerce number<->string<->bool at the decode
// boundary; use them in a shadow struct's UnmarshalJSON and copy into the plain exported
// fields, so the contract and every consumer stay untouched. Non-numeric or empty input
// yields the zero value rather than an error (a best-effort field is better dropped than
// crashing the record).
//
// SanitizeControlChars handles a byte no decoder should have been handed, which is what
// scraped HTML produces: a page that template-renders embedded JSON by interpolating raw
// text into a string literal without escaping its newlines. It runs one layer earlier, over
// the bytes, before any decode. Its callers are HTML adapters, not model output — the shared
// subject is the abandonment, not where the document came from.
//
// Siblings with deliberately DIFFERENT semantics keep their own package-local types, named
// and commented to mark the difference: internal/ai/enrich (stringOrFirst / sliceOrWrap —
// scalar<->array slips, which flexjson does not cover; roundInt — strict number-only, so a
// string fails the decode instead of silently coercing), internal/candidate/resumeextract
// (verbatimString — a bare scalar kept as written, not coerced; truncInt — leading integer,
// truncating where flexjson.Int rounds), and internal/candidate/linkedinprofile (textOf /
// itemsOf — scalar<->array again, but over JSON-LD, where the tolerance must be per MEMBER:
// it holds a node's members raw and lifts each independently, so one unexpected shape costs
// that member and not the node).
package flexjson

import (
	"bytes"
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

// Int decodes from a JSON number (rounded to nearest) OR a string, taking the leading
// integer ("85" → 85, "85%" → 85, "8/10" → 8). Empty/non-numeric input yields 0.
type Int int

func (f *Int) UnmarshalJSON(b []byte) error {
	*f = Int(int(math.Round(decodeNumber(b))))
	return nil
}

// Int64 is Int for 64-bit ids/counts (e.g. a matched job id the model may quote as "42").
type Int64 int64

func (f *Int64) UnmarshalJSON(b []byte) error {
	*f = Int64(int64(math.Round(decodeNumber(b))))
	return nil
}

// Float decodes from a JSON number OR a string, taking the leading float ("0.8" → 0.8).
// Empty/non-numeric input yields 0.
type Float float64

func (f *Float) UnmarshalJSON(b []byte) error {
	*f = Float(decodeNumber(b))
	return nil
}

// Bool decodes from a JSON bool, a string ("true"/"yes"/"y"/"t"/"1" → true), or a number
// (non-zero → true). Empty/unrecognized input yields false.
type Bool bool

func (f *Bool) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*f = false
		return nil
	}
	var bo bool
	if err := json.Unmarshal(b, &bo); err == nil {
		*f = Bool(bo)
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "true", "yes", "y", "t", "1":
			*f = true
		default:
			*f = false
		}
		return nil
	}
	var n float64
	if err := json.Unmarshal(b, &n); err != nil {
		// A syntactically valid but out-of-range numeric literal (e.g. 1e400) fails here
		// too; unrecognized input is false, same as the string branch above.
		*f = false
		return nil
	}
	*f = Bool(n != 0)
	return nil
}

// decodeNumber extracts a float64 from a JSON number or a string carrying a leading
// numeric token. Empty, null, non-numeric, or out-of-range input (e.g. a bare 1e400,
// which is syntactically valid JSON but overflows float64) yields 0, never an error, so a
// single unparseable field never aborts the surrounding record.
func decodeNumber(b []byte) float64 {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return 0
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return 0
		}
		return leadingFloat(s)
	}
	var n float64
	if err := json.Unmarshal(b, &n); err != nil {
		return 0
	}
	return n
}

// leadingFloat parses the leading numeric token of s (optional sign, digits, one decimal
// point), e.g. "0.9 ok" → 0.9, "85%" → 85, "n/a" → 0.
func leadingFloat(s string) float64 {
	s = strings.TrimSpace(s)
	end, seenDot := 0, false
	for end < len(s) {
		c := s[end]
		switch {
		case c >= '0' && c <= '9':
		case c == '-' && end == 0:
		case c == '.' && !seenDot:
			seenDot = true
		default:
			goto done
		}
		end++
	}
done:
	if end == 0 {
		return 0
	}
	n, err := strconv.ParseFloat(s[:end], 64)
	if err != nil {
		return 0
	}
	return n
}
