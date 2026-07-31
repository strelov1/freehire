package tracerlink

import (
	"regexp"
	"strings"
	"testing"
)

func TestTargetsFindsHeaderAndProjectLinks(t *testing.T) {
	got := Targets(
		[]string{"github.com/ada", "https://linkedin.com/in/ada"},
		[]string{"opensched.dev"},
	)
	want := []Target{
		{SourcePath: "header.links[0]", URL: "https://github.com/ada"},
		{SourcePath: "header.links[1]", URL: "https://linkedin.com/in/ada"},
		{SourcePath: "projects[0].link", URL: "https://opensched.dev"},
	}
	if len(got) != len(want) {
		t.Fatalf("Targets() = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Targets()[%d] = %+v, want %+v", i, got[i], want[i])
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
		got := Targets([]string{tc.in}, nil)
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
		if got := Targets([]string{in}, nil); len(got) != 0 {
			t.Errorf("Targets(%q) = %+v, want none", in, got)
		}
	}
}

// Tracing our own redirect would nest a token inside a token, so the product's own host is
// skipped however it is spelled.
func TestTargetsSkipsOurOwnHost(t *testing.T) {
	for _, in := range []string{
		"freehire.me/cv/acme-x7abc",
		"https://freehire.me/jobs",
		"https://www.freehire.me/jobs",
		"HTTPS://FreeHire.me/jobs",
	} {
		if got := Targets([]string{in}, nil); len(got) != 0 {
			t.Errorf("Targets(%q) = %+v, want none — that is our own host", in, got)
		}
	}
}

// Index alignment is what lets the renderer put each href back where it came from, so a
// skipped link must not shift the ones after it.
func TestTargetsKeepsIndexesOfTheOriginalSlice(t *testing.T) {
	got := Targets([]string{"mailto:ada@example.com", "github.com/ada"}, []string{"", "opensched.dev"})
	want := []Target{
		{SourcePath: "header.links[1]", URL: "https://github.com/ada"},
		{SourcePath: "projects[1].link", URL: "https://opensched.dev"},
	}
	if len(got) != len(want) {
		t.Fatalf("Targets() = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Targets()[%d] = %+v, want %+v", i, got[i], want[i])
		}
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

// Two letters would be 676 tokens per prefix, and hundreds of candidates share the prefix of a
// popular company. Five characters is what keeps collisions rare rather than routine.
func TestTokensDoNotRepeat(t *testing.T) {
	const n = 2000
	seen := make(map[string]struct{}, n)
	for range n {
		tok := Token("acme")
		if _, dup := seen[tok]; dup {
			t.Fatalf("Token() repeated %q within %d draws", tok, n)
		}
		seen[tok] = struct{}{}
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
