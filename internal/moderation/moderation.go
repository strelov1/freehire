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
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobderive"
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
type CreateInput struct {
	URL         string
	Source      string
	Title       string
	Company     string
	Location    string
	Remote      bool
	Description string
	PostedAt    *time.Time
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

// Repository is the persistence contract. Create runs the upsert and enrichment enqueue
// atomically; BySlug loads a moderator-authored job (ErrJobNotFound when missing or not
// moderator-created); Update writes the full resulting row, scoped to created_by IS NOT NULL.
type Repository interface {
	Create(ctx context.Context, p db.UpsertManualJobParams) (db.Job, error)
	BySlug(ctx context.Context, slug string) (db.Job, error)
	Update(ctx context.Context, p db.UpdateManualJobParams) (db.Job, error)
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
func (s *Service) Create(ctx context.Context, actorID int64, in CreateInput) (db.Job, error) {
	if err := in.Validate(); err != nil {
		return db.Job{}, err
	}
	source := strings.TrimSpace(in.Source)
	if source == "" {
		source = defaultSource
	}
	// Moderator descriptions are bulk-imported from scraped pages and rendered with
	// {@html}; sanitize to the same allowlist as every other source so no active
	// markup is ever persisted (stored XSS). Done once and reused for derivation.
	description := sources.SanitizeHTML(in.Description)
	d := jobderive.Derive(jobderive.Input{
		Title:       in.Title,
		Company:     in.Company,
		Source:      source,
		ExternalID:  in.URL,
		Location:    in.Location,
		Description: description,
		WorkMode:    remoteWorkMode(in.Remote),
	})
	return s.repo.Create(ctx, db.UpsertManualJobParams{
		Source:      source,
		ExternalID:  in.URL,
		URL:         in.URL,
		Title:       in.Title,
		Company:     in.Company,
		CompanySlug: d.CompanySlug,
		Location:    in.Location,
		Remote:      in.Remote,
		Description: description,
		PostedAt:    toTimestamptz(in.PostedAt),
		PublicSlug:  d.PublicSlug,
		Countries:   d.Countries,
		Regions:     d.Regions,
		WorkMode:    d.WorkMode,
		Skills:      d.Skills,
		Seniority:   d.Seniority,
		Category:    d.Category,

		PostingLanguage:    d.PostingLanguage,
		EmploymentType:     d.EmploymentType,
		EducationLevel:     d.EducationLevel,
		ExperienceYearsMin: toInt4(d.ExperienceYearsMin),

		CreatedBy: actorID,
		UpdatedBy: actorID,
	})
}

// Update loads the manual job, overlays the supplied (nil-means-unchanged) fields, and
// re-derives the deterministic facets from the merged content — so editing the location,
// description, or company keeps geography/skills/company-slug consistent. The source
// identity (url/external_id/public_slug) is never recomputed, keeping the public slug
// stable. A missing or non-moderator-created slug surfaces ErrJobNotFound.
func (s *Service) Update(ctx context.Context, actorID int64, slug string, p UpdatePatch) (db.Job, error) {
	if err := p.validate(); err != nil {
		return db.Job{}, err
	}
	cur, err := s.repo.BySlug(ctx, slug)
	if err != nil {
		return db.Job{}, err
	}

	title := stringOr(p.Title, cur.Title)
	company := stringOr(p.Company, cur.Company)
	location := stringOr(p.Location, cur.Location)
	// Sanitize a supplied description before persisting (stored XSS); re-sanitizing the
	// already-clean current value is idempotent.
	description := sources.SanitizeHTML(stringOr(p.Description, cur.Description))
	remote := cur.Remote
	if p.Remote != nil {
		remote = *p.Remote
	}
	postedAt := cur.PostedAt
	if p.PostedAt != nil {
		postedAt = toTimestamptz(p.PostedAt)
	}

	// External id and source stay the create-time identity; only the dictionary facets
	// re-derive (the recomputed public slug is discarded — identity is immutable).
	d := jobderive.Derive(jobderive.Input{
		Title:       title,
		Company:     company,
		Source:      cur.Source,
		ExternalID:  cur.ExternalID,
		Location:    location,
		Description: description,
		WorkMode:    remoteWorkMode(remote),
	})
	return s.repo.Update(ctx, db.UpdateManualJobParams{
		PublicSlug:  slug,
		Title:       title,
		Company:     company,
		CompanySlug: d.CompanySlug,
		Location:    location,
		Remote:      remote,
		Description: description,
		PostedAt:    postedAt,
		Countries:   d.Countries,
		Regions:     d.Regions,
		WorkMode:    d.WorkMode,
		Skills:      d.Skills,
		Seniority:   d.Seniority,
		Category:    d.Category,

		PostingLanguage:    d.PostingLanguage,
		EmploymentType:     d.EmploymentType,
		EducationLevel:     d.EducationLevel,
		ExperienceYearsMin: toInt4(d.ExperienceYearsMin),

		UpdatedBy: actorID,
	})
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

// toTimestamptz maps an optional time to the pgtype the params expect; nil becomes NULL.
func toTimestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

// toInt4 maps an optional int (experience_years_min) to the pgtype the params expect;
// nil becomes NULL.
func toInt4(n *int) pgtype.Int4 {
	if n == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(*n), Valid: true}
}
