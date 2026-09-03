package boardcatalog

import "testing"

func TestPlaceholderCompanyHumanizesHyphens(t *testing.T) {
	if got := PlaceholderCompany("acme-corp"); got != "Acme Corp" {
		t.Errorf("PlaceholderCompany(acme-corp) = %q, want %q", got, "Acme Corp")
	}
}

func TestPlaceholderCompanyHumanizesUnderscores(t *testing.T) {
	if got := PlaceholderCompany("arvato_systems"); got != "Arvato Systems" {
		t.Errorf("PlaceholderCompany(arvato_systems) = %q, want %q", got, "Arvato Systems")
	}
}

func TestPlaceholderCompanySingleWord(t *testing.T) {
	if got := PlaceholderCompany("cohere"); got != "Cohere" {
		t.Errorf("PlaceholderCompany(cohere) = %q, want %q", got, "Cohere")
	}
}

func TestPlaceholderCompanyEmptyBoard(t *testing.T) {
	if got := PlaceholderCompany(""); got != "" {
		t.Errorf("PlaceholderCompany(\"\") = %q, want empty", got)
	}
}
