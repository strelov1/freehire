package sources

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/enrich"
	"github.com/strelov1/freehire/internal/skilltag"
)

// getmatch adapts getmatch.ru, a curated Russian IT job marketplace. Its public, keyless feed
// (/api/offers) returns a paginated list where every offer carries its own employer, so one
// paged crawl assembles every Job — but the list description is a short summary, so each offer
// is enriched from the per-offer detail endpoint (/api/offers/{id}) for the full HTML body.
// Unlike a single-company adapter, the company comes from the offer (the marketplace lists many
// employers), so its boardless config entry's company is only a validation placeholder.
type getmatch struct {
	http JSONGetter
}

const (
	getmatchBaseURL   = "https://getmatch.ru"
	getmatchListURL   = "https://getmatch.ru/api/offers?offset=%d&limit=%d"
	getmatchDetailURL = "https://getmatch.ru/api/offers/%d"
	getmatchPageLimit = 100
	// getmatchMaxPages bounds pagination so a wrong or missing meta.total cannot loop.
	getmatchMaxPages = 200
	// getmatchDateLayout matches the zone-less published_at getmatch emits
	// ("2026-06-19T12:55:17.948391"). The .9 fractional form parses any sub-second width.
	getmatchDateLayout = "2006-01-02T15:04:05.999999999"
)

// NewGetmatch builds the getmatch adapter over the given HTTP client.
func NewGetmatch(c JSONGetter) Source { return getmatch{http: c} }

func (getmatch) Provider() string { return "getmatch" }

// getmatch is a marketplace with one global feed, so its config entries carry no board.
func (getmatch) boardless() {}

// getmatch aggregates postings from many companies, so it stays in the source facet.
func (getmatch) aggregator() {}

// getmatchListResponse is the /api/offers page: Meta.Total is the catalogue size used to stop
// pagination; Offers is the page.
type getmatchListResponse struct {
	Meta struct {
		Total int `json:"total"`
	} `json:"meta"`
	Offers []getmatchOffer `json:"offers"`
}

// getmatchOffer is one posting. Company nests the employer's own name (the marketplace lists
// many employers); OfferDescription is the list summary and Description the full detail body.
// Seniority, Specializations, SkillsObjects, and RequiredYearsOfExperience are structured
// facet fields the platform exposes only on the detail endpoint (the list page omits them).
type getmatchOffer struct {
	ID               int    `json:"id"`
	Position         string `json:"position"`
	URL              string `json:"url"`
	PublishedAt      string `json:"published_at"`
	OfferDescription string `json:"offer_description"`
	Description      string `json:"description"`
	Company          struct {
		Name string `json:"name"`
	} `json:"company"`
	LocationItems             []getmatchLocation `json:"location_items"`
	Seniority                 string             `json:"seniority"`
	Specializations           []string           `json:"specializations"`
	SkillsObjects             []getmatchSkill    `json:"skills_objects"`
	RequiredYearsOfExperience *int               `json:"required_years_of_experience"`
}

// getmatchSkill is one entry of an offer's skills_objects; only the display Name is used,
// canonicalized through the skilltag dictionary (see getmatchSkills).
type getmatchSkill struct {
	Name string `json:"name"`
}

// getmatchLocation is one of an offer's locations: a display Label and a Format that is either
// a work mode (remote/hybrid/office) or a relocation flag (relocation_company/_candidate).
type getmatchLocation struct {
	Label  string `json:"label"`
	Format string `json:"format"`
}

func (g getmatch) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	var jobs []Job
	for page := 0; page < getmatchMaxPages; page++ {
		offset := page * getmatchPageLimit
		var resp getmatchListResponse
		if err := g.http.GetJSON(ctx, fmt.Sprintf(getmatchListURL, offset, getmatchPageLimit), &resp); err != nil {
			if offset == 0 {
				return nil, fmt.Errorf("getmatch: list offset %d: %w", offset, err)
			}
			break // a later page failing ends enumeration with the jobs gathered so far
		}
		if len(resp.Offers) == 0 {
			break
		}
		for _, o := range resp.Offers {
			jobs = append(jobs, g.toJob(ctx, o))
		}
		if resp.Meta.Total > 0 && offset+getmatchPageLimit >= resp.Meta.Total {
			break
		}
	}
	return jobs, nil
}

// toJob maps an offer to a Job. The company is the offer's own employer, not the configured
// entry; the work mode is structured only when the offer's locations agree on one (see
// getmatchWorkMode). The per-offer detail is fetched once and supplies both the full HTML body
// and the structured facets (grade/specialization/skills/experience), which the list page omits.
func (g getmatch) toJob(ctx context.Context, o getmatchOffer) Job {
	mode := getmatchWorkMode(o.LocationItems)
	detail, _ := g.detail(ctx, o.ID)
	// The full detail body wins over the list summary, falling back when the detail is
	// empty (event cards) or its request failed (a zero detail) — so an offer is never
	// dropped over a missing description.
	desc := o.OfferDescription
	if strings.TrimSpace(detail.Description) != "" {
		desc = detail.Description
	}
	return Job{
		ExternalID:         strconv.Itoa(o.ID),
		URL:                getmatchBaseURL + o.URL,
		Title:              o.Position,
		Company:            o.Company.Name,
		Description:        sanitizeHTML(desc),
		Location:           distinctJoin(o.LocationItems, ", ", func(l getmatchLocation) string { return l.Label }),
		Remote:             mode == "remote",
		WorkMode:           mode,
		PostedAt:           parseLayout(getmatchDateLayout, o.PublishedAt),
		Seniority:          getmatchSeniority(detail.Seniority),
		Category:           getmatchCategory(detail.Specializations),
		Skills:             getmatchSkills(detail.SkillsObjects),
		ExperienceYearsMin: detail.RequiredYearsOfExperience,
	}
}

