package handler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/candidate/coverletter"
	"github.com/strelov1/freehire/internal/platform/modroot"
)

func TestToolBandReadsShortAndDefaultsOtherwise(t *testing.T) {
	if got := toolBand("short"); got != coverletter.BandShort {
		t.Errorf("band = %q, want short", got)
	}
	for _, s := range []string{"", "standard", "SHORT", "brief"} {
		if got := toolBand(s); got != coverletter.BandStandard {
			t.Errorf("toolBand(%q) = %q, want standard", s, got)
		}
	}
}

// Both entry points must build a letter from the same material. They do so by construction:
// neither assembles the chain's input itself, both go through letterDrafter, and the
// provenance gate lives inside Draft below it. This test pins that construction - if either
// path grows its own call to the chain, the two can drift and only one would fail closed.
func TestBothEntryPointsGoThroughTheSharedDrafter(t *testing.T) {
	for _, path := range []string{
		"internal/api/handler/cv_cover_letter.go",
		"internal/api/handler/assistant_cover_letter_tool.go",
	} {
		src := readSourceFile(t, path)
		if containsAny(src, "Publishable(") {
			t.Errorf("%s filters atoms itself; the gate belongs to Draft alone, so that a caller "+
				"cannot apply a weaker one", path)
		}
		if containsAny(src, "coverletter.Input{", "coverletter.Gather(") {
			t.Errorf("%s assembles the chain's input itself instead of going through letterDrafter", path)
		}
		if !containsAny(src, "drafter.draft(") {
			t.Errorf("%s does not go through the shared drafter", path)
		}
	}
}

// Both paths charge through the same helper and on the same attempt stamp. The tool once used
// a constant reference, which made every redraft from chat free forever while the endpoint
// charged for each — the two diverging on the one axis they were built to share.
func TestBothEntryPointsChargeThroughTheSameHelper(t *testing.T) {
	for _, path := range []string{
		"internal/api/handler/cv_cover_letter.go",
		"internal/api/handler/assistant_cover_letter_tool.go",
	} {
		src := readSourceFile(t, path)
		if !containsAny(src, "chargeLetter(") {
			t.Errorf("%s does not charge through the shared helper", path)
		}
		if !containsAny(src, "letterAttempt(") {
			t.Errorf("%s does not derive its reference from the shared attempt stamp", path)
		}
		if containsAny(src, `coverLetterRef(`) {
			t.Errorf("%s builds its own ledger reference; that belongs to chargeLetter alone", path)
		}
	}
}

// Both releases must be detached from the request. A client that disconnects mid-draft cancels
// the context, the chain fails with it, and a release on that same context could not open its
// transaction — leaving the candidate charged for a letter they never got, in exactly the case
// the release exists for.
func TestNeitherEntryPointReleasesOnTheRequestContext(t *testing.T) {
	for _, path := range []string{
		"internal/api/handler/cv_cover_letter.go",
		"internal/api/handler/assistant_cover_letter_tool.go",
	} {
		src := readSourceFile(t, path)
		if containsAny(src, "plans.Release(") {
			t.Errorf("%s releases inline; use releaseLetterCharge, which detaches from the request", path)
		}
		if !containsAny(src, "releaseLetterCharge(") {
			t.Errorf("%s never releases a charge", path)
		}
	}
}

// readSourceFile reads one of this repo own files, resolved from the module root so the test
// does not depend on the working directory a runner happens to pick.
func readSourceFile(t *testing.T, rel string) string {
	t.Helper()
	root, err := modroot.Find()
	if err != nil {
		t.Fatalf("locating the module root: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("reading %s: %v", rel, err)
	}
	return string(raw)
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
