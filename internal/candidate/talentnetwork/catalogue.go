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

	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/platform/db"
)

// ErrNotFound is what every way of not being in the catalogue looks like from outside:
// a handle nobody holds, a member who has left, one whose extract has gone stale, and a
// string that could not be a handle at all. One error because the route must answer all
// four identically — otherwise it becomes a way to ask whether an account exists.
var ErrNotFound = errors.New("talentnetwork: not found")

// Store is the slice of *db.Queries this package reads.
type Store interface {
	ListTalentNetworkMembers(ctx context.Context) ([]db.ListTalentNetworkMembersRow, error)
	GetTalentNetworkMemberByHandle(ctx context.Context, handle string) (db.GetTalentNetworkMemberByHandleRow, error)
}

// Query is the catalogue's whole filter vocabulary. Every field is a closed-vocabulary
// value or a number: there is no free-text search, because a card carries no free text
// to search.
//
// A nil slice and an empty one mean the same thing — no constraint — so a caller never
// has to distinguish "absent" from "present but empty" when reading a query string.
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
// Reading the whole membership and filtering in Go is not a shortcut around SQL — it is
// what the data allows. The two facets that matter most, category and seniority, do not
// exist as columns: they are derived from a job title by a dictionary that changes
// weekly, so a `WHERE category = ?` would first need them stored, backfilled, and kept
// in step. At the scale this serves (a membership in the hundreds; the whole catalogue
// is a few megabytes) the derivation is microseconds and the storage is a rounding error.
//
// THE SEAM, stated so nobody has to guess where it is: when the membership outgrows a
// snapshot that fits comfortably in memory — order of thousands — the projection moves
// to a table written by a worker and the filtering moves to SQL. Neither the handler nor
// the wire shape has to change for that; only this type's internals do.
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

	// The snapshot is already ordered (freshest extract first, tie-broken by handle —
	// the query does that), and filtering preserves order, so the page is a window.
	return Page{Members: window(matched, q.Limit, q.Offset), Total: len(matched)}, nil
}

// ByHandle returns one member, read from the DATABASE rather than the snapshot: a
// candidate who leaves must stop resolving on the next request, not when the snapshot
// next refreshes.
func (c *Catalogue) ByHandle(ctx context.Context, handle string) (CatalogueMember, error) {
	// Refused before the query, so a crafted path costs a string comparison. It also
	// keeps every "not a member" answer identical: a malformed handle and a real one
	// nobody holds both come back as ErrNotFound.
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
	return projectMember(row.TalentHandle.String, row.Timezone.String, row.Cities,
		row.Specializations, row.ResumeStructured, row.ResumeStructuredUploadedAt.Time), nil
}

// current returns a snapshot no older than the TTL, refreshing if needed.
//
// A refresh that FAILS while a snapshot already exists is swallowed: a stale list is
// worth more than an empty one, and an empty one reads as "nobody is in the network"
// rather than as an outage. A failure with no snapshot at all has nothing to fall back
// on and is reported.
func (c *Catalogue) current(ctx context.Context) (*snapshot, error) {
	if s := c.snap.Load(); s != nil && c.now().Sub(s.builtAt) < c.ttl {
		return s, nil
	}

	c.refreshing.Lock()
	defer c.refreshing.Unlock()

	// Re-check under the lock: whoever held it may already have refreshed, and a
	// thundering herd on the whole membership is a self-inflicted outage on the same
	// table every authenticated request already touches.
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

	members := make([]CatalogueMember, 0, len(rows))
	for _, r := range rows {
		members = append(members, projectMember(r.TalentHandle.String, r.Timezone.String,
			r.Cities, r.Specializations, r.ResumeStructured, r.ResumeStructuredUploadedAt.Time))
	}

	s := &snapshot{members: members, builtAt: c.now()}
	c.snap.Store(s)
	return s, nil
}

// projectMember turns one stored row into a catalogue entry.
//
// An unreadable stored structure yields an EMPTY card rather than dropping the member:
// they joined, and disappearing from the catalogue is indistinguishable from having
// left. The same treatment resume.Store.Structured gives an unmarshal failure.
func projectMember(handle, timezone string, cities, specializations []string, structured []byte, updatedAt time.Time) CatalogueMember {
	var s resumeextract.Structured
	if len(structured) > 0 {
		_ = json.Unmarshal(structured, &s)
	}
	return CatalogueMember{
		Handle:          handle,
		Card:            ProjectCard(s),
		Timezone:        timezone,
		TimezoneRegion:  timezoneRegion(timezone),
		Cities:          nonNil(cities),
		Specializations: nonNil(specializations),
		UpdatedAt:       updatedAt,
	}
}

// timezoneRegion is the part of an IANA zone before the slash — "Europe" from
// "Europe/Berlin". Empty for an unset or malformed zone, which is what makes a member
// without one fall out of a region filter rather than quietly pass it.
func timezoneRegion(tz string) string {
	region, _, ok := strings.Cut(tz, "/")
	if !ok {
		return ""
	}
	return region
}

// nonNil keeps a nil slice from reaching the wire as `null` where the contract says it
// is a list. The LEFT JOIN behind these fields makes nil the ordinary case, not an edge.
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// matches reports whether a member satisfies every constraint in q. Values within one
// filter are OR and different filters are AND: "any of these skills" is what narrowing a
// search means, while requiring all of them empties the result on the second term.
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
// have never satisfies a non-empty want, which is what excludes a member with no
// timezone from a timezone filter instead of passing them through.
func anyOf(want []string, have string) bool {
	if len(want) == 0 {
		return true
	}
	return have != "" && slices.Contains(want, have)
}

// overlaps reports whether have carries any of want. Same empty-means-unconstrained rule
// as anyOf.
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

// window is the LIMIT/OFFSET slice, clamped so an offset past the end is an empty page
// rather than a panic — a visitor who edits the URL is not an error condition.
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
