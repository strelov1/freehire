package sources

import (
	"context"
	"fmt"
	"log"
	"slices"
)

// list reads the whole catalogue: one slice per field, the IT fields first, and a field the
// search cannot hold in one query (more than jobsczReach rows) split again by region. A posting
// filed under several fields, or placed in several regions, is read more than once and kept
// once.
func (s jobscz) list(ctx context.Context) ([]jobsczItem, error) {
	var out []jobsczItem
	byID := map[string]bool{}
	add := func(items []jobsczItem) {
		for _, it := range items {
			if !byID[it.ID] {
				byID[it.ID] = true
				out = append(out, it)
			}
		}
	}
	fields := append([]string{}, jobsczITFields...)
	for _, f := range jobsczFields {
		if !slices.Contains(jobsczITFields, f) {
			fields = append(fields, f)
		}
	}
	for _, f := range fields {
		if err := s.walkField(ctx, f, add); err != nil {
			return nil, err
		}
	}
	// The catalogue is never empty; a listing that read nothing is a changed page or a refusal.
	if len(out) == 0 {
		return nil, fmt.Errorf("jobscz: the listing carries no postings")
	}
	return out, nil
}

// walkField reads one field, or its regions when the field is too large for one query.
func (s jobscz) walkField(ctx context.Context, field string, add func([]jobsczItem)) error {
	first, err := s.get(ctx, jobsczQuery("", field, 1))
	if err != nil {
		return fmt.Errorf("jobscz: field %s: %w", field, err)
	}
	total, ok := jobsczTotal(first)
	if !ok {
		// A field with no open posting states no count.
		return nil
	}
	if total <= jobsczReach {
		return s.walkPages(ctx, "", field, first, total, add)
	}
	for _, region := range jobsczRegions {
		if err := s.walkSlice(ctx, region, field, add); err != nil {
			return err
		}
	}
	return nil
}

// walkSlice reads one (region, field) slice from its first page.
func (s jobscz) walkSlice(ctx context.Context, region, field string, add func([]jobsczItem)) error {
	first, err := s.get(ctx, jobsczQuery(region, field, 1))
	if err != nil {
		return fmt.Errorf("jobscz: region %s field %s: %w", region, field, err)
	}
	total, ok := jobsczTotal(first)
	if !ok {
		return nil
	}
	if total > jobsczReach {
		// The newest jobsczReach rows are read; the rest of the slice is out of the search's reach.
		log.Printf("jobscz: region %s field %s holds %d postings, more than the search reaches (%d); the rest is not read", region, field, total, jobsczReach)
	}
	return s.walkPages(ctx, region, field, first, total, add)
}

// walkPages reads a slice's pages from the first, which is already in hand, to the last page the
// stated total (capped at the search's deepest page) names. A page with no card ends the slice
// early: the total can run a card or two above what the pages carry.
func (s jobscz) walkPages(ctx context.Context, region, field, first string, total int, add func([]jobsczItem)) error {
	pages := min((total+jobsczPageSize-1)/jobsczPageSize, jobsczMaxPages)
	page := first
	for n := 1; n <= pages; n++ {
		if n > 1 {
			var err error
			page, err = s.get(ctx, jobsczQuery(region, field, n))
			if err != nil {
				return fmt.Errorf("jobscz: region %q field %s page %d: %w", region, field, n, err)
			}
		}
		cards := jobsczCards(page)
		if len(cards) == 0 {
			return nil
		}
		add(cards)
	}
	return nil
}
