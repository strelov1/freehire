package sources

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// laneExit is one pinned exit of a fake pool: refused exits answer 403, the rest 200, and a URL
// ending in "gone" answers 404 on any exit.
type laneExit struct {
	n     int
	pool  *lanePool
	calls int
}

type lanePool struct {
	mu      sync.Mutex
	refused map[int]bool
	exits   map[int]*laneExit
}

func (e *laneExit) GetJSONWithHeaders(_ context.Context, u string, _ map[string]string, _ any) error {
	e.pool.mu.Lock()
	defer e.pool.mu.Unlock()
	e.calls++
	if strings.HasSuffix(u, "gone") {
		return &StatusError{Method: "GET", Code: 404, URL: u}
	}
	if e.pool.refused[e.n] {
		return &StatusError{Method: "GET", Code: 403, URL: u}
	}
	return nil
}

func newTestLane(exits int, refused ...int) (*mojposaoLane, *lanePool) {
	p := &lanePool{refused: map[int]bool{}, exits: map[int]*laneExit{}}
	for _, n := range refused {
		p.refused[n] = true
	}
	l := &mojposaoLane{exits: exits, cur: 1, clients: map[int]HeaderJSONGetter{}}
	l.exit = func(n int) HeaderJSONGetter {
		e := &laneExit{n: n, pool: p}
		p.exits[n] = e
		return e
	}
	return l, p
}

func (p *lanePool) calls(n int) int {
	if e, ok := p.exits[n]; ok {
		return e.calls
	}
	return 0
}

func TestMojposaoLaneKeepsAServedExit(t *testing.T) {
	l, p := newTestLane(10, 1, 2)
	for i := 0; i < 5; i++ {
		if err := l.GetJSONWithHeaders(context.Background(), "https://x/ok", nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	if p.calls(1) != 1 || p.calls(2) != 1 || p.calls(3) != 5 {
		t.Errorf("calls per exit = %d/%d/%d, want the two refused exits tried once and exit 3 kept", p.calls(1), p.calls(2), p.calls(3))
	}
	if l.requests.Load() != 7 || l.refused.Load() != 2 {
		t.Errorf("requests/refused = %d/%d, want 7/2", l.requests.Load(), l.refused.Load())
	}
}

func TestMojposaoLaneReturnsGoneAtOnce(t *testing.T) {
	l, p := newTestLane(10)
	err := l.GetJSONWithHeaders(context.Background(), "https://x/gone", nil, nil)
	var se *StatusError
	if !errors.As(err, &se) || se.Code != 404 {
		t.Fatalf("err = %v, want the 404", err)
	}
	if p.calls(1) != 1 || l.cur != 1 {
		t.Errorf("a 404 moved the lane: calls=%d cur=%d", p.calls(1), l.cur)
	}
}

func TestMojposaoLaneBoundsTriesAndStopsAfterRepeatedRefusals(t *testing.T) {
	refused := make([]int, 0, 100)
	for n := 1; n <= 100; n++ {
		refused = append(refused, n)
	}
	l, _ := newTestLane(100, refused...)
	if err := l.GetJSONWithHeaders(context.Background(), "https://x/a", nil, nil); err == nil {
		t.Fatal("a request no exit served succeeded")
	}
	if got := l.requests.Load(); got != mojposaoLaneTries {
		t.Errorf("tries = %d, want %d", got, mojposaoLaneTries)
	}
	for i := 0; i < 3; i++ {
		_ = l.GetJSONWithHeaders(context.Background(), "https://x/b", nil, nil)
	}
	if got := l.requests.Load(); got != mojposaoLaneMaxRefusals {
		t.Errorf("requests = %d, want the lane to stop at %d consecutive refusals", got, mojposaoLaneMaxRefusals)
	}
	if err := l.GetJSONWithHeaders(context.Background(), "https://x/c", nil, nil); !errors.Is(err, errMojposaoLaneExhausted) {
		t.Errorf("err = %v, want the lane exhausted", err)
	}
}

func TestMojposaoLaneWrapsAroundThePool(t *testing.T) {
	l, p := newTestLane(3, 3)
	l.cur = 3
	if err := l.GetJSONWithHeaders(context.Background(), "https://x/ok", nil, nil); err != nil {
		t.Fatal(err)
	}
	if p.calls(1) != 1 {
		t.Errorf("after the last exit the lane did not wrap to exit 1")
	}
}

func TestNewMojposaoLanePinsWebshareExits(t *testing.T) {
	u, _ := url.Parse("http://acct-rotate:secret@p.example:80")
	l := newMojposaoLane(u)
	if l.exits != mojposaoLanePoolExits || l.cur < 1 || l.cur > l.exits {
		t.Errorf("exits/cur = %d/%d", l.exits, l.cur)
	}
	plain, _ := url.Parse("http://acct:secret@p.example:80")
	if newMojposaoLane(plain).exits != 0 {
		t.Error("a gateway user without -rotate was treated as pinnable")
	}
}

func TestMojPosaoSendsOnlyTheCroatianMarketThroughTheLane(t *testing.T) {
	direct := &mojposaoFake{}
	laneFake := &mojposaoFake{}
	l := &mojposaoLane{exits: 1, cur: 1, clients: map[int]HeaderJSONGetter{}, exit: func(int) HeaderJSONGetter { return laneFake }}
	s := newTestMojPosao(direct)
	s.lane = l
	if _, err := s.FetchNew(context.Background(), CompanyEntry{Board: "mojposao.hr"}, func(string) bool { return false }); err != nil {
		t.Fatal(err)
	}
	if len(direct.lists)+len(direct.details) != 0 || len(laneFake.lists) == 0 || len(laneFake.details) == 0 {
		t.Errorf("hr: direct %d/%d lane %d/%d requests, want all through the lane", len(direct.lists), len(direct.details), len(laneFake.lists), len(laneFake.details))
	}
	if _, err := s.FetchNew(context.Background(), CompanyEntry{Board: "mojposao.ba"}, func(string) bool { return false }); err != nil {
		t.Fatal(err)
	}
	if len(direct.lists) == 0 || laneFake.badHeads != 0 || direct.badHeads != 0 {
		t.Errorf("ba: direct lists %d, bad headers lane/direct %d/%d", len(direct.lists), laneFake.badHeads, direct.badHeads)
	}
}
