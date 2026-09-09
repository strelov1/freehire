//go:build integration

// Integration tests for the moderator-only pending-profile photo route against a real
// Postgres: it serves a pending, opted-in mentor's photo (which the public, slug-keyed
// route refuses since the profile is not yet approved), refuses a mentor who has not
// opted in, refuses an unknown id, and refuses a non-moderator caller.
// Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/candidate/headshot"
	"github.com/strelov1/freehire/internal/engage/mentorship"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestModeratorCanPreviewAPendingMentorsPhoto(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()

	var modID, optedInUserID, optedOutUserID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email, role) VALUES ('mod-photo@example.test', 'moderator') RETURNING id`,
	).Scan(&modID); err != nil {
		t.Fatalf("seed moderator: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('mentor-photo-in@example.test') RETURNING id`,
	).Scan(&optedInUserID); err != nil {
		t.Fatalf("seed opted-in mentor: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('mentor-photo-out@example.test') RETURNING id`,
	).Scan(&optedOutUserID); err != nil {
		t.Fatalf("seed opted-out mentor: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO companies (slug, name) VALUES ('photoco', 'photoco') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatalf("seed company: %v", err)
	}

	queries := db.New(pool)
	svc := mentorship.New(mentorship.NewQueriesRepository(queries, pool), mentorship.Config{})

	optedIn, err := svc.SubmitProfile(ctx, mentorship.ProfileInput{
		UserID: optedInUserID, CompanySlug: "photoco", Slug: "photo-in-mentor",
		DisplayName: "Photo In", Headline: "Staff Engineer",
		Topics: []string{"career"}, Languages: []string{"en"}, Timezone: "Europe/Berlin",
		Session:    mentorship.SessionParams{Duration: time.Hour, MinimumNotice: 2 * time.Hour, Horizon: 30 * 24 * time.Hour},
		MeetingURL: "https://meet.example.test/photo-in",
		ShowPhoto:  true,
	})
	if err != nil {
		t.Fatalf("submit opted-in profile: %v", err)
	}
	optedOut, err := svc.SubmitProfile(ctx, mentorship.ProfileInput{
		UserID: optedOutUserID, CompanySlug: "photoco", Slug: "photo-out-mentor",
		DisplayName: "Photo Out", Headline: "Staff Engineer",
		Topics: []string{"career"}, Languages: []string{"en"}, Timezone: "Europe/Berlin",
		Session:    mentorship.SessionParams{Duration: time.Hour, MinimumNotice: 2 * time.Hour, Horizon: 30 * 24 * time.Hour},
		MeetingURL: "https://meet.example.test/photo-out",
		ShowPhoto:  false,
	})
	if err != nil {
		t.Fatalf("submit opted-out profile: %v", err)
	}

	blobs := newFakePhotoBlobs()
	blobs.objs["headshots/photo-in"] = []byte("jpeg-bytes")
	photos := headshot.New(blobs, &fakePhotoRepo{key: "headshots/photo-in", set: true})

	h := newMentorshipHandlers(svc, photos)

	iss := auth.NewIssuer("test-secret", time.Hour)
	modCookie, _ := iss.Issue(modID, testTokenVersion)
	strangerCookie, _ := iss.Issue(optedInUserID, testTokenVersion)
	requireMod := auth.RequireRole(queries, "moderator")

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	keyAuth := auth.RequireAuthOrKey(iss, testVersions, apiKeys{queries})
	app.Get("/api/v1/mentorship/profiles/:id/photo", keyAuth, requireMod, h.GetPendingMentorPhoto)

	authedRequest := func(path, cookie string) *http.Request {
		r := httptest.NewRequestWithContext(ctx, fiber.MethodGet, path, nil)
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		return r
	}

	t.Run("moderator sees the opted-in mentor's photo", func(t *testing.T) {
		resp, err := app.Test(authedRequest(mentorPhotoPath(optedIn.ID), modCookie))
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			t.Errorf("status = %d, want 200", resp.StatusCode)
		}
	})

	t.Run("a mentor who has not opted in has no photo to preview", func(t *testing.T) {
		resp, err := app.Test(authedRequest(mentorPhotoPath(optedOut.ID), modCookie))
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusNotFound {
			t.Errorf("status = %d, want 404", resp.StatusCode)
		}
	})

	t.Run("an unknown id is not found", func(t *testing.T) {
		resp, err := app.Test(authedRequest(mentorPhotoPath(999999999), modCookie))
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusNotFound {
			t.Errorf("status = %d, want 404", resp.StatusCode)
		}
	})

	t.Run("a non-moderator is forbidden", func(t *testing.T) {
		resp, err := app.Test(authedRequest(mentorPhotoPath(optedIn.ID), strangerCookie))
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusForbidden {
			t.Errorf("status = %d, want 403", resp.StatusCode)
		}
	})
}

func mentorPhotoPath(id int64) string {
	return "/api/v1/mentorship/profiles/" + strconv.FormatInt(id, 10) + "/photo"
}
