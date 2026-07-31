package tracerlink

import (
	"regexp"
	"strings"
	"testing"
)

// The hosts a deployment serves, as configuration would supply them.
var ourHosts = []string{"freehire.me", "freehire.dev"}

func TestTargetsFindsHeaderAndProjectLinks(t *testing.T) {
	got := Targets(ourHosts,
		[]string{"github.com/ada", "https://linkedin.com/in/ada"},
		[]string{"opensched.dev"},
	)
	want := []struct{ path, url string }{
		{"header.links[0]", "https://github.com/ada"},
		{"header.links[1]", "https://linkedin.com/in/ada"},
		{"projects[0].link", "https://opensched.dev"},
	}
	if len(got) != len(want) {
		t.Fatalf("Targets() = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i].SourcePath() != want[i].path || got[i].URL != want[i].url {
			t.Errorf("Targets()[%d] = {%s %s}, want {%s %s}",
				i, got[i].SourcePath(), got[i].URL, want[i].path, want[i].url)
		}
	}
}

// A scheme-less link is what the CV actually stores, and the stored destination must be
// absolute: a relative URI is not something a redirect can send a visitor to.
func TestTargetsNormalisesToAbsoluteHTTPS(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"github.com/ada", "https://github.com/ada"},
		{"https://github.com/ada", "https://github.com/ada"},
		{"http://example.org/cv", "http://example.org/cv"},
		{"  github.com/ada  ", "https://github.com/ada"},
	} {
		got := Targets(ourHosts, []string{tc.in}, nil)
		if len(got) != 1 {
			t.Fatalf("Targets(%q) dropped the link: %+v", tc.in, got)
		}
		if got[0].URL != tc.want {
			t.Errorf("Targets(%q).URL = %q, want %q", tc.in, got[0].URL, tc.want)
		}
	}
}

// Anything that is not a web destination has nothing to trace: rewriting it would break a
// working mailto: or dial link and gain nothing.
func TestTargetsSkipsWhatCannotBeTraced(t *testing.T) {
	for _, in := range []string{
		"",
		"   ",
		"mailto:ada@example.com",
		"tel:+15550101",
		"javascript:alert(1)",
		"ftp://example.org/cv.pdf",
	} {
		if got := Targets(ourHosts, []string{in}, nil); len(got) != 0 {
			t.Errorf("Targets(%q) = %+v, want none", in, got)
		}
	}
}

// Index alignment is what lets the renderer put each href back where it came from, so a
// skipped link must not shift the ones after it.
func TestTargetsKeepsIndexesOfTheOriginalSlice(t *testing.T) {
	got := Targets(ourHosts, []string{"mailto:ada@example.com", "github.com/ada"}, []string{"", "opensched.dev"})
	want := []string{"header.links[1]", "projects[1].link"}
	if len(got) != len(want) {
		t.Fatalf("Targets() = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i].SourcePath() != want[i] {
			t.Errorf("Targets()[%d].SourcePath() = %q, want %q", i, got[i].SourcePath(), want[i])
		}
	}
}

// A link that already points at any host we serve is left alone: tracing it would nest a token
// inside a token. The set comes from configuration, so a second product domain is a deployment
// fact rather than a code change.
func TestTargetsSkipsEveryHostWeServe(t *testing.T) {
	for _, in := range []string{
		"freehire.me/cv/acme-x7abc",
		"https://freehire.dev/jobs",
		"https://www.freehire.me/jobs",
		"HTTPS://FreeHire.me/jobs",
		// A trailing dot names the same host to DNS, and is the cheapest way past a
		// string comparison.
		"https://freehire.me./jobs",
		"https://freehire.me:443/jobs",
	} {
		if got := Targets(ourHosts, []string{in}, nil); len(got) != 0 {
			t.Errorf("Targets(%q) = %+v, want none — that is a host we serve", in, got)
		}
	}
}

// A host that merely contains ours is somebody else's.
func TestTargetsTracesLookalikeHosts(t *testing.T) {
	for _, in := range []string{"https://freehire.me.evil.com/x", "https://notfreehire.me/x"} {
		if got := Targets(ourHosts, []string{in}, nil); len(got) != 1 {
			t.Errorf("Targets(%q) = %+v, want it traced — that host is not ours", in, got)
		}
	}
}

// Credentials in a URL are a phishing construction, and storing someone's password because they
// pasted it into a CV serves nobody.
func TestTargetsRejectsEmbeddedCredentials(t *testing.T) {
	for _, in := range []string{
		"https://freehire.me@evil.com/",
		"https://user:pass@example.org/cv",
	} {
		if got := Targets(ourHosts, []string{in}, nil); len(got) != 0 {
			t.Errorf("Targets(%q) = %+v, want none — it carries userinfo", in, got)
		}
	}
}

