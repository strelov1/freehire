package main

import "testing"

func TestConvertGroupsRecognizedRowsByProvider(t *testing.T) {
	rows := []inventoryRow{
		{Name: "Acme", URL: "https://boards.greenhouse.io/acme"},
		{Name: "Beta", URL: "https://jobs.lever.co/beta"},
	}

	byProvider, unrecognized := convert(rows)

	if unrecognized != 0 {
		t.Errorf("unrecognized = %d, want 0", unrecognized)
	}
	if got := byProvider["greenhouse"]; len(got) != 1 || got[0] != (seedItem{Board: "acme", Company: "Acme"}) {
		t.Errorf("greenhouse seed = %+v, want one entry for board acme/Acme", got)
	}
	if got := byProvider["lever"]; len(got) != 1 || got[0] != (seedItem{Board: "beta", Company: "Beta"}) {
		t.Errorf("lever seed = %+v, want one entry for board beta/Beta", got)
	}
}

func TestConvertSkipsAndCountsUnrecognizedURLs(t *testing.T) {
	rows := []inventoryRow{
		{Name: "Acme", URL: "https://boards.greenhouse.io/acme"},
		{Name: "VanityCo", URL: "https://careers.vanityco.example"},
	}

	byProvider, unrecognized := convert(rows)

	if unrecognized != 1 {
		t.Errorf("unrecognized = %d, want 1", unrecognized)
	}
	total := 0
	for _, items := range byProvider {
		total += len(items)
	}
	if total != 1 {
		t.Errorf("total recognized entries = %d, want 1", total)
	}
}

func TestConvertDedupesSameProviderAndBoard(t *testing.T) {
	rows := []inventoryRow{
		{Name: "Acme", URL: "https://boards.greenhouse.io/acme"},
		{Name: "Acme (dup)", URL: "https://boards.greenhouse.io/acme"},
	}

	byProvider, _ := convert(rows)

	if got := byProvider["greenhouse"]; len(got) != 1 {
		t.Errorf("greenhouse seed = %+v, want exactly one entry after dedupe", got)
	}
}
