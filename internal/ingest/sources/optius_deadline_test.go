package sources

import (
	"context"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestOptiusRefreshReadsDeadlineWithoutFetchingDetail(t *testing.T) {
	f := &optiusFake{}
	jobs, err := newTestOptius(f).FetchNew(context.Background(), opEntry, func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]string{opProjektant: "2026-11-13", opKrka: "2026-10-31", opKariera: "2026-11-13", opAustria: "2026-11-25"} {
		j := optiusByID(jobs)[id]
		if j.ApplicationDeadline == nil || j.ApplicationDeadline.Format("2006-01-02") != want {
			t.Errorf("%s deadline = %v, want %s from the listing", id, j.ApplicationDeadline, want)
		}
		if !j.SeenRefresh {
			t.Errorf("%s should remain a content-preserving refresh", id)
		}
	}
	if got := f.postingRequests(); got != 0 {
		t.Errorf("posting requests = %d, want 0", got)
	}
}

func TestOptiusListingDeadlineFormats(t *testing.T) {
	for _, tc := range []struct{ value, want string }{
		{"4. 11. 2026", "2026-11-04"},
		{"04.11.2026", "2026-11-04"},
		{" 4.\u00a011.\u00a02026 ", "2026-11-04"},
		{"31. 2. 2026", ""},
		{"", ""},
	} {
		t.Run(tc.value, func(t *testing.T) {
			root, err := html.Parse(strings.NewReader(`<li class="item" id="job1"><a class="hover-link" href="/iskalci/prosta-delovna-mesta/engineer-1-1/"></a><h3>Engineer</h3><div class="company-name">Acme</div><ul class="job-infos"><li><div class="left">Prijave do</div><div class="right"><span class="date">` + tc.value + `</span><span>Še 15 dni</span></div></li></ul></li>`))
			if err != nil {
				t.Fatal(err)
			}
			cards := optiusCards(root)
			if len(cards) != 1 {
				t.Fatalf("cards = %v, want one", cards)
			}
			got := ""
			if d := optiusBaseJob(cards[0]).ApplicationDeadline; d != nil {
				got = d.Format("2006-01-02")
			}
			if got != tc.want {
				t.Errorf("deadline = %q, want %q", got, tc.want)
			}
		})
	}
}
