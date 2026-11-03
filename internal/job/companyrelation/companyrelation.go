// Package companyrelation decides the company relations a signed-in person proposes and a
// moderator approves (freehire#3895).
//
// There are two kinds, and they differ on purpose:
//
//   - KindSame: two slugs are ONE employer that the name fold cannot reach, like
//     "Krka, tovarna zdravil, d.d., Novo mesto" beside "Krka". Approving it does what
//     cmd/merge-companies does for a curated pair. It records a company_slug_aliases row so
//     ingest files the retiring spelling's next posting under the canon and the old URL 301s,
//     then moves the retiring slug's jobs. The difference is who made the judgement.
//   - KindSubsidiary: two DIFFERENT employers, one owned by the other (Novartis Slovenija
//     under Novartis). Nothing moves. The pair is linked in company_parents, so each page
//     names the other while keeping its own jobs, URL and name.
//
// A proposal changes nothing on its own. A wrong merge moves a public URL onto the wrong
// employer, so every check runs again inside the approval's transaction, against the
// registry as it stands then.
package companyrelation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/job/companymerge"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgerr"
)

// Kind is what a proposal claims about its two companies.
type Kind string

const (
	// KindSame claims CompanySlug and TargetSlug are one employer. CompanySlug retires.
	KindSame Kind = "same"
	// KindSubsidiary claims CompanySlug is owned by TargetSlug. Both stay.
	KindSubsidiary Kind = "subsidiary"
)

// aliasReason labels the registry rows an approval writes (migration 0230). It is a class of
// its own because reversing a human judgement must not touch the rule-made rows.
const aliasReason = "moderator"

// maxNote bounds a proposal's free-text note. The table's CHECK holds the same figure.
const maxNote = 1000

var (
	// ErrInvalid is a malformed proposal. Its message is safe to show the caller.
	ErrInvalid = errors.New("invalid proposal")
	// ErrNotFound is a company slug no catalogue row holds, or an unknown proposal id.
	ErrNotFound = errors.New("not found")
	// ErrDuplicate is a proposal that is already waiting in the queue.
	ErrDuplicate = errors.New("this change is already proposed")
	// ErrConflict is a relation the registry as it stands cannot take. Its message names why.
	ErrConflict = errors.New("conflict")
	// ErrDecided is a decision on a proposal a moderator has already decided.
	ErrDecided = errors.New("this proposal has already been decided")
)

// Service owns the proposals and their decisions.
type Service struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// New builds the service over a pool. The pool is needed, not just the queries, because an
// approval is a transaction.
func New(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, q: db.New(pool)}
}

// Proposal is what a person asks for.
type Proposal struct {
	Kind        Kind
	CompanySlug string
	TargetSlug  string
	Note        string
}

// Propose records a proposal for a moderator to decide. The registry checks run now as well
// as at approval, so a proposal that could never be approved is refused while the person who
// made it is still looking.
func (s *Service) Propose(ctx context.Context, userID int64, p Proposal) (db.CompanyRelationProposal, error) {
	p.Note = strings.TrimSpace(p.Note)
	p.TargetSlug = strings.TrimSpace(p.TargetSlug)
	if err := p.validate(); err != nil {
		return db.CompanyRelationProposal{}, err
	}
	if _, err := check(ctx, s.q, p.Kind, p.CompanySlug, p.TargetSlug); err != nil {
		return db.CompanyRelationProposal{}, err
	}
	row, err := s.q.InsertCompanyRelationProposal(ctx, db.InsertCompanyRelationProposalParams{
		Kind:        string(p.Kind),
		CompanySlug: p.CompanySlug,
		TargetSlug:  p.TargetSlug,
		Note:        p.Note,
		ProposedBy:  pgtype.Int8{Int64: userID, Valid: userID != 0},
	})
	if pgerr.IsUniqueViolation(err) {
		return db.CompanyRelationProposal{}, ErrDuplicate
	}
	return row, err
}

