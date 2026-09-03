package linkedinprofile

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/platform/safehttp"
)

// ErrFetch means the page did not arrive: the request failed, the host answered with
// something other than 200, or the body ran past the cap. It is deliberately distinct
// from ErrNoProfile (a page that arrived and said nothing) and ErrNotAProfileURL (a
// request never made) — a run of these three means three different things, and only
// this one means LinkedIn stopped answering us.
var ErrFetch = errors.New("linkedinprofile: could not fetch the profile page")

const (
	fetchTimeout = 10 * time.Second
	// The profile pages measured on 2026-09-03 were 600-630 KB. This leaves room for
	// the page to grow several times over while keeping a hostile response bounded.
	defaultMaxBody = 4 << 20
	// We identify ourselves rather than wearing a browser's user-agent. This was
	// measured, not assumed: LinkedIn serves the same public page, with the same
	// JSON-LD, to this string as it does to Chrome.
	userAgent = "freehire/1.0 (+https://freehire.me)"
	// LinkedIn normalises a profile to one canonical page in a hop or two; more than
	// this is a loop or a redirector, neither of which leads to a profile.
	maxRedirects = 5
)

// Client reads public LinkedIn member profiles. The zero value is not usable; call
// NewClient.
type Client struct {
	httpClient *http.Client
	// base is the scheme and host to read profiles from. It exists so tests can
	// point at a stub, and is never varied in production.
	base    string
	maxBody int64
}

// NewClient returns a reader that fetches over the platform's guarded HTTP client —
// which refuses private address space, including across a redirect, so a profile URL
// cannot be turned into a request against our own network.
//
// It additionally refuses to follow a redirect off LinkedIn. That began as defence against
// a merely wrong outcome — another public host's JSON-LD read as the user's profile — and
// became load-bearing when the proxy went in: safehttp's guard vets the PROXY's address on
// a proxied transport, not the target's, because the proxy resolves the target itself. The
// two things that make a proxied fetch safe here are that publicID admits one host and this
// policy keeps it that way.
//
// Measured on 2026-09-03: LinkedIn answers a residential connection with 200 and answers the
// production host with 999 (its block status) and no JSON-LD, so the proxy is not an
// optimisation — without it this feature does not work in production at all.
func NewClient() *Client {
	c := safehttp.NewClientWithProxy(fetchTimeout, egressProxy())
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return fmt.Errorf("stopped after %d redirects", maxRedirects)
		}
		if !isProfileHost(req.URL.Hostname()) {
			return fmt.Errorf("refusing redirect off LinkedIn to %q", req.URL.Hostname())
		}
		return nil
	}
	return &Client{httpClient: c, base: profileBase, maxBody: defaultMaxBody}
}

// egressProxy reads the host's outbound proxy, or nil to go direct.
//
// It reads SOURCES_PROXY_URL — the crawl fleet's variable — deliberately, rather than
// introducing a second name. There is one proxy, configured once on the host; a
// LINKEDIN_PROXY_URL beside it would be a second thing to set, a second thing to rotate,
// and the day they disagree is the day one of them is silently wrong. The name is
// historical, not a scope: it says which subscription pays for the egress, not who may use
// it.
//
// A value that does not parse degrades to a direct fetch rather than failing startup. The
// crawl fleet fails fast on this because a worker that quietly crawls from the blocked IP
// looks like a source with no jobs; here the same mistake surfaces immediately and in
// words, as "LinkedIn didn't answer" on the very next import, with the reason logged.
func egressProxy() *url.URL {
	raw := strings.TrimSpace(os.Getenv("SOURCES_PROXY_URL"))
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		// Never log raw: the value carries the proxy's credentials.
		log.Printf("linkedinprofile: SOURCES_PROXY_URL is not a URL; reading LinkedIn directly, which production is blocked from")
		return nil
	}
	return u
}

// Fetch reads the public profile named by input — a profile URL in any of the forms a
// user pastes, or a bare public id — and returns what LinkedIn released.
//
// The request carries no credential of any kind. That is not a configuration choice
// but the premise: this reads what any stranger can read, so there is nothing for a
// session to add and nothing for LinkedIn to revoke.
//
// Errors are ErrNotAProfileURL (nothing was requested), ErrFetch (the page did not
// arrive), or ErrNoProfile (the page arrived and carried no readable profile).
func (c *Client) Fetch(ctx context.Context, input string) (Profile, error) {
	id, err := publicID(input)
	if err != nil {
		return Profile{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/in/"+id, nil)
	if err != nil {
		return Profile{}, fmt.Errorf("%w: %w", ErrFetch, err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return Profile{}, fmt.Errorf("%w: %w", ErrFetch, err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		return Profile{}, fmt.Errorf("%w: status %d", ErrFetch, res.StatusCode)
	}

	// Reading one byte past the cap is what tells a body that is exactly at the limit
	// from one that runs past it. A truncated page would parse, and would report
	// whatever happened to fit — a measurement nobody asked for.
	body, err := io.ReadAll(io.LimitReader(res.Body, c.maxBody+1))
	if err != nil {
		return Profile{}, fmt.Errorf("%w: %w", ErrFetch, err)
	}
	if int64(len(body)) > c.maxBody {
		return Profile{}, fmt.Errorf("%w: body exceeds %d bytes", ErrFetch, c.maxBody)
	}

	return parse(body, id)
}
