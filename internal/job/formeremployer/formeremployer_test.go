package formeremployer

import (
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/dict/normalize"
)

func TestResolve(t *testing.T) {
	companies := []Company{
		{Slug: "accenture", Countries: []string{"in", "us"}},
		{Slug: "jp-morgan-chase", Countries: []string{"us"}},
		{Slug: "vk", Countries: []string{"ru"}},
		{Slug: "v-k", Countries: []string{"ru"}},
		{Slug: "ibm", Countries: []string{"in", "us"}},
		{Slug: "care", Countries: []string{"us"}},
		{Slug: "upwork", Countries: []string{"us"}},
		{Slug: "freelance", Countries: []string{"in"}},
	}
	canons := map[string][]string{
		"jpmorganchase": {"jp-morgan-chase"},
	}

	tests := []struct {
		name string
		emps []Employment
		want []string
	}{
		{"a distinctive name resolves through the fold", []Employment{{Company: "Accenture"}}, []string{"accenture"}},
		{"a legal form does not stop it", []Employment{{Company: "Accenture plc"}}, []string{"accenture"}},
		{"the alias registry answers a spelling the fold alone would miss", []Employment{{Company: "JPMorganChase"}}, []string{"jp-morgan-chase"}},
		{"two slugs sharing a fold are a contest, refused", []Employment{{Company: "VK", Location: "Moscow, Russia"}}, nil},
		{"a name that names no employer is refused even when a company carries it", []Employment{{Company: "Freelance"}, {Company: "Upwork"}}, nil},
		{"a short name with no place is refused", []Employment{{Company: "IBM"}}, nil},
		{"a short name in a country the company hires in resolves", []Employment{{Company: "IBM", Location: "Bengaluru, India"}}, []string{"ibm"}},
		{"a short name in a country it does not hire in is refused", []Employment{{Company: "CARE Pvt. Ltd.", Location: "Mumbai, India"}}, nil},
		{"a company with no searchable job resolves to nothing", []Employment{{Company: "Nowhere Industries"}}, nil},
		{"one company banked twice is one slug", []Employment{{Company: "Accenture"}, {Company: "ACCENTURE"}}, []string{"accenture"}},
		{"nothing banked", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Resolve(tt.emps, canons, companies); !slices.Equal(got, tt.want) {
				t.Errorf("Resolve = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolve_ARegistryHoldingTwoCanonsForOneKeyIsRefused(t *testing.T) {
	// The writer keeps one canon per folded key; a registry that holds two is a decision
	// nobody made, and picking either would be the guess the filter exists to refuse.
	companies := []Company{{Slug: "acme-one"}, {Slug: "acme-two"}}
	canons := map[string][]string{"acmewidgets": {"acme-one", "acme-two"}}
	if got := Resolve([]Employment{{Company: "Acme Widgets"}}, canons, companies); got != nil {
		t.Errorf("Resolve = %v, want nothing", got)
	}
}

func TestResolve_TheRegistryOutranksAnUnmergedTwin(t *testing.T) {
	// Both spellings still carry jobs, but the registry has decided which one is the employer.
	companies := []Company{{Slug: "dollar-tree"}, {Slug: "dollartree"}}
	canons := map[string][]string{"dollartree": {"dollar-tree"}}
	got := Resolve([]Employment{{Company: "DollarTree"}}, canons, companies)
	if !slices.Equal(got, []string{"dollar-tree"}) {
		t.Errorf("Resolve = %v, want [dollar-tree]", got)
	}
}

func TestKeys_SkipsNonEmployersAndDeduplicates(t *testing.T) {
	got := Keys([]Employment{
		{Company: "Self-employed"}, {Company: "Accenture"}, {Company: "Accenture Inc."},
		{Company: "Фриланс"}, {Company: "  "}, {Company: "Globant"},
	})
	if want := []string{"accenture", "globant"}; !slices.Equal(got, want) {
		t.Errorf("Keys = %v, want %v", got, want)
	}
}

func TestNonEmployerNamesAllFold(t *testing.T) {
	// A phrase that folds to "" would silently drop out of the list at init.
	for _, n := range nonEmployerNames {
		if Key(n) != "" {
			t.Errorf("Key(%q) = %q, want it refused", n, Key(n))
		}
		if normalize.CompanyKey(n) == "" {
			t.Errorf("%q folds to nothing and refuses nothing", n)
		}
	}
}
