package companyname

import "testing"

// GroundedName is what lets a model READ a name out of a posting without being able to
// invent one: the answer must appear verbatim in the text it was given.
func TestGroundedName(t *testing.T) {
	texts := []string{
		"Intermountain Children’s Services is growing and we want you!",
		"Join the Fosdick Team! Fosdick Fulfillment Corporation is a family-owned 3PL.",
	}
	cases := []struct {
		candidate, want string
		ok              bool
	}{
		{"Fosdick Fulfillment Corporation", "Fosdick Fulfillment Corporation", true},
		{"  fosdick   fulfillment corporation ", "Fosdick Fulfillment Corporation", true}, // the text's own form
		// A straight apostrophe matches the text's curly one; the answer keeps the text's form.
		{"Intermountain Children's Services", "Intermountain Children’s Services", true},
		{"Fosdick Logistics", "", false}, // not in the text: invented
		{"The company", "", false},       // in no text, and generic anyway
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := GroundedName(c.candidate, texts)
		if got != c.want || ok != c.ok {
			t.Errorf("GroundedName(%q) = %q, %v; want %q, %v", c.candidate, got, ok, c.want, c.ok)
		}
	}
	if _, ok := GroundedName("", texts); ok {
		t.Error(`GroundedName("") = ok, want refused`)
	}
}

// A generic phrase is refused even when the text really contains it.
func TestGroundedNameRefusesGenericPhrases(t *testing.T) {
	texts := []string{"Our client: a leading firm. The Company offers benefits. (Confidential)"}
	for _, c := range []string{"Our client", "The Company", "Confidential", "Our client:", "(Confidential)"} {
		if got, ok := GroundedName(c, texts); ok {
			t.Errorf("GroundedName(%q) = %q, ok; want refused", c, got)
		}
	}
}
