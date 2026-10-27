package sources

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"

	"golang.org/x/net/html"
)

// homerunLaneFake is one egress lane: it counts the requests it was sent and answers each with
// err (nil = served).
type homerunLaneFake struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (f *homerunLaneFake) answer() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.err
}

func (f *homerunLaneFake) GetXML(context.Context, string, any) error { return f.answer() }

func (f *homerunLaneFake) GetHTML(context.Context, string) (*html.Node, error) {
	if err := f.answer(); err != nil {
		return nil, err
	}
	return &html.Node{Type: html.DocumentNode}, nil
}

func (f *homerunLaneFake) sent() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func homerunRefusal(code int) error { return &StatusError{Method: http.MethodGet, Code: code} }

func TestHomerunEgressStaysDirectWhileServed(t *testing.T) {
	direct, proxy := &homerunLaneFake{}, &homerunLaneFake{}
	g := newHomerunEgress(direct, proxy)
	for range 3 {
		if err := g.GetXML(context.Background(), "https://feed.homerun.co/a", nil); err != nil {
			t.Fatalf("GetXML: %v", err)
		}
	}
	if direct.sent() != 3 || proxy.sent() != 0 {
		t.Errorf("direct=%d proxy=%d, want 3 and 0", direct.sent(), proxy.sent())
	}
}

// The 2026-10-25..27 runs: once Homerun's edge refuses the crawl host it refuses every request
// for hours, and the runs kept sending one every 500 ms — 138 to 215 failed boards a run. The
// first refusal must move the REST of the run to the proxy, and the refused request with it.
func TestHomerunEgressMovesTheRunToTheProxyOnTheFirstRefusal(t *testing.T) {
	for _, code := range []int{http.StatusForbidden, http.StatusTooManyRequests} {
		direct, proxy := &homerunLaneFake{err: homerunRefusal(code)}, &homerunLaneFake{}
		g := newHomerunEgress(direct, proxy)
		if err := g.GetXML(context.Background(), "https://feed.homerun.co/a", nil); err != nil {
			t.Fatalf("%d: the refused request must be retried on the proxy, got %v", code, err)
		}
		for range 5 {
			if err := g.GetXML(context.Background(), "https://feed.homerun.co/b", nil); err != nil {
				t.Fatalf("%d: GetXML: %v", code, err)
			}
			if _, err := g.GetHTML(context.Background(), "https://b.homerun.co/x"); err != nil {
				t.Fatalf("%d: GetHTML: %v", code, err)
			}
		}
		if direct.sent() != 1 {
			t.Errorf("%d: direct got %d requests, want exactly the one it refused", code, direct.sent())
		}
		if proxy.sent() != 11 {
			t.Errorf("%d: proxy got %d requests, want 11", code, proxy.sent())
		}
	}
}

// With no proxy lane the first refusal stops the run's Homerun traffic outright: every later
// request fails without being sent, so the block is not prolonged by our own requests.
func TestHomerunEgressWithoutProxyStopsOnTheFirstRefusal(t *testing.T) {
	direct := &homerunLaneFake{err: homerunRefusal(http.StatusForbidden)}
	g := newHomerunEgress(direct, nil)
	err := g.GetXML(context.Background(), "https://feed.homerun.co/a", nil)
	var se *StatusError
	if !errors.As(err, &se) || se.Code != http.StatusForbidden {
		t.Fatalf("first refusal = %v, want the 403 itself", err)
	}
	for range 5 {
		if err := g.GetXML(context.Background(), "https://feed.homerun.co/b", nil); !errors.Is(err, errHomerunStopped) {
			t.Fatalf("after the refusal: %v, want errHomerunStopped", err)
		}
		if _, err := g.GetHTML(context.Background(), "https://b.homerun.co/x"); !errors.Is(err, errHomerunStopped) {
			t.Fatalf("after the refusal: %v, want errHomerunStopped", err)
		}
	}
	if direct.sent() != 1 {
		t.Errorf("direct got %d requests, want 1", direct.sent())
	}
}

