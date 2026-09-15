package searchping

import "testing"

func TestGoogleRefusesCompanyPages(t *testing.T) {
	g := &GoogleEngine{}
	if g.Accepts(KindCompany) {
		t.Fatal("Google must refuse company pages — its Indexing API admits only JobPosting")
	}
	for _, k := range []Kind{KindCreated, KindClosed} {
		if !g.Accepts(k) {
			t.Fatalf("Google must accept %s", k)
		}
	}
	i := &IndexNowEngine{}
	for _, k := range []Kind{KindCreated, KindClosed, KindCompany} {
		if !i.Accepts(k) {
			t.Fatalf("IndexNow must accept %s", k)
		}
	}
}
