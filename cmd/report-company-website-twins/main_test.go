package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
)

func TestTwinGroupsKeepsOnlyWhatTheFoldCannotReach(t *testing.T) {
	rows := []db.ListCompanyWebsitesRow{
		// The fold already collapses these two, so merge-companies handles them.
		{Website: "https://acme.de", Slug: "acme", Name: "Acme", JobCount: 5},
		{Website: "acme.de/jobs", Slug: "acme-gmbh", Name: "Acme GmbH", JobCount: 1},
		// Three branches of one agency the fold keeps apart, the site spelled three ways.
		{Website: "http://www.forumjobs.be", Slug: "magazijnier", Name: "Magazijnier", JobCount: 1},
		{Website: "https://forumjobs.be/nl", Slug: "forum-jobs", Name: "Forum Jobs", JobCount: 9},
		{Website: "FORUMJOBS.BE", Slug: "forum-jobs-mechelen", Name: "Forum Jobs Mechelen", JobCount: 3},
		// Alone on its host.
		{Website: "https://solo.example", Slug: "solo", Name: "Solo", JobCount: 2},
	}
	groups := twinGroups(rows)
	if len(groups) != 1 || groups[0].Host != "forumjobs.be" || len(groups[0].Members) != 3 {
		t.Fatalf("groups = %+v, want only forumjobs.be with three members", groups)
	}
	if groups[0].Members[0].Slug != "forum-jobs" {
		t.Errorf("first member = %q, want the biggest employer first", groups[0].Members[0].Slug)
	}
	var b bytes.Buffer
	if err := report(&b, groups); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "forum-jobs-mechelen") {
		t.Errorf("report = %q, want every member listed", b.String())
	}
}
