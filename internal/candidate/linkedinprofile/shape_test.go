package linkedinprofile

import (
	"errors"
	"strings"
	"testing"
)

func page(node string) []byte {
	return []byte(`<html><head><script type="application/ld+json">` + node + `</script></head></html>`)
}

// schema.org lets a publisher write "@type" as a string or a list, an entity as a bare name
// or an object, and a list as a single item. Decoding into a Go struct abandons the WHOLE
// document on the first such choice it dislikes — so one shape change in knowsLanguage, a
// member nothing downstream reads, used to take the name, the headline and the location down
// with it and report the profile as unreadable.
func TestParseToleratesLegalShapeVariations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		node string
	}{
		{
			"@type as a one-element list, the standard JSON-LD idiom",
			`{"@type":["Person"],"name":"Dana Okonkwo","description":"Senior Backend Engineer"}`,
		},
		{
			"@type as a list naming more than one type",
			`{"@type":["Thing","Person"],"name":"Dana Okonkwo","description":"Senior Backend Engineer"}`,
		},
		{
			"knowsLanguage as bare strings",
			`{"@type":"Person","name":"Dana Okonkwo","description":"Senior Backend Engineer",
			  "knowsLanguage":["English","Russian"]}`,
		},
		{
			"worksFor as a single object rather than a list",
			`{"@type":"Person","name":"Dana Okonkwo","description":"Senior Backend Engineer",
			  "worksFor":{"name":"Northwind Systems"}}`,
		},
		{
			"address as a plain string",
			`{"@type":"Person","name":"Dana Okonkwo","description":"Senior Backend Engineer",
			  "address":"Lisbon, Portugal"}`,
		},
		{
			"a bare array of nodes with no @graph wrapper",
			`[{"@type":"WebPage"},{"@type":"Person","name":"Dana Okonkwo","description":"Senior Backend Engineer"}]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parse(page(tt.node), "danaokonkwo")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if got.Name != "Dana Okonkwo" {
				t.Errorf("Name = %q — an unrelated member's shape lost the whole profile", got.Name)
			}
			if got.Headline != "Senior Backend Engineer" {
				t.Errorf("Headline = %q", got.Headline)
			}
		})
	}
}

// The variations above must not merely survive: the members that carry them still have to be
// read.
func TestParseReadsTheVariantShapes(t *testing.T) {
	t.Parallel()

	got, err := parse(page(`{"@type":["Person"],"name":"Dana Okonkwo",
		"knowsLanguage":["English","Russian"],
		"worksFor":{"name":"Northwind Systems"},
		"address":"Lisbon, Portugal"}`), "danaokonkwo")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Company != "Northwind Systems" {
		t.Errorf("Company = %q", got.Company)
	}
	if got.Location != "Lisbon, Portugal" {
		t.Errorf("Location = %q", got.Location)
	}
}

// A raw newline inside a JSON string literal is invalid JSON, and Go's decoder — unlike a
// lenient one — rejects the whole document. This repository has been bitten by exactly that
// on a real site before; the sanitiser it added then is now shared.
func TestParseToleratesARawNewlineInsideAStringLiteral(t *testing.T) {
	t.Parallel()

	node := "{\"@type\":\"Person\",\"name\":\"Dana Okonkwo\",\"description\":\"Senior\nBackend Engineer\"}"
	got, err := parse(page(node), "danaokonkwo")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Name != "Dana Okonkwo" {
		t.Errorf("Name = %q", got.Name)
	}
}

// The script element's type may carry parameters or no quotes at all; both are ordinary HTML
// and a real parser handles them, where a pattern matching one exact spelling does not.
func TestParseAcceptsTypeSpellingVariations(t *testing.T) {
	t.Parallel()

	const node = `{"@type":"Person","name":"Dana Okonkwo"}`
	for _, attr := range []string{
		`type="application/ld+json"`,
		`type='application/ld+json'`,
		`type=application/ld+json`,
		`type="application/ld+json; charset=utf-8"`,
		`type="APPLICATION/LD+JSON"`,
	} {
		t.Run(attr, func(t *testing.T) {
			t.Parallel()
			got, err := parse([]byte(`<html><script `+attr+`>`+node+`</script></html>`), "danaokonkwo")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if got.Name != "Dana Okonkwo" {
				t.Errorf("Name = %q", got.Name)
			}
		})
	}
}

// A page can mention more than one member. Returning the wrong one does not fail loudly —
// it shows a stranger's name and headline to the user as their own, and stages it into
// their profile.
func TestParsePrefersTheMemberWhoseProfileWasAskedFor(t *testing.T) {
	t.Parallel()

	node := `{"@graph":[
		{"@type":"Person","name":"Someone Else","description":"Principal Designer",
		 "url":"https://www.linkedin.com/in/someoneelse"},
		{"@type":"Person","name":"Dana Okonkwo","description":"Senior Backend Engineer",
		 "url":"https://br.linkedin.com/in/danaokonkwo"}]}`

	got, err := parse(page(node), "danaokonkwo")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Name != "Dana Okonkwo" {
		t.Fatalf("Name = %q — parse returned a member other than the one requested", got.Name)
	}
}

// When the owner's node is present but withheld entirely, the answer is "we could not read
// your profile" — not somebody else's profile.
func TestParseDoesNotFallBackToAStrangerWhenTheOwnerIsWithheld(t *testing.T) {
	t.Parallel()

	node := `{"@graph":[
		{"@type":"Person","name":"***** *****","description":"****** ******",
		 "url":"https://www.linkedin.com/in/danaokonkwo"},
		{"@type":"Person","name":"Someone Else","description":"Principal Designer",
		 "url":"https://www.linkedin.com/in/someoneelse"}]}`

	got, err := parse(page(node), "danaokonkwo")
	if !errors.Is(err, ErrNoProfile) {
		t.Fatalf("err = %v, want ErrNoProfile — got %+v", err, got)
	}
}

// Every ld+json block is read, so a page that grows a second one does not start failing.
func TestParseReadsBeyondTheFirstBlock(t *testing.T) {
	t.Parallel()

	doc := `<html><head>` +
		`<script type="application/ld+json">{"@type":"WebSite","name":"LinkedIn"}</script>` +
		`<script type="application/ld+json">{"@type":"Person","name":"Dana Okonkwo"}</script>` +
		`</head></html>`

	got, err := parse([]byte(doc), "danaokonkwo")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Name != "Dana Okonkwo" {
		t.Errorf("Name = %q", got.Name)
	}
}

// One sentinel, three messages. A run of "no JSON-LD block" on HTTP 200 is what LinkedIn
// shutting us out looks like, and an operator must be able to tell that from users pasting
// private profiles.
func TestParseFailureMessagesNameTheirCause(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		doc  string
		want string
	}{
		{"an authwall carries no JSON-LD", `<html><body>Sign in</body></html>`, "no JSON-LD block"},
		{"a page about something else", `<html><script type="application/ld+json">{"@type":"WebPage"}</script></html>`, "no Person node"},
		{"a member who released nothing", `<html><script type="application/ld+json">{"@type":"Person","name":"*****"}</script></html>`, "every field withheld"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := parse([]byte(tt.doc), "danaokonkwo")
			if !errors.Is(err, ErrNoProfile) {
				t.Fatalf("err = %v, want ErrNoProfile", err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %q, want it to mention %q", err, tt.want)
			}
		})
	}
}
