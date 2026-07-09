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
	Tags            []string `json:"tags"`
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
	EmployeeCount int    // 0 = unknown
	YearFounded   int    // 0 = unknown
	HQCountry     string // "" = unknown
	Batch         string
	Status        string
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
		Industries:    industries(e.Industry, e.Tags),
		EmployeeCount: e.TeamSize,
		YearFounded:   launchYear(e.LaunchedAt),
		HQCountry:     hqCountry(e.AllLocations),
		Batch:         strings.TrimSpace(e.Batch),
		Status:        strings.TrimSpace(e.Status),
		Info:          info(e),
	}
	if r.EmployeeCount < 0 {
		r.EmployeeCount = 0
	}
	return r, true
}

// industries returns the industry followed by each tag, de-duplicated (exact,
// order-preserving), dropping blanks.
func industries(industry string, tags []string) []string {
	var out []string
	seen := map[string]struct{}{}
	for _, v := range append([]string{industry}, tags...) {
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
