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
