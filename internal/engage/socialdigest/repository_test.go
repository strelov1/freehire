package socialdigest

import (
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
)

// Source feeds hand us titles with HTML entities in them. The row below is real: it
// was the third item of the 2026-09-02 list measured against production, and without
// this it would have gone out with a literal "&amp;" in the post.
func TestPostingFromRowUnescapesEntities(t *testing.T) {
	got := postingFromRow(db.TopPageViewedJobsForDayRow{
		ID:          42,
		PublicSlug:  "udhyam-learning-design-42",
		Title:       "Senior Specialist- Learning Design &amp; Capacity",
		Company:     "Udhyam Learning Foundation &amp; Co",
		CompanySlug: "udhyam-learning-foundation",
		Location:    "Karnataka, Bengaluru &amp; Mysuru",
		Remote:      true,
		PageUniques: 88,
	})

	if want := "Senior Specialist- Learning Design & Capacity"; got.Title != want {
		t.Errorf("title = %q, want %q", got.Title, want)
	}
	if want := "Udhyam Learning Foundation & Co"; got.Company != want {
		t.Errorf("company = %q, want %q", got.Company, want)
	}
	if want := "Karnataka, Bengaluru & Mysuru"; got.Location != want {
		t.Errorf("location = %q, want %q", got.Location, want)
	}

	// The slug is an identifier, not prose: it is what the URL is built from and must
	// survive byte for byte.
	if got.Slug != "udhyam-learning-design-42" {
		t.Errorf("slug = %q, want it untouched", got.Slug)
	}
	if got.CompanySlug != "udhyam-learning-foundation" {
		t.Errorf("company slug = %q, want it untouched", got.CompanySlug)
	}
	if got.JobID != 42 || got.PageUniques != 88 || !got.Remote {
		t.Errorf("scalar fields did not survive: %+v", got)
	}
}

// A title with no entities must come through byte for byte — unescaping is not licence
// to normalise anything else.
func TestPostingFromRowLeavesPlainTextAlone(t *testing.T) {
	got := postingFromRow(db.TopPageViewedJobsForDayRow{
		Title:   "Sr. Staff Software Development Engineer - Java/Golang",
		Company: "Zscaler",
	})
	if got.Title != "Sr. Staff Software Development Engineer - Java/Golang" {
		t.Errorf("title = %q", got.Title)
	}
	if got.Company != "Zscaler" {
		t.Errorf("company = %q", got.Company)
	}
}
