// Package moderation contains the moderator-authored job use cases: create a
// hand-curated vacancy and edit an existing one. It owns validation and the
// deterministic derivation (via jobderive); the Repository owns persistence (the
// transactional create + enrichment enqueue, and the source-scoped update). The HTTP
// handler stays thin: it translates the wire body into these inputs and renders the
// returned row.
package moderation

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/enrich"
	"github.com/strelov1/freehire/internal/job"
	"github.com/strelov1/freehire/internal/sources"
)

// Sentinel errors. ErrInvalid wraps every validation failure (the handler maps it to
// 400, surfacing the wrapped message); ErrJobNotFound is the missing-or-non-manual edit
// target (mapped to 404).
var (
	// ErrInvalid wraps validation failures. Its text is user-facing: the handler
	// surfaces the wrapped message in the 400 body, so it carries no package prefix.
	ErrInvalid     = errors.New("invalid request")
	ErrJobNotFound = errors.New("moderation: job not found")
)

// defaultSource is the origin recorded when the moderator does not name one. The URL is
// the external id, so re-creating the same URL under the same source is an idempotent
// upsert. Manual provenance is tracked by created_by, not by this value.
const defaultSource = "manual"

// CreateInput is the moderator-supplied content for a new vacancy. URL (the dedup key),
// Title, and Company are required; the rest is optional. Source is the posting's real
// origin (e.g. "workatastartup"); empty defaults to "manual".
//
// The structured facets are the values a submitter/moderator states explicitly. Regions,
// Cities, WorkMode, and Skills override the dictionary derivation (see jobderive), and
// the Salary* fields become an authoritative manual salary on the job. All are optional
// and sanitized before use (unknown WorkMode/Regions dropped, non-positive salary
// dropped), so a malformed value degrades to derivation rather than corrupting the job.
type CreateInput struct {
	URL         string
	Source      string
	Title       string
	Company     string
	Location    string
	Remote      bool
	Description string
	PostedAt    *time.Time

	Skills         []string
	Regions        []string
	Cities         []string
	WorkMode       string
	SalaryMin      *int
	SalaryMax      *int
	SalaryCurrency string
	SalaryPeriod   string
}

// Validate enforces the required fields and that the URL is an absolute http(s) link
// (the URL is the dedup key, so it must be well-formed and stable). It is exported so the
// submission queue validates contributed content against the same contract a moderator
// create uses — one source of truth for "what is a valid vacancy".
func (in CreateInput) Validate() error {
	if strings.TrimSpace(in.URL) == "" {
		return fmt.Errorf("%w: url is required", ErrInvalid)
	}
	if strings.TrimSpace(in.Title) == "" {
		return fmt.Errorf("%w: title is required", ErrInvalid)
	}
	if strings.TrimSpace(in.Company) == "" {
		return fmt.Errorf("%w: company is required", ErrInvalid)
	}
	u, err := url.Parse(in.URL)
	// err is checked first so u is non-nil before the scheme/host checks (short-circuit).
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("%w: url must be an absolute http(s) URL", ErrInvalid)
	}
	return nil
}

// UpdatePatch is a partial edit: a nil field is left unchanged. The source identity is
// not editable, so URL is absent here.
type UpdatePatch struct {
	Title       *string
	Company     *string
	Location    *string
	Remote      *bool
	Description *string
	PostedAt    *time.Time
}

// validate rejects an edit that would blank a required field: a supplied (non-nil)
// title or company must not be empty, mirroring Create's required-field guard. URL is
// the immutable identity and is not editable here, so it is not checked.
func (p UpdatePatch) validate() error {
	if p.Title != nil && strings.TrimSpace(*p.Title) == "" {
		return fmt.Errorf("%w: title must not be empty", ErrInvalid)
	}
	if p.Company != nil && strings.TrimSpace(*p.Company) == "" {
		return fmt.Errorf("%w: company must not be empty", ErrInvalid)
	}
	return nil
}

