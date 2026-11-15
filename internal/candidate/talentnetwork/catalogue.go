package talentnetwork

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/platform/db"
)

// ErrNotFound covers every way of not being in the catalogue: an unheld handle, a member
// who left, a stale extract, a malformed handle. One error so the route answers all four
// identically and cannot be used to ask whether an account exists.
var ErrNotFound = errors.New("talentnetwork: not found")

// Store is the slice of *db.Queries this package reads.
type Store interface {
	ListTalentNetworkMembers(ctx context.Context) ([]db.ListTalentNetworkMembersRow, error)
	GetTalentNetworkMemberByHandle(ctx context.Context, handle string) (db.GetTalentNetworkMemberByHandleRow, error)
	GetTalentNetworkMemberUserIDByHandle(ctx context.Context, handle string) (int64, error)
}

// Query is the catalogue's whole filter vocabulary: closed-vocabulary values and numbers,
// no free-text search (a card carries no free text). A nil slice and an empty one both
// mean no constraint.
type Query struct {
	Categories      []string
	Seniorities     []string
	Skills          []string
	TimezoneRegions []string
	Cities          []string
	Specializations []string
	MinYears        int

	Limit  int
	Offset int
}

// Page is one page of the catalogue and the total behind the same filter.
type Page struct {
	Members []CatalogueMember
	Total   int
}

// Catalogue serves the public list from an in-memory snapshot refreshed on a TTL.
//
// Filtering in Go fits the data: category and seniority are not columns, they are derived
// from job titles by a dictionary that changes often, so SQL would first need them stored,
// backfilled and kept in step.
//
// shortcut: the whole membership is held in memory, fine for hundreds. At thousands, move
// the projection to a worker-written table and the filtering to SQL; only this type's
// internals change.
type Catalogue struct {
	store Store
	ttl   time.Duration
	now   func() time.Time

	// refreshing serialises refreshes so a cold catalogue facing concurrent readers
	// reads the membership once, not once per reader.
	refreshing sync.Mutex
	snap       atomic.Pointer[snapshot]
}

type snapshot struct {
	members []CatalogueMember
	builtAt time.Time
}

// NewCatalogue builds a catalogue over store. now is injected so tests can move time
// without sleeping.
func NewCatalogue(store Store, ttl time.Duration, now func() time.Time) *Catalogue {
	if now == nil {
		now = time.Now
	}
	return &Catalogue{store: store, ttl: ttl, now: now}
}

// List returns one filtered, ordered page and the total behind the same filter.
func (c *Catalogue) List(ctx context.Context, q Query) (Page, error) {
	snap, err := c.current(ctx)
	if err != nil {
		return Page{}, err
	}

	matched := make([]CatalogueMember, 0, len(snap.members))
	for _, m := range snap.members {
		if q.matches(m) {
			matched = append(matched, m)
		}
	}

	// The snapshot is ordered by the query (freshest extract, then handle) and filtering
	// preserves order, so the page is a window.
	return Page{Members: window(matched, q.Limit, q.Offset), Total: len(matched)}, nil
}

// ByHandle returns one member, read from the DATABASE rather than the snapshot so a
// candidate who leaves stops resolving immediately.
func (c *Catalogue) ByHandle(ctx context.Context, handle string) (CatalogueMember, error) {
	// Refused before the query, so a crafted path costs a string comparison.
	if !ValidHandle(handle) {
		return CatalogueMember{}, ErrNotFound
	}

	row, err := c.store.GetTalentNetworkMemberByHandle(ctx, handle)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CatalogueMember{}, ErrNotFound
		}
		return CatalogueMember{}, err
	}
	return projectMember(storedMember{
		handle:          row.TalentHandle.String,
		timezone:        row.Timezone.String,
		cities:          row.Cities,
		specializations: row.Specializations,
		structured:      row.ResumeStructured,
		owned:           row.CandidateContacts,
		updatedAt:       row.ResumeStructuredUploadedAt.Time,
		hasPhoto:        row.HasPhoto,
	}, c.now()), nil
}

// HeadshotOwner resolves a handle to its current member's account id, read from the
// DATABASE like ByHandle so a departed member's headshot is never served. It is apart from
// ByHandle because the id is not part of the public projection and must not travel through
// the code that builds one.
func (c *Catalogue) HeadshotOwner(ctx context.Context, handle string) (int64, error) {
	if !ValidHandle(handle) {
		return 0, ErrNotFound
	}

	id, err := c.store.GetTalentNetworkMemberUserIDByHandle(ctx, handle)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, err
	}
	return id, nil
}

