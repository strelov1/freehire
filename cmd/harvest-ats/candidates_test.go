package main

import (
	"reflect"
	"testing"
)

func TestCandidateSlugs(t *testing.T) {
	cases := []struct {
		name string
		site companySite
		want []string
	}{
		{
			name: "domain, profile slug and name each contribute",
			site: companySite{Name: "Doodle", Website: "https://doodle.com", LinkedIn: "https://ch.linkedin.com/company/doodle-ag"},
			want: []string{"doodle", "doodle-ag"},
		},
		{
			name: "a legal-form suffix on the profile slug yields the bare slug too",
			site: companySite{Name: "Simplesurance GmbH", Website: "https://www.simplesurance.com", LinkedIn: "https://de.linkedin.com/company/simplesurance-gmbh"},
			want: []string{"simplesurance", "simplesurance-gmbh"},
		},
		{
			name: "a multi-word name contributes both spellings",
			site: companySite{Name: "Delivery Hero SE", Website: "https://www.deliveryhero.com"},
			want: []string{"deliveryhero", "delivery-hero"},
		},
		{
			name: "a careers host does not become the slug",
			site: companySite{Name: "Picnic Technologies", Website: "https://jobs.picnic.app/en/"},
			want: []string{"picnic", "picnic-technologies", "picnictechnologies"},
		},
		{
			name: "no website and no profile still yields the name's slug",
			site: companySite{Name: "Acme"},
			want: []string{"acme"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := candidateSlugs(tc.site)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("candidateSlugs = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCandidateSlugsAreBounded(t *testing.T) {
	site := companySite{
		Name:     "Some Extremely Long Company Name Ltd",
		Website:  "https://www.someextremelylongcompanyname.example.com",
		LinkedIn: "https://linkedin.com/company/some-extremely-long-company-name-ltd",
	}
	if got := candidateSlugs(site); len(got) > maxCandidateSlugs {
		t.Errorf("candidateSlugs returned %d slugs (%v), want at most %d", len(got), got, maxCandidateSlugs)
	}
}

func TestGuessedCandidates(t *testing.T) {
	t.Run("every slug is proposed to every narrowed provider, carrying the id", func(t *testing.T) {
		site := companySite{
			Name:       "Acme",
			Website:    "https://acme.com",
			LinkedIn:   "https://linkedin.com/company/acme-inc",
			ExternalID: "c2627bcd-915c-4076-98f5-1a2b3c4d5e6f",
		}
		got := guessedCandidates(site)
		// slugs {acme, acme-inc} × providers {ashby, lever}
		if len(got) != 4 {
			t.Fatalf("got %d candidates (%v), want 4", len(got), got)
		}
		for _, h := range got {
			if h.expectID != site.ExternalID {
				t.Errorf("candidate %v carries expectID %q, want the posting id", h, h.expectID)
			}
			if h.provider != "ashby" && h.provider != "lever" {
				t.Errorf("candidate proposed to %q, want only the narrowed providers", h.provider)
			}
			if h.company != "Acme" {
				t.Errorf("candidate company = %q, want Acme", h.company)
			}
		}
	})

	t.Run("no posting id yields no candidates", func(t *testing.T) {
		site := companySite{Name: "Acme", Website: "https://acme.com"}
		if got := guessedCandidates(site); len(got) != 0 {
			t.Errorf("got %v, want none without a posting id", got)
		}
	})

	t.Run("an id whose shape narrows to nothing yields no candidates", func(t *testing.T) {
		site := companySite{Name: "Acme", Website: "https://acme.com", ExternalID: "JR37734"}
		if got := guessedCandidates(site); len(got) != 0 {
			t.Errorf("got %v, want none for an unrecognised id shape", got)
		}
	})
}

func TestProvidersForID(t *testing.T) {
	cases := []struct {
		id   string
		want []string
	}{
		// The platform names itself in the id.
		{"teamtailor-8094978", []string{"teamtailor"}},
		// Ten digits is the Greenhouse job id shape.
		{"4698693006", []string{"greenhouse"}},
		{"5788132680", []string{"greenhouse"}},
		// A UUID is Lever's and Ashby's shape; both are worth asking.
		{"c2627bcd-915c-4076-98f5-1a2b3c4d5e6f", []string{"ashby", "lever"}},
		// Shapes that narrow to nothing must yield nothing rather than fan out.
		{"JR37734", nil},
		{"R00314608_en", nil},
		{"3584-en_GB", nil},
		{"99580", nil},
		{"", nil},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			got := providersForID(tc.id)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("providersForID(%q) = %v, want %v", tc.id, got, tc.want)
			}
		})
	}
}

// TestTrimLegalForm_UsesTheSharedFormList proves harvest no longer carries its own copy of the
// legal-form vocabulary. The tokens below are ones the shared list has and harvest's private
// list did not, so before the two were joined a board registered under the bare name was simply
// never guessed.
func TestTrimLegalForm_UsesTheSharedFormList(t *testing.T) {
	cases := map[string]string{
		"doodle-kg":     "doodle",        // German GmbH & Co. KG
		"doodle-cic":    "doodle",        // UK community interest company
		"doodle-lp":     "doodle",        // limited partnership
		"acme-robotics": "acme-robotics", // nothing to trim
	}
	for in, want := range cases {
		if got := trimLegalForm(in); got != want {
			t.Errorf("trimLegalForm(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestTrimLegalForm_KeepsItsOwnBoardTails guards the tails harvest needs that are NOT legal
// forms, and must therefore stay out of the catalogue's slug rule: "group" is part of a brand
// name, and "spa" collides with the literal word (a resort's "& Spa"). Guessing an extra board
// candidate is free; merging two employers is not.
func TestTrimLegalForm_KeepsItsOwnBoardTails(t *testing.T) {
	for in, want := range map[string]string{"bosch-group": "bosch", "oggi-lavoro-spa": "oggi-lavoro"} {
		if got := trimLegalForm(in); got != want {
			t.Errorf("trimLegalForm(%q) = %q, want %q", in, got, want)
		}
	}
}