// Repository is the persistence contract, expressed in the Job aggregate rather than the
// generated db rows. Create runs the upsert and enrichment enqueue atomically; BySlug loads
// a moderator-authored job (ErrJobNotFound when missing or not moderator-created); Update
// writes the full resulting row, scoped to created_by IS NOT NULL. Create and Update take
// the derived job.Fields plus the acting moderator; the adapter builds the db params from
// them. All three return the persisted aggregate and its read-only Extras.
type Repository interface {
	Create(ctx context.Context, f job.Fields, actorID int64) (job.Job, job.Extras, error)
	BySlug(ctx context.Context, slug string) (job.Job, job.Extras, error)
	Update(ctx context.Context, slug string, f job.Fields, actorID int64) (job.Job, job.Extras, error)
}

// Service implements the moderator-authored job use cases.
type Service struct {
	repo Repository
}

// New creates a Service backed by the given Repository.
func New(repo Repository) *Service {
	return &Service{repo: repo}
}

// Create validates the input, derives the slugs and dictionary facets, and persists the
// new job (source 'manual', external_id = url). created_by and updated_by are both stamped
// with the acting moderator: created_by is written on insert, updated_by on a re-create of
// the same URL.
func (s *Service) Create(ctx context.Context, actorID int64, in CreateInput) (job.Job, job.Extras, error) {
	if err := in.Validate(); err != nil {
		return job.Job{}, job.Extras{}, err
	}
	source := strings.TrimSpace(in.Source)
	if source == "" {
		source = defaultSource
	}
	// Moderator descriptions are bulk-imported from scraped pages and rendered with
	// {@html}; sanitize to the same allowlist as every other source so no active
	// markup is ever persisted (stored XSS). Done once and reused for derivation.
	description := sources.SanitizeHTML(in.Description)
	f, err := derive(source, in.URL, in.Title, in.Company, in.Location, description, in.Remote, in.structured())
	if err != nil {
		return job.Job{}, job.Extras{}, err
	}
	// job.New's Draft derives the facets but leaves the identity/content fields the manual
	// write also persists unset; carry them onto the derived Fields for the adapter.
	f.URL = in.URL
	f.Remote = in.Remote
	f.PostedAt = in.PostedAt
	return s.repo.Create(ctx, f, actorID)
}

// Update loads the manual job, overlays the supplied (nil-means-unchanged) fields, and
// re-derives the deterministic facets from the merged content — so editing the location,
// description, or company keeps geography/skills/company-slug consistent. The source
// identity (url/external_id/public_slug) is never recomputed, keeping the public slug
// stable. A missing or non-moderator-created slug surfaces ErrJobNotFound.
func (s *Service) Update(ctx context.Context, actorID int64, slug string, p UpdatePatch) (job.Job, job.Extras, error) {
	if err := p.validate(); err != nil {
		return job.Job{}, job.Extras{}, err
	}
	cur, _, err := s.repo.BySlug(ctx, slug)
	if err != nil {
		return job.Job{}, job.Extras{}, err
	}
	curF := cur.Fields()

	title := stringOr(p.Title, curF.Title)
	company := stringOr(p.Company, curF.Company)
	location := stringOr(p.Location, curF.Location)
	// Sanitize a supplied description before persisting (stored XSS); re-sanitizing the
	// already-clean current value is idempotent.
	description := sources.SanitizeHTML(stringOr(p.Description, curF.Description))
	remote := curF.Remote
	if p.Remote != nil {
		remote = *p.Remote
	}
	postedAt := curF.PostedAt
	if p.PostedAt != nil {
		postedAt = p.PostedAt
	}

	// External id and source stay the create-time identity; only the dictionary facets
	// re-derive (the recomputed public slug is discarded — identity is immutable, so the
	// adapter writes the passed-in slug, not f.PublicSlug). The edit path re-derives every
	// facet from content and carries no explicit structured overrides.
	f, err := derive(curF.Source, curF.ExternalID, title, company, location, description, remote, structuredFacets{})
	if err != nil {
		return job.Job{}, job.Extras{}, err
	}
	f.Remote = remote
	f.PostedAt = postedAt
	return s.repo.Update(ctx, slug, f, actorID)
}

