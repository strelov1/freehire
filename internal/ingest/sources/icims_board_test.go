package sources

import (
	"context"
	"errors"
	"testing"
)

// icimsResolvingFake answers every sitemap fetch with a fixed final URL (or error).
type icimsResolvingFake struct {
	icimsHTTP
	final string
	err   error
}

func (f icimsResolvingFake) GetXMLResolved(_ context.Context, _ string, _ any) (string, error) {
	return f.final, f.err
}

func TestICIMSCanonicalBoardFollowsRedirectToAnotherICIMSHost(t *testing.T) {
	cases := []struct {
		name, board, final, want string
		err                      error
	}{
		{"vanity alias to vanity host", "securitycareers-alliedbarton.icims.com", "https://securitycareers-aus.icims.com/sitemap.xml", "securitycareers-aus.icims.com", nil},
		{"slug alias to classic host folds to slug", "pcso", "https://careers-polkcountysheriffsoffice.icims.com/sitemap.xml", "polkcountysheriffsoffice", nil},
		{"slug alias to vanity host", "templeschoolofmedicine", "https://lksomphysiciancareers-temple.icims.com/sitemap.xml", "lksomphysiciancareers-temple.icims.com", nil},
		{"no redirect keeps board", "Acme", "https://careers-acme.icims.com/sitemap.xml", "Acme", nil},
		{"redirect off iCIMS keeps board", "acme", "https://www.acme.com/sitemap.xml", "acme", nil},
		{"fetch error keeps board", "acme", "", "acme", errors.New("boom")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := NewICIMS(icimsResolvingFake{final: c.final, err: c.err}).(BoardCanonicalizer).CanonicalBoard(context.Background(), c.board)
			if err != nil || got != c.want {
				t.Fatalf("CanonicalBoard(%q) = %q, %v; want %q", c.board, got, err, c.want)
			}
		})
	}
	if got, _ := NewICIMS(nil).(BoardCanonicalizer).CanonicalBoard(context.Background(), "pcso"); got != "pcso" {
		t.Fatalf("no transport: got %q", got)
	}
}
