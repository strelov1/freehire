package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/resume"
	"github.com/strelov1/freehire/internal/resumeextract"
	"github.com/strelov1/freehire/internal/userprofile"
)

// fakeStructuredResume is a structuredResumeReader returning a canned structured résumé,
// so the profile read can be exercised without a database or an LLM.
type fakeStructuredResume struct {
	ret resumeextract.Structured
	ok  bool
	err error
	// provisional is identity-only contacts when the stamp is stale; provisionalOK is
	// whether ProvisionalContacts should report them available.
	provisional   resumeextract.Structured
	provisionalOK bool
	// geo is the derived candidate geography; geoOK mirrors the freshness bool. Zero
	// values model a caller for whom nothing was derived.
	geo   resume.Geography
	geoOK bool
}

func (f fakeStructuredResume) Structured(context.Context, int64) (resumeextract.Structured, bool, error) {
	return f.ret, f.ok, f.err
}

func (f fakeStructuredResume) ProvisionalContacts(context.Context, int64) (resumeextract.Structured, bool, error) {
	if f.err != nil {
		return resumeextract.Structured{}, false, f.err
	}
	return f.provisional, f.provisionalOK, nil
}

func (f fakeStructuredResume) Geography(context.Context, int64) (resume.Geography, bool, error) {
	return f.geo, f.geoOK, nil
}

func (f fakeStructuredResume) CandidateContacts(context.Context, int64) (resume.Contacts, error) {
	if f.err != nil {
		return resume.Contacts{}, f.err
	}
	if f.ok {
		return resume.ContactsFromStructured(f.ret), nil
	}
	if f.provisionalOK {
		return resume.ContactsFromStructured(f.provisional), nil
	}
	return resume.Contacts{}, nil
}

func (f fakeStructuredResume) StructureForSeed(context.Context, int64) (resumeextract.Structured, bool, error) {
	if f.err != nil {
		return resumeextract.Structured{}, false, f.err
	}
	if f.ok {
		return f.ret, true, nil
	}
	// Pending matches Store.StructureForSeed: contacts only, no superseded semantics.
	if f.provisionalOK {
		return resumeextract.Structured{
			FullName: f.provisional.FullName,
			Email:    f.provisional.Email,
			Phone:    f.provisional.Phone,
			Location: f.provisional.Location,
			Links:    append([]string(nil), f.provisional.Links...),
		}, true, nil
	}
	return resumeextract.Structured{}, false, nil
}

// profileAppWithResume mounts the profile read on a handler wired to the given résumé
// reader. A nil reader models a deployment where the résumé surface is not configured.
func profileAppWithResume(t *testing.T, repo *fakeProfileRepo, cv structuredResumeReader) (*fiber.App, string) {
	t.Helper()
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	h := &profileHandlers{userProfile: userprofile.New(repo), resume: cv}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Get("/me/profile", auth.RequireAuth(iss, testVersions), h.GetProfile)
	return app, token
}

// savedProfile is a stored profile, so GetProfile serves a payload rather than null.
func savedProfile() *fakeProfileRepo {
	return &fakeProfileRepo{getRet: userprofile.Profile{
		UserID:          1,
		Specializations: []string{"backend"},
		Skills:          []string{"go"},
	}}
}

// profileCV reads the response's cv block. present reports whether the key carried an
// object at all, separating "no structured résumé" (null) from "absent field".
func profileCV(t *testing.T, resp *http.Response) (cv map[string]json.RawMessage, present bool) {
	t.Helper()
	var body struct {
		Data struct {
			CV *map[string]json.RawMessage `json:"cv"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode profile response: %v", err)
	}
	resp.Body.Close()
	if body.Data.CV == nil {
		return nil, false
	}
	return *body.Data.CV, true
}

func TestGetProfile_CVBlockCarriesTheResumeWithoutContacts(t *testing.T) {
	app, token := profileAppWithResume(t, savedProfile(), fakeStructuredResume{
		ret: resumeextract.Structured{
			FullName:   "Ada Lovelace",
			Email:      "ada@example.com",
			Phone:      "+351 900 000 000",
			Links:      []string{"https://github.com/ada"},
			Headline:   "Staff Backend Engineer",
			TotalYears: 11,
			Skills:     []string{"Go"},
		},
		ok: true,
	})

	resp := doProfile(t, app, http.MethodGet, "", token)
	defer resp.Body.Close()
	cv, present := profileCV(t, resp)

	if !present {
		t.Fatal("cv block is null for a caller who has a structured résumé")
	}
	for _, key := range []string{"full_name", "email", "phone", "links"} {
		if _, leaked := cv[key]; leaked {
			t.Errorf("cv block carries the contact field %q", key)
		}
	}
	for _, key := range []string{"headline", "total_years", "skills"} {
		if _, ok := cv[key]; !ok {
			t.Errorf("cv block is missing the professional field %q", key)
		}
	}
}

// TestGetProfile_DegradesToNullCV covers every way the résumé can be unavailable. The
// résumé supplements the profile rather than gating it, so none of them may cost the
// caller their own profile — each serves 200 with a null cv block.
func TestGetProfile_DegradesToNullCV(t *testing.T) {
	for _, tc := range []struct {
		name   string
		reader structuredResumeReader
	}{
		{"no current structured résumé", fakeStructuredResume{ok: false}},
		{"the lookup fails", fakeStructuredResume{err: errors.New("database is down")}},
		{"no résumé reader is wired", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, token := profileAppWithResume(t, savedProfile(), tc.reader)

			resp := doProfile(t, app, http.MethodGet, "", token)
			if resp.StatusCode != fiber.StatusOK {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}
			if _, present := profileCV(t, resp); present {
				t.Error("cv block should be null")
			}
		})
	}
}

// The derived block is a SIBLING of location_preferences, not a merge into it: one is
// what the candidate asserted, the other what was derived for them, and the client has to
// know which it is holding before it pre-fills a control with it.
func TestProfileRead_CarriesDerivedLocation(t *testing.T) {
	cv := fakeStructuredResume{
		ret: resumeextract.Structured{Headline: "Backend engineer"}, ok: true,
		geo:   resume.Geography{Countries: []string{"pl"}, Regions: []string{"eu"}, Cities: []string{"Kraków"}},
		geoOK: true,
	}
	app, token := profileAppWithResume(t, savedProfile(), cv)

	body := profileDerived(t, app, token)
	if body == nil {
		t.Fatal("derived_location = null, want the geography derived from the CV")
	}
	if len(body.Countries) != 1 || body.Countries[0] != "pl" {
		t.Errorf("derived countries = %v, want [pl]", body.Countries)
	}
}

// Nothing derived reads as null rather than as an empty block — an empty block would
// invite the client to render a pre-fill control as though it had something to offer.
func TestProfileRead_DerivedLocationNullWhenNothingResolved(t *testing.T) {
	cv := fakeStructuredResume{
		ret: resumeextract.Structured{Headline: "Backend engineer"}, ok: true,
		geo:   resume.Geography{Countries: []string{}, Regions: []string{}},
		geoOK: true,
	}
	app, token := profileAppWithResume(t, savedProfile(), cv)

	if got := profileDerived(t, app, token); got != nil {
		t.Errorf("derived_location = %+v, want null when nothing resolved", got)
	}
}

// profileDerived reads the response's derived_location block, nil when the key is null.
func profileDerived(t *testing.T, app *fiber.App, token string) *derivedLocation {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/me/profile", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	var body struct {
		Data struct {
			DerivedLocation *derivedLocation `json:"derived_location"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode profile response: %v", err)
	}
	return body.Data.DerivedLocation
}
