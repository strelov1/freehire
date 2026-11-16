package sources

import (
	"context"
	"strings"
	"testing"
)

func TestClinchHydrationConvergesAfterChallenge(t *testing.T) {
	const base = "https://careers.withwaymo.com/jobs/"
	urls := []string{
		base + "one-london-united-kingdom",
		base + "two-london-united-kingdom",
		base + "three-london-united-kingdom",
	}
	sitemap := (&routedHTTP{}).route("/sitemap.xml", clinchSitemapXML(urls...))
	detail := &routedDetail{routes: map[string]string{
		"/jobs/one-":   clinchDetailHTML("First body"),
		"/jobs/two-":   "CHALLENGE",
		"/jobs/three-": clinchDetailHTML("Third body"),
	}}
	src, ok := NewClinch(sitemap, detail).(HydratingSource)
	if !ok {
		t.Fatal("Clinch must use the stored hydration seen-set")
	}
	entry := CompanyEntry{Board: "careers.withwaymo.com"}
	seen := map[string]bool{}
	for run := range 3 {
		jobs, err := src.FetchNew(context.Background(), entry, func(id string) bool { return seen[id] })
		if err != nil || len(jobs) != 3 {
			t.Fatalf("run %d: jobs=%d err=%v", run, len(jobs), err)
		}
		if run == 0 {
			if len(detail.calls) != 2 || !jobs[1].Unreadable || !jobs[2].Unreadable {
				t.Fatalf("challenge must retain all listing identities as unreadable: %+v", jobs)
			}
			detail.routes["/jobs/two-"] = clinchDetailHTML("Recovered body")
		}
		for _, j := range jobs {
			if j.Description != "" {
				seen[j.ExternalID] = true
			}
		}
		if run > 0 && (!jobs[0].SeenRefresh || jobs[0].Description != "") {
			t.Fatal("known prefix must only refresh liveness")
		}
	}
	if len(seen) != 3 || len(detail.calls) != 4 {
		t.Fatalf("bodies=%d requests=%v; want 3 bodies in 4 requests", len(seen), detail.calls)
	}
	// The refresh policy can withhold a stored ID to reread employer edits.
	detail.routes["/jobs/one-"] = clinchDetailHTML("Edited body")
	jobs, err := src.FetchNew(context.Background(), entry, func(id string) bool { return !strings.HasPrefix(id, "one-") })
	if err != nil || jobs[0].Description != "Edited body" {
		t.Fatalf("due refresh: %+v, %v", jobs, err)
	}
}

func TestClinchEmptyDetailRemainsUnreadable(t *testing.T) {
	const url = "https://careers.withwaymo.com/jobs/backend-engineer-london-united-kingdom"
	sitemap := (&routedHTTP{}).route("/sitemap.xml", clinchSitemapXML(url))
	for _, body := range []string{"<html>No description component</html>", clinchDetailHTML("  "), "CHALLENGE"} {
		detail := &routedDetail{routes: map[string]string{"/jobs/": body}}
		jobs, err := NewClinch(sitemap, detail).Fetch(context.Background(), CompanyEntry{Board: "careers.withwaymo.com"})
		if err != nil || len(jobs) != 1 || !jobs[0].Unreadable || jobs[0].SeenRefresh {
			t.Fatalf("empty/challenged detail must not be saved as hydrated: %+v, %v", jobs, err)
		}
	}
}
