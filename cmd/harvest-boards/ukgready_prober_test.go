package main

import (
	"context"
	"testing"
)

func TestUKGReadyProbe(t *testing.T) {
	p := ukgreadyProber{}
	getter := fakeGetter{
		"https://secure4.saashr.com/ta/rest/ui/recruitment/companies/%7C6162397/job-requisitions?offset=1&size=1": `{"_paging":{"offset":0,"size":1,"total":24},"job_requisitions":[{"id":1}]}`,
		"https://secure4.saashr.com/ta/rest/ui/recruitment/companies/%7C6161782/job-requisitions?offset=1&size=1": `{"_paging":{"offset":0,"size":1,"total":0},"job_requisitions":[]}`,
	}
	// A live board is counted by _paging.total, not by the one row asked for, and the employer
	// name is left to the seed.
	if name, n, err := p.probe(context.Background(), getter, "secure4.saashr.com/6162397"); err != nil || name != "" || n != 24 {
		t.Errorf("live: got (%q,%d,%v), want (\"\",24,nil)", name, n, err)
	}
	// A tenant whose portal carries no open posting is not a board worth committing.
	if _, n, err := p.probe(context.Background(), getter, "secure4.saashr.com/6161782"); err != nil || n != 0 {
		t.Errorf("empty: got n=%d err=%v, want 0,nil", n, err)
	}
	// A tenant with no career portal answers 410; the client reports an error and the prober
	// reads that as "not a live board" rather than aborting the harvest.
	if _, n, err := p.probe(context.Background(), getter, "secure4.saashr.com/9999999"); err != nil || n != 0 {
		t.Errorf("gone: got n=%d err=%v, want 0,nil", n, err)
	}
	// A board id that is not "<host>/<tenant>" addresses no URL this prober can build, and is
	// refused without a request rather than probed as some other tenant.
	for _, bad := range []string{"6162397", "secure4.saashr.com/", "/6162397", "secure4.saashr.com/6162397/extra"} {
		if _, n, err := p.probe(context.Background(), getter, bad); err != nil || n != 0 {
			t.Errorf("malformed %q: got n=%d err=%v, want 0,nil", bad, n, err)
		}
	}
}

func TestUKGReadyProberRegistered(t *testing.T) {
	p, ok := proberFor("ukgready")
	if !ok {
		t.Fatal(`proberFor("ukgready") not found`)
	}
	// The bespoke prober must win: the adapter fallback would run a whole crawl per candidate,
	// and UKG Ready detail bodies run to ~100 KB each.
	if _, fellBack := p.(adapterProber); fellBack {
		t.Errorf("ukgready resolved to the adapter fallback, got %T", p)
	}
}
