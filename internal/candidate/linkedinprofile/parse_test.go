package linkedinprofile

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

func TestParseRealShape(t *testing.T) {
	t.Parallel()

	got, err := parse(fixture(t, "profile.html"), "danaokonkwo")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if got.Name != "Dana Okonkwo" {
		t.Errorf("Name = %q, want %q", got.Name, "Dana Okonkwo")
	}
	// The headline arrives truncated with an ellipsis; it is passed on as served,
	// because trimming it is the caller's business and the dictionaries do not care.
	wantHeadline := "Senior Backend Engineer working in TypeScript/Node.js, Go, and Python, with, focused on…"
	if got.Headline != wantHeadline {
		t.Errorf("Headline = %q, want %q", got.Headline, wantHeadline)
	}
	if got.Location != "Florianópolis, Santa Catarina, Brazil" {
		t.Errorf("Location = %q", got.Location)
	}
	// The first worksFor entry is the only one LinkedIn left readable.
	if got.Company != "Northwind Systems" {
		t.Errorf("Company = %q, want %q", got.Company, "Northwind Systems")
	}
}

// The one failure that would actually reach a user's profile: a withheld value written
// into a field as if it were one.
//
// The contract is per value, not per character: a field is dropped when the WHOLE of it is
// withheld. An asterisk inside real text stays, deliberately — "Senior Engineer*" is a
// footnote marker on a real title, and scrubbing it would be inventing a value rather than
// declining one. LinkedIn withholds whole strings, so this is the shape that matches the
// source.
func TestParseNeverEmitsAWithheldValue(t *testing.T) {
	t.Parallel()

	got, err := parse(fixture(t, "profile.html"), "danaokonkwo")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	for _, f := range []string{got.Name, got.Headline, got.Location, got.Company} {
		if masked(f) {
			t.Errorf("field %q is a withheld run", f)
		}
		// The fixture carries no partially-masked strings, so on this input the stronger
		// property happens to hold too — asserting it here would pin a promise the
		// contract does not make.
		if strings.Contains(f, "*") {
			t.Errorf("fixture regression: field %q now carries an asterisk", f)
		}
	}
}

// The decoy nodes ahead of the Person in the real @graph include a DiscussionForumPosting
// whose author is itself a Person — walking to the first "Person" anywhere would pick up
// a stranger's name.
func TestParseIgnoresANestedPersonInADecoyNode(t *testing.T) {
	t.Parallel()

	got, err := parse(fixture(t, "profile.html"), "danaokonkwo")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Name == "Not The Profile Owner" {
		t.Fatal("parse picked up the Person nested inside a DiscussionForumPosting")
	}
}

func TestParseFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		page string
	}{
		{"no ld+json block at all", `<html><body>Sign in to continue</body></html>`},
		{
			"an ld+json block that is not valid JSON",
			`<html><script type="application/ld+json">{"@graph": [</script></html>`,
		},
		{
			"a graph carrying no Person node",
			`<html><script type="application/ld+json">{"@graph":[{"@type":"WebPage","url":"x"}]}</script></html>`,
		},
		{
			"an empty document",
			``,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parse([]byte(tt.page), "danaokonkwo")
			if !errors.Is(err, ErrNoProfile) {
				t.Fatalf("err = %v, want ErrNoProfile", err)
			}
			if !reflect.DeepEqual(got, Profile{}) {
				t.Errorf("a failed parse returned %+v, want the zero Profile", got)
			}
		})
	}
}

// A Person node every field of which is masked is not a profile that happens to be
// sparse — it is a profile we were not given, and reporting it as a success would
// send the user back an empty form with no explanation.
func TestParseFullyMaskedPersonIsNotAProfile(t *testing.T) {
	t.Parallel()

	got, err := parse(fixture(t, "profile_fully_masked.html"), "danaokonkwo")
	if !errors.Is(err, ErrNoProfile) {
		t.Fatalf("err = %v, want ErrNoProfile", err)
	}
	if !reflect.DeepEqual(got, Profile{}) {
		t.Errorf("got %+v, want the zero Profile", got)
	}
}

// A profile with a readable name but a withheld headline is a partial success: there
// is something to show the user, and the caller decides that nothing was derived.
func TestParsePartiallyMaskedPersonSucceeds(t *testing.T) {
	t.Parallel()

	page := `<html><script type="application/ld+json">{"@graph":[{"@type":"Person",
		"name":"Dana Okonkwo",
		"description":"****** ******** ********",
		"address":{"addressLocality":"Lisbon, Portugal"},
		"worksFor":[{"name":"**********"}]}]}</script></html>`

	got, err := parse([]byte(page), "danaokonkwo")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Name != "Dana Okonkwo" {
		t.Errorf("Name = %q", got.Name)
	}
	if got.Headline != "" {
		t.Errorf("Headline = %q, want empty (it was masked)", got.Headline)
	}
	if got.Location != "Lisbon, Portugal" {
		t.Errorf("Location = %q", got.Location)
	}
	if got.Company != "" {
		t.Errorf("Company = %q, want empty (it was masked)", got.Company)
	}
}

// LinkedIn serves the Person as a bare object on some pages rather than inside an
// @graph; both shapes are the same document to a reader.
func TestParseAcceptsABarePersonDocument(t *testing.T) {
	t.Parallel()

	page := `<html><script type="application/ld+json">
		{"@context":"http://schema.org","@type":"Person","name":"Dana Okonkwo",
		 "description":"Staff Frontend Engineer"}</script></html>`

	got, err := parse([]byte(page), "danaokonkwo")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Name != "Dana Okonkwo" || got.Headline != "Staff Frontend Engineer" {
		t.Errorf("got %+v", got)
	}
}
