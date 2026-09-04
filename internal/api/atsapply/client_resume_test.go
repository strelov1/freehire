package atsapply

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/application/autoapply"
	"github.com/strelov1/freehire/internal/candidate/cv"
)

type fakeCVReader struct {
	rec cv.Record
	err error
}

func (f fakeCVReader) Get(context.Context, uuid.UUID, int64) (cv.Record, error) {
	return f.rec, f.err
}

type fakeCVRenderer struct {
	pdf []byte
	err error
}

func (f fakeCVRenderer) Render(context.Context, cv.Document, cv.Template, []byte, cv.LinkHrefs) ([]byte, error) {
	return f.pdf, f.err
}

// attachApprovedResume is a no-op when the plan has no file field at all — nothing this
// attempt needs to attach, so it must not touch cvs/renderer or return anything to clean up.
func TestAttachApprovedResume_NoFileFieldIsANoOp(t *testing.T) {
	c := &Client{cvs: fakeCVReader{err: errors.New("must not be called")}}
	plan := Plan{Fields: []ResolvedField{{ID: "first_name", Kind: "text", Value: "Ada"}}}

	cleanup, parked, err := c.attachApprovedResume(context.Background(), autoapply.Claimed{}, &plan)
	if err != nil || parked != nil || cleanup != nil {
		t.Fatalf("attachApprovedResume = (cleanup!=nil:%v, %v, %v), want (false, nil, nil)", cleanup != nil, parked, err)
	}
}

// A render failure parks the attempt naming the résumé field, rather than a plain error the
// runner would retry as transient (design.md: "never guess, park instead").
func TestAttachApprovedResume_RenderFailureParksNamingTheField(t *testing.T) {
	c := &Client{
		cvs:      fakeCVReader{rec: cv.Record{}},
		renderer: fakeCVRenderer{err: errors.New("typst: compile failed")},
	}
	plan := Plan{Fields: []ResolvedField{{ID: "resume", Kind: "file"}}}

	cleanup, parked, err := c.attachApprovedResume(context.Background(), autoapply.Claimed{TailoredCVID: uuid.New()}, &plan)
	if err != nil {
		t.Fatalf("attachApprovedResume returned an error, want a parked result: %v", err)
	}
	if cleanup != nil {
		t.Error("cleanup returned alongside a parked result, want nil")
	}
	if parked == nil || parked.Status != autoapply.StatusParked {
		t.Fatalf("parked = %+v, want StatusParked", parked)
	}
	if len(parked.Unmapped) != 1 || parked.Unmapped[0].ID != "resume" {
		t.Fatalf("parked.Unmapped = %+v, want the resume field named", parked.Unmapped)
	}
}

// An unconfigured renderer (no typst binary — the same nil-safe degrade every other
// optional dependency here follows) parks exactly like a real render failure, not a panic.
func TestAttachApprovedResume_UnconfiguredRendererParks(t *testing.T) {
	c := &Client{} // cvs and renderer both nil
	plan := Plan{Fields: []ResolvedField{{ID: "resume", Kind: "file"}}}

	_, parked, err := c.attachApprovedResume(context.Background(), autoapply.Claimed{TailoredCVID: uuid.New()}, &plan)
	if err != nil {
		t.Fatalf("attachApprovedResume returned an error, want a parked result: %v", err)
	}
	if parked == nil || parked.Status != autoapply.StatusParked {
		t.Fatalf("parked = %+v, want StatusParked", parked)
	}
}

// On success, the résumé field's Value becomes the rendered PDF's temp file path, and
// cleanup removes it.
func TestAttachApprovedResume_SuccessSetsTheFieldValueToATempFileAndCleansUp(t *testing.T) {
	c := &Client{
		cvs:      fakeCVReader{rec: cv.Record{}},
		renderer: fakeCVRenderer{pdf: []byte("%PDF-1.4 fake")},
	}
	plan := Plan{Fields: []ResolvedField{{ID: "resume", Kind: "file"}}}

	cleanup, parked, err := c.attachApprovedResume(context.Background(), autoapply.Claimed{TailoredCVID: uuid.New()}, &plan)
	if err != nil || parked != nil {
		t.Fatalf("attachApprovedResume = (parked=%v, err=%v), want success", parked, err)
	}
	if cleanup == nil {
		t.Fatal("cleanup = nil, want a cleanup func for the temp file")
	}
	path := plan.Fields[0].Value
	if path == "" {
		t.Fatal("plan.Fields[0].Value not set to the rendered temp file's path")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("temp file %q not created: %v", path, err)
	}

	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("temp file %q still exists after cleanup", path)
	}
}
