package sources

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/strelov1/freehire/internal/platform/scraplingfetch"
)

func scraplingEnv(t *testing.T) {
	t.Helper()
	t.Setenv("SCRAPLING_FETCH_URL", "http://127.0.0.1:8098")
	t.Setenv("SCRAPLING_FETCH_TOKEN", "tok")
}

// The allowlist is the spike's measured answer, one row per provider, and nothing else: a
// provider a plain client serves must not pay a browser for it.
func TestScraplingTierCarriesTheMeasuredProviders(t *testing.T) {
	var got []string
	for name := range scraplingProviders {
		got = append(got, name)
	}
	sort.Strings(got)
	want := []string{"bayt", "cadremploi", "clearedjobs", "cleverstaff", "clinch", "cryptojobslist", "echojobs", "epreselec", "foundit", "glints", "gradcracker", "gulftalent", "hh", "infojobs", "infojobsbr", "instahyre", "job104", "jobteaser", "kariyernet", "kontur", "levtech", "linkedin", "neogov", "occ", "pageup", "pracuj", "rwb", "senior", "spainjobs", "startupjobs", "turijobs", "vagas", "viterbit", "wantapply", "wellfound", "wuzzuf"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("scraplingProviders = %v, want %v", got, want)
	}
}

func TestApplyScraplingEgressIsANoOpWithoutAURL(t *testing.T) {
	t.Setenv("SCRAPLING_FETCH_URL", "")
	registry := map[string]Source{"bayt": NewBayt(nil), "infojobs": NewInfoJobs(nil)}
	before := map[string]Source{"bayt": registry["bayt"], "infojobs": registry["infojobs"]}
	if err := ApplyScraplingEgress(registry); err != nil {
		t.Fatalf("ApplyScraplingEgress: %v", err)
	}
	if !reflect.DeepEqual(registry, before) {
		t.Error("a provider was rewired without SCRAPLING_FETCH_URL")
	}
}

// A URL with no token is a half-configured deploy; refusing at startup beats every crawl
// through the tier failing with a 401 that looks like a refusal by the source.
func TestApplyScraplingEgressRefusesAURLWithoutAToken(t *testing.T) {
	t.Setenv("SCRAPLING_FETCH_URL", "http://127.0.0.1:8098")
	t.Setenv("SCRAPLING_FETCH_TOKEN", "")
	if err := ApplyScraplingEgress(map[string]Source{"bayt": NewBayt(nil)}); err == nil {
		t.Fatal("ApplyScraplingEgress: want an error for a URL without a token")
	}
}

func TestApplyScraplingEgressRewiresOnlyRegisteredProviders(t *testing.T) {
	scraplingEnv(t)
	other := NewGreenhouse(NewClient())
	registry := map[string]Source{"bayt": NewBayt(nil), "greenhouse": other}
	before := registry["bayt"]
	if err := ApplyScraplingEgress(registry); err != nil {
		t.Fatalf("ApplyScraplingEgress: %v", err)
	}
	if registry["bayt"] == before {
		t.Error("bayt was not rewired onto the Scrapling tier")
	}
	if registry["greenhouse"] != other {
		t.Error("a provider outside the allowlist was touched")
	}
	if _, added := registry["infojobs"]; added {
		t.Error("an allowlisted provider absent from the registry was added")
	}
}

