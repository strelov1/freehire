// Package serveprobe checks, on a random sample of the catalogue, that what a posting stores
// reaches what it serves: a field the row carries must appear in the wire shape a reader gets
// back. It reports gaps and changes nothing.
//
// The failure it targets is silent: a stored field the projection or index drops looks healthy
// from every side (the row stores it, the endpoint answers 200) and is simply omitted from the JSON.
//
// Two layers are checked, because they fail for different reasons:
//
//   - LayerProjection runs jobview.FromRow over the row. A gap here is a code bug: the
//     projection drops a column it was given.
//   - LayerIndex reads the posting's Meilisearch document, which search and the agent endpoints
//     serve. A gap here with none in the projection means the index is stale and a reindex is owed.
package serveprobe

import (
	"encoding/json"

	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

// Layer names where a served shape was read from; it becomes a metric label.
type Layer string

const (
	LayerProjection Layer = "projection"
	LayerIndex      Layer = "index"
)

// Layers is every layer in report order.
var Layers = []Layer{LayerProjection, LayerIndex}

// rule is one stored-to-served invariant: when stored says the row carries the field, served must
// find it in the wire shape. Presence, not equality: equality would fire on every legitimate
// reshaping (dictionary fold, normalised set) rather than on a loss.
type rule struct {
	field  string
	stored func(db.Job) bool
	served func(jobview.Job) bool
}

// rules is the invariant table. A column joins it once it is served; internal-only fields
// (fingerprint, cursor) have no served counterpart.
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
	{"engagement",
		func(r db.Job) bool { return len(r.Engagement) > 0 },
		func(v jobview.Job) bool { return len(v.Engagement) > 0 }},
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

// Missing is the Unindexed reason for a row no search rule withholds: its document should
// exist and does not — lost by the index, or still queued for the drain.
const Missing = "missing"

// UnindexedReasons lists every Unindexed key, for a stable report.
var UnindexedReasons = []string{
	string(search.WithheldNoCategory), string(search.WithheldNoDescription), string(search.WithheldUnseen), Missing,
}

// Report is one probe's result. Unindexed counts sampled rows with no Meilisearch document, by
// why: a search.WithholdReason when search keeps the row out on purpose, Missing when nothing
// does. Neither is a field gap, since there is no document to compare.
type Report struct {
	Sampled   int
	Unindexed map[string]int
	// Unreadable counts rows the projection itself refused; every rule is skipped for
	// them, since there is no served shape to check.
	Unreadable int
	Counts     map[Layer]map[string]Count
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
// absent from it is counted Unindexed under withheld's reason (Missing when there is
// none) and checked in the projection layer only.
func Check(rows []db.Job, docs map[int64]jobview.Job, withheld func(db.Job) search.WithholdReason) Report {
	rep := Report{Sampled: len(rows), Unindexed: map[string]int{}, Counts: map[Layer]map[string]Count{}}
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
			reason := string(withheld(row))
			if reason == "" {
				reason = Missing
			}
			rep.Unindexed[reason]++
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
