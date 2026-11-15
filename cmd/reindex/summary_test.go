package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/job/jobdedup"
	"github.com/strelov1/freehire/internal/platform/db"
)

// Public myitjob postings captured 2026-11-15 from the reported Swiss DevOps search.
func summaryFixture(t *testing.T) *fakeFuzzyQuerier {
	t.Helper()
	data, err := os.ReadFile("testdata/whatjobs_summaries.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct{ Title, Description, Location string }
	if err := json.Unmarshal(data, &rows); err != nil {
		t.Fatal(err)
	}
	f := &fakeFuzzyQuerier{bodies: map[int64]string{}}
	for i, row := range rows {
		id := int64(i + 1)
		f.titles = append(f.titles, db.FuzzyDedupCandidateTitlesForCompanyRow{
			ID: id, Title: row.Title, Source: "whatjobs-ch", Location: row.Location,
			Category: "devops", Seniority: "senior", PostingLanguage: "de",
		})
		f.bodies[id] = row.Description
	}
	return f
}

func TestCollapseFuzzyForCompany_WhatJobsSummary(t *testing.T) {
	f := summaryFixture(t)
	if _, err := collapseFuzzyDuplicatesForCompany(context.Background(), f, "myitjob"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.got.Ids, []int64{2}) || !reflect.DeepEqual(f.got.Canons, []int64{1}) {
		t.Fatalf("assignments = %v -> %v, want summary 2 -> full posting 1", f.got.Ids, f.got.Canons)
	}
}

func TestCollapseFuzzyForCompany_WhatJobsSummaryBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*fakeFuzzyQuerier)
	}{
		{"different market", func(f *fakeFuzzyQuerier) { f.titles[1].Source = "whatjobs-de" }},
		{"different location", func(f *fakeFuzzyQuerier) { f.titles[1].Location = "Zürich, Switzerland" }},
		{"unknown location", func(f *fakeFuzzyQuerier) { f.titles[1].Location = "" }},
		{"different category", func(f *fakeFuzzyQuerier) { f.titles[1].Category = "backend" }},
		{"different seniority", func(f *fakeFuzzyQuerier) { f.titles[1].Seniority = "junior" }},
		{"different language", func(f *fakeFuzzyQuerier) { f.titles[1].PostingLanguage = "en" }},
		{"different programming language", func(f *fakeFuzzyQuerier) {
			f.titles[0].Title = "Senior Software Engineer C Kubernetes OpenShift"
			f.titles[1].Title = "Senior Software Engineer R Kubernetes OpenShift"
		}},
		{"different requisition", func(f *fakeFuzzyQuerier) {
			f.titles[0].Title += " Ref #26316"
			f.titles[1].Title += " Ref #26317"
		}},
		{"different cloud", func(f *fakeFuzzyQuerier) { f.titles[1].Title += " AWS" }},
		{"different role", func(f *fakeFuzzyQuerier) { f.titles[1].Title = "Senior Backend Developer" }},
		{"other source", func(f *fakeFuzzyQuerier) { f.titles[0].Source = "xing"; f.titles[1].Source = "xing" }},
		{"empty description", func(f *fakeFuzzyQuerier) { f.bodies[2] = "" }},
		{"different responsibilities", func(f *fakeFuzzyQuerier) { f.bodies[2] = bodyB }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := summaryFixture(t)
			tc.change(f)
			if _, err := collapseFuzzyDuplicatesForCompany(context.Background(), f, "myitjob"); err != nil {
				t.Fatal(err)
			}
			if len(f.got.Ids) != 0 {
				t.Fatalf("merged distinct postings: %v -> %v", f.got.Ids, f.got.Canons)
			}
		})
	}
}

func TestCollapseFuzzyForCompany_RejectsAmbiguousSummary(t *testing.T) {
	f := summaryFixture(t)
	third := f.titles[0]
	third.ID = 3
	f.titles = append(f.titles, third)
	// A second role with a distinct full description but the same shared summary.
	f.bodies[3] = f.bodies[1] + " " + bodyB
	if _, err := collapseFuzzyDuplicatesForCompany(context.Background(), f, "myitjob"); err != nil {
		t.Fatal(err)
	}
	if len(f.got.Ids) != 0 {
		t.Fatalf("ambiguous summary merged: %v -> %v", f.got.Ids, f.got.Canons)
	}
}

