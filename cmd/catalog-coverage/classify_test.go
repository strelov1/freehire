package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/job/jobhash"
)

func testCatalogue() catalogue {
	acme := normalize.CompanyKey("Acme Inc")
	return catalogue{
		roles:     map[string]bool{jobhash.RoleKey(acme, "Senior Go Engineer"): true},
		companies: map[string]bool{acme: true},
		board: func(provider, board string) (boardPostings, error) {
			if provider != "greenhouse" || board != "acme" {
				return boardPostings{}, nil
			}
			return boardPostings{
				known: true,
				ids:   []string{"5280683007"},
				roles: map[string]bool{jobhash.RoleKey(boardRoleScope, "Systems Administrator"): true},
			}, nil
		},
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		name string
		s    sample
		want verdict
	}{
		{"same role, other spelling and legal form", sample{Name: "ACME, Inc.", Title: "senior go engineer", URL: "https://elsewhere.example/1"}, verdict{Bucket: bucketHave}},
		{"posting missing from a board we crawl", sample{Name: "Acme", Title: "Designer", URL: "https://job-boards.greenhouse.io/acme/jobs/123"}, verdict{bucketKnownBoard, "greenhouse", "acme"}},
		{"posting on our board, matched by its id", sample{Name: "Acme Corp", Title: "Renamed", URL: "https://boards.greenhouse.io/acme/jobs/5280683007?gh_jid=5280683007"}, verdict{bucketHave, "greenhouse", "acme"}},
		{"posting on our board, matched by title under another company spelling", sample{Name: "Acme Holdings", Title: "Systems Administrator", URL: "https://job-boards.greenhouse.io/acme/jobs/1"}, verdict{bucketHave, "greenhouse", "acme"}},
		{"an id inside a longer id is not the posting", sample{Name: "Beta", Title: "Designer", URL: "https://boards.greenhouse.io/acme/jobs/52806830071"}, verdict{bucketKnownBoard, "greenhouse", "acme"}},
		{"board we do not crawl", sample{Name: "Beta", Title: "Designer", URL: "https://jobs.lever.co/beta/abc"}, verdict{bucketNewBoard, "lever", "beta"}},
		{"known employer, unrecognised link", sample{Name: "Acme", Title: "Designer", URL: "https://aggregator.example/j/9"}, verdict{Bucket: bucketKnownCompany}},
		{"unknown employer", sample{Name: "Gamma", Title: "Designer", URL: "https://aggregator.example/j/9"}, verdict{Bucket: bucketUnknownCompany}},
		{"no company", sample{Name: "", Title: "Designer"}, verdict{Bucket: bucketUnusable}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := classify(tc.s, testCatalogue())
			if err != nil || got != tc.want {
				t.Fatalf("classify = %+v, %v; want %+v", got, err, tc.want)
			}
		})
	}
}

func TestClassifyBoardLookupError(t *testing.T) {
	c := testCatalogue()
	c.board = func(string, string) (boardPostings, error) { return boardPostings{}, errors.New("db down") }
	if _, err := classify(sample{Name: "Beta", Title: "Designer", URL: "https://jobs.lever.co/beta/abc"}, c); err == nil {
		t.Fatal("a failed board lookup must not read as a new board")
	}
}

func TestParseSamples(t *testing.T) {
	got, err := parseSamples(strings.NewReader("url,extra,title,name,ref\nhttps://x/1,z,Dev,Acme,cat-a\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != (sample{Ref: "cat-a", Name: "Acme", Title: "Dev", URL: "https://x/1"}) {
		t.Fatalf("got %+v", got)
	}
	if _, err := parseSamples(strings.NewReader("name,url\nAcme,https://x\n")); err == nil {
		t.Fatal("a missing title column must fail")
	}
}

func TestReportAndSummary(t *testing.T) {
	samples := []sample{{Ref: "a", Name: "Acme"}, {Ref: "a", Name: "Beta"}, {Ref: "a"}}
	verdicts := []verdict{{Bucket: bucketHave}, {Bucket: bucketNewBoard, Provider: "lever", Board: "beta"}, {Bucket: bucketUnusable}}

	var report bytes.Buffer
	if err := writeReport(&report, samples, verdicts); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(report.String(), "a,Beta,,,new_board,lever,beta") {
		t.Fatalf("report:\n%s", report.String())
	}

	var sum bytes.Buffer
	writeSummary(&sum, samples, verdicts)
	if !strings.Contains(sum.String(), "have 50.0% of 2") {
		t.Fatalf("summary: %s", sum.String())
	}
}