// A port is not a scheme. Telling them apart is what stops a personal site on a non-standard
// port being read as an untraceable protocol and silently dropped.
func TestTargetsAcceptsAHostWithAPort(t *testing.T) {
	got := Targets(ourHosts, []string{"myserver.dev:8080/cv"}, nil)
	if len(got) != 1 || got[0].URL != "https://myserver.dev:8080/cv" {
		t.Errorf("Targets(port) = %+v, want it traced as https", got)
	}
}

func TestTargetSourcePathNamesThePositionItCameFrom(t *testing.T) {
	got := Targets(ourHosts, []string{"github.com/ada"}, []string{"opensched.dev"})
	if len(got) != 2 {
		t.Fatalf("Targets() = %+v, want two", got)
	}
	if p := got[0].SourcePath(); p != "header.links[0]" {
		t.Errorf("header SourcePath() = %q, want %q", p, "header.links[0]")
	}
	if p := got[1].SourcePath(); p != "projects[0].link" {
		t.Errorf("project SourcePath() = %q, want %q", p, "projects[0].link")
	}
}

var tokenPattern = regexp.MustCompile(`^[a-z0-9-]+-[a-z0-9]{5}$`)

func TestTokenCarriesThePrefixAndFiveRandomCharacters(t *testing.T) {
	tok := Token("acme")
	if !tokenPattern.MatchString(tok) {
		t.Fatalf("Token(%q) = %q, want <prefix>-<5 alphanumerics>", "acme", tok)
	}
	if !strings.HasPrefix(tok, "acme-") {
		t.Errorf("Token() = %q, want the company prefix", tok)
	}
}

// The property that matters is how much entropy the random part carries, and that is a fact
// about the alphabet and the length — not something to sample. Drawing tokens and demanding no
// collision is itself a birthday problem: 2000 draws from 36^5 collide in ~3% of runs, so that
// test would redden CI for nothing while asserting a weaker claim than this one.
//
// Two letters — what the tool this borrows from uses — is 676 per prefix, and hundreds of
// candidates apply to the same company and share its prefix.
func TestTokenRandomPartCarriesEnoughEntropyForASharedPrefix(t *testing.T) {
	const wantAtLeast = 1 << 20
	space := 1
	for range tokenRandomLen {
		space *= len(tokenAlphabet)
	}
	if space < wantAtLeast {
		t.Errorf("token space is %d per prefix (%d chars of %d), want at least %d",
			space, tokenRandomLen, len(tokenAlphabet), wantAtLeast)
	}
}

// Cheap guard against the one way the generator could be catastrophically wrong — returning a
// constant — without pretending to measure collision rates.
func TestTokenDoesNotReturnAConstant(t *testing.T) {
	seen := make(map[string]struct{})
	for range 50 {
		seen[Token("acme")] = struct{}{}
	}
	if len(seen) < 40 {
		t.Errorf("50 draws produced only %d distinct tokens", len(seen))
	}
}

func TestTokenFallsBackWhenThereIsNoCompany(t *testing.T) {
	tok := Token("")
	if !tokenPattern.MatchString(tok) {
		t.Fatalf("Token(\"\") = %q, want a well-formed token", tok)
	}
	if !strings.HasPrefix(tok, "cv-") {
		t.Errorf("Token(\"\") = %q, want the %q fallback prefix", tok, "cv")
	}
}

func TestClassifyFlagsAutomatedTraffic(t *testing.T) {
	for _, ua := range []string{
		"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
		"Slackbot-LinkExpanding 1.0",
		"facebookexternalhit/1.1",
		"curl/8.4.0",
		"Mozilla/5.0 HeadlessChrome/120.0.0.0",
		"",
	} {
		if got := Classify("GET", ua); !got.IsBot {
			t.Errorf("Classify(GET, %q).IsBot = false, want true", ua)
		}
	}
}

// A person reading a CV in a browser issues GET. A link checker issues HEAD, and no
// user-agent rule catches the ones that lie about being a browser.
func TestClassifyFlagsEveryMethodOtherThanGet(t *testing.T) {
	const human = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	if Classify("GET", human).IsBot {
		t.Error("Classify(GET, <browser>).IsBot = true, want false")
	}
	for _, method := range []string{"HEAD", "POST", "OPTIONS"} {
		if !Classify(method, human).IsBot {
			t.Errorf("Classify(%s, <browser>).IsBot = false, want true", method)
		}
	}
}

