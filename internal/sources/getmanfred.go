package sources

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/skilltag"
)

// getmanfred adapts getmanfred.com, a curated Spanish developer job marketplace. Its public,
// keyless API returns one global feed of offers (/api/v2/public/offers?lang=ES), each carrying
// its own employer, so one call assembles every Job — but the list omits the body text and the
// tech stack, so each ACTIVE offer is enriched from the per-offer detail endpoint
// (/api/v2/public/offers/{id}?lang=ES). Like the other marketplace adapters the company comes
// from the offer, not the boardless config entry (a validation placeholder). The list mixes
// ACTIVE and CLOSED offers, so only ACTIVE ones are ingested.
type getmanfred struct {
	http JSONGetter
}

const (
	getmanfredListURL   = "https://www.getmanfred.com/api/v2/public/offers?lang=ES"
	getmanfredDetailURL = "https://www.getmanfred.com/api/v2/public/offers/%d?lang=ES"
	getmanfredJobURL    = "https://www.getmanfred.com/ofertas-empleo/%d/%s"
)

// NewGetmanfred builds the getmanfred adapter over the given HTTP client.
func NewGetmanfred(c JSONGetter) Source { return getmanfred{http: c} }

func (getmanfred) Provider() string { return "getmanfred" }

// getmanfred is a marketplace with one global feed, so its config entries carry no board.
func (getmanfred) boardless() {}

// getmanfred aggregates postings from many companies, so it stays in the source facet.
func (getmanfred) aggregator() {}

// getmanfredOffer is one offer. The list carries identity + company + location + salary; the
// detail endpoint adds the Markdown body fields (Introduction…WhatOffering) and the Techs stack,
// both absent from the list. Status filters out CLOSED offers; RemotePercentage is the structured
// work-mode signal (100 remote / 1–99 hybrid / 0 onsite).
type getmanfredOffer struct {
	ID               int      `json:"id"`
	Slug             string   `json:"slug"`
	Position         string   `json:"position"`
	Status           string   `json:"status"`
	Locations        []string `json:"locations"`
	RemotePercentage int      `json:"remotePercentage"`
	UpdatedAt        string   `json:"updatedAt"`
	Company          struct {
		Name string `json:"name"`
	} `json:"company"`
	// Detail-only fields (the list omits them): the Markdown body split across named sections
	// and the required tech stack.
	Introduction     string           `json:"introduction"`
	WhatWillYouDo    string           `json:"whatWillYouDo"`
	Responsibilities []string         `json:"responsibilities"`
	WhatTheyAskFor   string           `json:"whatTheyAskFor"`
	HowWillYouDoIt   string           `json:"howWillYouDoIt"`
	WhatOffering     string           `json:"whatOffering"`
	Techs            []getmanfredTech `json:"techs"`
}

// getmanfredTech is one entry of an offer's techs stack; only Name is used, canonicalized
// through the skilltag dictionary.
type getmanfredTech struct {
	Name string `json:"name"`
}

func (g getmanfred) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	var offers []getmanfredOffer
	if err := g.http.GetJSON(ctx, getmanfredListURL, &offers); err != nil {
		return nil, fmt.Errorf("getmanfred: list: %w", err)
	}
	var jobs []Job
	for _, o := range offers {
		if !strings.EqualFold(o.Status, "ACTIVE") {
			continue // CLOSED offers are historical; the list is the whole catalogue
		}
		jobs = append(jobs, g.toJob(ctx, o))
	}
	return jobs, nil
}

// toJob maps an ACTIVE offer to a Job. The company is the offer's own employer; the work mode
// comes from remotePercentage. The per-offer detail is fetched once for the Markdown body and the
// tech stack, which the list omits — a failed detail falls back to an empty body rather than
// dropping the offer.
func (g getmanfred) toJob(ctx context.Context, o getmanfredOffer) Job {
	detail, _ := g.detail(ctx, o.ID)
	mode := getmanfredWorkMode(o.RemotePercentage)
	return Job{
		ExternalID:  strconv.Itoa(o.ID),
		URL:         fmt.Sprintf(getmanfredJobURL, o.ID, o.Slug),
		Title:       o.Position,
		Company:     o.Company.Name,
		Description: getmanfredDescription(detail),
		Location:    strings.Join(o.Locations, "; "),
		Remote:      mode == "remote",
		WorkMode:    mode,
		PostedAt:    parseRFC3339(o.UpdatedAt),
		Skills:      getmanfredSkills(detail.Techs),
	}
}

// detail fetches the per-offer detail, returning a zero offer and false on a failed request so
// the caller falls back to an empty body — an offer is never dropped over a missing detail.
func (g getmanfred) detail(ctx context.Context, id int) (getmanfredOffer, bool) {
	var detail getmanfredOffer
	if err := g.http.GetJSON(ctx, fmt.Sprintf(getmanfredDetailURL, id), &detail); err != nil {
		return getmanfredOffer{}, false
	}
	return detail, true
}

// getmanfredWorkMode maps the structured remotePercentage to freehire's work-mode vocabulary:
// 100 is fully remote, any partial value hybrid, 0 onsite.
func getmanfredWorkMode(pct int) string {
	switch {
	case pct >= 100:
		return "remote"
	case pct > 0:
		return "hybrid"
	default:
		return "onsite"
	}
}

// getmanfredDescription assembles the offer's Markdown body from its named sections (in reading
// order) and renders it to sanitized HTML, mirroring the other Markdown-bodied adapters
// (join). Empty sections are skipped; responsibilities is a bullet list.
func getmanfredDescription(o getmanfredOffer) string {
	var b strings.Builder
	appendSection := func(s string) {
		if strings.TrimSpace(s) == "" {
			return
		}
		b.WriteString(s)
		b.WriteString("\n\n")
	}
	appendSection(o.Introduction)
	appendSection(o.WhatWillYouDo)
	for _, r := range o.Responsibilities {
		if strings.TrimSpace(r) != "" {
			b.WriteString("- ")
			b.WriteString(r)
			b.WriteString("\n")
		}
	}
	if len(o.Responsibilities) > 0 {
		b.WriteString("\n")
	}
	appendSection(o.HowWillYouDoIt)
	appendSection(o.WhatTheyAskFor)
	appendSection(o.WhatOffering)
	return sanitizeHTML(markdownToHTML(strings.TrimSpace(b.String())))
}

// getmanfredSkills canonicalizes an offer's tech stack through the skilltag dictionary, keeping
// only resolved technologies. The names are joined into one blob so skilltag.Parse applies the
// same matching it uses on a description.
func getmanfredSkills(techs []getmanfredTech) []string {
	names := make([]string, 0, len(techs))
	for _, t := range techs {
		names = append(names, t.Name)
	}
	return skilltag.Parse(strings.Join(names, " "))
}
