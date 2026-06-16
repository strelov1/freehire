package main

import (
	"context"
	"fmt"
	"slices"
	"testing"
)

func TestGupyRegisteredAsDiscoverer(t *testing.T) {
	p, ok := probers["gupy"]
	if !ok {
		t.Fatal(`probers["gupy"] missing`)
	}
	if _, isDiscoverer := p.(discoverer); !isDiscoverer {
		t.Error("gupy prober should implement the discoverer interface")
	}
}

func TestGupyProbe(t *testing.T) {
	g := gupyProber{}
	getter := fakeGetter{
		gupyFeedURL + "?companyId=316&limit=1":   `{"data":[{"careerPageName":"Vivo Digital","companyId":316}],"pagination":{"total":823}}`,
		gupyFeedURL + "?companyId=89896&limit=1": `{"data":[{"careerPageName":"Starian","companyId":89896}],"pagination":{"total":3}}`,
		// has jobs but the feed reports no name -> falls back to the companyId
		gupyFeedURL + "?companyId=555&limit=1": `{"data":[{"careerPageName":"","companyId":555}],"pagination":{"total":4}}`,
		// present company, zero open jobs -> skip
		gupyFeedURL + "?companyId=999&limit=1": `{"data":[],"pagination":{"total":0}}`,
	}

	cases := []struct {
		id       string
		wantName string
		wantN    int
	}{
		{"316", "Vivo Digital", 823},
		{"89896", "Starian", 3},
		{"555", "555", 4},
		{"999", "", 0},
		{"missing", "", 0}, // unmapped URL -> getter error -> skip, never fatal
	}
	for _, c := range cases {
		name, n, err := g.probe(context.Background(), getter, c.id)
		if err != nil {
			t.Errorf("probe(%s) err = %v, want nil", c.id, err)
		}
		if name != c.wantName || n != c.wantN {
			t.Errorf("probe(%s) = (%q, %d), want (%q, %d)", c.id, name, n, c.wantName, c.wantN)
		}
	}
}

func gupyPage(t *testing.T, ids ...int64) string {
	t.Helper()
	body := `{"data":[`
	for i, id := range ids {
		if i > 0 {
			body += ","
		}
		body += fmt.Sprintf(`{"companyId":%d,"careerPageName":"c%d"}`, id, id)
	}
	return body + `],"pagination":{"total":9999}}`
}

func TestGupyDiscoverPaginatesDedupsAndStops(t *testing.T) {
	g := gupyProber{}
	getter := fakeGetter{
		gupyFeedURL + fmt.Sprintf("?limit=%d&offset=0", gupyPageSize):                  gupyPage(t, 1, 2, 1),                      // 1 repeats within the page
		gupyFeedURL + fmt.Sprintf("?limit=%d&offset=%d", gupyPageSize, gupyPageSize):   gupyPage(t, 2, 3),                         // 2 repeats across pages
		gupyFeedURL + fmt.Sprintf("?limit=%d&offset=%d", gupyPageSize, 2*gupyPageSize): `{"data":[],"pagination":{"total":9999}}`, // empty page -> stop
	}

	got, err := g.discover(context.Background(), getter)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	want := []string{"1", "2", "3"} // distinct companyIds, first-seen order
	if !slices.Equal(got, want) {
		t.Errorf("discover() = %v, want %v (distinct, first-seen)", got, want)
	}
}

// A page that fails to fetch mid-sweep truncates the discovery to what was collected so far,
// rather than aborting the whole harvest.
func TestGupyDiscoverTruncatesOnPageError(t *testing.T) {
	g := gupyProber{}
	getter := fakeGetter{
		gupyFeedURL + fmt.Sprintf("?limit=%d&offset=0", gupyPageSize): gupyPage(t, 1, 2),
		// offset=gupyPageSize is unmapped -> getter error -> stop with [1 2] collected
	}

	got, err := g.discover(context.Background(), getter)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if want := []string{"1", "2"}; !slices.Equal(got, want) {
		t.Errorf("discover() = %v, want %v (truncated at the failed page)", got, want)
	}
}