func TestClassifyReadsTheClientOffTheUserAgent(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		ua                   string
		device, osFam, uaFam string
	}{
		{
			name:   "mac chrome",
			ua:     "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			device: "desktop", osFam: "macos", uaFam: "chrome",
		},
		{
			name:   "iphone safari",
			ua:     "Mozilla/5.0 (iPhone; CPU iPhone OS 17_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Mobile/15E148 Safari/604.1",
			device: "mobile", osFam: "ios", uaFam: "safari",
		},
		{
			name:   "windows firefox",
			ua:     "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:121.0) Gecko/20100101 Firefox/121.0",
			device: "desktop", osFam: "windows", uaFam: "firefox",
		},
		{
			name:   "android chrome",
			ua:     "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36",
			device: "mobile", osFam: "android", uaFam: "chrome",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify("GET", tc.ua)
			if got.DeviceType != tc.device || got.OSFamily != tc.osFam || got.UAFamily != tc.uaFam {
				t.Errorf("Classify() = {%s %s %s}, want {%s %s %s}",
					got.DeviceType, got.OSFamily, got.UAFamily, tc.device, tc.osFam, tc.uaFam)
			}
		})
	}
}

// Dictionaries in this codebase never guess: an unrecognised client reports unknown rather
// than a plausible-looking invention.
func TestClassifyReportsUnknownRatherThanGuessing(t *testing.T) {
	got := Classify("GET", "SomeClient/1.0")
	if got.DeviceType != "unknown" || got.OSFamily != "unknown" || got.UAFamily != "unknown" {
		t.Errorf("Classify(unrecognised) = {%s %s %s}, want all unknown",
			got.DeviceType, got.OSFamily, got.UAFamily)
	}
}

// A browser that borrows Chrome's engine is not Chrome. Reporting it as Chrome is the same
// guessing the unknown fallback exists to refuse.
func TestClassifyDoesNotReportEngineBorrowersAsChrome(t *testing.T) {
	for _, tc := range []struct{ name, ua, want string }{
		{"opera", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 OPR/106.0.0.0", "opera"},
		{"samsung", "Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 (KHTML, like Gecko) SamsungBrowser/23.0 Chrome/115.0.0.0 Mobile Safari/537.36", "samsung"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Classify("GET", tc.ua).UAFamily; got != tc.want {
				t.Errorf("UAFamily = %q, want %q", got, tc.want)
			}
		})
	}
}

// The bot flag is frozen at write time, so a false positive follows a click for as long as it is
// kept. These two are the cases a bare substring list gets wrong: a phone brand whose name ends
// in "bot", and a real browser named "Preview".
func TestClassifyDoesNotFlagHumansWhoseNamesLookAutomated(t *testing.T) {
	for _, tc := range []struct{ name, ua string }{
		{"cubot phone", "Mozilla/5.0 (Linux; Android 10; CUBOT_X30) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36"},
		{"safari technology preview", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/618.1 (KHTML, like Gecko) Version/17.4 Safari/618.1 Safari Technology Preview"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if Classify("GET", tc.ua).IsBot {
				t.Errorf("Classify(GET, %s).IsBot = true, want false", tc.name)
			}
		})
	}
}

func TestVisitorHashIsStableForOneVisitorAndDistinctAcrossThem(t *testing.T) {
	const salt, ua = "s3cr3t", "Mozilla/5.0"
	a := VisitorHash(salt, "203.0.113.7", ua)
	if a == "" {
		t.Fatal("VisitorHash() = \"\", want a hash")
	}
	if b := VisitorHash(salt, "203.0.113.7", ua); a != b {
		t.Errorf("VisitorHash() is not stable: %q then %q", a, b)
	}
	if b := VisitorHash(salt, "203.0.113.8", ua); a == b {
		t.Error("VisitorHash() collides across addresses")
	}
	if b := VisitorHash(salt, "203.0.113.7", "curl/8.4.0"); a == b {
		t.Error("VisitorHash() ignores the user agent")
	}
}

// The salt is what stops the stored value being reversed by walking the address space, so a
// different salt must yield a different hash — and no salt must yield nothing at all rather
// than an unsalted digest that only looks anonymised.
func TestVisitorHashDependsOnTheSaltAndRefusesToWorkWithoutOne(t *testing.T) {
	const ip, ua = "203.0.113.7", "Mozilla/5.0"
	if a, b := VisitorHash("salt-a", ip, ua), VisitorHash("salt-b", ip, ua); a == b {
		t.Error("VisitorHash() ignores the salt")
	}
	if got := VisitorHash("", ip, ua); got != "" {
		t.Errorf("VisitorHash(no salt) = %q, want \"\" — an unsalted digest of an IP is reversible", got)
	}
}
