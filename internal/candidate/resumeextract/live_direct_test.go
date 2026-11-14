//go:build llmlive

package resumeextract

import (
	"context"
	"strings"
	"testing"
	"time"
)

// Exercise the production extractor, including the schema, decoder, date checks and
// sanitation. These synthetic CVs contain no real candidate data.
func TestLive_DirectCVExtraction(t *testing.T) {
	client := liveClient(t).WithTimeout(120 * time.Second)
	for _, tc := range []struct {
		name, cv, fullName, email, location, link string
		phoneDigits                               string
	}{
		{
			name: "english",
			cv: `Jane Doe
Backend Engineer
jane.doe@example.com | +1 415 555 0123 | https://github.com/janedoe
Residence: Lisbon, Portugal
Experience
Backend Engineer - Acme
05/2016 - 06/2024
Built Go APIs backed by PostgreSQL.
Skills: Go, PostgreSQL`,
			fullName: "Jane Doe", email: "jane.doe@example.com", location: "Lisbon",
			link: "https://github.com/janedoe", phoneDigits: "14155550123",
		},
		{
			name: "cyrillic",
			cv: `Иван Петров
Backend Engineer
ivan.petrov@example.com | +7 999 123 45 67 | https://github.com/ivanpetrov
Место проживания: Berlin, Germany
Опыт работы
Backend Engineer - Acme
05/2016 - 06/2024
Разрабатывал API на Go с PostgreSQL.
Навыки: Go, PostgreSQL`,
			fullName: "Иван Петров", email: "ivan.petrov@example.com", location: "Berlin",
			link: "https://github.com/ivanpetrov", phoneDigits: "79991234567",
		},
		{
			name: "portuguese",
			cv: `João da Silva
Backend Engineer
joao.silva@example.com | +55 11 99999 1234 | https://github.com/joaosilva
Residência: Florianópolis, Brazil
Experiência
Backend Engineer - Acme
05/2016 - 06/2024
Desenvolveu APIs em Go com PostgreSQL.
Competências: Go, PostgreSQL`,
			fullName: "João da Silva", email: "joao.silva@example.com", location: "Florianópolis",
			link: "https://github.com/joaosilva", phoneDigits: "5511999991234",
		},
		{
			name: "absent_contacts",
			cv: `Backend Engineer
Experience
Backend Engineer - Acme
05/2016 - 06/2024
Built Go APIs backed by PostgreSQL.
Skills: Go, PostgreSQL`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
			defer cancel()
			started := time.Now()
			got, err := NewExtractor(client).Extract(ctx, tc.cv)
			if err != nil {
				t.Fatalf("Extract: %v", err)
			}
			t.Logf("elapsed=%s roles=%d skills=%d", time.Since(started).Round(time.Millisecond), len(got.Experience), len(got.Skills))
			if got.FullName != tc.fullName || got.Email != tc.email {
				t.Errorf("name/email = %q/%q, want %q/%q", got.FullName, got.Email, tc.fullName, tc.email)
			}
			if !strings.Contains(got.Location, tc.location) {
				t.Errorf("location = %q, want it to include %q", got.Location, tc.location)
			}
			digits := strings.Map(func(r rune) rune {
				if r >= '0' && r <= '9' {
					return r
				}
				return -1
			}, got.Phone)
			if digits != tc.phoneDigits {
				t.Errorf("phone digits = %q, want %q", digits, tc.phoneDigits)
			}
			if tc.link == "" {
				if got.Location != "" || len(got.Links) != 0 {
					t.Errorf("invented missing location/links: %q/%v", got.Location, got.Links)
				}
			} else if len(got.Links) != 1 || got.Links[0] != tc.link {
				t.Errorf("links = %v, want [%s]", got.Links, tc.link)
			}
			if len(got.Experience) != 1 {
				t.Fatalf("roles = %d, want 1", len(got.Experience))
			}
			e := got.Experience[0]
			if e.Company != "Acme" || e.Start == nil || e.End == nil {
				t.Fatalf("role identity/dates missing: %+v", e)
			}
			if e.Start.Year != 2016 || e.Start.Month != 5 || e.End.Year != 2024 || e.End.Month != 6 || e.Current {
				t.Errorf("employment dates changed: %+v", e)
			}
			if len(got.Skills) < 2 {
				t.Errorf("skills lost: %v", got.Skills)
			}
		})
	}
}
