package worker

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/strelov1/freehire/internal/db"
)

// fakeReader is a scripted pageReader for resilientPage tests. batchErr, when set,
// is returned by batch(); rows map holds per-id results for the degrade path
// (a nil error with an all-zero Job means "corrupted" only if listed in rowErr).
type fakeReader struct {
	batchRows  []db.Job
	batchErr   error
	idList     []int64
	idErr      error
	rowResults map[int64]db.Job
	rowErrs    map[int64]error
	batchCalls int
	idCalls    int
}

func (f *fakeReader) Batch(_ context.Context, _ int64, _ int32) ([]db.Job, error) {
	f.batchCalls++
	return f.batchRows, f.batchErr
}

func (f *fakeReader) IDs(_ context.Context, _ int64, _ int32) ([]int64, error) {
	f.idCalls++
	return f.idList, f.idErr
}

func (f *fakeReader) Row(_ context.Context, id int64) (db.Job, error) {
	if err := f.rowErrs[id]; err != nil {
		return db.Job{}, err
	}
	return f.rowResults[id], nil
}

// corruptErr is the SQLSTATE the resilient scan degrades on. The classification itself is
// pgerr's (and pgerr tests it); what these tests exercise is the POLICY built on it.
func corruptErr() error { return &pgconn.PgError{Code: "XX001"} }

func TestResilientPage_HealthyBatch(t *testing.T) {
	r := &fakeReader{batchRows: []db.Job{{ID: 1}, {ID: 2}, {ID: 5}}}
	rows, lastID, skipped, err := ResilientPage(context.Background(), r, 0, 2000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 3 || lastID != 5 || len(skipped) != 0 {
		t.Fatalf("rows=%d lastID=%d skipped=%v", len(rows), lastID, skipped)
	}
	if r.idCalls != 0 {
		t.Fatalf("degrade path used on healthy batch: idCalls=%d", r.idCalls)
	}
}

func TestResilientPage_EmptyBatchExhausted(t *testing.T) {
	r := &fakeReader{batchRows: nil}
	rows, lastID, _, err := ResilientPage(context.Background(), r, 42, 2000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 0 || lastID != 42 {
		t.Fatalf("expected no rows and lastID unchanged (42), got rows=%d lastID=%d", len(rows), lastID)
	}
}

func TestResilientPage_CorruptedRowSkipped(t *testing.T) {
	r := &fakeReader{
		batchErr:   corruptErr(),
		idList:     []int64{10, 11, 12},
		rowResults: map[int64]db.Job{10: {ID: 10}, 12: {ID: 12}},
		rowErrs:    map[int64]error{11: corruptErr()},
	}
	rows, lastID, skipped, err := ResilientPage(context.Background(), r, 9, 2000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 2 || rows[0].ID != 10 || rows[1].ID != 12 {
		t.Fatalf("expected rows 10,12; got %+v", rows)
	}
	if len(skipped) != 1 || skipped[0] != 11 {
		t.Fatalf("expected skipped [11]; got %v", skipped)
	}
	if lastID != 12 {
		t.Fatalf("expected keyset advanced to 12 (past skipped row); got %d", lastID)
	}
}

func TestResilientPage_VanishedRowInDegradeSkipped(t *testing.T) {
	// Row 11 disappears (pgx.ErrNoRows) between the id-list and the fetch — a
	// concurrent close/delete. The fast SELECT would omit it; the degrade path must
	// too, without aborting and without counting it as corrupted.
	r := &fakeReader{
		batchErr:   corruptErr(),
		idList:     []int64{10, 11, 12},
		rowResults: map[int64]db.Job{10: {ID: 10}, 12: {ID: 12}},
		rowErrs:    map[int64]error{11: pgx.ErrNoRows},
	}
	rows, lastID, skipped, err := ResilientPage(context.Background(), r, 9, 2000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 2 || rows[0].ID != 10 || rows[1].ID != 12 {
		t.Fatalf("expected rows 10,12; got %+v", rows)
	}
	if len(skipped) != 0 {
		t.Fatalf("a vanished row must not count as corrupted; skipped=%v", skipped)
	}
	if lastID != 12 {
		t.Fatalf("expected keyset advanced to 12; got %d", lastID)
	}
}

func TestResilientPage_NonCorruptionBatchErrorPropagates(t *testing.T) {
	sentinel := errors.New("connection reset")
	r := &fakeReader{batchErr: sentinel}
	_, _, _, err := ResilientPage(context.Background(), r, 0, 2000)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error propagated, got %v", err)
	}
	if r.idCalls != 0 {
		t.Fatalf("degrade path must not run for non-XX001 error")
	}
}

func TestResilientPage_NonCorruptionRowErrorPropagates(t *testing.T) {
	sentinel := errors.New("connection reset mid-row")
	r := &fakeReader{
		batchErr:   corruptErr(),
		idList:     []int64{20, 21},
		rowErrs:    map[int64]error{21: sentinel},
		rowResults: map[int64]db.Job{20: {ID: 20}},
	}
	_, _, _, err := ResilientPage(context.Background(), r, 19, 2000)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel propagated from per-row read, got %v", err)
	}
}