// Each row's mode and proxy flag are what the spike measured; a wrong flag sends a provider
// down a path that is refused (gulftalent's sitemap to http+proxy got a challenge).
func TestScraplingRoutesAreTheMeasuredOnes(t *testing.T) {
	want := map[string]scraplingRoute{
		"clinch":         {scraplingfetch.ModeHTTP, true},
		"pageup":         {scraplingfetch.ModeHTTP, true},
		"rwb":            {scraplingfetch.ModeBrowser, false},
		"senior":         {scraplingfetch.ModeHTTP, true},
		"gradcracker":    {scraplingfetch.ModeHTTP, false},
		"infojobsbr":     {scraplingfetch.ModeHTTP, true},
		"spainjobs":      {scraplingfetch.ModeHTTP, true},
		"infojobs":       {scraplingfetch.ModeBrowser, true},
		"bayt":           {scraplingfetch.ModeBrowser, false},
		"gulftalent":     {scraplingfetch.ModeBrowser, false},
		"wantapply":      {scraplingfetch.ModeBrowser, false},
		"wellfound":      {scraplingfetch.ModeHTTP, true},
		"hh":             {scraplingfetch.ModeHTTP, false},
		"startupjobs":    {scraplingfetch.ModeBrowser, false},
		"cleverstaff":    {scraplingfetch.ModeHTTP, false},
		"kontur":         {scraplingfetch.ModeHTTP, true},
		"neogov":         {scraplingfetch.ModeHTTP, true},
		"linkedin":       {scraplingfetch.ModeHTTP, true},
		"echojobs":       {scraplingfetch.ModeBrowser, true},
		"wuzzuf":         {scraplingfetch.ModeBrowser, false},
		"occ":            {scraplingfetch.ModeBrowser, false},
		"vagas":          {scraplingfetch.ModeHTTP, true},
		"instahyre":      {scraplingfetch.ModeHTTP, false},
		"epreselec":      {scraplingfetch.ModeHTTP, true},
		"turijobs":       {scraplingfetch.ModeHTTP, false},
		"cadremploi":     {scraplingfetch.ModeHTTP, false},
		"clearedjobs":    {scraplingfetch.ModeHTTP, false},
		"pracuj":         {scraplingfetch.ModeHTTP, false},
		"jobteaser":      {scraplingfetch.ModeBrowser, false},
		"viterbit":       {scraplingfetch.ModeHTTP, false},
		"cryptojobslist": {scraplingfetch.ModeHTTP, false},
		"levtech":        {scraplingfetch.ModeHTTP, true},
		"foundit":        {scraplingfetch.ModeHTTP, true},
		"glints":         {scraplingfetch.ModeHTTP, false},
		"kariyernet":     {scraplingfetch.ModeHTTP, false},
		"job104":         {scraplingfetch.ModeHTTP, true},
	}
	for name, p := range scraplingProviders {
		if p.route != want[name] {
			t.Errorf("%s routes %+v, want %+v", name, p.route, want[name])
		}
	}
}

// wellfound's role slices are crawled concurrently; its in-flight cap holds through this tier
// as it did through the hosted one.
func TestWellfoundStaysConcurrencyLimited(t *testing.T) {
	scraplingEnv(t)
	registry := map[string]Source{"wellfound": NewWellfound(nil)}
	if err := ApplyScraplingEgress(registry); err != nil {
		t.Fatal(err)
	}
	wf, ok := registry["wellfound"].(wellfound)
	if !ok {
		t.Fatalf("wellfound rebuilt as %T", registry["wellfound"])
	}
	if _, limited := wf.http.(concurrencyLimitedHTMLGetter); !limited {
		t.Error("wellfound over the Scrapling tier is not concurrency-limited")
	}
}

// wuzzuf's search pages answer 429 after about ten in a minute, so they stay paced through
// this tier.
func TestWuzzufStaysPaced(t *testing.T) {
	scraplingEnv(t)
	registry := map[string]Source{"wuzzuf": NewWuzzuf(nil)}
	if err := ApplyScraplingEgress(registry); err != nil {
		t.Fatal(err)
	}
	w, ok := registry["wuzzuf"].(wuzzuf)
	if !ok {
		t.Fatalf("wuzzuf rebuilt as %T", registry["wuzzuf"])
	}
	if _, paced := w.http.(rateLimitedTextGetter); !paced {
		t.Error("wuzzuf over the Scrapling tier is not paced")
	}
}

