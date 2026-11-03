package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
)

func TestTwinGroupsKeepsOnlyWhatTheFoldCannotReach(t *testing.T) {
	rows := []db.ListCompanyWebsiteTwinsRow{
		// The fold already collapses these two, so merge-companies handles them.
		{Host: "acme.de", Slug: "acme", Name: "Acme", JobCount: 5},
		{Host: "acme.de", Slug: "acme-gmbh", Name: "Acme GmbH", JobCount: 1},
		// Three branches of one agency the fold keeps apart.
		{Host: "forumjobs.be", Slug: "forum-jobs", Name: "Forum Jobs", JobCount: 9},
		{Host: "forumjobs.be", Slug: "forum-jobs-mechelen", Name: "Forum Jobs Mechelen", JobCount: 3},
		{Host: "forumjobs.be", Slug: "magazijnier", Name: "Magazijnier", JobCount: 1},
	}
	groups := twinGroups(rows)
	if len(groups) != 1 || groups[0].Host != "forumjobs.be" || len(groups[0].Members) != 3 {
		t.Fatalf("groups = %+v, want only forumjobs.be with three members", groups)
	}
	var b bytes.Buffer
	if err := report(&b, groups); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "forum-jobs-mechelen") {
		t.Errorf("report = %q, want every member listed", b.String())
	}
}
