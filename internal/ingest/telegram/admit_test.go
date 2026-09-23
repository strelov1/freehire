package telegram

import "testing"

type fakeMatcher struct{ hit bool }

func (m fakeMatcher) Matches([]Link) bool { return m.hit }

// TestAdmitsPost pins the ONE rule that decides whether a crawled post enters the
// extraction queue. It exists because that rule now has two readers — the crawl
// (cmd/tg-ingest) and the re-filter backfill (cmd/backfill-telegram-prefilter) — and a
// rule held by one reader is not a rule: the backfill deciding differently from the crawl
// would requeue posts the next crawl would refuse, or leave behind posts it would admit.
func TestAdmitsPost(t *testing.T) {
	links := []Link{{URL: "https://boards.greenhouse.io/acme/jobs/1"}}

	cases := []struct {
		name  string
		text  string
		links []Link
		m     LinkMatcher
		want  bool
	}{
		{"text alone admits", "Вакансия: Go разработчик", nil, nil, true},
		{"link alone admits", "Свежая подборка на сайте 👇", links, fakeMatcher{hit: true}, true},
		{"neither admits", "Пятница! Всем хороших выходных 🎉", links, fakeMatcher{hit: false}, false},
		{"both admit", "Вакансия: Go разработчик", links, fakeMatcher{hit: true}, true},
		// A nil matcher is the crawl's own "no registry configured" case, and it must not
		// panic — hasDestinationLink guarded for this before the rule was extracted.
		{"nil matcher falls back to text", "Вакансия: Go разработчик", links, nil, true},
		{"nil matcher cannot admit on links", "Пятница!", links, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := AdmitsPost(tc.text, tc.links, tc.m); got != tc.want {
				t.Errorf("AdmitsPost(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}
