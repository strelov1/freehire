package normalize

import "testing"

func TestSameCompany(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		reported string
		want     bool
	}{
		{"identical", "Adoreal", "Adoreal", true},
		{"case differs", "FLOSUM", "Flosum", true},
		{"legal suffix on the expected side", "Arch Capital Group Ltd.", "Arch Capital Group", true},
		{"legal suffix on the reported side", "Derq", "Derq, Inc.", true},
		{"punctuation and spacing differ", "Much Better Adventures", "much-better_adventures", true},
		{"ampersand spelled out is still a difference", "Ben & Jerry", "Ben and Jerry", false},
		{"compound legal form", "Atlassian Pty Ltd", "Atlassian", true},
		{"non-anglo legal form", "Siemens AG", "Siemens", true},
		{"punctuated legal form", "Trafalgar A/S", "Trafalgar", true},
		{"diacritics folded", "Grupo Éxito", "Grupo Exito", true},
		{"different employers", "Prequel", "A. C. Coy", false},
		{"both normalize to nothing", "???", "—", false},
		{"placeholder tenant", "Anatta Design", "Fake job", false},
		{"non-latin script folded", "Яндекс", "Iandeks", true},
		{"one name is a prefix of the other", "Base", "Basecamp", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SameCompany(tt.expected, tt.reported); got != tt.want {
				t.Errorf("SameCompany(%q, %q) = %v, want %v", tt.expected, tt.reported, got, tt.want)
			}
		})
	}
}

func TestBoardNameSlug(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"punctuated Italian form the company key must keep", "Acme S.p.A.", "acme"},
		{"bare spelling of the same form", "Acme SpA", "acme"},
		{"legal form CompanySlug already strips", "Datapizza S.r.l.", "datapizza"},
		{"brand tail", "Thales Group", "thales"},
		{"compound tail", "Acme Group S.p.A.", "acme"},
		{"tail mid-name is not a tail", "Spa Holiday Systems", "spa-holiday-systems"},
		{"single word is never stripped", "Spa", "spa"},
		{"no tail", "Datapizza", "datapizza"},
		// The over-trim this vocabulary knowingly buys: a resort folds onto a hotel chain's
		// name. Harmless here, where it is one extra probe an unrelated board simply fails —
		// and the reason the tails may not join legalSuffixes, guarded just below.
		{"over-trims a literal spa, which is the accepted cost", "Hilton Luxor Resort & Spa", "hilton-luxor-resort"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BoardNameSlug(tt.in); got != tt.want {
				t.Errorf("BoardNameSlug(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestBoardNameTailsNeverReachTheCompanyKey is the guard the vocabulary exists for: these
// tails are safe to drop from a board-id GUESS and must never re-key an employer.
func TestBoardNameTailsNeverReachTheCompanyKey(t *testing.T) {
	for _, name := range []string{"Hilton Luxor Resort & Spa", "Thales Group", "Acme S.p.A."} {
		t.Run(name, func(t *testing.T) {
			if CompanySlug(name) == BoardNameSlug(name) {
				t.Errorf("CompanySlug(%q) dropped a board-name tail; it must keep %q apart from its board guess",
					name, BoardNameSlug(name))
			}
		})
	}
}

func TestIsBoardNameTail(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"spa", true},
		{"S.p.A.", true},
		{"Group", true},
		{"ltd", false}, // a legal form: IsLegalForm's vocabulary, not this one
		{"datapizza", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := IsBoardNameTail(tt.in); got != tt.want {
				t.Errorf("IsBoardNameTail(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
