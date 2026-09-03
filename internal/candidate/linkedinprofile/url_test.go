package linkedinprofile

import (
	"errors"
	"testing"
)

func TestPublicIDAccepts(t *testing.T) {
	t.Parallel()

	const canonical = "istrelov"

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"the canonical form", "https://www.linkedin.com/in/istrelov", canonical},
		{"a trailing slash", "https://www.linkedin.com/in/istrelov/", canonical},
		// Copying a profile link from LinkedIn's own share menu appends tracking
		// parameters; a user who pastes one is pasting a profile URL.
		{"tracking parameters", "https://www.linkedin.com/in/istrelov?utm_source=share&trk=x", canonical},
		{"a fragment", "https://www.linkedin.com/in/istrelov#experience", canonical},
		// The country subdomain serves the same profile with localised chrome. It is
		// normalised away so one profile is always fetched from one host.
		{"a country subdomain", "https://br.linkedin.com/in/istrelov", canonical},
		{"the apex host", "https://linkedin.com/in/istrelov", canonical},
		{"http", "http://www.linkedin.com/in/istrelov", canonical},
		{"no scheme", "linkedin.com/in/istrelov", canonical},
		{"no scheme, with www", "www.linkedin.com/in/istrelov", canonical},
		{"mixed case host", "https://WWW.LinkedIn.COM/in/istrelov", canonical},
		{"surrounding whitespace", "  https://www.linkedin.com/in/istrelov  ", canonical},
		// A user reading the address bar on their own profile may copy a sub-page.
		{"a profile sub-page", "https://www.linkedin.com/in/istrelov/details/experience/", canonical},
		{"a bare public id", "istrelov", canonical},
		{
			"a public id with hyphens and digits",
			"https://www.linkedin.com/in/dana-okonkwo-1a2b3c",
			"dana-okonkwo-1a2b3c",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := publicID(tt.in)
			if err != nil {
				t.Fatalf("publicID(%q): %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("publicID(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestPublicIDRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"blank", "   "},
		{"another host wearing the same path", "https://example.com/in/someone"},
		// The one that matters: a suffix match would accept this and fetch an
		// attacker's page with the user's request.
		{"linkedin.com as a prefix of another domain", "https://linkedin.com.evil.example/in/istrelov"},
		{"linkedin.com inside another domain", "https://evil-linkedin.com/in/istrelov"},
		{"a company page", "https://www.linkedin.com/company/ringcentral"},
		{"a job posting", "https://www.linkedin.com/jobs/view/4012345678"},
		{"a school page", "https://www.linkedin.com/school/dstu"},
		{"the site root", "https://www.linkedin.com"},
		{"a profile path with no id", "https://www.linkedin.com/in/"},
		{"a non-http scheme", "ftp://www.linkedin.com/in/istrelov"},
		{"a file scheme", "file:///etc/passwd"},
		{"a subdomain that is neither www nor a country code", "https://internal.linkedin.com/in/istrelov"},
		{"a bare id carrying a path", "istrelov/details"},
		{"a bare id carrying a space", "ilya strelov"},
		{"a bare id that is really a hostname", "example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := publicID(tt.in)
			if !errors.Is(err, ErrNotAProfileURL) {
				t.Fatalf("publicID(%q) = (%q, %v), want ErrNotAProfileURL", tt.in, got, err)
			}
			if got != "" {
				t.Errorf("a rejected input returned %q, want the empty string", got)
			}
		})
	}
}
