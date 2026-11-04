package savedsearch

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"
)

// EncodeQuery writes the values the way the website's serializer does (facetModel's
// filtersToParams): one param per facet, its values comma-joined. The matcher reads
// either form; this one is what the website's own dedupe compares against, so a search
// saved by a server-side caller is recognised there as the same set.
func EncodeQuery(vals url.Values) string {
	flat := url.Values{}
	for k, vs := range vals {
		flat.Set(k, strings.Join(vs, ","))
	}
	return flat.Encode()
}

// ParseQuery reads a stored query into one value per list element, whichever list
// form it was written in.
func ParseQuery(query string) url.Values {
	vals, _ := url.ParseQuery(query)
	out := url.Values{}
	for k, vs := range vals {
		for _, v := range vs {
			for _, p := range strings.Split(v, ",") {
				if p = strings.TrimSpace(p); p != "" && !slices.Contains(out[k], p) {
					out[k] = append(out[k], p)
				}
			}
		}
	}
	return out
}

// SameSelection reports whether two stored queries select the same postings: the same
// params with the same value sets, in any order and either list form. The ordering
// param is ignored, as the website's canonicalQuery ignores it.
func SameSelection(a, b string) bool {
	return maps.Equal(selectionOf(a), selectionOf(b))
}

func selectionOf(query string) map[string]string {
	out := map[string]string{}
	for k, vs := range ParseQuery(query) {
		if k == "sort" {
			continue
		}
		out[k] = strings.Join(slices.Sorted(slices.Values(vs)), ",")
	}
	return out
}

// FilterSummary renders a stored query for a person: `"rust" · salary_min: 80000 ·
// skills: go, rust`. Params are sorted so the same search always reads the same.
func FilterSummary(query string) string {
	sel := selectionOf(query)
	var parts []string
	if q, ok := sel["q"]; ok {
		parts = append(parts, fmt.Sprintf("%q", q))
		delete(sel, "q")
	}
	for _, k := range slices.Sorted(maps.Keys(sel)) {
		parts = append(parts, k+": "+strings.ReplaceAll(sel[k], ",", ", "))
	}
	return strings.Join(parts, " · ")
}

// DefaultName names a saved search the person did not name, from its summary — the
// website names one from its filters the same way (saveSearchAlert's alertName).
func DefaultName(query string) string {
	name := FilterSummary(query)
	if utf8.RuneCountInString(name) > maxNameLen {
		name = string([]rune(name)[:maxNameLen-1]) + "…"
	}
	if name == "" {
		return "Job alert"
	}
	return name
}

// EnsureForQuery reuses the user's saved search selecting the same postings, or creates
// one — the website's ensureSaved, server-side. An empty name is auto-generated and, if it
// collides, retried with a numbered suffix; a name the caller chose is theirs to change.
func (s *Service) EnsureForQuery(ctx context.Context, userID int64, name, query string) (SavedSearch, error) {
	existing, err := s.repo.List(ctx, userID)
	if err != nil {
		return SavedSearch{}, err
	}
	for _, e := range existing {
		if SameSelection(e.Query, query) {
			return e, nil
		}
	}
	chosen := strings.TrimSpace(name) != ""
	if !chosen {
		name = DefaultName(query)
	}
	created, err := s.Create(ctx, userID, name, query, false)
	if errors.Is(err, ErrDuplicateName) && !chosen {
		suffix := fmt.Sprintf(" (%d)", len(existing)+1)
		base := []rune(name)
		if len(base) > maxNameLen-len(suffix) {
			base = base[:maxNameLen-len(suffix)]
		}
		created, err = s.Create(ctx, userID, string(base)+suffix, query, false)
	}
	return created, err
}
