package handler

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/matchanalysis"
)

func TestBuildAnalysisItems(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	ts := func(tm time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: tm, Valid: true} }
	txt := func(s string) pgtype.Text { return pgtype.Text{String: s, Valid: true} }
	blob := func(score int, verdict string) []byte {
		b, _ := json.Marshal(&matchanalysis.Analysis{OverallScore: score, Verdict: verdict})
		return b
	}

	rows := []db.ListUserJobAnalysesRow{
		{ // fresh, open
			PublicSlug: "go-role", Title: "Senior Go", Company: "Acme",
			Analysis: blob(80, "Strong Fit"), Model: "model-a",
			CvUploadedAt: ts(now), JobContentHash: txt("h1"), ContentHash: txt("h1"), CreatedAt: ts(now),
		},
		{ // closed job → Closed true
			PublicSlug: "closed-role", Title: "Backend", Company: "Beta",
			ClosedAt: ts(now), Analysis: blob(55, "Moderate Fit"), Model: "model-a",
			CvUploadedAt: ts(now), JobContentHash: txt("h2"), ContentHash: txt("h2"), CreatedAt: ts(now),
		},
		{ // model changed since analysis → stale
			PublicSlug: "stale-role", Title: "Platform", Company: "Gamma",
			Analysis: blob(60, "Good Fit"), Model: "model-OLD",
			CvUploadedAt: ts(now), JobContentHash: txt("h3"), ContentHash: txt("h3"), CreatedAt: ts(now),
		},
		{ // corrupt blob → skipped
			PublicSlug: "bad", Title: "X", Company: "Y", Analysis: []byte("{not json"), Model: "model-a", CreatedAt: ts(now),
		},
	}

	items := buildAnalysisItems(rows, &now, "model-a")
	if len(items) != 3 {
		t.Fatalf("got %d items, want 3 (corrupt row skipped)", len(items))
	}
	if items[0].Slug != "go-role" || items[0].OverallScore != 80 || items[0].Verdict != "Strong Fit" {
		t.Errorf("item0 = %+v, want go-role/80/Strong Fit", items[0])
	}
	if items[0].Closed {
		t.Error("item0 should be open")
	}
	if items[0].Stale {
		t.Error("item0 stamps all match → not stale")
	}
	if !items[1].Closed {
		t.Error("item1 job is closed → Closed=true")
	}
	if !items[2].Stale {
		t.Error("item2 model changed since analysis → Stale=true")
	}
	if !items[0].AnalysedAt.Equal(now) {
		t.Errorf("item0 analysed_at = %s, want %s", items[0].AnalysedAt, now)
	}
}