// A failure that is not a refusal says nothing about the crawl host and must not switch lanes.
func TestHomerunEgressIgnoresOrdinaryFailures(t *testing.T) {
	for _, err := range []error{homerunRefusal(http.StatusNotFound), homerunRefusal(http.StatusBadGateway), errors.New("timeout")} {
		direct, proxy := &homerunLaneFake{err: err}, &homerunLaneFake{}
		g := newHomerunEgress(direct, proxy)
		_ = g.GetXML(context.Background(), "https://feed.homerun.co/a", nil)
		_ = g.GetXML(context.Background(), "https://feed.homerun.co/b", nil)
		if direct.sent() != 2 || proxy.sent() != 0 {
			t.Errorf("%v: direct=%d proxy=%d, want 2 and 0", err, direct.sent(), proxy.sent())
		}
	}
}

// The proxy lane is the ROTATING gateway: a fresh exit per request, so one refused exit says
// nothing about the next and must not stop the run.
func TestHomerunEgressKeepsTheProxyLaneAfterAProxyRefusal(t *testing.T) {
	direct := &homerunLaneFake{err: homerunRefusal(http.StatusForbidden)}
	proxy := &homerunLaneFake{err: homerunRefusal(http.StatusForbidden)}
	g := newHomerunEgress(direct, proxy)
	if err := g.GetXML(context.Background(), "https://feed.homerun.co/a", nil); err == nil {
		t.Fatal("both lanes refused: want an error")
	}
	proxy.mu.Lock()
	proxy.err = nil
	proxy.mu.Unlock()
	if err := g.GetXML(context.Background(), "https://feed.homerun.co/b", nil); err != nil {
		t.Fatalf("next request on the proxy lane: %v", err)
	}
	if direct.sent() != 1 {
		t.Errorf("direct got %d requests, want 1", direct.sent())
	}
}

// A page the stopped lane did not fetch is deferred, not stored as the bare feed entry: a stored
// vacancy's page is never read again, so storing it now would lose its address for good.
func TestHomerunDefersANewVacancyWhenTheLaneHasStopped(t *testing.T) {
	feed := (&routedHTTP{}).route("feed.homerun.co/autokan", homerunFixture(t, "feed_autokan.xml")).
		routeErr("www.werkenbijautokan.nl/", errHomerunStopped)
	jobs, err := NewHomerun(feed).(HydratingSource).FetchNew(context.Background(), homerunAutokanEntry,
		func(id string) bool { return id == "job_W4O6B45ovsKe11fvXCMV" })
	if err != nil {
		t.Fatalf("FetchNew: %v", err)
	}
	if len(jobs) != 1 || !jobs[0].SeenRefresh {
		t.Errorf("jobs = %+v, want only the seen vacancy's refresh", jobs)
	}
}

// ApplyProxyEgress must give homerun the rotating lane when the gateway is configured; without
// it the switch has nowhere to go and the first refusal stops the run.
func TestApplyProxyEgressGivesHomerunTheRotatingLane(t *testing.T) {
	t.Setenv("SOURCES_PROXY_URL", "http://user:pass@proxy.example:8080")
	t.Setenv("SOURCES_ROTATING_PROXY_URL", "http://user-rotate:pass@proxy.example:80")
	registry := All(NewClient())
	if err := ApplyProxyEgress(registry); err != nil {
		t.Fatalf("ApplyProxyEgress: %v", err)
	}
	h, ok := registry["homerun"].(homerun)
	if !ok {
		t.Fatalf("homerun = %T", registry["homerun"])
	}
	paced, ok := h.http.(pacedHomerun)
	if !ok {
		t.Fatalf("homerun transport = %T, want the pacer outermost", h.http)
	}
	eg, ok := paced.homerunGetter.(*homerunEgress)
	if !ok || eg.proxy == nil {
		t.Fatalf("homerun egress = %#v, want a proxy lane", paced.homerunGetter)
	}

	t.Setenv("SOURCES_ROTATING_PROXY_URL", "")
	registry = All(NewClient())
	if err := ApplyProxyEgress(registry); err != nil {
		t.Fatalf("ApplyProxyEgress: %v", err)
	}
	eg = registry["homerun"].(homerun).http.(pacedHomerun).homerunGetter.(*homerunEgress)
	if eg.proxy != nil {
		t.Error("without the rotating gateway homerun must not borrow the fixed exit")
	}
}
