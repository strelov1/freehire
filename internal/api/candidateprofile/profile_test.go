package candidateprofile

import (
	"testing"

	"github.com/strelov1/freehire/internal/candidate/cv"
)

func TestBuildProfile(t *testing.T) {
	header := cv.Header{
		FullName: "Ilya Strelov",
		Email:    "",
		Phone:    "+1 555 0100",
		Location: "Lisbon, Portugal",
		Links:    []string{"https://linkedin.com/in/ilya", "https://github.com/ilya", "https://ilya.dev"},
	}

	got := BuildProfile(header, "account@example.com")

	if got.FirstName != "Ilya" || got.LastName != "Strelov" {
		t.Errorf("name split = %q / %q, want Ilya / Strelov", got.FirstName, got.LastName)
	}
	if got.Email != "account@example.com" {
		t.Errorf("email = %q, want account fallback when header email empty", got.Email)
	}
	if got.Phone != "+1 555 0100" || got.Location != "Lisbon, Portugal" {
		t.Errorf("phone/location = %q / %q", got.Phone, got.Location)
	}
	if got.LinkedIn != "https://linkedin.com/in/ilya" {
		t.Errorf("linkedin = %q", got.LinkedIn)
	}
	if got.GitHub != "https://github.com/ilya" {
		t.Errorf("github = %q", got.GitHub)
	}
	if got.Portfolio != "https://ilya.dev" {
		t.Errorf("portfolio = %q", got.Portfolio)
	}
}

func TestBuildProfile_PrefersHeaderEmail(t *testing.T) {
	got := BuildProfile(cv.Header{FullName: "A B", Email: "cv@example.com"}, "account@example.com")
	if got.Email != "cv@example.com" {
		t.Errorf("email = %q, want the CV header email when present", got.Email)
	}
}

func TestProfileFields(t *testing.T) {
	p := Profile{FullName: "Ada Lovelace", Email: "ada@example.com", AuthorizedCountries: "US, UK"}
	fields := p.Fields()
	if fields["full_name"] != "Ada Lovelace" || fields["email"] != "ada@example.com" {
		t.Errorf("fields = %+v, want identity carried through", fields)
	}
	if fields["authorized_countries"] != "US, UK" {
		t.Errorf("authorized_countries = %q", fields["authorized_countries"])
	}
}
