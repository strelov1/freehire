// Package privatejob creates jobs rows visible only to their creator: the
// jd-tailor-intake private-JD path (pasted text, or a URL only a generic scrape could
// read). Unlike every other job write path (see internal/moderation, internal/pipeline)
// it never enqueues enrichment — a private, single-tailoring-session row doesn't recoup
// that cost.
package privatejob

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/job"
	"github.com/strelov1/freehire/internal/jobderive"
	"github.com/strelov1/freehire/internal/sources"
)

// Source values for the two private-job origins the jd-tailor-intake endpoint writes.
const (
	SourcePasted  = "pasted"
	SourceWeblink = "weblink"
)

// fallbackTitle is used when the submitter gave no title and none could be inferred
// from the description — job.New requires a non-blank title.
const fallbackTitle = "Pasted job description"

// maxFallbackTitleLen bounds a title inferred from the description's first line.
const maxFallbackTitleLen = 120

// Input is the content a private job is created from. Company and Location are
// optional. Title is optional too: it falls back to the first line of Description,
// then to a fixed placeholder.
type Input struct {
	Title       string
	Company     string
	Location    string
	Description string
	URL         string // empty for the pasted-text origin
}

// Queries is the persistence Writer depends on — the slice of *db.Queries it actually
// calls, kept as an interface so tests can fake it without a database.
type Queries interface {
	InsertPrivateJob(ctx context.Context, arg db.InsertPrivateJobParams) (db.Job, error)
}

// Writer creates private jobs rows.
type Writer struct{ q Queries }

// NewWriter constructs a Writer backed by q (typically *db.Queries).
func NewWriter(q Queries) *Writer { return &Writer{q: q} }

// Create derives facets from in (see internal/jobderive) and persists a new private job
// owned by userID. source must be SourcePasted or SourceWeblink. The description is
// sanitized to the same HTML allowlist every other write path uses (see
// internal/sources.SanitizeHTML) before derivation and storage — a private submission
// can carry a scraped third-party page's markup, and the job-detail view renders
// jobs.description as trusted HTML.
func (w *Writer) Create(ctx context.Context, userID int64, source string, in Input) (job.Job, error) {
	description := sources.SanitizeHTML(in.Description)
	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = firstLine(description)
	}
	if title == "" {
		title = fallbackTitle
	}
	j, err := job.New(job.Draft{
		Input: jobderive.Input{
			Source:      source,
			ExternalID:  uuid.NewString(),
			Title:       title,
			Company:     strings.TrimSpace(in.Company),
			Location:    in.Location,
			Description: description,
		},
		URL: in.URL,
	})
	if err != nil {
		return job.Job{}, fmt.Errorf("privatejob: derive: %w", err)
	}
	row, err := w.q.InsertPrivateJob(ctx, j.Fields().InsertPrivateParams(userID))
	if err != nil {
		return job.Job{}, fmt.Errorf("privatejob: insert: %w", err)
	}
	persisted, _, err := job.FromRow(row)
	if err != nil {
		return job.Job{}, fmt.Errorf("privatejob: from row: %w", err)
	}
	return persisted, nil
}

// firstLine returns the first non-blank line of s, trimmed and truncated to
// maxFallbackTitleLen runes, or "" when s has no non-blank line.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		r := []rune(line)
		if len(r) > maxFallbackTitleLen {
			return string(r[:maxFallbackTitleLen])
		}
		return line
	}
	return ""
}