// structuredFacets are the sanitized explicit signals a create carries into derivation:
// each overrides the dictionary for its facet (work-mode/regions/cities/skills) and the
// salary becomes the job's authoritative manual salary. The zero value (an edit) carries
// no overrides, so derivation decides every facet.
type structuredFacets struct {
	WorkMode string
	Regions  []string
	Cities   []string
	Skills   []string
	Salary   *job.Salary
}

// structured sanitizes the create input's explicit facets: an unknown work-mode or region
// value is dropped (degrading to derivation), blank cities/skills are trimmed away, and a
// salary is present only when a positive bound is stated.
func (in CreateInput) structured() structuredFacets {
	return structuredFacets{
		WorkMode: validEnum(in.WorkMode, enrich.WorkModeValues),
		Regions:  filterVocab(in.Regions, enrich.RegionValues),
		Cities:   nonBlank(in.Cities),
		Skills:   nonBlank(in.Skills),
		Salary:   manualSalary(in.SalaryMin, in.SalaryMax, in.SalaryCurrency, in.SalaryPeriod),
	}
}

// derive builds the Job aggregate through the factory and returns its projected
// fields — the moderator write path's single door to the deterministic slugs and
// dictionary facets, shared with ingest and Telegram extraction. The structured facets
// override derivation where present; WorkMode also carries the moderator's structured
// remote flag when no explicit work mode was given (job.New cleans the location and
// derives the rest). It errors only when the identity or title is blank, which the
// caller's validation already precludes.
func derive(source, externalID, title, company, location, description string, remote bool, s structuredFacets) (job.Fields, error) {
	workMode := s.WorkMode
	if workMode == "" {
		workMode = remoteWorkMode(remote)
	}
	j, err := job.New(job.Draft{
		Source:       source,
		ExternalID:   externalID,
		Title:        title,
		Company:      company,
		Location:     location,
		Description:  description,
		WorkMode:     workMode,
		Regions:      s.Regions,
		Cities:       s.Cities,
		Skills:       s.Skills,
		ManualSalary: s.Salary,
	})
	if err != nil {
		return job.Fields{}, err
	}
	return j.Fields(), nil
}

// validEnum returns v when it is a member of the allowed vocabulary, else "" — an
// out-of-vocabulary value is dropped so it never overrides derivation with garbage.
func validEnum(v string, allowed []string) string {
	v = strings.TrimSpace(v)
	if slices.Contains(allowed, v) {
		return v
	}
	return ""
}

// filterVocab keeps only the members of in that are in the allowed vocabulary, trimming
// each — unknown values are dropped (the dictionary-facets contract: never emit unknowns).
func filterVocab(in, allowed []string) []string {
	var out []string
	for _, v := range in {
		if v = strings.TrimSpace(v); slices.Contains(allowed, v) {
			out = append(out, v)
		}
	}
	return out
}

// nonBlank trims and drops empty entries, leaving an open-vocabulary list (skills, cities).
func nonBlank(in []string) []string {
	var out []string
	for _, v := range in {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// manualSalary builds the authoritative manual salary, or nil when neither bound is a
// positive figure (the presence signal). The currency is upper-cased and the period is
// dropped unless it is a known salary period.
func manualSalary(min, max *int, currency, period string) *job.Salary {
	lo, hi := positiveOrNil(min), positiveOrNil(max)
	if lo == nil && hi == nil {
		return nil
	}
	return &job.Salary{
		Min:      lo,
		Max:      hi,
		Currency: strings.ToUpper(strings.TrimSpace(currency)),
		Period:   validEnum(period, enrich.SalaryPeriodValues),
	}
}

// positiveOrNil drops a nil or non-positive figure to nil (an unstated bound).
func positiveOrNil(n *int) *int {
	if n == nil || *n <= 0 {
		return nil
	}
	return n
}

// remoteWorkMode maps the moderator's structured remote flag onto a work-mode signal
// (the same role the ATS adapters' workplace-type enum plays): remote=true yields the
// "remote" facet; otherwise the value is left to the location parser's hint.
func remoteWorkMode(remote bool) string {
	if remote {
		return "remote"
	}
	return ""
}

// stringOr returns *p when set, else the fallback — the nil-means-unchanged merge.
func stringOr(p *string, fallback string) string {
	if p != nil {
		return *p
	}
	return fallback
}
