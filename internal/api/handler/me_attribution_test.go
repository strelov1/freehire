package handler

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/auth"
)

func TestNormalizeReferrerHost(t *testing.T) {
	cases := map[string]string{
		"linkedin.com":                      "linkedin.com",
		"WWW.LinkedIn.com":                  "linkedin.com",
		"https://www.chatgpt.com/c/abc?x=1": "chatgpt.com",
		"":                                  "",
		"not a host":                        "",
		"javascript:alert(1)":               "",
		"a.b-c.example.org":                 "a.b-c.example.org",
		strings.Repeat("a", 300) + ".com":   "",
	}
	for in, want := range cases {
		if got := normalizeReferrerHost(in); got != want {
			t.Errorf("normalizeReferrerHost(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeLandingPathDropsQueryAndFragment(t *testing.T) {
	cases := map[string]string{
		"/jobs?token=secret": "/jobs",
		"/jobs/abc#frag":     "/jobs/abc",
		"/":                  "/",
		"jobs":               "",
		"https://evil/jobs":  "",
		"":                   "",
		"/a\x00b":            "/ab",
	}
	for in, want := range cases {
		if got := normalizeLandingPath(in); got != want {
			t.Errorf("normalizeLandingPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAttributionParamsBoundAndLowercaseTags(t *testing.T) {
	p := signupAttributionRequest{
		UTMSource:   "  LinkedIn ",
		UTMMedium:   "Social",
		UTMCampaign: strings.Repeat("x", 500),
	}.params(7)
	if p.UserID != 7 || p.UtmSource.String != "linkedin" || p.UtmMedium.String != "social" {
		t.Fatalf("params = %+v", p)
	}
	if n := len([]rune(p.UtmCampaign.String)); n != 200 {
		t.Fatalf("campaign length = %d, want 200", n)
	}
	if p.UtmTerm.Valid || p.ReferrerHost.Valid || p.LandingPath.Valid {
		t.Fatalf("empty inputs must be NULL, got %+v", p)
	}
}

func TestReportSignupAttribution_IsCookieOnly(t *testing.T) {
	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &authHandlers{}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/api/v1/me/attribution", auth.RequireAuth(iss, testVersions), newContract(contractHandlers{authHandlers: h}))

	for _, bearer := range []bool{false, true} {
		req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/me/attribution", nil)
		if bearer {
			req.Header.Set("Authorization", "Bearer fhk_whatever")
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != fiber.StatusUnauthorized {
			t.Errorf("bearer=%v status = %d, want 401", bearer, resp.StatusCode)
		}
	}
}
