// Package jdresolve turns one of three inputs — an existing job's slug, an external URL,
// or pasted JD text — into a job usable by the tailor workspace, which is hard-keyed to a
// real jobs.id. A slug passes through unchanged. A URL a recognized ATS adapter resolves
// (see internal/linkimport) is ingested as a normal public job, identical to any other
// crawled posting. A URL only the generic JSON-LD fallback can read, or plain text, becomes
// a new private job (see internal/privatejob) — visible only to its creator, excluded from
// search and public listings, never enrichment-queued.
package jdresolve

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/job"
	"github.com/strelov1/freehire/internal/linkimport"
	"github.com/strelov1/freehire/internal/linksource"
	"github.com/strelov1/freehire/internal/privatejob"
)

// Sentinel errors the HTTP handler maps to status codes.
var (
	// ErrJobNotFound is a JobSlug request naming no job.
	ErrJobNotFound = errors.New("jdresolve: job not found")
	// ErrUnreadableURL is a URL no adapter — including the generic fallback — could read as
	// a single vacancy.
	ErrUnreadableURL = errors.New("jdresolve: could not read a vacancy from the url")
)

// Request is exactly one of JobSlug, URL, or Text — validated by the caller (the HTTP
// handler) before Resolve is called. Title/Company are optional hints used only with Text.
type Request struct {
	JobSlug string
	URL     string
	Text    string
	Title   string
	Company string
}

// Queries is the persistence Resolver reads directly for the job_slug passthrough.
type Queries interface {
	GetJobBySlug(ctx context.Context, slug string) (db.Job, error)
}

// Importer is the URL-resolution dependency — satisfied by *linkimport.Importer.
type Importer interface {
	Resolve(ctx context.Context, raw string, known linkimport.Board) (linksource.Resolved, bool, error)
	Write(ctx context.Context, r linksource.Resolved) (linkimport.Result, bool, error)
}

// PrivateWriter creates a private job — satisfied by *privatejob.Writer.
type PrivateWriter interface {
	Create(ctx context.Context, userID int64, source string, in privatejob.Input) (job.Job, error)
}

// Resolver ties the three input kinds together.
type Resolver struct {
	q        Queries
	importer Importer
	private  PrivateWriter
}

// New constructs a Resolver. private may be nil if the caller only ever resolves job
// slugs (as some tests do); it must be non-nil once URL or text requests are possible.
func New(q Queries, importer Importer, private PrivateWriter) *Resolver {
	return &Resolver{q: q, importer: importer, private: private}
}

// Resolve returns the public_slug of a job usable by the tailor workspace for req, scoped
// to userID (used only for the private-job path — attributing created_by and, for URL/text,
// deciding who owns the new row).
func (r *Resolver) Resolve(ctx context.Context, userID int64, req Request) (string, error) {
	switch {
	case req.JobSlug != "":
		return r.resolveSlug(ctx, req.JobSlug)
	case req.URL != "":
		return r.resolveURL(ctx, userID, req.URL)
	default:
		return r.resolveText(ctx, userID, req)
	}
}

func (r *Resolver) resolveSlug(ctx context.Context, slug string) (string, error) {
	j, err := r.q.GetJobBySlug(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrJobNotFound
	}
	if err != nil {
		return "", fmt.Errorf("jdresolve: get job by slug: %w", err)
	}
	return j.PublicSlug, nil
}

// resolveURL branches on which adapter matched — see internal/linkimport.Resolve — not on
// whether the fetch succeeded: a recognized ATS match is written through the normal public
// path (Write), while a generic-fallback match (an unverified third-party scrape) becomes a
// private job instead of joining the catalog.
func (r *Resolver) resolveURL(ctx context.Context, userID int64, url string) (string, error) {
	resolved, ok, err := r.importer.Resolve(ctx, url, linkimport.Board{})
	if err != nil {
		// A fetch/parse failure here is caller input (an unreachable page, a timeout, a
		// non-2xx response), not a server fault — the same distinction intake.go draws for
		// the sibling contribution flow. Surfacing it as a 500 would mislabel routine bad
		// URLs as internal errors and page on something that happens in ordinary use.
		return "", fmt.Errorf("%w: %v", ErrUnreadableURL, err)
	}
	if !ok {
		return "", ErrUnreadableURL
	}
	if resolved.Source != linksource.GenericSource {
		res, ok, err := r.importer.Write(ctx, resolved)
		if err != nil {
			return "", fmt.Errorf("jdresolve: write resolved job: %w", err)
		}
		// *linkimport.Importer.Write never returns ok=false with a nil error — every
		// failure path there carries an error. This guards the Importer interface's
		// documented contract, not a reachable case against the concrete
		// implementation; a future Importer must not use ok=false/err=nil to mean
		// anything other than "nothing readable", or this would mislabel it.
		if !ok {
			return "", ErrUnreadableURL
		}
		return res.PublicSlug, nil
	}
	j, err := r.private.Create(ctx, userID, privatejob.SourceWeblink, privatejob.Input{
		Title:       resolved.Job.Title,
		Company:     resolved.Job.Company,
		Location:    resolved.Job.Location,
		Description: resolved.Job.Description,
		URL:         resolved.Job.URL,
	})
	if err != nil {
		return "", fmt.Errorf("jdresolve: create private job from url: %w", err)
	}
	return j.Fields().PublicSlug, nil
}

func (r *Resolver) resolveText(ctx context.Context, userID int64, req Request) (string, error) {
	j, err := r.private.Create(ctx, userID, privatejob.SourcePasted, privatejob.Input{
		Title:       req.Title,
		Company:     req.Company,
		Description: req.Text,
	})
	if err != nil {
		return "", fmt.Errorf("jdresolve: create private job from text: %w", err)
	}
	return j.Fields().PublicSlug, nil
}
