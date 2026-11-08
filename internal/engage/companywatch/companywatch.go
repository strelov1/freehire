// Package companywatch is a person's list of companies they want to hear about, up to
// MaxEntries of them.
//
// The list does not send anything itself. Its alert is an ordinary saved search whose query
// is `watchlist=true`: the search endpoints and the notify worker resolve that param, per
// person, to `company_slug IN <their list>` (Slugs), so the alert reuses the channels, the
// quiet hours and the digest sections a saved search already has.
//
// An entry is a company slug with no foreign key to companies, because that table is derived
// from jobs and a company with nothing open is deleted from it. The list must survive that,
// and it must survive a merge, so a stored slug a merge retired is followed to its canonical
// one on every read (the SQL does it) rather than rewritten.
package companywatch

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/platform/db"
)

// MaxEntries is the most companies one person can watch. One early user keeps about 3,000,
// so the limit leaves room without letting a list grow into the filter an alert's search
// would have to carry.
const MaxEntries = 5000

var (
	// ErrNotFound means a slug names no company.
	ErrNotFound = errors.New("companywatch: company not found")
	// ErrFull means the addition would take the list past MaxEntries.
	ErrFull = errors.New("companywatch: watchlist is full")
	// ErrNothingToImport means an import carried no usable entry.
	ErrNothingToImport = errors.New("companywatch: nothing to import")
	// ErrTooManyToImport means an import carried more entries than a list can hold.
	ErrTooManyToImport = errors.New("companywatch: too many entries")
)

// Entry is one watched company as the list shows it.
type Entry struct {
	Slug    string    `json:"slug"`
	Name    string    `json:"name"`
	AddedAt time.Time `json:"added_at"`
}

// ImportResult is what a bulk import did: how many companies it added, how many were on the
// list already, and the entries it could not resolve to exactly one company, as typed.
type ImportResult struct {
	Added     int      `json:"added"`
	Already   int      `json:"already"`
	Unmatched []string `json:"unmatched"`
}

type queries interface {
	ListWatchlist(ctx context.Context, arg db.ListWatchlistParams) ([]db.ListWatchlistRow, error)
	ListWatchlistSlugs(ctx context.Context, userID int64) ([]string, error)
	IsWatchingCompany(ctx context.Context, arg db.IsWatchingCompanyParams) (bool, error)
	ResolveWatchableCompany(ctx context.Context, slug string) (db.ResolveWatchableCompanyRow, error)
	AddToWatchlist(ctx context.Context, arg db.AddToWatchlistParams) (pgtype.Timestamptz, error)
	AddManyToWatchlist(ctx context.Context, arg db.AddManyToWatchlistParams) (db.AddManyToWatchlistRow, error)
	RemoveFromWatchlist(ctx context.Context, arg db.RemoveFromWatchlistParams) error
	ResolveCompanySlugAliases(ctx context.Context, foldedKeys []string) ([]db.ResolveCompanySlugAliasesRow, error)
	ListWatchableCompaniesByFoldedSlug(ctx context.Context, arg db.ListWatchableCompaniesByFoldedSlugParams) ([]db.ListWatchableCompaniesByFoldedSlugRow, error)
}

// Service reads and writes watchlists. *db.Queries satisfies its queries.
type Service struct {
	q queries
}

// New wraps the queries a watchlist needs.
func New(q queries) *Service { return &Service{q: q} }