func (p Proposal) validate() error {
	switch p.Kind {
	case KindSame, KindSubsidiary:
	default:
		return fmt.Errorf("%w: kind must be %q or %q", ErrInvalid, KindSame, KindSubsidiary)
	}
	if p.CompanySlug == "" || p.TargetSlug == "" {
		return fmt.Errorf("%w: both companies are required", ErrInvalid)
	}
	if p.CompanySlug == p.TargetSlug {
		return fmt.Errorf("%w: a company cannot be related to itself", ErrInvalid)
	}
	if utf8.RuneCountInString(p.Note) > maxNote {
		return fmt.Errorf("%w: the note is longer than %d characters", ErrInvalid, maxNote)
	}
	return nil
}

// Pending is the moderator queue, oldest first.
func (s *Service) Pending(ctx context.Context) ([]db.ListPendingCompanyRelationProposalsRow, error) {
	return s.q.ListPendingCompanyRelationProposals(ctx)
}

// Decision is what an approval did.
type Decision struct {
	Kind Kind
	// MovedJobs is how many job rows a KindSame approval re-keyed onto the canon. It is 0
	// for KindSubsidiary, which moves nothing.
	MovedJobs int64
}

// Approve applies a proposal. A KindSame approval commits the registry row first and then
// moves the jobs in chunks outside that transaction, as the merge worker does. If a chunk
// fails, the proposal stays approved and the redirect stands, and approving it AGAIN resumes
// the move: the re-key is idempotent, since it selects only rows still carrying the old slug.
func (s *Service) Approve(ctx context.Context, id, moderatorID int64) (Decision, error) {
	p, err := s.decide(ctx, id, moderatorID, true)
	if err != nil {
		return Decision{}, err
	}
	d := Decision{Kind: Kind(p.Kind)}
	if d.Kind != KindSame {
		return d, nil
	}
	d.MovedJobs, err = s.move(ctx, p.CompanySlug, p.TargetSlug)
	return d, err
}

// Reject closes a pending proposal without applying it.
func (s *Service) Reject(ctx context.Context, id, moderatorID int64) error {
	_, err := s.decide(ctx, id, moderatorID, false)
	return err
}

// decide locks the proposal, applies the registry half of an approval, and records the
// decision, in one transaction. An approved KindSame proposal may be approved again, which
// only resumes its job move. Every other decided proposal is ErrDecided.
func (s *Service) decide(ctx context.Context, id, moderatorID int64, approve bool) (db.CompanyRelationProposal, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return db.CompanyRelationProposal{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)

	if err := qtx.LockCompanyRelationDecisions(ctx); err != nil {
		return db.CompanyRelationProposal{}, err
	}
	p, err := qtx.GetCompanyRelationProposalForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	if err != nil {
		return p, err
	}
	switch {
	case p.Status == "approved" && approve && Kind(p.Kind) == KindSame:
		return p, nil // resume the move, nothing to decide
	case p.Status != "pending":
		return p, ErrDecided
	}

	status := "rejected"
	if approve {
		status = "approved"
		name, err := check(ctx, qtx, Kind(p.Kind), p.CompanySlug, p.TargetSlug)
		if err != nil {
			return p, err
		}
		if err := apply(ctx, qtx, Kind(p.Kind), p.CompanySlug, name, p.TargetSlug); err != nil {
			return p, err
		}
	}
	if err := qtx.DecideCompanyRelationProposal(ctx, db.DecideCompanyRelationProposalParams{
		ID:        id,
		Status:    status,
		DecidedBy: pgtype.Int8{Int64: moderatorID, Valid: moderatorID != 0},
	}); err != nil {
		return p, err
	}
	return p, tx.Commit(ctx)
}

// check refuses a relation the registry as it stands cannot take, and returns the name a
// merge's folded key is built from. Each refusal is something that would otherwise go wrong
// silently.
func check(ctx context.Context, q *db.Queries, kind Kind, slug, target string) (string, error) {
	c, err := company(ctx, q, slug)
	if err != nil {
		return "", err
	}
	if _, err := company(ctx, q, target); err != nil {
		return "", err
	}
	if kind == KindSubsidiary {
		return c.Name, checkSubsidiary(ctx, q, slug, target)
	}
	name, err := spelling(ctx, q, slug, c.Name)
	if err != nil {
		return "", err
	}
	return name, checkSame(ctx, q, slug, name, target)
}

