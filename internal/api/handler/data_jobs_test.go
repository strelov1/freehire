package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestDataPlanGateAdmitsOnlyTheDataPlan(t *testing.T) {
	future := pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true}
	for _, tc := range []struct {
		name   string
		signed bool
		row    db.GetUserAPIEntitlementRow
		status int
	}{
		{"anonymous", false, db.GetUserAPIEntitlementRow{}, http.StatusUnauthorized},
		{"no plan", true, db.GetUserAPIEntitlementRow{}, http.StatusForbidden},
		{"api tier", true, db.GetUserAPIEntitlementRow{ApiUntilStripe: future}, http.StatusForbidden},
		{"data", true, db.GetUserAPIEntitlementRow{ApiDataUntilStripe: future}, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := fiber.New(fiber.Config{ErrorHandler: RenderError})
			app.Get("/", func(c *fiber.Ctx) error {
				if tc.signed {
					c.Locals(auth.LocalsUserID, int64(7))
				}
				return c.Next()
			}, dataPlanGate(fakeEntitlements(tc.row)), func(c *fiber.Ctx) error {
				if !isDataPlanRequest(c.Context()) {
					t.Error("admitted request is not marked as a Data request")
				}
				return c.SendStatus(http.StatusOK)
			})
			resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), "GET", "/", nil))
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != tc.status {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tc.status)
			}
			if tc.status == http.StatusOK && resp.Header.Get("Cache-Control") != "private, no-store" {
				t.Errorf("Cache-Control = %q, want private, no-store", resp.Header.Get("Cache-Control"))
			}
		})
	}
}

func TestHideOriginsKeepsTheLinksOnlyForDataRequests(t *testing.T) {
	view := func() []jobview.Job {
		return []jobview.Job{{URL: "https://boards.example/1", SourceURL: "https://boards.example/p/1", ApplyURL: "https://freehire.me/api/v1/go/a?src=web"}}
	}
	public := view()
	hideOrigins(context.Background(), public)
	if public[0].URL != "" || public[0].SourceURL != "" || public[0].ApplyURL == "" {
		t.Errorf("public view = %+v, want url and source_url dropped, apply_url kept", public[0])
	}
	data := view()
	hideOrigins(context.WithValue(context.Background(), localsDataPlan, true), data) //nolint:staticcheck // Fiber Locals keys are plain strings
	if data[0].URL == "" || data[0].SourceURL == "" {
		t.Errorf("data view = %+v, want the links kept", data[0])
	}
}
