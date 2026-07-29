//go:build integration

// Integration tests for the autopilot columns on cvs (see the tailor-autopilot change): the
// pre-run snapshot, the run report, and the revert that restores one and clears both. Every
// query is owner-scoped, so a foreign id must change nothing and report zero rows.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"encoding/json"
	"testing"
)

func TestAutopilotSnapshotAndRevert(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncateCVs(t, pool)
	ctx := context.Background()

	owner := seedCVUser(t, pool, "autopilot-owner@example.com")
	stranger := seedCVUser(t, pool, "autopilot-stranger@example.com")

	created, err := q.CreateCV(ctx, CreateCVParams{
		UserID: owner, Title: "Tailored", TemplateID: "classic-ats",
		Data: []byte(`{"summary":"before the run"}`),
	})
	if err != nil {
		t.Fatalf("create cv: %v", err)
	}

	// Nothing to revert before a run.
	if _, err := q.RevertCVAutopilot(ctx, RevertCVAutopilotParams{ID: created.ID, UserID: owner}); err == nil {
		t.Error("revert without a snapshot returned a row; want no row so the handler can 409")
	}

	// The snapshot copies the document as it stands.
	n, err := q.SnapshotCVForAutopilot(ctx, SnapshotCVForAutopilotParams{ID: created.ID, UserID: owner})
	if err != nil || n != 1 {
		t.Fatalf("snapshot: err=%v rows=%d", err, n)
	}

	// A stranger cannot snapshot or revert someone else's CV.
	if n, err := q.SnapshotCVForAutopilot(ctx, SnapshotCVForAutopilotParams{ID: created.ID, UserID: stranger}); err != nil || n != 0 {
		t.Errorf("foreign snapshot: err=%v rows=%d, want 0 rows", err, n)
	}

	// The run edits the document and reports what it did.
	if _, err := q.UpdateCV(ctx, UpdateCVParams{
		ID: created.ID, UserID: owner, Title: "Tailored", TemplateID: "classic-ats",
		Data: []byte(`{"summary":"after the run"}`),
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	report := []byte(`[{"requirement":"Kafka","status":"closed_bank","note":"reframed"}]`)
	if n, err := q.SetCVAutopilotReport(ctx, SetCVAutopilotReportParams{
		ID: created.ID, UserID: owner, AutopilotReport: report,
	}); err != nil || n != 1 {
		t.Fatalf("set report: err=%v rows=%d", err, n)
	}
	if n, err := q.SetCVAutopilotReport(ctx, SetCVAutopilotReportParams{
		ID: created.ID, UserID: stranger, AutopilotReport: report,
	}); err != nil || n != 0 {
		t.Errorf("foreign report write: err=%v rows=%d, want 0 rows", err, n)
	}

	// The read carries the report and says the run is revertable.
	got, err := q.GetCVByID(ctx, GetCVByIDParams{ID: created.ID, UserID: owner})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.AutopilotRevertable {
		t.Error("revertable = false after a snapshot; want true")
	}
	var items []map[string]any
	if err := json.Unmarshal(got.AutopilotReport, &items); err != nil {
		t.Fatalf("report not valid json: %v (%s)", err, got.AutopilotReport)
	}
	if len(items) != 1 || items[0]["requirement"] != "Kafka" {
		t.Errorf("report not round-tripped: %s", got.AutopilotReport)
	}

	// Reverting restores the pre-run document and clears both columns.
	if _, err := q.RevertCVAutopilot(ctx, RevertCVAutopilotParams{ID: created.ID, UserID: owner}); err != nil {
		t.Fatalf("revert: %v", err)
	}

	after, err := q.GetCVByID(ctx, GetCVByIDParams{ID: created.ID, UserID: owner})
	if err != nil {
		t.Fatalf("get after revert: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(after.Data, &doc); err != nil {
		t.Fatalf("reverted data not valid json: %v", err)
	}
	if doc["summary"] != "before the run" {
		t.Errorf("reverted document = %s, want the pre-run one", after.Data)
	}
	if after.AutopilotRevertable {
		t.Error("revertable = true after a revert; want false — the snapshot is spent")
	}
	if len(after.AutopilotReport) != 0 {
		t.Errorf("report = %s after a revert; want cleared — it would describe edits that are gone", after.AutopilotReport)
	}

	// A second revert has nothing to restore.
	if _, err := q.RevertCVAutopilot(ctx, RevertCVAutopilotParams{ID: created.ID, UserID: owner}); err == nil {
		t.Error("second revert returned a row; want no row")
	}
}

func TestAutopilotSnapshotIsTakenFresh(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	truncateCVs(t, pool)
	ctx := context.Background()

	owner := seedCVUser(t, pool, "autopilot-second-run@example.com")
	created, err := q.CreateCV(ctx, CreateCVParams{
		UserID: owner, Title: "Tailored", TemplateID: "classic-ats", Data: []byte(`{"summary":"first"}`),
	})
	if err != nil {
		t.Fatalf("create cv: %v", err)
	}

	// A second run snapshots the document as it is at ITS start, not the first run's.
	if _, err := q.SnapshotCVForAutopilot(ctx, SnapshotCVForAutopilotParams{ID: created.ID, UserID: owner}); err != nil {
		t.Fatalf("first snapshot: %v", err)
	}
	if _, err := q.UpdateCV(ctx, UpdateCVParams{
		ID: created.ID, UserID: owner, Title: "Tailored", TemplateID: "classic-ats", Data: []byte(`{"summary":"second"}`),
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, err := q.SnapshotCVForAutopilot(ctx, SnapshotCVForAutopilotParams{ID: created.ID, UserID: owner}); err != nil {
		t.Fatalf("second snapshot: %v", err)
	}
	if _, err := q.UpdateCV(ctx, UpdateCVParams{
		ID: created.ID, UserID: owner, Title: "Tailored", TemplateID: "classic-ats", Data: []byte(`{"summary":"third"}`),
	}); err != nil {
		t.Fatalf("update: %v", err)
	}

	if _, err := q.RevertCVAutopilot(ctx, RevertCVAutopilotParams{ID: created.ID, UserID: owner}); err != nil {
		t.Fatalf("revert: %v", err)
	}
	back, err := q.GetCVByID(ctx, GetCVByIDParams{ID: created.ID, UserID: owner})
	if err != nil {
		t.Fatalf("get after revert: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(back.Data, &doc); err != nil {
		t.Fatalf("reverted data not valid json: %v", err)
	}
	if doc["summary"] != "second" {
		t.Errorf("reverted to %s, want the document as it stood when the LAST run started", back.Data)
	}
}
