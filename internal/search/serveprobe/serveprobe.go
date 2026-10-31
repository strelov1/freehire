// Package serveprobe checks, on a random sample of the catalogue, that what a posting
// STORES reaches what it SERVES: a field the row carries must appear in the wire shape
// a reader gets back. It reports gaps; it changes nothing.
//
// It exists because the opposite failure is silent. An Ashby posting stating CA$180K-250K
// served no salary at all, because the structured range sat in salary_*_source and only
// the enrichment write copied it into the payload — 872,117 of 891,859 open postings
// with a source salary served nothing, and it surfaced as a user's question, not as a
// signal. requirements_derived had the same shape before it. Each such field looks
// healthy from every side: the adapter reads it, the row stores it, the endpoint answers
// 200, and the field is simply absent, since an empty value is omitted from the JSON.
//
// Two layers are checked, because they fail for different reasons and are fixed by
// different people:
//
//   - LayerProjection runs jobview.FromRow over the row itself. A gap here is a code
//     bug: the projection drops a column it was given.
//   - LayerIndex reads the posting's Meilisearch document, which is what search and the
//     agent endpoints serve. A gap here with none in the projection means the index is
//     stale — a reindex is owed, as after any change to what a document carries.
package serveprobe

import (
	"encoding/json"

	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
)

// Layer names where a served shape was read from; it becomes a metric label.
type Layer string

const (
	LayerProjection Layer = "projection"
	LayerIndex      Layer = "index"
)

// Layers is every layer in report order.
var Layers = []Layer{LayerProjection, LayerIndex}

// rule is one stored-to-served invariant: when stored says the row carries the field,
// served must find it in the wire shape. Presence, not equality — a value the
// projection legitimately reshapes (a dictionary fold, a normalised set) still counts,
// and an equality rule would fire on every such reshaping rather than on a loss.
type rule struct {
	field  string
	stored func(db.Job) bool
	served func(jobview.Job) bool
}

// rules is the invariant table. A column joins it once it is served: a field stored for
// internal use only (a fingerprint, a cursor) has no served counterpart to check.
var rules = []rule{
	{"salary",
		func(r db.Job) bool {
			return r.SalaryMinSource.Valid || r.SalaryMaxSource.Valid || r.SalaryMinManual.Valid || r.SalaryMaxManual.Valid
		},
		func(v jobview.Job) bool { return v.Enrichment.SalaryMin != nil || v.Enrichment.SalaryMax != nil }},
	{"countries",
		func(r db.Job) bool { return len(r.Countries) > 0 },
		func(v jobview.Job) bool { return len(v.Countries) > 0 }},
	{"regions",
		func(r db.Job) bool { return len(r.Regions) > 0 },
		func(v jobview.Job) bool { return len(v.Regions) > 0 }},
	{"cities",
		func(r db.Job) bool { return len(r.Cities) > 0 },
		func(v jobview.Job) bool { return len(v.Cities) > 0 }},
	{"work_mode",
		func(r db.Job) bool { return r.WorkMode != "" },
		func(v jobview.Job) bool { return v.WorkMode != "" }},
	{"skills",
		func(r db.Job) bool { return len(r.Skills) > 0 },
		func(v jobview.Job) bool { return len(v.Skills) > 0 }},
	{"seniority",
		func(r db.Job) bool { return r.Seniority != "" },
		func(v jobview.Job) bool { return v.Enrichment.Seniority != "" }},
	{"category",
		func(r db.Job) bool { return r.Category != "" },
		func(v jobview.Job) bool { return v.Enrichment.Category != "" }},
	{"employment_type",
		func(r db.Job) bool { return r.EmploymentType != "" },
		func(v jobview.Job) bool { return v.Enrichment.EmploymentType != "" }},
	{"experience_years_min",
		func(r db.Job) bool { return r.ExperienceYearsMin.Valid },
		func(v jobview.Job) bool { return v.Enrichment.ExperienceYearsMin != nil }},
	{"requirements",
		func(r db.Job) bool { return hasDerivedRequirements(r.RequirementsDerived) },
		func(v jobview.Job) bool { return len(v.Enrichment.Requirements) > 0 }},
}

// hasDerivedRequirements reads the stored list the way the projection does: an
// unreadable value is no list, so it is not counted as stored.
func hasDerivedRequirements(raw []byte) bool {
	var list []json.RawMessage
	return json.Unmarshal(raw, &list) == nil && len(list) > 0
}

// Count is one (layer, field) result: how many sampled rows stored the field, and how
// many of those did not serve it.
type Count struct {
	Stored int
	Gaps   int
}

// Report is one probe's result. Unindexed counts sampled rows with no Meilisearch
// document; it is reported, not counted as a gap, because the rebuild deliberately
// skips rows SQL cannot rule out (no category, no visible description) and a fresh
// posting waits for the drain.
type Report struct {
	Sampled   int
	Unindexed int
	Counts    map[Layer]map[string]Count
	// Unreadable counts rows the projection itself refused; every rule is skipped for
	// them, since there is no served shape to check.
	Unreadable int
}

// Fields lists every checked field in table order, for a stable report.
func Fields() []string {
	out := make([]string, len(rules))
	for i, r := range rules {
		out[i] = r.field
	}
	return out
}

// Check runs every rule over rows. docs holds the indexed shape by job id; a row
// absent from it is counted Unindexed and checked in the projection layer only.
func Check(rows []db.Job, docs map[int64]jobview.Job) Report {
	rep := Report{Sampled: len(rows), Counts: map[Layer]map[string]Count{}}
	for _, l := range Layers {
		rep.Counts[l] = map[string]Count{}
	}
	for _, row := range rows {
		projected, err := jobview.FromRow(row)
		if err != nil {
			rep.Unreadable++
			continue
		}
		indexed, ok := docs[row.ID]
		if !ok {
			rep.Unindexed++
		}
		for _, r := range rules {
			if !r.stored(row) {
				continue
			}
			tally(rep.Counts[LayerProjection], r.field, r.served(projected))
			if ok {
				tally(rep.Counts[LayerIndex], r.field, r.served(indexed))
			}
		}
	}
	return rep
}

func tally(m map[string]Count, field string, served bool) {
	c := m[field]
	c.Stored++
	if !served {
		c.Gaps++
	}
	m[field] = c
}