// current returns a snapshot no older than the TTL, refreshing if needed.
//
// A refresh that FAILS while a snapshot exists is swallowed: a stale list beats an empty
// one, which reads as "nobody is in the network". With no snapshot the error is reported.
func (c *Catalogue) current(ctx context.Context) (*snapshot, error) {
	if s := c.snap.Load(); s != nil && c.now().Sub(s.builtAt) < c.ttl {
		return s, nil
	}

	c.refreshing.Lock()
	defer c.refreshing.Unlock()

	// Re-check under the lock: the previous holder may have refreshed already.
	if s := c.snap.Load(); s != nil && c.now().Sub(s.builtAt) < c.ttl {
		return s, nil
	}

	rows, err := c.store.ListTalentNetworkMembers(ctx)
	if err != nil {
		if s := c.snap.Load(); s != nil {
			return s, nil
		}
		return nil, err
	}

	now := c.now()
	members := make([]CatalogueMember, 0, len(rows))
	for _, r := range rows {
		members = append(members, projectMember(storedMember{
			handle:          r.TalentHandle.String,
			timezone:        r.Timezone.String,
			cities:          r.Cities,
			specializations: r.Specializations,
			structured:      r.ResumeStructured,
			owned:           r.CandidateContacts,
			updatedAt:       r.ResumeStructuredUploadedAt.Time,
			hasPhoto:        r.HasPhoto,
		}, now))
	}

	s := &snapshot{members: members, builtAt: now}
	c.snap.Store(s)
	return s, nil
}

// storedMember unifies the list and by-handle rows (same columns, separate sqlc types).
// Named fields because a transposed positional pair would compile and publish cities as
// specialisations.
type storedMember struct {
	handle          string
	timezone        string
	cities          []string
	specializations []string
	structured      []byte
	owned           []byte // candidate_contacts: only the candidate's own years figure is read
	updatedAt       time.Time
	hasPhoto        bool
}

// projectMember turns one stored row into a catalogue entry. An unreadable stored
// structure yields an EMPTY card rather than dropping the member, since vanishing looks
// like having left (as resume.Store.Structured does).
func projectMember(row storedMember, now time.Time) CatalogueMember {
	var s resumeextract.Structured
	if len(row.structured) > 0 {
		_ = json.Unmarshal(row.structured, &s)
	}
	// Years match the candidate's own profile; nothing else they own is read.
	resume.DecodeOwned(row.owned).ApplyTotalYears(&s, now)
	return CatalogueMember{
		Handle:          row.handle,
		Card:            ProjectCard(s),
		Timezone:        row.timezone,
		TimezoneRegion:  timezoneRegion(row.timezone),
		Cities:          nonNil(row.cities),
		Specializations: nonNil(row.specializations),
		UpdatedAt:       row.updatedAt,
		HasPhoto:        row.hasPhoto,
	}
}

// timezoneRegion is the part of an IANA zone before the slash ("Europe"). Empty for an
// unset or malformed zone, so such a member falls out of a region filter.
func timezoneRegion(tz string) string {
	region, _, ok := strings.Cut(tz, "/")
	if !ok {
		return ""
	}
	return region
}

// nonNil keeps a nil slice from reaching the wire as `null`; the LEFT JOIN makes nil ordinary.
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// matches reports whether a member satisfies every constraint in q. Values within one
// filter are OR, different filters AND (requiring all skills would empty the result).
func (q Query) matches(m CatalogueMember) bool {
	return anyOf(q.Categories, m.Card.Category) &&
		anyOf(q.Seniorities, m.Card.Seniority) &&
		overlaps(q.Skills, m.Card.Skills) &&
		anyOf(q.TimezoneRegions, m.TimezoneRegion) &&
		overlaps(q.Cities, m.Cities) &&
		overlaps(q.Specializations, m.Specializations) &&
		m.Card.TotalYears >= q.MinYears
}

// anyOf reports whether have is one of want. An empty want is no constraint; an empty
// have never satisfies a non-empty want.
func anyOf(want []string, have string) bool {
	if len(want) == 0 {
		return true
	}
	return have != "" && slices.Contains(want, have)
}

// overlaps reports whether have carries any of want; an empty want is no constraint.
func overlaps(want, have []string) bool {
	if len(want) == 0 {
		return true
	}
	for _, w := range want {
		if slices.Contains(have, w) {
			return true
		}
	}
	return false
}

// window is the LIMIT/OFFSET slice; an offset past the end is an empty page, not a panic.
func window(members []CatalogueMember, limit, offset int) []CatalogueMember {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(members) {
		return []CatalogueMember{}
	}
	rest := members[offset:]
	if limit > 0 && limit < len(rest) {
		rest = rest[:limit]
	}
	return slices.Clone(rest)
}
