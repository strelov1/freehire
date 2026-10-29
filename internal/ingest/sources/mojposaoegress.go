package sources

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// mojposaoLane is how the Croatian MojPosao market is reached at all. mojposao.hr sits behind a
// CloudFront WAF that answers "403 Request blocked" to both crawl hosts, to the fixed proxy exit
// and to the Scrapling tier, while the rotating gateway's exits split: of 24 exits pinned by
// number, 13 were served and 11 refused, and each kept its verdict across rounds (2026-10-29).
// So the lane pins one exit of the rotating gateway, keeps it while it is served, and moves to
// the next exit when it is refused — a handful of tries finds a served exit, where a fresh random
// exit per request would pay the ~45% refusal rate on every request.
//
// A request takes at most mojposaoLaneTries exits; a 404/410 is the platform's answer and is
// returned at once. After mojposaoLaneMaxRefusals consecutive refusals the lane refuses every
// later request of the run without sending it, so a WAF that has turned on the whole pool costs
// a few dozen requests, not one per posting.
type mojposaoLane struct {
	exit func(n int) HeaderJSONGetter // the client pinned to exit n (1-based)
	// exits is how many exits the pool numbers. 0 means the gateway cannot be pinned, and every
	// try takes a fresh rotating exit instead.
	exits int

	mu          sync.Mutex
	cur         int
	consecutive int
	clients     map[int]HeaderJSONGetter
	exhausted   bool

	requests, refused atomic.Int64
	bytes             atomic.Int64
}

const (
	mojposaoLaneTries        = 8
	mojposaoLaneMaxRefusals  = 20
	mojposaoLanePoolExits    = 250 // exits 1-250 answered through the gateway, 500 did not
	mojposaoLaneRotateSuffix = "-rotate"
)

// errMojposaoLaneExhausted is the lane's answer once the run has seen too many refusals in a row.
var errMojposaoLaneExhausted = errors.New("mojposao: proxy lane stopped after repeated refusals")

// newMojposaoLane builds the lane over the rotating gateway. A Webshare-style gateway names its
// rotating user "<user>-rotate" and each exit of the pool "<user>-<n>"; a gateway whose user does
// not end that way cannot be pinned, and the lane falls back to a fresh rotating exit per try.
func newMojposaoLane(rotating *url.URL) *mojposaoLane {
	l := &mojposaoLane{clients: map[int]HeaderJSONGetter{}}
	user := rotating.User.Username()
	pass, _ := rotating.User.Password()
	base, pinnable := strings.CutSuffix(user, mojposaoLaneRotateSuffix)
	if !pinnable || base == "" {
		c := l.counted(NewRotatingProxyClient(rotating))
		l.exit = func(int) HeaderJSONGetter { return c }
		return l
	}
	l.exits = mojposaoLanePoolExits
	l.cur = rand.IntN(l.exits) + 1
	l.exit = func(n int) HeaderJSONGetter {
		u := *rotating
		u.User = url.UserPassword(base+"-"+strconv.Itoa(n), pass)
		return l.counted(NewProxyClient(&u))
	}
	return l
}

// counted strips the client's own retries — a dead exit is the lane's to move past, not the
// client's to retry — and counts the bytes it reads, for the run's report.
func (l *mojposaoLane) counted(c *Client) *Client {
	c.maxRetries = 0
	inner := c.httpClient.Transport
	c.httpClient.Transport = mojposaoRoundTrip(func(req *http.Request) (*http.Response, error) {
		resp, err := inner.RoundTrip(req)
		if err == nil && resp.Body != nil {
			resp.Body = mojposaoCountingBody{ReadCloser: resp.Body, n: &l.bytes}
		}
		return resp, err
	})
	return c
}

type mojposaoRoundTrip func(*http.Request) (*http.Response, error)

func (f mojposaoRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type mojposaoCountingBody struct {
	io.ReadCloser
	n *atomic.Int64
}

func (b mojposaoCountingBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.n.Add(int64(n))
	return n, err
}

// client returns the exit to use now and its number.
func (l *mojposaoLane) client() (HeaderJSONGetter, int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.exhausted {
		return nil, 0, errMojposaoLaneExhausted
	}
	c, ok := l.clients[l.cur]
	if !ok {
		c = l.exit(l.cur)
		l.clients[l.cur] = c
	}
	return c, l.cur, nil
}

// served resets the refusal streak; refusedBy moves past exit n unless another request already
// has, and stops the lane once the streak is long enough.
func (l *mojposaoLane) served() {
	l.mu.Lock()
	l.consecutive = 0
	l.mu.Unlock()
}

func (l *mojposaoLane) refusedBy(n int) {
	l.refused.Add(1)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.exits > 0 && l.cur == n {
		l.cur = l.cur%l.exits + 1
	}
	l.consecutive++
	if l.consecutive >= mojposaoLaneMaxRefusals && !l.exhausted {
		l.exhausted = true
		log.Printf("mojposao: proxy lane refused %d times in a row; no further request this run", l.consecutive)
	}
}

func (l *mojposaoLane) GetJSONWithHeaders(ctx context.Context, u string, h map[string]string, v any) error {
	var last error
	for try := 0; try < mojposaoLaneTries; try++ {
		c, n, err := l.client()
		if err != nil {
			return err
		}
		l.requests.Add(1)
		err = c.GetJSONWithHeaders(ctx, u, h, v)
		if err == nil {
			l.served()
			return nil
		}
		if ctx.Err() != nil {
			return err
		}
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			l.served()
			return err
		}
		last = err
		l.refusedBy(n)
	}
	return fmt.Errorf("mojposao: no proxy exit served %s in %d tries: %w", u, mojposaoLaneTries, last)
}

// report logs what the lane spent this run.
func (l *mojposaoLane) report() {
	if r := l.requests.Load(); r > 0 {
		log.Printf("mojposao: proxy lane sent %d requests (%d refused), read %d KB", r, l.refused.Load(), l.bytes.Load()/1024)
	}
}