// detail fetches the per-offer detail endpoint, returning a zero offer and false on a failed
// request so the caller falls back to the list summary and leaves the structured facets empty —
// an offer is never dropped over a missing detail.
func (g getmatch) detail(ctx context.Context, id int) (getmatchOffer, bool) {
	var detail getmatchOffer
	if err := g.http.GetJSON(ctx, fmt.Sprintf(getmatchDetailURL, id), &detail); err != nil {
		return getmatchOffer{}, false
	}
	return detail, true
}

// getmatchSeniority maps a getmatch grade to freehire's seniority vocabulary. getmatch's grades
// (junior/middle/senior/lead/c_level) are a subset of enrich.SeniorityValues with identical
// spelling, so vocabulary membership IS the map: a recognized grade passes through, anything
// else is dropped (never guessed).
func getmatchSeniority(grade string) string {
	g := strings.ToLower(strings.TrimSpace(grade))
	if slices.Contains(enrich.SeniorityValues, g) {
		return g
	}
	return ""
}

// getmatchSpecializationCategory maps getmatch specialization codes to freehire's category
// vocabulary. The keys are the real codes getmatch emits (sampled from the live API); only
// high-confidence, unambiguous codes are mapped. Codes with no clean single equivalent —
// analyst roles (business_analyst, system_analyst, product_analyst), architect, generic C/C++
// (c_cpp/c_language), dba, recruitment/writer roles — are intentionally absent and resolve to no
// category, so the title dictionary decides rather than the map guessing.
var getmatchSpecializationCategory = map[string]string{
	"python":                              "backend",
	"golang":                              "backend",
	"java_scala":                          "backend",
	"c_sharp":                             "backend",
	"php":                                 "backend",
	"ruby":                                "backend",
	"js_backend":                          "backend",
	"js_frontend":                         "frontend",
	"fullstack":                           "fullstack",
	"android":                             "mobile",
	"ios":                                 "mobile",
	"cross_platform_flutter_react_native": "mobile",
	"dev_ops":                             "devops",
	"system_administrator":                "devops",
	"sre":                                 "sre",
	"data_engineering":                    "data_engineering",
	"dwh_data_warehouse":                  "data_engineering",
	"data_science":                        "data_science",
	"data_analyst_bi":                     "data_analytics",
	"qa_auto":                             "qa",
	"qa_manual":                           "qa",
	"information_security":                "security",
	"embedded":                            "embedded",
	"blockchain_crypto":                   "blockchain",
	"product_design":                      "design",
	"product_management":                  "product",
	"project_management":                  "project_management",
	"engineering_management":              "management",
	"sales":                               "sales",
	"marketing":                           "marketing",
	"support_engineer":                    "support",
}

// getmatchCategory resolves an offer's specializations to a single category, mirroring
// getmatchWorkMode: each mapped code contributes its category, and the result is that category
// only when all mapped codes agree; a conflict (more than one distinct category) or no mapped
// code yields "" so the title dictionary decides. Unmappable codes are ignored, not treated as
// a conflict.
func getmatchCategory(specs []string) string {
	var category string
	for _, code := range specs {
		c, ok := getmatchSpecializationCategory[strings.ToLower(strings.TrimSpace(code))]
		if !ok {
			continue
		}
		if category == "" {
			category = c
		} else if category != c {
			return ""
		}
	}
	return category
}

// getmatchSkills canonicalizes an offer's skills_objects through the skilltag dictionary,
// keeping only resolved technologies (so marketplace noise like "Kiss" or "152-ФЗ" is dropped).
// The names are joined into one text blob so skilltag.Parse applies the same matching it uses on
// a description.
func getmatchSkills(objs []getmatchSkill) []string {
	names := make([]string, 0, len(objs))
	for _, o := range objs {
		names = append(names, o.Name)
	}
	return skilltag.Parse(strings.Join(names, " "))
}

// getmatchWorkMode derives the structured work mode from the offer's location formats, mapping
// remote/hybrid/office via the shared workplaceTypeMode (which yields "" for the relocation
// flags, so they are ignored). It returns a mode only when the offer's locations resolve to a
// single distinct one; a mix (or none) yields "" so the pipeline's location parser decides.
func getmatchWorkMode(items []getmatchLocation) string {
	var mode string
	for _, it := range items {
		m := workplaceTypeMode(it.Format)
		if m == "" {
			continue
		}
		if mode == "" {
			mode = m
		} else if mode != m {
			return ""
		}
	}
	return mode
}
