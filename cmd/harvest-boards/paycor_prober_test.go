package main

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestPaycorProbe(t *testing.T) {
	p := paycorProber{}
	const live = "8a7883c664f8df9e0165119e33353606"
	listing := `<html><body>
<a href="https://recruitingbypaycor.com/career/JobIntroduction.action?clientId=` + live + `&amp;id=aa11&amp;source=&amp;lang=en">Line Cook</a>
<a href="https://recruitingbypaycor.com/career/JobIntroduction.action?clientId=` + live + `&amp;id=aa11">Apply</a>
<a href="https://recruitingbypaycor.com/career/JobIntroduction.action?clientId=` + live + `&amp;id=bb22">Dishwasher</a>
<a href="https://recruitingbypaycor.com/career/JobIntroduction.action?clientId=` + live + `">Submit Resume</a>
<a href="https://recruitingbypaycor.com/career/JobIntroduction.action?clientId=4028f88b2456f3b601247ccb93d40fa2&amp;id=cc33">Elsewhere</a>
<a href="https://example.com/about">About</a>
</body></html>`
	getter := fakeGetter{
		"https://recruitingbypaycor.com/career/CareerHome.action?clientId=" + live: listing,
		"https://recruitingbypaycor.com/career/CareerHome.action?clientId=empty":   `<html><body><a href="/about">About</a></body></html>`,
	}
	// Live: two DISTINCT postings — the repeated link counts once, the id-less "submit your
	// resume" link is not a posting, and neither is one on another board. No name is published.
	if name, n, err := p.probe(context.Background(), getter, live); err != nil || name != "" || n != 2 {
		t.Errorf("live: got (%q,%d,%v), want (\"\",2,nil)", name, n, err)
	}
	if _, n, err := p.probe(context.Background(), getter, "empty"); err != nil || n != 0 {
		t.Errorf("empty: got n=%d err=%v, want 0,nil", n, err)
	}
	// An unknown client id 404s, which the client surfaces as an error: not a live board.
	if _, n, err := p.probe(context.Background(), getter, "gone"); err != nil || n != 0 {
		t.Errorf("gone: got n=%d err=%v, want 0,nil", n, err)
	}

	ids, err := p.postingIDs(context.Background(), getter, live)
	if err != nil || !slices.Equal(ids, []string{"aa11", "bb22"}) {
		t.Errorf("postingIDs = %v, %v; want [aa11 bb22], nil", ids, err)
	}
}

func TestPaycorProberDedupKeyFoldsCase(t *testing.T) {
	if got := (paycorProber{}).dedupKey("8A7883C664F8DF9E"); got != "8a7883c664f8df9e" {
		t.Errorf("dedupKey = %q, want the lower-case client id", got)
	}
}

func TestPaycorProbeRejectsBoardsWithManyPlaceholderTitles(t *testing.T) {
	const board = "demo-board"
	var listing strings.Builder
	for i, title := range []string{
		"advertise2", "advertise", "EdgeTest", "IncognitoTest", "StandardNewDemo2",
		"Test Job Friday 25", "TestJob_Paycor", "Test Posting", "Data Analyst (Revi Test)", "Senior Financial Manager [maya test]",
		"Product Designer (Revi Test)", "Test", "advertise3", "Test job", "StandardNewDemo3",
		"Customer Support [demo]", "Software Developer (maya test)", "test 8493", "EdgeTest2", "IncognitoTest2",
		"advertise4", "advertise5", "advertise6", "advertise7", "advertise8", "Sample Posting", "Demo Posting", "Developer (John Test)",
	} {
		fmt.Fprintf(&listing, `<a ns-qa=%q href="/career/JobIntroduction.action?clientId=%s&amp;id=job-%d">Apply</a>`, title, board, i)
	}
	for i := 0; i < 1169; i++ {
		fmt.Fprintf(&listing, `<a ns-qa="Accountant" href="/career/JobIntroduction.action?clientId=%s&amp;id=real-%d">Apply</a>`, board, i)
	}
	getter := fakeGetter{
		"https://recruitingbypaycor.com/career/CareerHome.action?clientId=" + board: listing.String(),
	}
	if _, count, err := (paycorProber{}).probe(context.Background(), getter, board); err != nil || count != 0 {
		t.Fatalf("demo board: got count=%d err=%v, want 0,nil", count, err)
	}
}

