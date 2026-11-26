package qualityaudit

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// hostsClient sends every request to srv whatever its host, so a test can serve several named
// hosts — the pass tells a move by the host changing — from one server.
func hostsClient(srv *httptest.Server) *http.Client {
	addr := srv.Listener.Addr().String()
	return &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
	}}
}

func newFinder(direct, fallback *http.Client) *SuccessorFinder {
	return &SuccessorFinder{Direct: direct, Fallback: fallback, Name: fakeName, MaxRequests: 100, HostWorkers: 2, MaxCrawlDelay: time.Minute}
}

// fakeName stands in for cmd/quality-audit's atsboard/atsdetect namer (a layer above this
// package): tenant hosts of a few vendors, Lever's path, and a self-hosted marker in the body.
func fakeName(u *url.URL, body string) (provider, board string, ok bool) {
	host := u.Hostname()
	for _, vendor := range []string{"icims", "teamtailor", "bamboohr", "recruitee"} {
		if label, found := strings.CutSuffix(host, "."+vendor+".com"); found && label != "www" {
			return vendor, strings.TrimPrefix(label, "careers-"), true
		}
	}
	if host == "jobs.lever.co" {
		if seg, _, _ := strings.Cut(strings.TrimPrefix(u.Path, "/"), "/"); seg != "" {
			return "lever", seg, true
		}
	}
	for _, marker := range []string{"jibeapply", "teamtailor"} {
		if strings.Contains(body, marker) {
			return map[string]string{"jibeapply": "jibe", "teamtailor": "teamtailor"}[marker], host, true
		}
	}
	return "", "", false
}

