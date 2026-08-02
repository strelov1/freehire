package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/resumeextract"
)

// fakeBaseCV is a baseCVReader returning a canned base CV. ok=false models a user with no
// base CV — including one whose only CVs are tailored copies, which the store's query
// excludes.
type fakeBaseCV struct {
	doc cv.Document
	ok  bool
	err error
}

func (f fakeBaseCV) BaseCV(context.Context, int64) (cv.Record, bool, error) {
	return cv.Record{Document: f.doc}, f.ok, f.err
}

// fakeAccount is an accountReader returning a canned account row.
type fakeAccount struct {
	email string
	err   error
}

func (f fakeAccount) GetUserByID(context.Context, int64) (db.GetUserByIDRow, error) {
	return db.GetUserByIDRow{Email: f.email}, f.err
}

func autofillWith(cvs fakeBaseCV, st resumeextract.Structured, stOK bool, email string) *autofillHandlers {
	return &autofillHandlers{
		cvs:      cvs,
		resumes:  fakeStructuredResume{ret: st, ok: stOK},
		accounts: fakeAccount{email: email},
	}
}

func TestAutofillProfile_PrefersBaseCVOverStructuredResume(t *testing.T) {
	h := autofillWith(
		fakeBaseCV{doc: cv.Document{Header: cv.Header{FullName: "Ilya Strelov", Phone: "+1 555 0100"}}, ok: true},
		resumeextract.Structured{FullName: "I. Strelov", Phone: "+1 555 0199"}, true,
		"account@example.com",
	)

	got, err := h.autofillProfile(context.Background(), 7)
	if err != nil {
		t.Fatalf("autofillProfile: %v", err)
	}
	if got.FullName != "Ilya Strelov" || got.Phone != "+1 555 0100" {
		t.Errorf("got %q / %q, want the base CV's values", got.FullName, got.Phone)
	}
}

func TestAutofillProfile_FallsBackToStructuredResume(t *testing.T) {
	h := autofillWith(
		fakeBaseCV{ok: false},
		resumeextract.Structured{
			FullName: "Ilya Strelov",
			Phone:    "+1 555 0100",
			Links:    []string{"https://linkedin.com/in/ilya"},
		}, true,
		"account@example.com",
	)

	got, err := h.autofillProfile(context.Background(), 7)
	if err != nil {
		t.Fatalf("autofillProfile: %v", err)
	}
	if got.FullName != "Ilya Strelov" || got.FirstName != "Ilya" || got.LastName != "Strelov" {
		t.Errorf("name = %q (%q / %q), want the structured résumé's", got.FullName, got.FirstName, got.LastName)
	}
	if got.Phone != "+1 555 0100" {
		t.Errorf("phone = %q, want the structured résumé's", got.Phone)
	}
	if got.LinkedIn != "https://linkedin.com/in/ilya" {
		t.Errorf("linkedin = %q, want the structured résumé's link sorted by host", got.LinkedIn)
	}
}

func TestAutofillProfile_EmptyBaseCVFallsThroughToStructuredResume(t *testing.T) {
	h := autofillWith(
		fakeBaseCV{doc: cv.Document{Header: cv.Header{}}, ok: true},
		resumeextract.Structured{FullName: "Ilya Strelov", Phone: "+1 555 0100"}, true,
		"account@example.com",
	)

	got, err := h.autofillProfile(context.Background(), 7)
	if err != nil {
		t.Fatalf("autofillProfile: %v", err)
	}
	if got.FullName != "Ilya Strelov" || got.Phone != "+1 555 0100" {
		t.Errorf("got %q / %q, want the résumé's values through the empty CV", got.FullName, got.Phone)
	}
}

// The chosen source answers for the whole block. A gap in the base CV is a gap the
// candidate left there — the résumé that seeded it must not fill the gap back in.
func TestAutofillProfile_DoesNotMergeTheResumeIntoTheBaseCV(t *testing.T) {
	h := autofillWith(
		fakeBaseCV{doc: cv.Document{Header: cv.Header{FullName: "Ilya Strelov"}}, ok: true},
		resumeextract.Structured{
			FullName: "Ilya Strelov",
			Phone:    "+1 555 0199",
			Links:    []string{"https://linkedin.com/in/ilya"},
		}, true,
		"account@example.com",
	)

	got, err := h.autofillProfile(context.Background(), 7)
	if err != nil {
		t.Fatalf("autofillProfile: %v", err)
	}
	if got.Phone != "" {
		t.Errorf("phone = %q, want empty: the résumé must not fill a gap in the chosen CV", got.Phone)
	}
	if got.LinkedIn != "" {
		t.Errorf("linkedin = %q, want empty for the same reason", got.LinkedIn)
	}
}

func TestAutofillProfile_NoSourceYieldsAccountEmailOnly(t *testing.T) {
	h := autofillWith(fakeBaseCV{ok: false}, resumeextract.Structured{}, false, "account@example.com")

	got, err := h.autofillProfile(context.Background(), 7)
	if err != nil {
		t.Fatalf("autofillProfile: %v", err)
	}
	if got.Email != "account@example.com" {
		t.Errorf("email = %q, want the account address", got.Email)
	}
	if got.FullName != "" || got.Phone != "" || got.Location != "" || got.LinkedIn != "" {
		t.Errorf("got %+v, want every other field empty", got)
	}
}

