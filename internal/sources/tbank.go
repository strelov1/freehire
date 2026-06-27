package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// tbank adapts T-Bank's public careers API (tbank.ru/pfpjobs/papi), a single-company source
// with no per-tenant board id (boardless). The list endpoint paginates by publisher offset
// and carries no body, so each vacancy's description comes from its own POST detail request,
// fanned out like the other detail-fetching adapters. The "publisher" source covers all roles.
type tbank struct {
	http JSONPoster
}

const (
	tbankListURL   = "https://www.tbank.ru/pfpjobs/papi/getVacancies"
	tbankDetailURL = "https://www.tbank.ru/pfpjobs/papi/getVacancyDescription"
	tbankSource    = "publisher"
	tbankPageLimit = 20
)

// NewTBank builds the T-Bank adapter over the given HTTP client.
func NewTBank(c JSONPoster) Source { return tbank{http: c} }

func (tbank) Provider() string { return "tbank" }

// tbank is single-company, so its config entries carry no board.
func (tbank) boardless() {}

// tbankListRequest is the getVacancies POST body. Offset paginates the publisher source;
// the server fixes the page size, so the response's nextPagination drives the loop.
type tbankListRequest struct {
	Pagination struct {
		Publisher struct {
			Offset int `json:"offset"`
		} `json:"publisher"`
	} `json:"pagination"`
	Limit   int            `json:"limit"`
	Filters map[string]any `json:"filters"`
}

// tbankDetailRequest is the getVacancyDescription POST body, keyed by a vacancy's urlSlug
// and its category.
type tbankDetailRequest struct {
	Source  string `json:"source"`
	URLSlug string `json:"urlSlug"`
	Options struct {
		Category string `json:"category"`
	} `json:"options"`
}

// tbankVacancy is one vacancy from the list response (no description here).
type tbankVacancyItem struct {
	Title    string   `json:"title"`
	Subtitle string   `json:"subtitle"`
	Category string   `json:"category"`
	URLSlug  string   `json:"urlSlug"`
	SeoSlug  string   `json:"seoSlug"`
	Tags     []string `json:"tags"`
}

func (b tbank) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	items, err := b.list(ctx)
	if err != nil {
		return nil, err
	}

	return fetchDetails(items, defaultDetailWorkers, func(it tbankVacancyItem) (Job, bool) {
		return b.detail(ctx, e, it)
	}), nil
}

// list pages through every vacancy, starting at offset 0 and following the response's
// nextPagination offset until isFinished.
func (b tbank) list(ctx context.Context) ([]tbankVacancyItem, error) {
	var items []tbankVacancyItem
	for offset := 0; ; {
		req := tbankListRequest{Limit: tbankPageLimit, Filters: map[string]any{}}
		req.Pagination.Publisher.Offset = offset

		var resp struct {
			Payload struct {
				NextPagination struct {
					Publisher struct {
						Offset     int  `json:"offset"`
						IsFinished bool `json:"isFinished"`
					} `json:"publisher"`
				} `json:"nextPagination"`
				Vacancies []tbankVacancyItem `json:"vacancies"`
			} `json:"payload"`
		}
		if err := b.http.PostJSON(ctx, tbankListURL, req, &resp); err != nil {
			return nil, fmt.Errorf("tbank: list offset %d: %w", offset, err)
		}
		items = append(items, resp.Payload.Vacancies...)
		next := resp.Payload.NextPagination.Publisher.Offset
		// Terminate on the server's isFinished flag, but also guard against a stale gateway
		// that keeps isFinished=false without advancing the offset — otherwise the loop would
		// re-issue the same request forever.
		if resp.Payload.NextPagination.Publisher.IsFinished || next <= offset {
			break
		}
		offset = next
	}
	return items, nil
}

// detail fetches one vacancy's description blocks and maps it to a Job, returning ok=false
// when the request fails so the caller skips just that vacancy.
func (b tbank) detail(ctx context.Context, e CompanyEntry, it tbankVacancyItem) (Job, bool) {
	req := tbankDetailRequest{Source: tbankSource, URLSlug: it.URLSlug}
	req.Options.Category = it.Category

	var resp struct {
		Payload struct {
			Description []tbankBlock `json:"description"`
		} `json:"payload"`
	}
	if err := b.http.PostJSON(ctx, tbankDetailURL, req, &resp); err != nil {
		return Job{}, false
	}

	return Job{
		ExternalID: it.URLSlug,
		// The careers SPA addresses a vacancy solely by its urlSlug (a UUID) in the final path
		// segment — the section/city/seoSlug segments are cosmetic (any non-empty value resolves
		// the same vacancy), but the route's five-segment shape must be intact or the server
		// 404s. "it" is a real, universally-rendering section; "all" is a neutral city; the real
		// seoSlug keeps the URL readable. (Verified against the live careers UI, 2026-06.)
		URL:         fmt.Sprintf("https://www.tbank.ru/career/it/vacancy/all/%s/%s/", it.SeoSlug, it.URLSlug),
		Title:       it.Title,
		Company:     e.Company,
		Location:    it.Subtitle,
		Description: sanitizeHTML(tbankAssembleBlocks(resp.Payload.Description)),
		Remote:      isRemote(strings.Join(it.Tags, " ")),
		PostedAt:    nil,
	}, true
}

// tbankBlock is one description block. Content is polymorphic: either an HTML string or an
// array of {title, description} items, so it is held raw and decoded by tbankAssembleBlocks.
type tbankBlock struct {
	Title   string          `json:"title"`
	Content json.RawMessage `json:"content"`
}

// tbankContentItem is one entry of a block whose content is an array.
type tbankContentItem struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// tbankAssembleBlocks walks the mixed description[] and concatenates it into one HTML string:
// each block's title becomes an <h3>; a string content is appended as-is; an array content's
// items become a <p> title (when present) followed by their description.
func tbankAssembleBlocks(blocks []tbankBlock) string {
	var b strings.Builder
	for _, blk := range blocks {
		if blk.Title != "" {
			b.WriteString("<h3>" + blk.Title + "</h3>")
		}
		if len(blk.Content) == 0 {
			continue
		}
		// Content is either a JSON string (raw HTML) or an array of {title, description}.
		var s string
		if json.Unmarshal(blk.Content, &s) == nil {
			b.WriteString(s)
			continue
		}
		var items []tbankContentItem
		if json.Unmarshal(blk.Content, &items) == nil {
			for _, it := range items {
				if it.Title != "" {
					b.WriteString("<h3>" + it.Title + "</h3>")
				}
				if it.Description != "" {
					b.WriteString("<p>" + it.Description + "</p>")
				}
			}
		}
	}
	return b.String()
}
