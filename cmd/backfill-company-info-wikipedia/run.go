package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/strelov1/freehire/internal/job/wikicompany"
)

// candidate is a company eligible for the backfill: no tagline yet, never checked.
type candidate struct {
	Slug string
	Name string
}

// store is the DB side of a run, narrowed to what the loop needs, so it can be
// exercised without a database.
type store interface {
	// ListMissing returns up to limit eligible companies with slug > afterSlug,
	// ordered by slug — the same keyset shape as ListCompaniesMissingWikipediaInfo.
	ListMissing(ctx context.Context, afterSlug string, limit int32) ([]candidate, error)
	// Fill applies a confident match: fills a blank tagline, merges companyInfo,
	// and marks the company checked.
	Fill(ctx context.Context, slug, tagline string, companyInfo json.RawMessage) error
	// MarkChecked records that the company was looked up and no confident match
	// was found, without touching tagline/company_info.
	MarkChecked(ctx context.Context, slug string) error
}

// matcher is the Wikidata lookup side, narrowed to one method so the loop can be
// tested without touching Wikidata. A nil *wikicompany.Match means no confident
// match, not an error.
type matcher interface {
	Lookup(ctx context.Context, name string) (*wikicompany.Match, error)
}

// result tallies one run's outcome.
type result struct {
	Matched  int
	Rejected int
}

// pageSize bounds how many candidates one ListMissing call requests at a time.
const pageSize = int32(200)

// runBackfill pages through eligible companies, looks each one up, and — only
// when apply is true — writes the outcome. It stops once maxPerRun companies
// have been processed (matched + rejected) or no eligible companies remain.
func runBackfill(ctx context.Context, st store, m matcher, apply bool, maxPerRun int64) (result, error) {
	var res result
	afterSlug := ""

	for int64(res.Matched+res.Rejected) < maxPerRun {
		remaining := maxPerRun - int64(res.Matched+res.Rejected)
		limit := pageSize
		if remaining < int64(pageSize) {
			limit = int32(remaining)
		}

		candidates, err := st.ListMissing(ctx, afterSlug, limit)
		if err != nil {
			return res, fmt.Errorf("list candidates: %w", err)
		}
		if len(candidates) == 0 {
			return res, nil
		}

		for _, c := range candidates {
			match, err := m.Lookup(ctx, c.Name)
			if err != nil {
				return res, fmt.Errorf("lookup %s: %w", c.Slug, err)
			}

			if match == nil {
				res.Rejected++
				if apply {
					if err := st.MarkChecked(ctx, c.Slug); err != nil {
						return res, fmt.Errorf("mark checked %s: %w", c.Slug, err)
					}
				}
			} else {
				res.Matched++
				if apply {
					info, err := json.Marshal(companyInfoSummary(match.Summary))
					if err != nil {
						return res, fmt.Errorf("encode company_info for %s: %w", c.Slug, err)
					}
					if err := st.Fill(ctx, c.Slug, match.Tagline, info); err != nil {
						return res, fmt.Errorf("fill %s: %w", c.Slug, err)
					}
				}
			}

			afterSlug = c.Slug
			if int64(res.Matched+res.Rejected) >= maxPerRun {
				return res, nil
			}
		}

		if err := ctx.Err(); err != nil {
			return res, err
		}
	}
	return res, nil
}

// companyInfoSummary builds the company_info JSONB fragment for a match's
// Wikipedia extract. An empty summary yields an empty object, so merging it
// changes nothing.
func companyInfoSummary(summary string) map[string]string {
	if summary == "" {
		return map[string]string{}
	}
	return map[string]string{"summary": summary}
}
