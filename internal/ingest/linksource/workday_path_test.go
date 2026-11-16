package linksource

import (
	"context"
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

func TestResolveOnBoardHydratesWorkdayPathIdentity(t *testing.T) {
	const host = "https://acme.wd5.myworkdayjobs.com"
	const target = "/job/Edinburgh/Back-End-Engineer_R65879-contract"
	for _, prefix := range []string{"/en-US/Careers", "/Careers"} {
		t.Run(prefix, func(t *testing.T) {
			board := &fakeHydratingBoard{provider: "workday", jobs: []sources.Job{
				{ExternalID: "/job/London/Back-End-Engineer_R65879", URL: host + "/en-US/Careers/job/London/Back-End-Engineer_R65879", Description: "Different location"},
				{ExternalID: "/job/Edinburgh/Back-End-Engineer_R65879", URL: host + "/en-US/Careers/job/Edinburgh/Back-End-Engineer_R65879", Description: "Distinct posting with a shared path prefix"},
				{ExternalID: target, URL: host + "/en-US/Careers" + target, Title: "Back End Engineer", Description: "Build cloud video services."},
				{ExternalID: "/job/Edinburgh/Back-End-Engineer_R65884", URL: host + "/en-US/Careers/job/Edinburgh/Back-End-Engineer_R65884", Description: "Different requisition"},
			}}
			reg := map[string]sources.Source{"workday": board}
			job, ok, err := ResolveOnBoard(context.Background(), reg, nil, "workday", "acme", host+prefix+target+"?source=LinkedIn")
			if err != nil || !ok || job.Description != "Build cloud video services." || job.ExternalID != "acme:"+target {
				t.Fatalf("resolved posting = %+v, found=%v, err=%v", job, ok, err)
			}
			if board.fetched || !slices.Equal(board.hydrated, []string{target}) {
				t.Fatalf("hydrated=%v, full fetch=%v; want only target path", board.hydrated, board.fetched)
			}
		})
	}
}
