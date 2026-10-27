package sources

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sync"

	"golang.org/x/net/html"
)

// errHomerunStopped is what a Homerun request answers, without being sent, once the run's
// traffic to Homerun has stopped (see homerunEgress).
var errHomerunStopped = errors.New("homerun: the crawl host was refused earlier this run; request not sent")

// homerunEgress picks the egress every Homerun request of one run takes, and owns the one
// decision no single board can make: what to do once Homerun's edge refuses the crawl host.
//
// That refusal is not per request. Measured on host2 2026-10-25..27: five of six runs drew a
// first 403 between 60 and 220 seconds in (~120 to ~440 feed requests at 2 a second), and from
// then on every request was refused, 500 ms apart, until the run ran out of boards — 138, 215,
// 209, 158 and 201 failed boards. The block had lifted by the next run, hours later, and through
// the rotating proxy gateway the same feeds answered 200 all along. Sending the rest of a run
// into the refusal both fails every board it touches and very likely keeps the block in force.
//
// So the first 403 or 429 on the direct IP moves the whole rest of the run — and the request that
// drew it — to the proxy lane: the ROTATING gateway, a fresh exit per request, so no single
// address accumulates the run's volume. A refusal there fails only that request, since the next
// one leaves from another address. Without a proxy lane the first refusal STOPS the run's
// Homerun traffic: every later request fails at once with errHomerunStopped, unsent.
//
// The state is per process, and cmd/ingest crawls one provider per process, so it binds within
// one run; every run re-probes the direct IP, which costs one refused request if the block holds.
type homerunEgress struct {
	direct homerunGetter
	proxy  homerunGetter // the rotating gateway; nil = no proxy lane

	mu   sync.Mutex
	lane homerunLane
	sent [homerunLaneStopped]int // requests sent per lane
}

type homerunLane int

const (
	homerunLaneDirect homerunLane = iota
	homerunLaneProxy
	homerunLaneStopped
)

// newHomerunEgress wires the run's egress; a nil proxy means the first refusal stops the run.
func newHomerunEgress(direct, proxy homerunGetter) *homerunEgress {
	return &homerunEgress{direct: direct, proxy: proxy}
}

func (g *homerunEgress) GetXML(ctx context.Context, url string, v any) error {
	return g.send(func(c homerunGetter) error { return c.GetXML(ctx, url, v) })
}

func (g *homerunEgress) GetHTML(ctx context.Context, url string) (*html.Node, error) {
	var root *html.Node
	err := g.send(func(c homerunGetter) error {
		var err error
		root, err = c.GetHTML(ctx, url)
		return err
	})
	return root, err
}

// send issues one request on the current lane, and when the direct IP refuses it, once more on
// whatever lane the refusal moved the run to.
func (g *homerunEgress) send(do func(homerunGetter) error) error {
	lane, c := g.acquire()
	if c == nil {
		return errHomerunStopped
	}
	err := do(c)
	if !homerunRefused(err) {
		return err
	}
	g.refused(lane)
	if lane != homerunLaneDirect {
		return err
	}
	// After a direct refusal the run is on the proxy lane or stopped, never still direct.
	if _, retry := g.acquire(); retry != nil {
		return do(retry)
	}
	return err
}

// acquire returns the lane a request takes and its getter, nil once the run has stopped.
func (g *homerunEgress) acquire() (homerunLane, homerunGetter) {
	g.mu.Lock()
	defer g.mu.Unlock()
	switch g.lane {
	case homerunLaneDirect:
		g.sent[g.lane]++
		return g.lane, g.direct
	case homerunLaneProxy:
		g.sent[g.lane]++
		return g.lane, g.proxy
	}
	return g.lane, nil
}

// refused records a refusal on lane. Only a refusal on the CURRENT direct lane moves the run:
// requests already in flight on it when the switch happened come back refused as well, and the
// proxy lane's refusals are one exit each.
func (g *homerunEgress) refused(lane homerunLane) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if lane != homerunLaneDirect || g.lane != homerunLaneDirect {
		return
	}
	if g.proxy != nil {
		g.lane = homerunLaneProxy
		log.Printf("homerun: the direct IP was refused after %d requests this run; "+
			"the rest of the run goes through the rotating proxy", g.sent[homerunLaneDirect])
		return
	}
	g.lane = homerunLaneStopped
	log.Printf("homerun: the direct IP was refused after %d requests this run and no rotating "+
		"proxy is configured (SOURCES_ROTATING_PROXY_URL); no further Homerun request is sent "+
		"this run", g.sent[homerunLaneDirect])
}

// homerunRefused reports Homerun's edge turning the request away for who sent it rather than
// for what it asked: a 403 (the block) or a 429 (the rate refusal that precedes it).
func homerunRefused(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && (se.Code == http.StatusForbidden || se.Code == http.StatusTooManyRequests)
}