// vagas stays paced through this tier: the rotating exit spreads its requests over many IPs,
// but the site's own budget is unknown and the pacer is what keeps a run polite.
func TestVagasStaysPaced(t *testing.T) {
	scraplingEnv(t)
	registry := map[string]Source{"vagas": NewVagas(nil)}
	if err := ApplyScraplingEgress(registry); err != nil {
		t.Fatal(err)
	}
	v, ok := registry["vagas"].(vagas)
	if !ok {
		t.Fatalf("vagas rebuilt as %T", registry["vagas"])
	}
	if _, paced := v.http.(rateLimitedHTMLGetter); !paced {
		t.Error("vagas over the Scrapling tier is not paced")
	}
}

// hh hydrates details through the tier and keeps listing on a plain direct client, never the
// shared proxied one — its listing was never blocked.
func TestHHListingStaysOnAPlainClient(t *testing.T) {
	scraplingEnv(t)
	t.Setenv("SOURCES_PROXY_URL", "http://proxy.invalid:1")
	registry := map[string]Source{"hh": NewHH(NewClient())}
	if err := ApplyScraplingEgress(registry); err != nil {
		t.Fatal(err)
	}
	h, ok := registry["hh"].(hh)
	if !ok {
		t.Fatalf("hh rebuilt as %T", registry["hh"])
	}
	if c, ok := h.http.(*Client); !ok || isScrapling(c) {
		t.Error("hh's listing must stay on a plain client")
	}
	if c, ok := h.detailHTTP.(*Client); !ok || !isScrapling(c) {
		t.Error("hh's detail pages must go through the Scrapling tier")
	}
}

func isScrapling(c *Client) bool {
	_, ok := c.httpClient.Transport.(*scraplingfetch.Transport)
	return ok
}

// The service already retries once and a browser fetch can take minutes, so the client's own
// retries would multiply a dead page into a quarter of an hour; and the outer timeout must not
// cut off the service's own error before it arrives.
func TestScraplingClientDoesNotRetryOrDoubleTime(t *testing.T) {
	for _, r := range []scraplingRoute{{scraplingfetch.ModeBrowser, false}, {scraplingfetch.ModeHTTP, true}} {
		c, err := newScraplingClient(scraplingfetch.Config{URL: "http://127.0.0.1:8098", Token: "t"}, r)
		if err != nil {
			t.Fatal(err)
		}
		if c.maxRetries != 0 {
			t.Errorf("%+v: maxRetries = %d, want 0", r, c.maxRetries)
		}
		if c.httpClient.Timeout != 0 {
			t.Errorf("%+v: outer timeout = %s, want none (the transport's own client bounds the fetch)", r, c.httpClient.Timeout)
		}
	}
}

// OCC's listing sits behind a Cloudflare challenge and its posting JSON does not, so only the
// listing goes through the tier; the JSON keeps a plain direct client.
func TestOCCPostingJSONStaysOffTheTier(t *testing.T) {
	scraplingEnv(t)
	registry := map[string]Source{"occ": NewOCC(nil)}
	if err := ApplyScraplingEgress(registry); err != nil {
		t.Fatal(err)
	}
	o, ok := registry["occ"].(occ)
	if !ok {
		t.Fatalf("occ rebuilt as %T", registry["occ"])
	}
	split, ok := o.http.(occSplit)
	if !ok {
		t.Fatalf("occ transport is %T, want occSplit", o.http)
	}
	if any(split.HTMLGetter) == any(split.JSONGetter) {
		t.Error("the posting JSON goes through the same client as the listing")
	}
}

