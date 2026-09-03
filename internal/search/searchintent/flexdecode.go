package searchintent

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

// The schema declares every facet as an array of strings and both bounds as integers,
// and a gateway honouring strict mode makes the shims below unreachable. They exist
// because one does not.
//
// Observed against the live gateway: asked for "seniority": ["senior"], the model
// answered "seniority": "senior". encoding/json aborts the WHOLE unmarshal on the first
// type mismatch, so a single scalar in a single field discarded the entire
// interpretation — the caller got a 500 for an answer that was perfectly usable.
//
// This is the same standing guard resumeextract keeps for the same reason: a provider
// that quietly stops honouring a schema answers 200 with ordinary JSON, and nothing
// else on the path would notice.

// flexStrings is a []string that also accepts a bare scalar, reading it as a
// single-element list. Null and an empty string read as no values, not as [""] — a
// model saying "nothing here" must not become a filter on the empty string.
type flexStrings []string

func (f *flexStrings) UnmarshalJSON(b []byte) error {
	*f = nil
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '[' {
		// Element by element, because one odd element must not cost the list. Decoded
		// straight into []string, a single number in "skills":["go",5] made
		// encoding/json abandon the whole field — the same over-reaction, one level
		// down, that this type exists to stop.
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
	// A bare number or other scalar token: keep it verbatim. Whether it means anything
	// is the resolvers' question, not this one's.
	*f = flexStrings{string(b)}
	return nil
}

// flexInt is a bound that reads a number however the model writes it — bare, quoted, or
// as empty/absent text meaning "not set". Unparseable text is "not set" too rather than
// an error: one confused bound must not cost the caller their whole search.
//
// Empty text is the point. The model writes 0 for a bound it does not mean to set, and
// for the experience CEILING zero is a real filter — it selects postings that ask for no
// prior experience. Observed live: asked for senior roles, the model wrote 0 and
// inverted the search. That field is therefore declared as text (see requestSchema), so
// "" and "0" are different answers rather than the same one.
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

// plain hands the bound on as an ordinary *int, so nothing past the decode boundary has
// to know this type exists.
func (f *flexInt) plain() *int {
	if f == nil || !f.set {
		return nil
	}
	n := f.value
	return &n
}

// flexBool is the proposal's one boolean, read however the model writes it. It exists
// for the same reason flexStrings does: asked for false, the gateway has answered
// "false", and encoding/json abandons the whole object on that first mismatch — so one
// quoted word discarded an interpretation that was otherwise perfectly usable.
//
// Unlike the others, this one gates a filter that HIDES postings. Sponsorship is asked
// for by people who need it; switching it on when nobody asked strips every posting
// that does not mention sponsorship, and they cannot tell it happened. So an answer
// this cannot read must leave the filter OFF, never on.
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

// plain hands the flag on as an ordinary bool, so nothing past the decode boundary has
// to know this type exists.
func (f *flexBool) plain() bool {
	return f != nil && bool(*f)
}

// readsAsTrue decides which written answers mean yes.
//
// A closed list rather than a rule, because the two mistakes are not symmetrical: a yes
// misread as no widens the search, which the person can see and correct; a no misread as
// yes strips every posting that does not mention sponsorship, which they cannot. So this
// knows a handful of spellings and calls everything else no — "maybe", "required" and
// "depends" are all answers the model has no business turning into a filter.
func readsAsTrue(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "yes", "y", "1":
		return true
	}
	return false
}
