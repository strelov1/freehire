package jobview

import (
	"strings"
	"time"
)

// This file is deliberately NOT in cmd/gen-contracts' tygo include list, unlike card.go: it is
// how a Card is built, not what a Card is. Emitted, its time.Time fields reach the SPA as `any`
// and fail the lint gate — a fair complaint about a type the browser has no business seeing.

// CardInput is the row a Card is projected from. Geography arrives from both sides because the
// wire value is a hybrid: the dictionary's reading wins where it resolved a concrete place, and
// the model's stands in only where it did not.
type CardInput struct {
	PublicSlug     string
	Title          string
	Company        string
	ClosedAt       *time.Time
	WorkMode       string
	Seniority      string
	EmploymentType string
	DictCountries  []string
	DictRegions    []string
	LLMCountries   []string
	LLMRegions     []string
	Skills         []string
	Collections    []string
	PostedAt       *time.Time
	CreatedAt      *time.Time
	Blurb          string
}

// NewCard projects a listing row into the card wire shape, applying the same geography rule the
// full projection applies.
func NewCard(in CardInput) Card {
	countries, regions := geoFacet(in.DictCountries, in.DictRegions, in.LLMCountries, in.LLMRegions)
	return Card{
		PublicSlug:     in.PublicSlug,
		Title:          in.Title,
		Company:        in.Company,
		ClosedAt:       rfc3339Ptr(in.ClosedAt),
		WorkMode:       in.WorkMode,
		Seniority:      in.Seniority,
		EmploymentType: in.EmploymentType,
		Countries:      countries,
		Regions:        regions,
		Skills:         normalizeSet(in.Skills),
		Collections:    in.Collections,
		PostedAt:       rfc3339Ptr(effectivePosted(in.PostedAt, in.CreatedAt, time.Now())),
		Blurb:          strings.TrimSpace(in.Blurb),
	}
}