func TestScraplingClientAllowsTimeForBrowserChallenges(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	for _, tc := range []struct {
		mode scraplingfetch.Mode
		want time.Duration
	}{{scraplingfetch.ModeHTTP, 90 * time.Second}, {scraplingfetch.ModeBrowser, 5 * time.Minute}} {
		t.Run(string(tc.mode), func(t *testing.T) {
			http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
				deadline, ok := req.Context().Deadline()
				remaining := time.Until(deadline)
				if !ok || remaining < tc.want-time.Second || remaining > tc.want {
					t.Errorf("fetch deadline has %s remaining, want approximately %s", remaining, tc.want)
				}
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"status":200,"url":"https://example.com","headers":{},"body":"b2s="}`))}, nil
			})
			c, err := newScraplingClient(scraplingfetch.Config{URL: "http://127.0.0.1:8098", Token: "test"}, scraplingRoute{mode: tc.mode})
			if err != nil {
				t.Fatal(err)
			}
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.com", nil)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := c.httpClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
		})
	}
}

func TestScraplingAggregatorBudgetsAndPacing(t *testing.T) {
	scraplingEnv(t)
	reg := map[string]Source{
		"gradcracker": NewGradcracker(nil, 0),
		"infojobsbr":  NewInfoJobsBR(nil),
		"spainjobs":   NewSpainJobs(nil),
	}
	if err := ApplyScraplingEgress(reg); err != nil {
		t.Fatal(err)
	}
	g := reg["gradcracker"].(gradcracker)
	paced, ok := g.http.(rateLimitedHTMLGetter)
	if !ok {
		t.Fatalf("Gradcracker getter %T is not paced", g.http)
	}
	limiter := paced.limiter.(*rate.Limiter)
	if limiter.Limit() != rate.Every(10*time.Second) || limiter.Burst() != 1 || g.maxNewPerRun != 200 {
		t.Fatal("Gradcracker pacing or hydration budget changed")
	}
	if c, ok := paced.inner.(*Client); !ok || !isScrapling(c) {
		t.Fatal("Gradcracker bypasses Scrapling")
	}
	i := reg["infojobsbr"].(infojobsBR)
	il := i.limiter.(*rate.Limiter)
	if il.Limit() != rate.Every(infojobsBRRequestInterval) || il.Burst() != 1 || i.maxNewPerBoard != 80 {
		t.Fatal("InfoJobs pacing or hydration budget changed")
	}
	s := reg["spainjobs"].(spainjobs)
	sl := s.limiter.(*rate.Limiter)
	if sl.Limit() != rate.Every(time.Second) || sl.Burst() != 1 || s.maxNewPerRun != 1500 || s.deadlineMargin != 5*time.Minute {
		t.Fatal("SpainJobs pacing or hydration budget changed")
	}
	if c, ok := s.http.(*Client); !ok || !isScrapling(c) {
		t.Fatal("SpainJobs bypasses Scrapling")
	}
}

// Run the real listing walk over the wire transport: without the bootstrap cookie the
// origin repeats page one, so a jar-less rebuild must fail this test.
func TestScraplingInfoJobsBRRetainsSessionAcrossFragments(t *testing.T) {
	pages := acreFake(t).pages
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			URL, Mode string
			Proxy     bool
			Headers   map[string]string
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		calls++
		if req.Mode != "http" || !req.Proxy {
			t.Error("wrong InfoJobs route")
		}
		key := req.URL
		headers := map[string]string{}
		if key == ijbrAcre {
			headers["Set-Cookie"] = "ab_session_id=test-session; Path=/; Secure; HttpOnly"
		} else {
			if req.Headers["Cookie"] != "ab_session_id=test-session" {
				t.Error("session cookie missing from fragment")
				w.WriteHeader(400)
				return
			}
			u, err := url.Parse(key)
			if err != nil {
				t.Error(err)
				return
			}
			key = u.Query().Get("url")
		}
		body, ok := pages[key]
		if !ok {
			t.Errorf("unexpected URL %s", req.URL)
			w.WriteHeader(400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  200,
			"url":     req.URL,
			"headers": headers,
			"body":    base64.StdEncoding.EncodeToString([]byte(body)),
		})
	}))
	defer server.Close()
	t.Setenv("SCRAPLING_FETCH_URL", server.URL)
	t.Setenv("SCRAPLING_FETCH_TOKEN", "test")
	reg := map[string]Source{"infojobsbr": NewInfoJobsBR(nil)}
	if err := ApplyScraplingEgress(reg); err != nil {
		t.Fatal(err)
	}
	s := reg["infojobsbr"].(infojobsBR)
	s.limiter = noWait{}
	cards, err := s.list(context.Background(), "ac")
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 7 || calls != 3 {
		t.Fatalf("cards=%d calls=%d, want seven Acre postings across three pages", len(cards), calls)
	}
}
