// Package flexjson provides tolerant JSON scalar types for decoding LLM output. Models
// non-deterministically pick the wrong JSON type for a scalar — a number as a string
// ("85"), a string as a number, a bool as "true"/1 — and encoding/json aborts the WHOLE
// unmarshal on the first mismatch, so one slip silently discards the entire decoded
// record. These types coerce number<->string<->bool at the decode boundary; use them in a
// shadow struct's UnmarshalJSON and copy into the plain exported fields, so the contract
// and every consumer stay untouched. Non-numeric or empty input yields the zero value
// rather than an error (a best-effort field is better dropped than crashing the record).
//
// Siblings internal/enrich and internal/resumeextract keep their own package-local flex*
// types (working, tested, on hot/critical paths); consolidating them here is a future
// cleanup, not required for correctness.
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
	n, err := decodeNumber(b)
	if err != nil {
		return err
	}
	*f = Int(int(math.Round(n)))
	return nil
}

// Int64 is Int for 64-bit ids/counts (e.g. a matched job id the model may quote as "42").
type Int64 int64

func (f *Int64) UnmarshalJSON(b []byte) error {
	n, err := decodeNumber(b)
	if err != nil {
		return err
	}
	*f = Int64(int64(math.Round(n)))
	return nil
}

// Float decodes from a JSON number OR a string, taking the leading float ("0.8" → 0.8).
// Empty/non-numeric input yields 0.
type Float float64

func (f *Float) UnmarshalJSON(b []byte) error {
	n, err := decodeNumber(b)
	if err != nil {
		return err
	}
	*f = Float(n)
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
		return err
	}
	*f = Bool(n != 0)
	return nil
}

// decodeNumber extracts a float64 from a JSON number or a string carrying a leading
// numeric token. Empty, null, or non-numeric input yields 0 (not an error), so a single
// unparseable field never aborts the surrounding record.
func decodeNumber(b []byte) (float64, error) {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return 0, nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return 0, err
		}
		return leadingFloat(s), nil
	}
	var n float64
	if err := json.Unmarshal(b, &n); err != nil {
		return 0, err
	}
	return n, nil
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
