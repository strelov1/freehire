package sources

import (
	"context"
	"os"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

type optiusRecordedDetail string

func (s optiusRecordedDetail) GetHTML(context.Context, string) (*html.Node, error) {
	return html.Parse(strings.NewReader(string(s)))
}

func TestOptiusImageAdvertWithUnescapedJSONLDQuotes(t *testing.T) {
	b, err := os.ReadFile("testdata/optius/posting_unescaped_quotes.html")
	if err != nil {
		t.Fatal(err)
	}
	s := optius{http: optiusRecordedDetail(b)}
	j, ok, err := s.detail(context.Background(), optiusCard{})
	if err != nil || !ok {
		t.Fatalf("detail: ok=%v err=%v", ok, err)
	}
	for _, want := range []string{"Skok v daljino", "okužiš", "Decathlon"} {
		if !strings.Contains(j.Description, want) {
			t.Errorf("description missing %q", want)
		}
	}
	if j.EmploymentType != "full_time" || j.ApplicationDeadline == nil {
		t.Errorf("lost structured facts: %+v", j)
	}
}

func TestOptiusQuoteRepairPreservesEscapesAndRejectsOtherBrokenFields(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want bool
	}{
		{"body quotes", `{"@type":"JobPosting","description":"Say "hello" and \"welcome\".","datePosted":"2026-11-15"}`, true},
		{"adjacent body fields", `{"@type":"JobPosting","description":"Say "hello" and \"welcome\".","responsibilities":"Use "Go".","datePosted":"2026-11-15"}`, true},
		{"last body field", `{"@type":"JobPosting","datePosted":"2026-11-15","description":"Say "hello" and \"welcome\"."}`, true},
		{"unrelated invalid field", `{"@type":"JobPosting","description":"Say "hello".","datePosted":broken}`, false},
		{"other schema", `{"@type":"Article","description":"Say "hello".","datePosted":"2026-11-15"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, err := html.Parse(strings.NewReader(`<script type="application/ld+json">` + tc.raw + `</script>`))
			if err != nil {
				t.Fatal(err)
			}
			var p optiusLD
			if got := optiusJobPosting(root, &p); got != tc.want {
				t.Fatalf("decoded=%v want %v", got, tc.want)
			}
			if tc.want && p.Description != `Say "hello" and "welcome".` {
				t.Errorf("description=%q", p.Description)
			}
		})
	}
}