// List returns one page of the person's list, newest first, and how many entries the name
// filter q leaves (all of them when q is empty).
func (s *Service) List(ctx context.Context, userID int64, q string, limit, offset int) ([]Entry, int, error) {
	rows, err := s.q.ListWatchlist(ctx, db.ListWatchlistParams{
		UserID: userID, Q: strings.TrimSpace(q), PageLimit: int32(limit), PageOffset: int32(offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("companywatch: list: %w", err)
	}
	out := make([]Entry, len(rows))
	total := 0
	for i, r := range rows {
		out[i] = Entry{Slug: r.Slug, Name: r.Name, AddedAt: r.AddedAt.Time}
		total = int(r.Total)
	}
	return out, total, nil
}

// Slugs is every company the person watches, as the canonical slugs a posting is indexed
// under. An empty answer is a real one — nothing watched — and the caller must narrow its
// search to nothing rather than drop the filter.
func (s *Service) Slugs(ctx context.Context, userID int64) ([]string, error) {
	slugs, err := s.q.ListWatchlistSlugs(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("companywatch: slugs: %w", err)
	}
	return slugs, nil
}

// Watching reports whether the person watches the company a page's slug names.
func (s *Service) Watching(ctx context.Context, userID int64, slug string) (bool, error) {
	watching, err := s.q.IsWatchingCompany(ctx, db.IsWatchingCompanyParams{UserID: userID, Slug: slug})
	if err != nil {
		return false, fmt.Errorf("companywatch: watching: %w", err)
	}
	return watching, nil
}

// Add puts a company on the list. It is idempotent, and a company with no open job is
// accepted — that is the one a person most wants to hear about. A slug a merge retired is
// stored under the company it became. ErrNotFound names a slug no company carries, ErrFull a
// list already at MaxEntries.
func (s *Service) Add(ctx context.Context, userID int64, slug string) (Entry, error) {
	c, err := s.q.ResolveWatchableCompany(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return Entry{}, ErrNotFound
	}
	if err != nil {
		return Entry{}, fmt.Errorf("companywatch: resolve %q: %w", slug, err)
	}
	added, err := s.q.AddToWatchlist(ctx, db.AddToWatchlistParams{UserID: userID, Slug: c.Slug, MaxEntries: MaxEntries})
	if errors.Is(err, pgx.ErrNoRows) {
		return Entry{}, ErrFull
	}
	if err != nil {
		return Entry{}, fmt.Errorf("companywatch: add %q: %w", c.Slug, err)
	}
	return Entry{Slug: c.Slug, Name: c.Name, AddedAt: added.Time}, nil
}

// Remove takes a company off the list. It is idempotent: a company that was not on it is
// not an error.
func (s *Service) Remove(ctx context.Context, userID int64, slug string) error {
	if err := s.q.RemoveFromWatchlist(ctx, db.RemoveFromWatchlistParams{UserID: userID, Slug: slug}); err != nil {
		return fmt.Errorf("companywatch: remove %q: %w", slug, err)
	}
	return nil
}

// Import adds the companies a list of names or slugs identifies. Each entry resolves the way
// the catalogue itself resolves a company name — the alias registry, then the folded slug —
// and counts only when that names exactly one company: an entry that names none, or that two
// companies share and no open posting tells apart, is returned in Unmatched as typed, so the
// person can fix it instead of finding a stranger on their list. The batch is all-or-nothing
// against MaxEntries (ErrFull), and what is already on the list is counted, not duplicated.
func (s *Service) Import(ctx context.Context, userID int64, names []string) (ImportResult, error) {
	entries := cleanEntries(names)
	if len(entries) == 0 {
		return ImportResult{}, ErrNothingToImport
	}
	if len(entries) > MaxEntries {
		return ImportResult{}, ErrTooManyToImport
	}

	resolved, err := s.resolveAll(ctx, entries)
	if err != nil {
		return ImportResult{}, err
	}
	res := ImportResult{Unmatched: []string{}}
	var slugs []string
	for _, e := range entries {
		slug, ok := resolved[e]
		if !ok {
			res.Unmatched = append(res.Unmatched, e)
			continue
		}
		slugs = append(slugs, slug)
	}
	slices.Sort(slugs)
	slugs = slices.Compact(slugs)
	if len(slugs) == 0 {
		return res, nil
	}

	got, err := s.q.AddManyToWatchlist(ctx, db.AddManyToWatchlistParams{UserID: userID, Slugs: slugs, MaxEntries: MaxEntries})
	if err != nil {
		return ImportResult{}, fmt.Errorf("companywatch: import: %w", err)
	}
	if got.Fresh > 0 && got.Inserted == 0 {
		return ImportResult{}, ErrFull
	}
	res.Added = int(got.Inserted)
	res.Already = len(slugs) - res.Added
	return res, nil
}

// cleanEntries trims the entries, drops blanks, and keeps the first of any two spelled the
// same, so a pasted list with repeats reports each once.
func cleanEntries(names []string) []string {
	seen := make(map[string]bool, len(names))
	var out []string
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

// resolveAll maps each entry that names exactly one company to its slug. Three reads cover
// the whole batch: the alias registry by folded key, then the companies those keys and
// canonical slugs, and the entries typed as slugs, can land on.
func (s *Service) resolveAll(ctx context.Context, entries []string) (map[string]string, error) {
	keyOf := make(map[string]string, len(entries))
	var keys []string
	for _, e := range entries {
		k := normalize.CompanyKey(e)
		if k == "" {
			continue
		}
		keyOf[e] = k
		keys = append(keys, k)
	}
	slices.Sort(keys)
	keys = slices.Compact(keys)

	aliasRows, err := s.q.ResolveCompanySlugAliases(ctx, keys)
	if err != nil {
		return nil, fmt.Errorf("companywatch: aliases: %w", err)
	}
	canons := make(map[string][]string, len(aliasRows))
	slugs := slices.Clone(entries)
	for _, r := range aliasRows {
		if !slices.Contains(canons[r.FoldedKey], r.CanonicalSlug) {
			canons[r.FoldedKey] = append(canons[r.FoldedKey], r.CanonicalSlug)
			slugs = append(slugs, r.CanonicalSlug)
		}
	}

	companyRows, err := s.q.ListWatchableCompaniesByFoldedSlug(ctx, db.ListWatchableCompaniesByFoldedSlugParams{FoldedKeys: keys, Slugs: slugs})
	if err != nil {
		return nil, fmt.Errorf("companywatch: companies: %w", err)
	}
	bySlug := make(map[string]db.ListWatchableCompaniesByFoldedSlugRow, len(companyRows))
	byFold := make(map[string][]db.ListWatchableCompaniesByFoldedSlugRow, len(companyRows))
	for _, c := range companyRows {
		bySlug[c.Slug] = c
		f := normalize.FoldSlug(c.Slug)
		byFold[f] = append(byFold[f], c)
	}

	out := make(map[string]string, len(entries))
	for _, e := range entries {
		if c, ok := bySlug[e]; ok {
			out[e] = c.Slug
			continue
		}
		if slug, ok := settle(keyOf[e], canons, bySlug, byFold); ok {
			out[e] = slug
		}
	}
	return out, nil
}

// settle is the one company a folded key names, or false when none or several do. The alias
// registry, when it speaks, is the recorded answer. Without it the companies sharing the fold
// decide: one is the company, and of several only a single one with open postings is — a
// namesake that is hiring is the one a person typing the name means, but two of them are a
// name the catalogue has not decided.
func settle(key string, canons map[string][]string, bySlug map[string]db.ListWatchableCompaniesByFoldedSlugRow, byFold map[string][]db.ListWatchableCompaniesByFoldedSlugRow) (string, bool) {
	if key == "" {
		return "", false
	}
	if cs := canons[key]; len(cs) > 0 {
		if len(cs) > 1 {
			return "", false
		}
		c, ok := bySlug[cs[0]]
		return c.Slug, ok
	}
	cs := byFold[key]
	if len(cs) == 1 {
		return cs[0].Slug, true
	}
	var hiring []string
	for _, c := range cs {
		if c.JobCount > 0 {
			hiring = append(hiring, c.Slug)
		}
	}
	if len(hiring) == 1 {
		return hiring[0], true
	}
	return "", false
}
