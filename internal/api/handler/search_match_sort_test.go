package handler

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/dict/skillvec"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/platform/db"
)

type fakeProfiles struct {
	profile userprofile.Profile
	err     error
}

func (f fakeProfiles) Get(context.Context, int64) (userprofile.Profile, error) {
	return f.profile, f.err
}

type fakeStats struct {
	rows []db.InsightsFacetStat
	err  error
}

func (f fakeStats) ListFacetStats(context.Context) ([]db.InsightsFacetStat, error) {
	return f.rows, f.err
}

// skillRows is a rarity snapshot naming two real dictionary slugs.
var skillRows = []db.InsightsFacetStat{
	{Facet: "skills", Value: "go", Count: 5000},
	{Facet: "skills", Value: "docker", Count: 4000},
	{Facet: "countries", Value: "DE", Count: 40000},
}

// matchSortApp mounts /jobs/search with the match-sort dependencies. userID 0 means
// an anonymous caller — the route is public, so no session is attached.
func matchSortApp(s searcher, userID int64, profiles profileReader, stats facetStatsReader) *fiber.App {
	h := &searchHandlers{search: s, userProfile: profiles, facetStats: stats}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/jobs/search", func(c *fiber.Ctx) error {
		if userID != 0 {
			c.Locals(auth.LocalsUserID, userID)
		}
		return h.SearchJobs(c)
	})
	return app
}

func TestSearchSortMatch_SignedInCallerIsRankedByVector(t *testing.T) {
	fake := &fakeSearcher{}
	app := matchSortApp(fake, 7, fakeProfiles{profile: userprofile.Profile{Skills: []string{"go", "docker"}}}, fakeStats{rows: skillRows})

	status, _ := doGet(t, app, "/jobs/search?sort=match")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if len(fake.got.Vector) != skillvec.Dimensions {
		t.Errorf("vector width = %d, want %d", len(fake.got.Vector), skillvec.Dimensions)
	}
	if fake.got.Sort != nil {
		t.Errorf("sort = %v, want nil — an attribute sort silently overrides vector ranking", fake.got.Sort)
	}
}

func TestSearchSortMatch_AnonymousCallerGetsTheDefaultFeed(t *testing.T) {
	fake := &fakeSearcher{}
	app := matchSortApp(fake, 0, fakeProfiles{profile: userprofile.Profile{Skills: []string{"go"}}}, fakeStats{rows: skillRows})

	status, _ := doGet(t, app, "/jobs/search?sort=match")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200 — a shared link carrying sort=match must not break for a signed-out visitor", status)
	}
	if len(fake.got.Vector) != 0 {
		t.Errorf("anonymous request carried a vector of %d floats, want none", len(fake.got.Vector))
	}
	if !slices.Equal(fake.got.Sort, []string{"posted_at:desc"}) {
		t.Errorf("sort = %v, want the default freshest-first feed", fake.got.Sort)
	}
}

func TestSearchSortMatch_ProfileWithNoSkillsGetsTheDefaultFeed(t *testing.T) {
	fake := &fakeSearcher{}
	app := matchSortApp(fake, 7, fakeProfiles{profile: userprofile.Profile{}}, fakeStats{rows: skillRows})

	status, _ := doGet(t, app, "/jobs/search?sort=match")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if len(fake.got.Vector) != 0 {
		t.Error("a skill-less profile still produced a vector")
	}
	if !slices.Equal(fake.got.Sort, []string{"posted_at:desc"}) {
		t.Errorf("sort = %v, want the default feed", fake.got.Sort)
	}
}

func TestSearchSortMatch_MissingProfileGetsTheDefaultFeed(t *testing.T) {
	fake := &fakeSearcher{}
	app := matchSortApp(fake, 7, fakeProfiles{err: errors.New("no profile")}, fakeStats{rows: skillRows})

	status, _ := doGet(t, app, "/jobs/search?sort=match")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200 — a caller who never filled a profile is not an error", status)
	}
	if len(fake.got.Vector) != 0 {
		t.Error("a caller with no profile still produced a vector")
	}
}

func TestSearchSortMatch_UnavailableWeightsGetTheDefaultFeed(t *testing.T) {
	fake := &fakeSearcher{}
	app := matchSortApp(fake, 7, fakeProfiles{profile: userprofile.Profile{Skills: []string{"go"}}}, fakeStats{err: errors.New("db down")})

	status, _ := doGet(t, app, "/jobs/search?sort=match")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200 — an unavailable rarity snapshot must not fail the feed", status)
	}
	if len(fake.got.Vector) != 0 {
		t.Error("weights failed to load but a vector was still sent")
	}
}

func TestSearchSortMatch_UnrecognisedSkillsGetTheDefaultFeed(t *testing.T) {
	fake := &fakeSearcher{}
	app := matchSortApp(fake, 7, fakeProfiles{profile: userprofile.Profile{Skills: []string{"definitely-not-a-skill"}}}, fakeStats{rows: skillRows})

	status, _ := doGet(t, app, "/jobs/search?sort=match")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if len(fake.got.Vector) != 0 {
		t.Error("a profile of unrecognised skills still produced a vector")
	}
}

func TestSearchSortMatch_ComposesWithFacetFilters(t *testing.T) {
	fake := &fakeSearcher{}
	app := matchSortApp(fake, 7, fakeProfiles{profile: userprofile.Profile{Skills: []string{"go"}}}, fakeStats{rows: skillRows})

	doGet(t, app, "/jobs/search?sort=match&countries=DE&seniority=senior")

	if len(fake.got.Vector) == 0 {
		t.Error("the vector was dropped when facets were present")
	}
	if fake.got.Filter == nil {
		t.Error("the facet filter was dropped when the match sort was requested")
	}
}

// An attribute sort and the match sort are mutually exclusive; asking for an
// attribute must not also send a vector.
func TestSearchSortMatch_AnAttributeSortSendsNoVector(t *testing.T) {
	fake := &fakeSearcher{}
	app := matchSortApp(fake, 7, fakeProfiles{profile: userprofile.Profile{Skills: []string{"go"}}}, fakeStats{rows: skillRows})

	doGet(t, app, "/jobs/search?sort=posted_at&order=asc")

	if len(fake.got.Vector) != 0 {
		t.Error("an attribute sort still sent a vector")
	}
	if !slices.Equal(fake.got.Sort, []string{"posted_at:asc"}) {
		t.Errorf("sort = %v, want posted_at:asc", fake.got.Sort)
	}
}