func TestCollapseFuzzyForCompany_PrefersFullPostingEvenWhenNewer(t *testing.T) {
	f := summaryFixture(t)
	f.titles[0].ID = 9
	f.bodies[9] = f.bodies[1]
	delete(f.bodies, 1)
	if _, err := collapseFuzzyDuplicatesForCompany(context.Background(), f, "myitjob"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.got.Ids, []int64{2}) || !reflect.DeepEqual(f.got.Canons, []int64{9}) {
		t.Fatalf("assignments = %v -> %v, want summary 2 -> full posting 9", f.got.Ids, f.got.Canons)
	}
}

func TestCollapseFuzzyForCompany_WhatJobsBudgetPreservesMarkers(t *testing.T) {
	f := summaryFixture(t)
	for i := 0; i < jobdedup.FuzzyMaxBucket; i++ {
		row := f.titles[0]
		row.ID = int64(i + 10)
		// A new suffix keeps exact-role buckets small while the summary group grows.
		row.Title += " Specialization" + string(rune('A'+i))
		f.titles = append(f.titles, row)
	}
	if _, err := collapseFuzzyDuplicatesForCompany(context.Background(), f, "myitjob"); err != nil {
		t.Fatal(err)
	}
	if f.calls != 0 {
		t.Fatalf("over-budget summary group was judged: %+v", f.got)
	}
}

func TestCollapseFuzzyForCompany_ExactBudgetStillAppliesToWhatJobs(t *testing.T) {
	f := summaryFixture(t)
	for i := 0; i < jobdedup.FuzzyMaxBucket; i++ {
		row := f.titles[0]
		row.ID = int64(i + 10)
		row.Location = "Different place"
		f.titles = append(f.titles, row)
	}
	if _, err := collapseFuzzyDuplicatesForCompany(context.Background(), f, "myitjob"); err != nil {
		t.Fatal(err)
	}
	for _, id := range f.got.Candidates {
		if id == 1 {
			t.Fatalf("over-budget exact bucket member was judged: %+v", f.got)
		}
	}
}

func TestCollapseFuzzyForCompany_ReleasesSummaryWhenFullPostingDisappears(t *testing.T) {
	f := summaryFixture(t)
	f.titles = f.titles[1:]
	if _, err := collapseFuzzyDuplicatesForCompany(context.Background(), f, "myitjob"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.got.Candidates, []int64{2}) || len(f.got.Ids) != 0 {
		t.Fatalf("surviving summary must be released: %+v", f.got)
	}
}

func TestCollapseFuzzyForCompany_SummariesDoNotChain(t *testing.T) {
	f := summaryFixture(t)
	third := f.titles[0]
	third.ID = 3
	f.titles = append(f.titles, third)
	for i := range f.titles {
		f.titles[i].Title = "Senior DevOps Engineer Kubernetes Platform"
	}
	words := func(start, end int) string {
		var out []string
		for i := start; i < end; i++ {
			out = append(out, fmt.Sprintf("term%d", i))
		}
		return strings.Join(out, " ")
	}
	f.bodies[1] = words(0, 100) + " Kubernetes Platform alpha beta gamma"
	f.bodies[2] = words(0, 48) + " " + words(100, 112) + " Kubernetes Platform"
	f.bodies[3] = words(0, 18) + " " + words(100, 112) + " Kubernetes Platform"
	if _, err := collapseFuzzyDuplicatesForCompany(context.Background(), f, "myitjob"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.got.Ids, []int64{2}) || !reflect.DeepEqual(f.got.Canons, []int64{1}) {
		t.Fatalf("summary chained through another summary: %v -> %v", f.got.Ids, f.got.Canons)
	}
}

func TestCollapseFuzzyForCompany_GenericTitlesAndBoilerplateAreNotIdentity(t *testing.T) {
	f := summaryFixture(t)
	for i := range f.titles {
		f.titles[i].Title = "Senior DevOps Engineer"
	}
	f.bodies[1] = bodyA
	f.bodies[2] = "Compensation reviews happen twice yearly, equipment budgets renew annually, conference attendance receives support, remote colleagues gather quarterly for planning workshops, and internal mobility between squads remains actively encouraged."
	if _, err := collapseFuzzyDuplicatesForCompany(context.Background(), f, "myitjob"); err != nil {
		t.Fatal(err)
	}
	if len(f.got.Ids) != 0 {
		t.Fatalf("employer boilerplate merged unrelated generic roles: %v -> %v", f.got.Ids, f.got.Canons)
	}
}
