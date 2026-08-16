package sources

import (
	"testing"
	"time"
)

// The rate exists to fit a measured run inside the ingest unit's budget. If either number moves
// without the other being reconsidered, the crawl either keeps 403ing (too fast) or gets killed
// at TimeoutStartSec with the tail uncrawled (too slow) — and a killed run looks exactly like a
// platform outage in board_health.
func TestTeamtailorPaceFitsTheRunBudget(t *testing.T) {
	// Measured on prod 2026-08-16: 1987 boards, ~2 listing pages each, 33375 postings each
	// costing one detail page.
	const (
		requestsPerRun = 1987*2 + 33375
		unitTimeout    = 3000 * time.Second // freehire-ingest@.service TimeoutStartSec
	)

	perSecond := float64(time.Second) / float64(teamtailorRequestInterval)
	runtime := time.Duration(float64(requestsPerRun) / perSecond * float64(time.Second))

	if runtime > unitTimeout {
		t.Errorf("a full run takes %v at %.1f req/s, past the unit's %v budget — the tail would be "+
			"killed mid-crawl", runtime.Round(time.Second), perSecond, unitTimeout)
	}
	// The measured unpaced rate was ~62 req/s and it lost ~46%% of the fleet, so a "pace" that
	// close to it would not be a pace at all.
	if perSecond > 30 {
		t.Errorf("%.1f req/s is not meaningfully below the unpaced ~62 req/s that Teamtailor refused", perSecond)
	}
}

// Both call sites must pace: the registry one serves local/dev and any run without a proxy, the
// proxy one serves prod. Pacing only one leaves prod or dev firing the burst that caused this.
func TestTeamtailorPacedInBothWirings(t *testing.T) {
	direct := All(NewClient())["teamtailor"]
	if _, paced := unwrapTeamtailorGetter(direct).(rateLimitedHTMLGetter); !paced {
		t.Error("registry teamtailor is not paced")
	}

	t.Setenv("SOURCES_PROXY_URL", "http://user:pass@proxy.example:8080")
	registry := All(NewClient())
	if err := ApplyProxyEgress(registry); err != nil {
		t.Fatalf("ApplyProxyEgress: %v", err)
	}
	if _, paced := unwrapTeamtailorGetter(registry["teamtailor"]).(rateLimitedHTMLGetter); !paced {
		t.Error("proxied teamtailor is not paced")
	}
}

// unwrapTeamtailorGetter reaches the HTMLGetter the adapter was built over.
func unwrapTeamtailorGetter(s Source) HTMLGetter {
	tt, ok := s.(teamtailor)
	if !ok {
		return nil
	}
	return tt.http
}
