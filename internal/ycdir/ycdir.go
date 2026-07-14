// Package ycdir maps entries of the yc-oss company directory
// (yc-oss.github.io/api/companies/all.json) to the company-info fields freehire
// stores. It is a pure mapping — the fetch and the DB upsert live in cmd/import-yc.
// Only the fields we consume are declared; the rest of each entry is ignored.
package ycdir

import (
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/location"
	"github.com/strelov1/freehire/internal/normalize"
)

// Entry is the subset of a yc-oss directory entry we read.
type Entry struct {
	Name            string   `json:"name"`
	OneLiner        string   `json:"one_liner"`
	LongDescription string   `json:"long_description"`
	Batch           string   `json:"batch"`
	Status          string   `json:"status"`
	Stage           string   `json:"stage"`
	Industry        string   `json:"industry"`
	Industries      []string `json:"industries"`
	Subindustry     string   `json:"subindustry"`
	Tags            []string `json:"tags"`
	FormerNames     []string `json:"former_names"`
	TeamSize        int      `json:"team_size"`
	LaunchedAt      int64    `json:"launched_at"`
	Website         string   `json:"website"`
	AllLocations    string   `json:"all_locations"`
	TopCompany      bool     `json:"top_company"`
	IsHiring        bool     `json:"isHiring"`
	URL             string   `json:"url"`
	LogoURL         string   `json:"small_logo_thumb_url"`
}

// Record is the mapped company-info an entry yields. Zero/empty fields mean
// "unknown" — the upsert stores them as SQL NULL / omits them from the JSONB.
type Record struct {
	Slug          string
	Name          string
	Tagline       string
	Industries    []string
	Subindustry   string // clean YC subindustry-path leaf ("" = unknown)
	EmployeeCount int    // 0 = unknown
	YearFounded   int    // 0 = unknown
	HQCountry     string // "" = unknown
	Batch         string
	Status        string
	Stage         string
	Flags         []string       // curated flags: "top_company", "hiring" (sorted)
	FormerSlugs   []string       // normalized slugs of former names, for matching
	Info          map[string]any // company_info JSONB extras (empties omitted)
}

// Map converts a directory entry to a Record. It returns ok=false for an entry
// with no usable name (empty slug).
func Map(e Entry) (Record, bool) {
	name := strings.TrimSpace(e.Name)
	slug := normalize.Slug(name)
	if slug == "" {
		return Record{}, false
	}

	r := Record{
		Slug:          slug,
		Name:          name,
		Tagline:       strings.TrimSpace(e.OneLiner),
		Industries:    industries(e),
		Subindustry:   subindustryLeaf(e.Subindustry),
		EmployeeCount: e.TeamSize,
		YearFounded:   launchYear(e.LaunchedAt),
		HQCountry:     hqCountry(e.AllLocations),
		Batch:         strings.TrimSpace(e.Batch),
		Status:        strings.TrimSpace(e.Status),
		Stage:         strings.TrimSpace(e.Stage),
		Flags:         flags(e),
		FormerSlugs:   formerSlugs(e.FormerNames),
		Info:          info(e),
	}
	if r.EmployeeCount < 0 {
		r.EmployeeCount = 0
	}
	return r, true
}

// industries unions the entry's industry, industries[], the leaf of subindustry,
// and tags — de-duplicated (exact, order-preserving), dropping blanks.
func industries(e Entry) []string {
	var vals []string
	vals = append(vals, e.Industry)
	vals = append(vals, e.Industries...)
	vals = append(vals, subindustryLeaf(e.Subindustry))
	vals = append(vals, e.Tags...)

	var out []string
	seen := map[string]struct{}{}
	for _, v := range vals {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, dup := seen[v]; dup {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// subindustryLeaf returns the last "->"-separated segment of a subindustry path
// ("Industrials -> Manufacturing and Robotics" → "Manufacturing and Robotics"), or
// "" for a blank input.
func subindustryLeaf(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	parts := strings.Split(s, "->")
	return strings.TrimSpace(parts[len(parts)-1])
}

// flags returns the curated flag set for the entry, sorted for a stable facet value.
func flags(e Entry) []string {
	var out []string
	if e.IsHiring {
		out = append(out, "hiring")
	}
	if e.TopCompany {
		out = append(out, "top_company")
	}
	return out
}

// formerSlugs returns the normalized slugs of the entry's former names, dropping
// blanks; used to match a company we ingest under an old name.
func formerSlugs(names []string) []string {
	var out []string
	for _, n := range names {
		if s := normalize.Slug(n); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// launchYear derives a founding year from the yc-oss launched_at unix timestamp; a
// non-positive timestamp is unknown (0).
func launchYear(ts int64) int {
	if ts <= 0 {
		return 0
	}
	return time.Unix(ts, 0).UTC().Year()
}

// hqCountry resolves the first country code from a free-text location via the
// shared location dictionary, or "" when it resolves nothing.
func hqCountry(loc string) string {
	if strings.TrimSpace(loc) == "" {
		return ""
	}
	if c := location.Parse(loc).Countries; len(c) > 0 {
		return c[0]
	}
	return ""
}

// info assembles the low-coverage extras into the company_info JSONB, omitting
// empty values so absent facts stay absent.
func info(e Entry) map[string]any {
	m := map[string]any{}
	put := func(k, v string) {
		if s := strings.TrimSpace(v); s != "" {
			m[k] = s
		}
	}
	put("description", e.LongDescription)
	put("website", e.Website)
	put("stage", e.Stage)
	put("yc_url", e.URL)
	put("logo", e.LogoURL)
	if e.TopCompany {
		m["top_company"] = true
	}
	if e.IsHiring {
		m["is_hiring"] = true
	}
	return m
}
