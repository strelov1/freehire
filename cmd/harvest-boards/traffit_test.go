package main

import (
	"context"
	"testing"
)

func TestTraffitRegistered(t *testing.T) {
	if _, ok := probers["traffit"]; !ok {
		t.Fatal(`probers["traffit"] missing`)
	}
}

func TestTraffitProbe(t *testing.T) {
	p := traffitProber{}
	getter := fakeGetter{
		"https://cloudfide.traffit.com/public/an/list/?limit=1": `{"count":5,"items":[{"advertId":50}]}`,
		// live tenant, zero open postings -> skip
		"https://empty.traffit.com/public/an/list/?limit=1": `{"count":0,"items":[]}`,
	}

	cases := []struct {
		slug     string
		wantName string
		wantN    int
	}{
		{"cloudfide", "cloudfide", 5},
		{"empty", "", 0},
		{"bogus", "", 0}, // unmapped URL (HTML placeholder in prod) -> getter error -> skip
	}
	for _, c := range cases {
		name, n, err := p.probe(context.Background(), getter, c.slug)
		if err != nil {
			t.Errorf("probe(%s) err = %v, want nil", c.slug, err)
		}
		if name != c.wantName || n != c.wantN {
			t.Errorf("probe(%s) = (%q, %d), want (%q, %d)", c.slug, name, n, c.wantName, c.wantN)
		}
	}
}
