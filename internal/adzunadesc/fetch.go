package adzunadesc

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"

	xhtml "golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/sources"
)

// errNoJobPosting means the page carried no schema.org JobPosting block — the shape Adzuna's
// own "Access Denied" page takes, but also (confirmed live 2026-08-09, majority failure mode
// of the first production runs) a perfectly normal, non-blocked posting page whose ld+json
// simply never carried the JobPosting block in the first place. Terminal either way: the same
// URL answers the same way on a retry.
var errNoJobPosting = errors.New("adzunadesc: no JobPosting block on the page")

// errEmptyDescription means the page's JobPosting carried no description text. Terminal, same
// reasoning as errNoJobPosting.
var errEmptyDescription = errors.New("adzunadesc: JobPosting block has an empty description")

// errNotFound means Adzuna no longer serves this posting at all. Terminal.
var errNotFound = errors.New("adzunadesc: posting not found on Adzuna's site")

// statusCoder is any error carrying an HTTP status — sources.StatusError satisfies it.
// Declared here rather than imported to mirror internal/applyform's own asGone, which notes
// why: the dependency runs the other way in that package. Here it is only mirrored for
// consistency, since internal/sources does not import this package.
type statusCoder interface{ StatusCode() int }

// asNotFound converts a 404 response into errNotFound and leaves everything else (429, 5xx,
// network errors) alone — those are the platform declining to answer right now, not stating
// the posting is gone, so they stay retryable. Mirrors applyform's asGone.
func asNotFound(err error) error {
	var sc statusCoder
	if errors.As(err, &sc) && sc.StatusCode() == http.StatusNotFound {
		return fmt.Errorf("%w: %v", errNotFound, err)
	}
	return err
}

// terminal reports whether err will answer the same way on a retry — Adzuna's own site
// stating, one way or another, that this posting will never yield a description. The runner
// dead-letters these on the first attempt rather than spending two more requests to learn
// the same thing twice.
func terminal(err error) bool {
	return errors.Is(err, errNoJobPosting) || errors.Is(err, errEmptyDescription) || errors.Is(err, errNotFound)
}

// Transport is the narrow HTTP role this package needs: a rendered page, the same shape
// applyform's Lever fetcher and internal/linksource's generic resolver already use.
type Transport interface {
	GetHTML(ctx context.Context, url string) (*xhtml.Node, error)
}

// adzunaJobPosting selects only the field this package reads. Declaring nothing whose
// schema.org shape varies keeps json.Unmarshal from failing on a shape mismatch and
// dropping an otherwise-readable page — LDJobPosting requires the whole decode to succeed.
type adzunaJobPosting struct {
	Description string `json:"description"`
}

// FetchDescription reads url's page and returns its JobPosting description, sanitized. It
// returns an error for anything that is not a full description: a transport failure, a page
// with no JobPosting block (Adzuna's ad-network "Access Denied" page among them — see
// Eligible for why that shape is filtered out before a job ever reaches this call), or a
// block whose description is empty.
func FetchDescription(ctx context.Context, t Transport, url string) (string, error) {
	root, err := t.GetHTML(ctx, url)
	if err != nil {
		return "", fmt.Errorf("adzunadesc: fetch %s: %w", url, asNotFound(err))
	}

	var p adzunaJobPosting
	if !sources.LDJobPosting(root, &p) {
		return "", fmt.Errorf("%w: %s", errNoJobPosting, url)
	}
	if p.Description == "" {
		return "", fmt.Errorf("%w: %s", errEmptyDescription, url)
	}

	// Unescape BEFORE sanitizing, matching internal/linksource's generic resolver: some
	// ATS pages entity-encode the HTML inside their ld+json string, and decoding first is
	// what lets the sanitizer see real tags to keep or strip rather than literal text.
	return sources.SanitizeHTML(html.UnescapeString(p.Description)), nil
}
