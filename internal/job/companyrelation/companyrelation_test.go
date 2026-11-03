package companyrelation

import (
	"errors"
	"strings"
	"testing"
)

func TestProposalValidate(t *testing.T) {
	cases := []struct {
		name string
		p    Proposal
		ok   bool
	}{
		{"same", Proposal{Kind: KindSame, CompanySlug: "a", TargetSlug: "b"}, true},
		{"subsidiary", Proposal{Kind: KindSubsidiary, CompanySlug: "a", TargetSlug: "b"}, true},
		{"unknown kind", Proposal{Kind: "merge", CompanySlug: "a", TargetSlug: "b"}, false},
		{"no target", Proposal{Kind: KindSame, CompanySlug: "a"}, false},
		{"itself", Proposal{Kind: KindSame, CompanySlug: "a", TargetSlug: "a"}, false},
		{"long note", Proposal{Kind: KindSame, CompanySlug: "a", TargetSlug: "b", Note: strings.Repeat("ž", maxNote+1)}, false},
		{"note at the bound, counted in runes", Proposal{Kind: KindSame, CompanySlug: "a", TargetSlug: "b", Note: strings.Repeat("ž", maxNote)}, true},
	}
	for _, c := range cases {
		err := c.p.validate()
		if c.ok && err != nil {
			t.Errorf("%s: unexpected error %v", c.name, err)
		}
		if !c.ok && !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", c.name, err)
		}
	}
}

func TestFoldedKeyIsTheNamesFold(t *testing.T) {
	// The seat and the form go, so ingest finds the key from the name a feed sends.
	if got := foldedKey("krka-tovarna-zdravil-d-d-novo-mesto", "Krka, tovarna zdravil, d. d., Novo mesto"); got != "krkatovarnazdravil" {
		t.Errorf("foldedKey = %q, want krkatovarnazdravil", got)
	}
	if got := foldedKey("acme-x", ""); got != "acmex" {
		t.Errorf("an empty name falls back to the slug's fold: got %q, want acmex", got)
	}
}