func TestAutofillProfile_SourceWithoutEmailGetsAccountAddress(t *testing.T) {
	h := autofillWith(
		fakeBaseCV{doc: cv.Document{Header: cv.Header{FullName: "Ilya Strelov", Phone: "+1 555 0100"}}, ok: true},
		resumeextract.Structured{}, false,
		"account@example.com",
	)

	got, err := h.autofillProfile(context.Background(), 7)
	if err != nil {
		t.Fatalf("autofillProfile: %v", err)
	}
	if got.Email != "account@example.com" {
		t.Errorf("email = %q, want the account address completing a source that states none", got.Email)
	}
	if got.FullName != "Ilya Strelov" {
		t.Errorf("full name = %q, want the CV's", got.FullName)
	}
}

// A failing CV read is a server fault, not an empty profile: silently autofilling a form
// with a blank identity is worse than reporting the failure.
func TestAutofillProfile_CVReadErrorIsReturned(t *testing.T) {
	h := autofillWith(fakeBaseCV{err: errors.New("boom")}, resumeextract.Structured{}, false, "account@example.com")

	if _, err := h.autofillProfile(context.Background(), 7); err == nil {
		t.Fatal("autofillProfile returned no error, want the store's failure surfaced")
	}
}

func TestFirstStatedHeader_EarlierSourceWins(t *testing.T) {
	base := cv.Header{FullName: "Ilya Strelov", Phone: "+1 555 0100"}
	fallback := cv.Header{FullName: "I. Strelov", Phone: "+1 555 0199"}

	got := firstStatedHeader(base, fallback)

	if got.FullName != "Ilya Strelov" || got.Phone != "+1 555 0100" {
		t.Errorf("got %q / %q, want the first source that states a contact", got.FullName, got.Phone)
	}
}

func TestFirstStatedHeader_SkipsASourceStatingNothing(t *testing.T) {
	empty := cv.Header{}
	stated := cv.Header{FullName: "Ilya Strelov"}

	got := firstStatedHeader(empty, stated)

	if got.FullName != "Ilya Strelov" {
		t.Errorf("full name = %q, want the later source when the earlier states nothing", got.FullName)
	}
}

// A CV that states only a location has been edited by its owner and answers for the whole
// block: location is a contact value like any other.
func TestFirstStatedHeader_LocationAloneCounts(t *testing.T) {
	locationOnly := cv.Header{Location: "Lisbon, Portugal"}
	stated := cv.Header{FullName: "Ilya Strelov", Phone: "+1 555 0100"}

	got := firstStatedHeader(locationOnly, stated)

	if got.FullName != "" || got.Phone != "" {
		t.Errorf("got %q / %q, want the location-only source to answer alone", got.FullName, got.Phone)
	}
	if got.Location != "Lisbon, Portugal" {
		t.Errorf("location = %q, want the location-only source's value", got.Location)
	}
}

// The chosen source answers for every field. Merging would restore a value the candidate
// deliberately removed from their CV, since the résumé is what seeded that CV.
func TestFirstStatedHeader_DoesNotMergeAcrossSources(t *testing.T) {
	base := cv.Header{FullName: "Ilya Strelov"}
	fallback := cv.Header{FullName: "Ilya Strelov", Phone: "+1 555 0199", Links: []string{"https://ilya.dev"}}

	got := firstStatedHeader(base, fallback)

	if got.Phone != "" {
		t.Errorf("phone = %q, want empty: the later source must not fill a gap in the chosen one", got.Phone)
	}
	if len(got.Links) != 0 {
		t.Errorf("links = %v, want none", got.Links)
	}
}

func TestFirstStatedHeader_NoSourceStatesAnything(t *testing.T) {
	got := firstStatedHeader(cv.Header{}, cv.Header{})

	if got.FullName != "" || got.Email != "" || got.Phone != "" || got.Location != "" || len(got.Links) != 0 {
		t.Errorf("got %+v, want the zero header", got)
	}
}

func TestContactHeaderFromStructured(t *testing.T) {
	st := resumeextract.Structured{
		FullName: "Ilya Strelov",
		Email:    "ilya@example.com",
		Phone:    "+1 555 0100",
		Location: "Lisbon, Portugal",
		Links:    []string{"https://linkedin.com/in/ilya"},
		Summary:  "Backend engineer",
	}

	got := contactHeaderFromStructured(st)

	want := cv.Header{
		FullName: "Ilya Strelov",
		Email:    "ilya@example.com",
		Phone:    "+1 555 0100",
		Location: "Lisbon, Portugal",
		Links:    []string{"https://linkedin.com/in/ilya"},
	}
	if got.FullName != want.FullName || got.Email != want.Email || got.Phone != want.Phone {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if got.Location != want.Location || len(got.Links) != 1 || got.Links[0] != want.Links[0] {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
