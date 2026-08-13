package companyname

import "testing"

func TestSlugLike(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"lbresearch", true},
		{"gs1ca", true},
		{"chetwood-bank", true}, // hyphens and digits are still slug-like
		{"franklin-electric", true},
		{"afcb", true},
		{"AFC Bournemouth", false}, // has space and uppercase
		{"Centellic", false},       // has uppercase
		{"Bob's Red Mill", false},  // has spaces
		{"", false},                // nothing to work with
		{"123", true},              // bare numeric platform id, e.g. a join.com company id
	}
	for _, c := range cases {
		if got := SlugLike(c.name); got != c.want {
			t.Errorf("SlugLike(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestNumericPlaceholder(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"175014", true},
		{"0", true},
		{"", false},
		{"175014a", false},
		{"gs1ca", false},
	}
	for _, c := range cases {
		if got := NumericPlaceholder(c.name); got != c.want {
			t.Errorf("NumericPlaceholder(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestExtractTitleName(t *testing.T) {
	cases := []struct {
		title string
		want  string
	}{
		{"Jobs at Centellic | Centellic Careers", "Centellic"},
		{"Jobs at AFC Bournemouth | AFC Bournemouth Careers", "AFC Bournemouth"},
		// A pipe without surrounding spaces is the same section separator.
		{"Jobs at Centellic|Centellic Careers", "Centellic"},
		{"Bath Spa University Careers", "Bath Spa University"},
		// A non-"Jobs at" lead-in prefix is stripped too.
		{"Employment Opportunities at BuzzFeed, Inc.", "BuzzFeed, Inc."},
		{"Careers at Acme", "Acme"},
		// A trailing " | ..." fragment is dropped and whitespace collapsed.
		{"AB InBev  | Growth Group Careers", "AB InBev"},
		{"Just a moment...", ""}, // no recognisable pattern
		{"Careers", ""},          // strips to empty
		{"", ""},
	}
	for _, c := range cases {
		if got := ExtractTitleName(c.title); got != c.want {
			t.Errorf("ExtractTitleName(%q) = %q, want %q", c.title, got, c.want)
		}
	}
}

func TestAccept(t *testing.T) {
	cases := []struct {
		source    string
		slug      string
		candidate string
		wantName  string
		wantOK    bool
	}{
		// Accepted: candidate is a spaced-out form of the slug (substring match).
		{"pinpoint", "afcb", "AFC Bournemouth", "AFC Bournemouth", true},
		{"pinpoint", "gs1ca", "GS1 Canada", "GS1 Canada", true},
		{"pinpoint", "bathspa", "Bath Spa University", "Bath Spa University", true},
		// Accepted with HTML-entity decode.
		{"pinpoint", "bobsredmill", "Bob&#39;s Red Mill", "Bob's Red Mill", true},
		{"pinpoint", "aspireallergy", "Aspire Allergy &amp; Sinus", "Aspire Allergy & Sinus", true},
		// Rejected: unrelated name (recruiter / rebrand / wrong subdomain).
		{"pinpoint", "kempinski", "Elena - Meta Recruitment", "", false},
		{"pinpoint", "mountainwarehouse", "Mountain Group", "", false},
		{"pinpoint", "nxcus", "NexCore", "", false},        // single-letter acronym is not enough
		{"pinpoint", "lbresearch", "Centellic", "", false}, // rebrand shares nothing with slug
		// Rejected: empty / junk.
		{"pinpoint", "anything", "", "", false},
		{"pinpoint", "joe-testing", "Joe's Test Platform", "", false},
		// Rejected: candidate is itself a slug — no improvement, and applying it
		// would keep the company slug-like (non-idempotent re-runs).
		{"pinpoint", "osapiens", "osapiens", "", false},
		// Accepted despite sharing no text: slug is a bare numeric platform id
		// resolved by join's authoritative exact-id lookup, so the confidence
		// check is bypassed — trust comes from the id match, not text overlap.
		{"join", "175014", "Goodweek", "Goodweek", true},
		// Rejected even though the slug is numeric: a non-join source's resolver
		// reaches its candidate by scraping a board derived from the URL, not by
		// an id-exact lookup, so an unrelated numeric-slug company must still
		// pass the confidence check like any other candidate.
		{"pinpoint", "175014", "Goodweek", "", false},
	}
	for _, c := range cases {
		gotName, gotOK := Accept(c.source, c.slug, c.candidate)
		if gotOK != c.wantOK || gotName != c.wantName {
			t.Errorf("Accept(%q, %q, %q) = (%q, %v), want (%q, %v)",
				c.source, c.slug, c.candidate, gotName, gotOK, c.wantName, c.wantOK)
		}
	}
}
