package handler

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
)

func TestStampsFresh(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	ts := func(tm time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: tm, Valid: true} }
	txt := func(s string) pgtype.Text { return pgtype.Text{String: s, Valid: true} }

	row := db.GetUserJobAnalysisRow{CvUploadedAt: ts(now), JobContentHash: txt("hash-1"), Model: "model-a"}

	t.Run("all stamps match → fresh", func(t *testing.T) {
		if !stampsFresh(row, &now, txt("hash-1"), "model-a") {
			t.Error("want fresh when CV time, job hash, and model all match")
		}
	})
	t.Run("CV re-uploaded → stale", func(t *testing.T) {
		later := now.Add(time.Hour)
		if stampsFresh(row, &later, txt("hash-1"), "model-a") {
			t.Error("want stale when the CV upload time changed")
		}
	})
	t.Run("job re-ingested → stale", func(t *testing.T) {
		if stampsFresh(row, &now, txt("hash-2"), "model-a") {
			t.Error("want stale when the job content hash changed")
		}
	})
	t.Run("model upgraded → stale", func(t *testing.T) {
		if stampsFresh(row, &now, txt("hash-1"), "model-b") {
			t.Error("want stale when LLM_MODEL changed since the analysis")
		}
	})
	t.Run("missing live CV time → stale (cannot confirm)", func(t *testing.T) {
		if stampsFresh(row, nil, txt("hash-1"), "model-a") {
			t.Error("want stale when the live CV upload time is unknown")
		}
	})
	t.Run("both job hashes null → fresh (non-board job, never re-crawled)", func(t *testing.T) {
		nullHashRow := db.GetUserJobAnalysisRow{CvUploadedAt: ts(now), Model: "model-a"}
		if !stampsFresh(nullHashRow, &now, pgtype.Text{}, "model-a") {
			t.Error("want fresh when neither the stored nor the live job hash exists")
		}
	})
	t.Run("job gains a hash later → stale", func(t *testing.T) {
		nullHashRow := db.GetUserJobAnalysisRow{CvUploadedAt: ts(now), Model: "model-a"}
		if stampsFresh(nullHashRow, &now, txt("hash-1"), "model-a") {
			t.Error("want stale when the job acquired a content hash after the analysis")
		}
	})
}
