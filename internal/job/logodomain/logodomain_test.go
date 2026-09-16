package logodomain

import "testing"

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"lowercases", "G2i", "g2i"},
		{"collapses whitespace runs", "  Network   Recruitment  ", "network recruitment"},
		{"keeps punctuation", "G2i Inc.", "g2i inc."},
		{"keeps hyphens distinct", "3-M", "3-m"},
		{"empty stays empty", "   ", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeName(tt.in); got != tt.want {
				t.Errorf("NormalizeName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestDomain(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"full url", "https://g2i.co", "g2i.co"},
		{"url with path", "https://acme.com/careers", "acme.com"},
		{"bare host", "acme.com", "acme.com"},
		{"strips www", "https://www.acme.com/", "acme.com"},
		{"strips port", "https://acme.com:8443/", "acme.com"},
		{"strips root label", "acme.com.", "acme.com"},
		{"uppercase folds", "HTTPS://ACME.COM", "acme.com"},
		{"no dot is not a domain", "https://localhost/", ""},
		{"junk", "not a url at all", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Domain(tt.in); got != tt.want {
				t.Errorf("Domain(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestBuildMapsEverySpellingOfACompanyToOneDomain(t *testing.T) {
	websites := map[string]string{"g2i": "https://g2i.co"}
	spellings := []Spelling{
		{Slug: "g2i", Name: "g2i"},
		{Slug: "g2i", Name: "G2i"},
		{Slug: "g2i", Name: "G2i Inc."},
	}
	entries, dropped := Build(websites, spellings)
	if dropped != 0 {
		t.Fatalf("dropped = %d, want 0", dropped)
	}
	// "g2i" and "G2i" normalize onto one key, so three spellings yield two entries.
	want := map[string]string{"g2i": "g2i.co", "g2i inc.": "g2i.co"}
	if len(entries) != len(want) {
		t.Fatalf("entries = %v, want %v", entries, want)
	}
	for k, v := range want {
		if entries[k] != v {
			t.Errorf("entries[%q] = %q, want %q", k, entries[k], v)
		}
	}
}

func TestBuildSkipsCompaniesWithNoUsableWebsite(t *testing.T) {
	websites := map[string]string{"ghost": "not a url at all"}
	entries, dropped := Build(websites, []Spelling{{Slug: "ghost", Name: "Ghost"}})
	if len(entries) != 0 {
		t.Errorf("entries = %v, want empty", entries)
	}
	if dropped != 0 {
		t.Errorf("dropped = %d, want 0 — an unusable website is not a collision", dropped)
	}
}

func TestBuildDropsANameTwoCompaniesShare(t *testing.T) {
	// A wrong domain returns a confident logo belonging to someone else, which is the
	// exact failure this package exists to remove. Neither answer is published.
	websites := map[string]string{"acme-one": "https://acme-one.com", "acme-two": "https://acme-two.com"}
	spellings := []Spelling{
		{Slug: "acme-one", Name: "Acme"},
		{Slug: "acme-two", Name: "ACME"},
	}
	entries, dropped := Build(websites, spellings)
	if _, ok := entries["acme"]; ok {
		t.Errorf("entries kept the colliding name: %v", entries)
	}
	if dropped != 1 {
		t.Errorf("dropped = %d, want 1", dropped)
	}
}

func TestBuildCountsAThreeWayCollisionOnce(t *testing.T) {
	websites := map[string]string{"a": "https://a.com", "b": "https://b.com", "c": "https://c.com"}
	spellings := []Spelling{
		{Slug: "a", Name: "Acme"},
		{Slug: "b", Name: "acme"},
		{Slug: "c", Name: "ACME"},
	}
	entries, dropped := Build(websites, spellings)
	if _, ok := entries["acme"]; ok {
		t.Errorf("entries kept the colliding name: %v", entries)
	}
	if dropped != 1 {
		t.Errorf("dropped = %d, want 1 — one refused NAME, not one per extra company", dropped)
	}
}

func TestBuildKeepsANameTwoCompaniesShareWhenTheDomainAgrees(t *testing.T) {
	// Two slugs for one employer that our alias registry has not merged yet still point
	// at one website. There is nothing ambiguous to protect against.
	websites := map[string]string{"acme": "https://acme.com", "acme-inc": "https://www.acme.com/"}
	spellings := []Spelling{
		{Slug: "acme", Name: "Acme"},
		{Slug: "acme-inc", Name: "Acme"},
	}
	entries, dropped := Build(websites, spellings)
	if entries["acme"] != "acme.com" {
		t.Errorf("entries[%q] = %q, want %q", "acme", entries["acme"], "acme.com")
	}
	if dropped != 0 {
		t.Errorf("dropped = %d, want 0", dropped)
	}
}
