package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/experience"
	"github.com/strelov1/freehire/internal/resumeextract"
)

// fakeBank records what the upload path handed the experience bank. The mapping itself is
// tested in internal/experience; what matters here is whether the import ran at all, and
// that it never surfaces to the upload when it cannot.
type fakeBank struct {
	calls     int
	entries   []experience.ImportEntry
	sourceRef string
	err       error
}

func (f *fakeBank) Import(_ context.Context, _ int64, entries []experience.ImportEntry, sourceRef string) (experience.ImportResult, error) {
	f.calls++
	f.entries = entries
	f.sourceRef = sourceRef
	return experience.ImportResult{}, f.err
}

func bankFixture() resumeextract.Structured {
	return resumeextract.Structured{
		Experience: []resumeextract.Experience{
			{Title: "Senior Software Engineer", Company: "RingCentral", End: "Present",
				Highlights: []string{"Cut latency 20s to 1s"}},
		},
	}
}

func TestImportExperienceRunsOnASuccessfulExtract(t *testing.T) {
	bank := &fakeBank{}
	h := &resumeHandlers{bank: bank}

	h.importExperience(context.Background(), 7, bankFixture(), "2026-07-28T10:00:00Z")

	if bank.calls != 1 {
		t.Fatalf("bank.calls = %d, want 1", bank.calls)
	}
	if bank.sourceRef != "2026-07-28T10:00:00Z" {
		t.Errorf("sourceRef = %q, want the upload stamp", bank.sourceRef)
	}
	if len(bank.entries) != 1 {
		t.Errorf("entries = %d, want 1", len(bank.entries))
	}
}

// The bank is best-effort exactly like the rest of the derive path: an unconfigured bank
// or a failing import must never surface to the upload.
func TestImportExperienceIsBestEffort(t *testing.T) {
	h := &resumeHandlers{bank: nil}
	h.importExperience(context.Background(), 7, bankFixture(), "stamp") // must not panic

	failing := &fakeBank{err: errors.New("database down")}
	h = &resumeHandlers{bank: failing}
	h.importExperience(context.Background(), 7, bankFixture(), "stamp")
	if failing.calls != 1 {
		t.Errorf("bank.calls = %d, want the failure swallowed after one attempt", failing.calls)
	}
}

// An extraction that produced no work history has nothing to bank; the import must not
// run at all rather than call with an empty slice.
func TestImportExperienceSkipsAnEmptyStructure(t *testing.T) {
	bank := &fakeBank{}
	h := &resumeHandlers{bank: bank}

	h.importExperience(context.Background(), 7, resumeextract.Structured{FullName: "Ilya Strelov"}, "stamp")

	if bank.calls != 0 {
		t.Errorf("bank.calls = %d, want 0 — nothing was extracted to bank", bank.calls)
	}
}
