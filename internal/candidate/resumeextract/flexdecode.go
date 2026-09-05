package resumeextract

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

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
