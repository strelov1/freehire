package searchintent

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

// The schema makes these decoders unreachable on a gateway that honours strict mode.
// They exist because some do not: the model has answered "seniority": "senior" for an
// array field, and encoding/json aborts the whole unmarshal on the first type mismatch.
// resumeextract keeps the same guard.

// flexStrings is a []string that also accepts a bare scalar as a single-element list.
// Null and "" read as no values, not as [""].
type flexStrings []string

func (f *flexStrings) UnmarshalJSON(b []byte) error {
	*f = nil
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '[' {
		// Element by element, so one odd element ("skills":["go",5]) does not cost the list.
		var raw []json.RawMessage
		if err := json.Unmarshal(b, &raw); err != nil {
			return err
		}
		for _, element := range raw {
			var one flexStrings
			if err := one.UnmarshalJSON(element); err != nil {
				continue
			}
			*f = append(*f, one...)
		}
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		if s != "" {
			*f = flexStrings{s}
		}
		return nil
	}
	// A bare number or other scalar token is kept verbatim; the resolvers judge it.
	*f = flexStrings{string(b)}
	return nil
}

// flexInt is a bound read as a bare number, quoted number, or empty/absent text meaning
// "not set". Unparseable text is also "not set", not an error.
//
// The model writes 0 for a bound it does not mean to set, but for the experience ceiling
// 0 is a real filter (no prior experience required), so the field is declared as text
// (see requestSchema) to keep "" and "0" distinct.
type flexInt struct {
	value int
	set   bool
}

func (f *flexInt) UnmarshalJSON(b []byte) error {
	*f = flexInt{}
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return nil
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			return nil
		}
		*f = flexInt{value: n, set: true}
		return nil
	}
	var n int
	if err := json.Unmarshal(b, &n); err != nil {
		return nil
	}
	*f = flexInt{value: n, set: true}
	return nil
}

// plain returns the bound as a *int, nil when unset.
func (f *flexInt) plain() *int {
	if f == nil || !f.set {
		return nil
	}
	n := f.value
	return &n
}

// flexBool is the proposal's one boolean, tolerating a quoted "false" for the same
// reason flexStrings exists.
//
// It gates a filter that hides postings (every one not mentioning sponsorship), so an
// answer it cannot read must leave the filter off.
type flexBool bool

func (f *flexBool) UnmarshalJSON(b []byte) error {
	*f = false
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return nil
		}
		*f = flexBool(readsAsTrue(s))
		return nil
	}
	var v bool
	if err := json.Unmarshal(b, &v); err != nil {
		return nil
	}
	*f = flexBool(v)
	return nil
}

// plain returns the flag as a bool.
func (f *flexBool) plain() bool {
	return f != nil && bool(*f)
}

// readsAsTrue reports which written answers mean yes. A closed list, everything else is
// no: a yes misread as no only widens the search, but a no misread as yes silently hides
// postings.
func readsAsTrue(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "yes", "y", "1":
		return true
	}
	return false
}
