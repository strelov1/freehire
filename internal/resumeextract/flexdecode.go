package resumeextract

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

// verbatimString decodes from a JSON string OR a bare number, kept VERBATIM as the
// string value. The model is asked to keep years and dates as written (strings), but it
// routinely emits them as numbers (e.g. "year": 2019). encoding/json aborts the whole
// unmarshal on the first type mismatch, so a single numeric date would otherwise
// silently discard the entire structured résumé. Used only for the free-form date/year
// fields via the UnmarshalJSON shims below; the exported struct fields stay plain
// string so the contract, Sanitize, and every consumer are untouched. Deliberately not
// a flexjson type: flexjson coerces numerically, while a date is text — the scalar
// token must survive as written.
//
// The request schema now declares these fields as strings, so a gateway honouring it
// makes this shim unreachable. It stays anyway, for the same reason Validate stays: a
// provider that quietly stops honouring a schema answers 200 with ordinary JSON, and
// nothing on the path would notice. Twenty-five tested lines are a cheap standing
// guard against silently losing a whole parsed CV to one numeric year.
type verbatimString string

func (f *verbatimString) UnmarshalJSON(b []byte) error {
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
		*f = verbatimString(s)
		return nil
	}
	// Bare number (or other scalar token) — keep it verbatim as the string value.
	*f = verbatimString(b)
	return nil
}

// truncInt decodes from a JSON number OR a string ("5", "5+ years"), taking the
// LEADING integer and TRUNCATING any fraction — never rounding, unlike flexjson.Int
// (round-half): a stray "5.9" must not inflate the candidate's years of experience.
// total_years is prompted as an integer, but the model can return it as a string or a
// phrase; a string there would abort the whole decode. Non-numeric or empty input
// yields 0.
type truncInt int

func (f *truncInt) UnmarshalJSON(b []byte) error {
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
		*f = truncInt(leadingInt(s))
		return nil
	}
	// JSON number: parse as float first so "5.0" (or a stray decimal) truncates cleanly.
	n, err := strconv.ParseFloat(string(b), 64)
	if err != nil {
		return err
	}
	*f = truncInt(int(n))
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

// UnmarshalJSON tolerates a string/phrase "total_years" (e.g. "5+ years") via truncInt,
// then delegates the rest to the normal struct decode via an alias (no recursion).
func (s *Structured) UnmarshalJSON(b []byte) error {
	type alias Structured
	aux := struct {
		TotalYears truncInt `json:"total_years"`
		*alias
	}{alias: (*alias)(s)}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	s.TotalYears = int(aux.TotalYears)
	return nil
}

// UnmarshalJSON tolerates a numeric "year" (e.g. 2019) by decoding it through verbatimString,
// then delegates the rest to the normal struct decode via an alias (no recursion).
func (e *Education) UnmarshalJSON(b []byte) error {
	type alias Education
	aux := struct {
		Year verbatimString `json:"year"`
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
		Start verbatimString `json:"start"`
		End   verbatimString `json:"end"`
		*alias
	}{alias: (*alias)(x)}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	x.Start = string(aux.Start)
	x.End = string(aux.End)
	return nil
}