// The uicalaska case: an iCIMS tenant whose posting page still answers, while its search frame
// sends the browser to a Jibe site on the employer's own domain.
func TestSuccessorFindsScriptRedirectToSelfHostedSite(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Host == "careers-acme.icims.com" && r.URL.Path == "/jobs/search":
			_, _ = w.Write([]byte(`<script>window.top.location.href = 'http:\/\/talent.acme.com\/acme\/jobs';</script>`))
		case r.Host == "careers-acme.icims.com":
			_, _ = w.Write([]byte(`<html>an iCIMS posting</html>`))
		case r.Host == "talent.acme.com" && r.URL.Path == "/acme/jobs":
			_, _ = w.Write([]byte(`<link href="/jibeapply/assets/app.css">`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	moves := newFinder(hostsClient(srv), nil).Run(context.Background(), []ChronicBoard{{
		Provider: "icims", Board: "acme", LatestURL: "http://careers-acme.icims.com/jobs/1/clerk/job",
	}})
	if len(moves) != 1 {
		t.Fatalf("moves = %+v, want one", moves)
	}
	m := moves[0]
	if m.Evidence != EvidencePageRedirect || m.SuccessorProvider != "jibe" || m.SuccessorBoard != "talent.acme.com" ||
		m.ProbedURL != "http://careers-acme.icims.com/jobs/search?in_iframe=1" {
		t.Fatalf("move = %+v", m)
	}
}

func TestSuccessorFindsHTTPRedirectToAnotherATS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host == "old.example.com" && r.URL.Path != "/robots.txt" {
			http.Redirect(w, r, "http://jobs.lever.co/acme", http.StatusMovedPermanently)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	moves := newFinder(hostsClient(srv), nil).Run(context.Background(), []ChronicBoard{{
		Provider: "teamtailor", Board: "old.example.com", LatestURL: "http://old.example.com/jobs/7",
	}})
	if len(moves) != 1 || moves[0].Evidence != EvidenceHTTPRedirect ||
		moves[0].SuccessorProvider != "lever" || moves[0].SuccessorBoard != "acme" {
		t.Fatalf("moves = %+v", moves)
	}
}

// A board that answers on its own host, or redirects to another host of the same board, has not
// moved.
func TestSuccessorIgnoresStayingBoards(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Host {
		case "jobs.lever.co":
			if r.URL.Path == "/acme/1" {
				http.Redirect(w, r, "http://jobs.lever.co/acme", http.StatusFound)
				return
			}
			_, _ = w.Write([]byte(`<html>still here</html>`))
		case "acme.teamtailor.com":
			http.Redirect(w, r, "http://acme.teamtailor.com/jobs", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	moves := newFinder(hostsClient(srv), nil).Run(context.Background(), []ChronicBoard{
		{Provider: "lever", Board: "acme", LatestURL: "http://jobs.lever.co/acme/1"},
		{Provider: "teamtailor", Board: "acme", LatestURL: "http://acme.teamtailor.com/jobs/1"},
	})
	if len(moves) != 0 {
		t.Fatalf("moves = %+v, want none", moves)
	}
}

// A refused first hop is retried through the fallback; a script target never is.
func TestSuccessorFallbackOnlyForRefusedFirstHop(t *testing.T) {
	direct := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Host {
		case "careers-acme.icims.com":
			w.WriteHeader(http.StatusForbidden)
		case "talent.acme.com":
			_, _ = w.Write([]byte(`jibeapply`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer direct.Close()
	var fallbackHosts []string
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackHosts = append(fallbackHosts, r.Host)
		_, _ = w.Write([]byte(`<script>location.replace("http://talent.acme.com/")</script>`))
	}))
	defer fallback.Close()

	moves := newFinder(hostsClient(direct), hostsClient(fallback)).Run(context.Background(), []ChronicBoard{{
		Provider: "icims", Board: "acme", LatestURL: "http://careers-acme.icims.com/jobs/1/clerk/job",
	}})
	if len(moves) != 1 || moves[0].SuccessorProvider != "jibe" {
		t.Fatalf("moves = %+v", moves)
	}
	for _, h := range fallbackHosts {
		if h != "careers-acme.icims.com" {
			t.Fatalf("fallback fetched %s, want only the refused first hop", h)
		}
	}
}

func TestPageRedirects(t *testing.T) {
	base, _ := url.Parse("https://a.example/x/")
	body := `<div onclick="window.location='https://apply.example/c/acme'"></div><button onClick='location.href="/back"'>x</button><button onclick=location.assign('https://apply.example/')>y</button><body onload="location.href='/loaded'"><script>const once = location.replace('https://c.example/jobs');</script><meta content="0; url=/reversed" http-equiv="refresh"><meta http-equiv="refresh" content="0; url=/moved"><script>if (location.href.indexOf('') >= 0) {}
window.location = "mailto:x@y"; location.assign('https://b.example/p?a=1&amp;b=2')</script>`
	var got []string
	for _, u := range pageRedirects(base, body) {
		got = append(got, u.String())
	}
	want := []string{"https://a.example/loaded", "https://c.example/jobs", "https://b.example/p?a=1&b=2", "https://a.example/moved", "https://a.example/reversed"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("pageRedirects = %v, want %v", got, want)
	}
}

// A dead hosted tenant redirected to the vendor's own site has not moved anywhere.
func TestSuccessorIgnoresVendorHomeRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host == "acme.bamboohr.com" && r.URL.Path != "/robots.txt" {
			http.Redirect(w, r, "http://www.bamboohr.com/", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	moves := newFinder(hostsClient(srv), nil).Run(context.Background(), []ChronicBoard{{
		Provider: "bamboohr", Board: "acme", LatestURL: "http://acme.bamboohr.com/careers/9",
	}})
	if len(moves) != 0 {
		t.Fatalf("moves = %+v, want none", moves)
	}
}

// A dead tenant sent to its vendor's site, which happens to carry another platform's name, has
// not moved to that platform.
func TestSuccessorIgnoresMarkerOnVendorSite(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Host == "acme.recruitee.com" && r.URL.Path != "/robots.txt":
			http.Redirect(w, r, "http://recruitee.com/", http.StatusFound)
		case r.Host == "recruitee.com":
			_, _ = w.Write([]byte(`compare us with teamtailor`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	moves := newFinder(hostsClient(srv), nil).Run(context.Background(), []ChronicBoard{{
		Provider: "recruitee", Board: "acme", LatestURL: "http://acme.recruitee.com/o/clerk",
	}})
	if len(moves) != 0 {
		t.Fatalf("moves = %+v, want none", moves)
	}
}

func TestNoisePage(t *testing.T) {
	for target, noise := range map[string]bool{
		"community.workday.com/maintenance-page": true,
		"login.prismhr-hire.com/users/login":     true,
		"app.careerplug.com/user/sign_in":        true,
		"careers.acme.com/jobs/registered-nurse": false,
		"talentconnect.acme.com/acme/jobs":       false,
	} {
		if noisePage.MatchString(target) != noise {
			t.Errorf("noisePage(%s) = %v, want %v", target, !noise, noise)
		}
	}
}

// An old posting sent to an unrecognised landing page whose script names the successor.
func TestSuccessorFollowsLandingPageScript(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Host == "acme.teamtailor.com" && r.URL.Path != "/robots.txt":
			http.Redirect(w, r, "http://www.acme.example/careers", http.StatusFound)
		case r.Host == "www.acme.example":
			_, _ = w.Write([]byte(`<script>location.replace("http://jobs.lever.co/acme")</script>`))
		case r.Host == "jobs.lever.co":
			_, _ = w.Write([]byte(`lever`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	moves := newFinder(hostsClient(srv), nil).Run(context.Background(), []ChronicBoard{{
		Provider: "teamtailor", Board: "acme", LatestURL: "http://acme.teamtailor.com/jobs/1",
	}})
	if len(moves) != 1 || moves[0].SuccessorProvider != "lever" || moves[0].SuccessorBoard != "acme" {
		t.Fatalf("moves = %+v", moves)
	}
}

// A self-hosted board moving within the employer's own domain keeps its marker read.
func TestSuccessorEmployerDomainMove(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Host == "careers.acme.example" && r.URL.Path != "/robots.txt":
			http.Redirect(w, r, "http://talent.acme.example/jobs", http.StatusFound)
		case r.Host == "talent.acme.example":
			_, _ = w.Write([]byte(`jibeapply`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	moves := newFinder(hostsClient(srv), nil).Run(context.Background(), []ChronicBoard{{
		Provider: "phenom", Board: "careers.acme.example", LatestURL: "http://careers.acme.example/job/1",
	}})
	if len(moves) != 1 || moves[0].SuccessorProvider != "jibe" || moves[0].SuccessorBoard != "talent.acme.example" {
		t.Fatalf("moves = %+v", moves)
	}
}

// A move already seen is kept when the budget runs out before the board's last probe.
func TestSuccessorKeepsMoveWhenBudgetEnds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host == "careers-acme.icims.com" && r.URL.Path != "/robots.txt" {
			http.Redirect(w, r, "http://www.acme.example/", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	board := ChronicBoard{Provider: "icims", Board: "acme", LatestURL: "http://careers-acme.icims.com/jobs/1/x/job"}
	for budget, want := range map[int64]int{2: 1, 1: 0} { // robots.txt, then the posting
		f := newFinder(hostsClient(srv), nil)
		f.MaxRequests = budget
		if moves := f.Run(context.Background(), []ChronicBoard{board}); len(moves) != want {
			t.Fatalf("budget %d: moves = %+v, want %d", budget, moves, want)
		}
	}
}
