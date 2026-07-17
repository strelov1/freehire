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
		{"123", false},             // no letter
	}
	for _, c := range cases {
		if got := SlugLike(c.name); got != c.want {
			t.Errorf("SlugLike(%q) = %v, want %v", c.name, got, c.want)
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
		{"Bath Spa University Careers", "Bath Spa University"},
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
		slug      string
		candidate string
		wantName  string
		wantOK    bool
	}{
		// Accepted: candidate is a spaced-out form of the slug (substring match).
		{"afcb", "AFC Bournemouth", "AFC Bournemouth", true},
		{"gs1ca", "GS1 Canada", "GS1 Canada", true},
		{"bathspa", "Bath Spa University", "Bath Spa University", true},
		// Accepted with HTML-entity decode.
		{"bobsredmill", "Bob&#39;s Red Mill", "Bob's Red Mill", true},
		{"aspireallergy", "Aspire Allergy &amp; Sinus", "Aspire Allergy & Sinus", true},
		// Rejected: unrelated name (recruiter / rebrand / wrong subdomain).
		{"kempinski", "Elena - Meta Recruitment", "", false},
		{"mountainwarehouse", "Mountain Group", "", false},
		{"nxcus", "NexCore", "", false},        // single-letter acronym is not enough
		{"lbresearch", "Centellic", "", false}, // rebrand shares nothing with slug
		// Rejected: empty / junk.
		{"anything", "", "", false},
		{"joe-testing", "Joe's Test Platform", "", false},
		// Rejected: candidate is itself a slug — no improvement, and applying it
		// would keep the company slug-like (non-idempotent re-runs).
		{"osapiens", "osapiens", "", false},
	}
	for _, c := range cases {
		gotName, gotOK := Accept(c.slug, c.candidate)
		if gotOK != c.wantOK || gotName != c.wantName {
			t.Errorf("Accept(%q, %q) = (%q, %v), want (%q, %v)",
				c.slug, c.candidate, gotName, gotOK, c.wantName, c.wantOK)
		}
	}
}
