package adzunadesc

import (
	"context"
	"errors"
	"fmt"
	"html"

	xhtml "golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/sources"
)

// errNoJobPosting means the page carried no schema.org JobPosting block — the shape Adzuna's
// own "Access Denied" page (and any other unexpected response) takes.
var errNoJobPosting = errors.New("adzunadesc: no JobPosting block on the page")

// errEmptyDescription means the page's JobPosting carried no description text.
var errEmptyDescription = errors.New("adzunadesc: JobPosting block has an empty description")

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
		return "", fmt.Errorf("adzunadesc: fetch %s: %w", url, err)
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
