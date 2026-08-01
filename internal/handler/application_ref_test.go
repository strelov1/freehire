package handler

import (
	"errors"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestParseApplicationRefNamesAnApplication(t *testing.T) {
	// The form ListInteractions mints for an application whose posting cmd/prune
	// removed (internal/jobtracking/repository.go): "a" + applications.id.
	ref, err := parseApplicationRef("a4271")
	if err != nil {
		t.Fatalf("parseApplicationRef: %v", err)
	}
	if ref.AppID != 4271 {
		t.Errorf("AppID = %d, want 4271", ref.AppID)
	}
	if ref.Slug != "" {
		t.Errorf("Slug = %q, want empty — the row named an application, not a posting", ref.Slug)
	}
}

func TestParseApplicationRefNamesAPosting(t *testing.T) {
	// Every other row is named by its posting's public slug, which always ends in
	// an 8-character base32 shortcode (internal/normalize/job_slug.go).
	ref, err := parseApplicationRef("senior-go-engineer-stripe-mfzg42lt")
	if err != nil {
		t.Fatalf("parseApplicationRef: %v", err)
	}
	if ref.Slug != "senior-go-engineer-stripe-mfzg42lt" {
		t.Errorf("Slug = %q, want the slug back unchanged", ref.Slug)
	}
	if ref.AppID != 0 {
		t.Errorf("AppID = %d, want 0 — the row named a posting", ref.AppID)
	}
}

func TestParseApplicationRefRejectsAnEmptyID(t *testing.T) {
	_, err := parseApplicationRef("")
	var fe *fiber.Error
	if !errors.As(err, &fe) || fe.Code != fiber.StatusNotFound {
		t.Fatalf("err = %v, want a 404 — an id that names nothing is not a bad request", err)
	}
}

func TestParseApplicationRefTreatsAnOverlongApplicationIDAsASlug(t *testing.T) {
	// "a" followed by digits that do not fit an int64 is not an application id. It
	// falls through to the slug branch rather than erroring: the lookup there will
	// answer 404, which is the same answer, reached without a second error path.
	ref, err := parseApplicationRef("a99999999999999999999999")
	if err != nil {
		t.Fatalf("parseApplicationRef: %v", err)
	}
	if ref.AppID != 0 || ref.Slug != "a99999999999999999999999" {
		t.Errorf("got AppID=%d Slug=%q, want it read as a slug", ref.AppID, ref.Slug)
	}
}

// The forms are told apart by shape, because the listing mints them that way and this
// change does not get to alter that (see the design's non-goals). A posting slug is
// title-company-shortcode, and both leading segments drop out when they slugify to
// nothing — a title and company written in a script Slug() strips. What is left is the
// 8-character base32 shortcode alone, and base32's alphabet includes digits, so a slug
// of exactly "a" + 7 digits is expressible and would be read as an application id.
//
// Accepted rather than defended against: it needs a posting whose title AND company
// both slugify to empty, and then 7 hash characters drawn from the 6 digits base32
// allows — about one in 1.5 million of those. The cost of getting it wrong is one 404
// on one card. The cost of defending is a second database round trip on every write,
// or a change to the id format that belongs to applications-outlive-jobs.
//
// Pinned so the next reader meets the boundary as a fact rather than discovering it.
func TestParseApplicationRefCollidesWithADigitOnlyShortcodeSlug(t *testing.T) {
	ref, err := parseApplicationRef("a2345672")
	if err != nil {
		t.Fatalf("parseApplicationRef: %v", err)
	}
	if ref.AppID != 2345672 {
		t.Errorf("AppID = %d — documenting that this shape reads as an application id", ref.AppID)
	}
}
