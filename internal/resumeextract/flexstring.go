package resumeextract

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

// flexString decodes from a JSON string OR a bare number. The model is asked to keep
// years and dates as written (strings), but it routinely emits them as numbers
// (e.g. "year": 2019). encoding/json aborts the whole unmarshal on the first type
// mismatch, so a single numeric date would otherwise silently discard the entire
// structured résumé. Used only for the free-form date/year fields via the UnmarshalJSON
// shims below; the exported struct fields stay plain string so the contract, Sanitize,
// and every consumer are untouched.
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*f = ""
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = flexString(s)
		return nil
	}
	// Bare number (or other scalar token) — keep it verbatim as the string value.
	*f = flexString(b)
	return nil
}

// flexInt decodes from a JSON number OR a string ("5", "5+ years"). total_years is
// prompted as an integer, but the model can return it as a string or a phrase; a string
// there would abort the whole decode. Non-numeric or empty input yields 0.
type flexInt int

func (f *flexInt) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*f = 0
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = flexInt(leadingInt(s))
		return nil
	}
	// JSON number: parse as float first so "5.0" (or a stray decimal) truncates cleanly.
	n, err := strconv.ParseFloat(string(b), 64)
	if err != nil {
		return err
	}
	*f = flexInt(int(n))
	return nil
}

// leadingInt returns the integer formed by the leading digits of s (e.g. "5+ years" → 5),
// or 0 if s has no leading digits.
func leadingInt(s string) int {
	s = strings.TrimSpace(s)
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0
	}
	n, _ := strconv.Atoi(s[:end])
	return n
}

// UnmarshalJSON tolerates a string/phrase "total_years" (e.g. "5+ years") via flexInt,
// then delegates the rest to the normal struct decode via an alias (no recursion).
func (s *Structured) UnmarshalJSON(b []byte) error {
	type alias Structured
	aux := struct {
		TotalYears flexInt `json:"total_years"`
		*alias
	}{alias: (*alias)(s)}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	s.TotalYears = int(aux.TotalYears)
	return nil
}

// UnmarshalJSON tolerates a numeric "year" (e.g. 2019) by decoding it through flexString,
// then delegates the rest to the normal struct decode via an alias (no recursion).
func (e *Education) UnmarshalJSON(b []byte) error {
	type alias Education
	aux := struct {
		Year flexString `json:"year"`
		*alias
	}{alias: (*alias)(e)}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	e.Year = string(aux.Year)
	return nil
}

// UnmarshalJSON tolerates numeric "start"/"end" dates (e.g. 2019) the same way.
func (x *Experience) UnmarshalJSON(b []byte) error {
	type alias Experience
	aux := struct {
		Start flexString `json:"start"`
		End   flexString `json:"end"`
		*alias
	}{alias: (*alias)(x)}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	x.Start = string(aux.Start)
	x.End = string(aux.End)
	return nil
}