func TestPaycorProbeKeepsBoardsWithFewPlaceholderTitles(t *testing.T) {
	const board = "mostly-real-employer"
	var listing strings.Builder
	for i := 0; i < paycorPlaceholderTitleMinimum; i++ {
		fmt.Fprintf(&listing, `<a ns-qa="advertise%d" href="/career/JobIntroduction.action?clientId=%s&amp;id=placeholder-%d">Apply</a>`, i, board, i)
	}
	for i := 0; i < 1000; i++ {
		fmt.Fprintf(&listing, `<a ns-qa="Accountant" href="/career/JobIntroduction.action?clientId=%s&amp;id=real-%d">Apply</a>`, board, i)
	}
	getter := fakeGetter{
		"https://recruitingbypaycor.com/career/CareerHome.action?clientId=" + board: listing.String(),
	}
	if _, count, err := (paycorProber{}).probe(context.Background(), getter, board); err != nil || count != 1010 {
		t.Fatalf("low placeholder share: got count=%d err=%v, want 1010,nil", count, err)
	}
}

func TestPaycorProbeKeepsBoardsWithLegitimateTestRoles(t *testing.T) {
	for _, title := range []string{
		"QA Automation Test Engineer", "Test Engineer", "Game Test Analyst", "Software Engineer in Test",
		"Senior Accountant", "Product Designer", "Test Specialist", "Manual Tester", "Test Engineer II",
		"Product Demo Specialist", "Demo Engineer", "Sample Preparation Technician", "Sample Engineer",
	} {
		const board = "real-employer"
		var listing strings.Builder
		for i := 0; i < paycorPlaceholderTitleMinimum; i++ {
			fmt.Fprintf(&listing, `<a ns-qa=%q href="/career/JobIntroduction.action?clientId=%s&amp;id=job-%d">Apply</a>`, title, board, i)
		}
		getter := fakeGetter{
			"https://recruitingbypaycor.com/career/CareerHome.action?clientId=" + board: listing.String(),
		}
		if _, count, err := (paycorProber{}).probe(context.Background(), getter, board); err != nil || count != paycorPlaceholderTitleMinimum {
			t.Errorf("legitimate title %q: got count=%d err=%v, want %d,nil", title, count, err, paycorPlaceholderTitleMinimum)
		}
	}
}

func TestPaycorProbeRequiresPlaceholderMinimumForRepeatedLinks(t *testing.T) {
	const board = "repeated-placeholder-links"
	var listing strings.Builder
	for i := 0; i < paycorPlaceholderTitleMinimum-1; i++ {
		href := fmt.Sprintf(`/career/JobIntroduction.action?clientId=%s&amp;id=job-%d`, board, i)
		fmt.Fprintf(&listing, `<a ns-qa="advertise" href="%s">Apply</a><a ns-qa="advertise" href="%s">Title</a>`, href, href)
	}
	getter := fakeGetter{
		"https://recruitingbypaycor.com/career/CareerHome.action?clientId=" + board: listing.String(),
	}
	if _, count, err := (paycorProber{}).probe(context.Background(), getter, board); err != nil || count != paycorPlaceholderTitleMinimum-1 {
		t.Fatalf("repeated links: got count=%d err=%v, want %d,nil", count, err, paycorPlaceholderTitleMinimum-1)
	}
}

func TestPaycorProbePlaceholderShareBoundary(t *testing.T) {
	for _, tc := range []struct {
		name      string
		postings  int
		wantCount int
	}{
		{name: "exactly two percent", postings: 500, wantCount: 0},
		{name: "below two percent", postings: 501, wantCount: 501},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const board = "share-boundary"
			var listing strings.Builder
			for i := 0; i < tc.postings; i++ {
				title := "Accountant"
				if i < paycorPlaceholderTitleMinimum {
					title = "advertise"
				}
				fmt.Fprintf(&listing, `<a ns-qa=%q href="/career/JobIntroduction.action?clientId=%s&amp;id=job-%d">Apply</a>`, title, board, i)
			}
			getter := fakeGetter{
				"https://recruitingbypaycor.com/career/CareerHome.action?clientId=" + board: listing.String(),
			}
			if _, count, err := (paycorProber{}).probe(context.Background(), getter, board); err != nil || count != tc.wantCount {
				t.Fatalf("share threshold: got count=%d err=%v, want %d,nil", count, err, tc.wantCount)
			}
		})
	}
}

func TestPaycorProbeFindsPlaceholderTitleAfterApplyLink(t *testing.T) {
	const board = "apply-before-title-demo"
	var listing strings.Builder
	for i := 0; i < paycorPlaceholderTitleMinimum; i++ {
		href := fmt.Sprintf(`/career/JobIntroduction.action?clientId=%s&amp;id=job-%d`, board, i)
		fmt.Fprintf(&listing, `<a href="%s">Apply</a><a ns-qa="advertise" href="%s">Title</a>`, href, href)
	}
	getter := fakeGetter{
		"https://recruitingbypaycor.com/career/CareerHome.action?clientId=" + board: listing.String(),
	}
	if _, count, err := (paycorProber{}).probe(context.Background(), getter, board); err != nil || count != 0 {
		t.Fatalf("placeholder titles after Apply links: got count=%d err=%v, want 0,nil", count, err)
	}
}
