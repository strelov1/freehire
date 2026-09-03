package linkedinprofile

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
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

	got, err := Parse(fixture(t, "profile.html"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
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
	if want := []string{"English", "Russian", "Portuguese"}; !slices.Equal(got.Languages, want) {
		t.Errorf("Languages = %v, want %v", got.Languages, want)
	}
}

// The one failure that would actually reach a user's profile: a masked run written
// into a field as if it were a value.
func TestParseNeverEmitsAMaskedRun(t *testing.T) {
	t.Parallel()

	got, err := Parse(fixture(t, "profile.html"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	fields := append([]string{got.Name, got.Headline, got.Location, got.Company}, got.Languages...)
	for _, f := range fields {
		if strings.Contains(f, "*") {
			t.Errorf("field %q carries a masked run", f)
		}
	}
}

// The decoy nodes ahead of the Person in the real @graph include a DiscussionForumPosting
// whose author is itself a Person — walking to the first "Person" anywhere would pick up
// a stranger's name.
func TestParseIgnoresANestedPersonInADecoyNode(t *testing.T) {
	t.Parallel()

	got, err := Parse(fixture(t, "profile.html"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Name == "Not The Profile Owner" {
		t.Fatal("Parse picked up the Person nested inside a DiscussionForumPosting")
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
			got, err := Parse([]byte(tt.page))
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

	got, err := Parse(fixture(t, "profile_fully_masked.html"))
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

	got, err := Parse([]byte(page))
	if err != nil {
		t.Fatalf("Parse: %v", err)
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

	got, err := Parse([]byte(page))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Name != "Dana Okonkwo" || got.Headline != "Staff Frontend Engineer" {
		t.Errorf("got %+v", got)
	}
}