// spelling is the name the retiring slug's newest posting carries, the one the next crawl
// sends and ingest folds to look the alias up, as cmd/merge-companies reads it from jobs.
// companies.name can be a curated display name that folds elsewhere. A company with no
// posting falls back to it.
func spelling(ctx context.Context, q *db.Queries, slug, fallback string) (string, error) {
	name, err := q.LatestCompanySpelling(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return fallback, nil
	}
	return name, err
}

func checkSame(ctx context.Context, q *db.Queries, alias, aliasName, canon string) error {
	// A slug others already redirect to cannot retire: the registry resolves one hop, so
	// those redirects would land on a slug with no jobs.
	isCanon, err := q.IsCanonicalCompanySlug(ctx, alias)
	if err != nil {
		return err
	}
	if isCanon {
		return fmt.Errorf("%w: other names already redirect to %q, so it should be the one that stays — propose the merge the other way", ErrConflict, alias)
	}
	// A canon that is itself a retired spelling would make a chain.
	if retiredInto, err := q.GetCompanySlugAlias(ctx, canon); err == nil {
		return fmt.Errorf("%w: %q already redirects to %q — propose a merge into %q instead", ErrConflict, canon, retiredInto, retiredInto)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	// An alias row for the retiring slug that names ANOTHER canon is a decision already
	// made. The insert would silently keep it (the canon freezes at first merge), so the
	// approval would report a merge that never happened.
	if prior, err := q.GetCompanySlugAlias(ctx, alias); err == nil && prior != canon {
		return fmt.Errorf("%w: %q is already merged into %q", ErrConflict, alias, prior)
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	// One canon per folded key. Ingest resolves a spelling by its fold, and a key held by two
	// canons resolves by sort order, which nobody chose.
	rows, err := q.ResolveCompanySlugAliases(ctx, []string{foldedKey(alias, aliasName)})
	if err != nil {
		return err
	}
	for _, r := range rows {
		if r.CanonicalSlug != canon {
			return fmt.Errorf("%w: the name of %q already resolves to %q", ErrConflict, alias, r.CanonicalSlug)
		}
	}
	return nil
}

func checkSubsidiary(ctx context.Context, q *db.Queries, child, parent string) error {
	// One level, so a company page never has to walk a chain.
	hasParent, err := q.CompanyHasParent(ctx, parent)
	if err != nil {
		return err
	}
	if hasParent {
		return fmt.Errorf("%w: %q is itself a subsidiary — link to its parent instead", ErrConflict, parent)
	}
	hasChildren, err := q.CompanyHasSubsidiaries(ctx, child)
	if err != nil {
		return err
	}
	if hasChildren {
		return fmt.Errorf("%w: %q has subsidiaries of its own, so it cannot become one", ErrConflict, child)
	}
	return nil
}

// apply writes the registry half of an approval.
func apply(ctx context.Context, q *db.Queries, kind Kind, slug, name, target string) error {
	if kind == KindSubsidiary {
		return q.UpsertCompanyParent(ctx, db.UpsertCompanyParentParams{ChildSlug: slug, ParentSlug: target})
	}
	return companymerge.Retire(ctx, q, slug, target, foldedKey(slug, name), aliasReason)
}

// move re-keys the retiring slug's jobs onto the canon and reconciles the companies rows,
// the same procedure cmd/merge-companies runs. The search index catches up on the scheduled
// reindex, as it does after a merge wave.
func (s *Service) move(ctx context.Context, alias, canon string) (int64, error) {
	moved, err := companymerge.MoveJobs(ctx, companymerge.Rekey(s.q), alias, canon, companymerge.Chunk)
	if err != nil {
		return moved, err
	}
	_, _, err = companymerge.Reconcile(ctx, s.q)
	return moved, err
}

func company(ctx context.Context, q *db.Queries, slug string) (db.Company, error) {
	c, err := q.GetCompany(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, fmt.Errorf("%w: no company %q", ErrNotFound, slug)
	}
	return c, err
}

// foldedKey is the key ingest will look the retiring spelling up by: the fold of its NAME,
// as cmd/merge-companies writes it (see docs/agents/company-identity.md, "It folds the
// NAME, not the slug"). A name that folds to nothing falls back to the slug's own fold, so
// the row is at least reachable from the slug.
func foldedKey(slug, name string) string {
	if k := normalize.CompanyKey(name); k != "" {
		return k
	}
	return normalize.FoldSlug(slug)
}
