// Package browser launches a headless Chrome and fetches URLs through it. It is transport,
// not a feature: it knows nothing about jobs, candidates or applications, in the same sense
// platform/llm and platform/aigateway know nothing about the domain they carry bytes for.
//
// It exists because two blocks need a browser and neither may import the other. api/atsapply
// drives one to fill an application form; ingest/sources needs one to read a source whose
// pages sit behind a JavaScript challenge. api is block 8 and ingest is block 7, so a shared
// piece in either would be an upward edge for the other — the same argument blocks.go already
// records for aigateway.
//
// What is shared is deliberately narrow: how to LAUNCH a browser that does not announce
// itself as automated, and how to fetch a URL through one. What is not shared is what either
// caller then does with the page — atsapply scans a DOM and submits a form, sources reads a
// body — because an abstraction over those two would be an abstraction over nothing.
package browser

import (
	"fmt"
	"net/url"

	"github.com/chromedp/chromedp"
)

// flag is one Chrome command-line switch, kept as data so the flag set can be asserted on.
// chromedp's own ExecAllocatorOption is an opaque func over an unexported struct, so a test
// cannot read one back — and the one thing that most needs a test here is a NEGATIVE, that a
// proxy credential never reaches this list.
type flag struct {
	name  string
	value any
}

// stealthFlags are what make a headless Chrome resemble an ordinary one. This is the single
// home for them in the repository: a second copy would mean the next anti-bot fix lands in
// one caller and silently not the other.
//
// The pair is what the 2026-09-02 auto-apply spike measured against bot.sannysoft.com:
// disable-blink-features=AutomationControlled alone flipped navigator.webdriver from true to
// false, matching what a dedicated stealth framework achieved, at no extra dependency. It is
// not a claim of undetectability — see the caveats in that change's design.md.
func stealthFlags() []flag {
	return []flag{
		{name: "headless", value: true},
		{name: "disable-blink-features", value: "AutomationControlled"},
	}
}

// launchFlags is the decision LaunchOptions makes, as inspectable data.
//
// A proxy contributes its scheme and host and NOTHING else. Chrome takes proxy-server as a
// command-line argument, and a command line is world-readable on the host, so credentials
// must not travel that way; Session answers Chrome's auth challenge over the debugging
// protocol instead. An unparseable proxy is an error rather than a silent direct launch: for
// the provider this exists to serve, a direct browser is refused outright, which would look
// exactly like a working crawl that found nothing.
func launchFlags(proxyURL string) ([]flag, error) {
	flags := stealthFlags()
	if proxyURL == "" {
		return flags, nil
	}
	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("browser: parse proxy url: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("browser: proxy url %q has no scheme or host", redactProxy(proxyURL))
	}
	return append(flags, flag{name: "proxy-server", value: u.Scheme + "://" + u.Host}), nil
}

// proxyCredentials reads the credentials launchFlags deliberately dropped, for the caller
// that answers Chrome's auth challenge over the protocol. Both empty means the proxy needs
// no authentication, which is not an error.
func proxyCredentials(proxyURL string) (user, pass string, err error) {
	if proxyURL == "" {
		return "", "", nil
	}
	u, err := url.Parse(proxyURL)
	if err != nil {
		return "", "", fmt.Errorf("browser: parse proxy url: %w", err)
	}
	if u.User == nil {
		return "", "", nil
	}
	pass, _ = u.User.Password()
	return u.User.Username(), pass, nil
}

// redactProxy renders a proxy URL for an error message without its credentials — an error
// about a malformed proxy must not be the thing that prints the password into a log.
func redactProxy(proxyURL string) string {
	u, err := url.Parse(proxyURL)
	if err != nil || u.User == nil {
		return proxyURL
	}
	u.User = url.User("redacted")
	return u.String()
}

// LaunchOptions are the chromedp allocator options every caller in this repository launches
// a browser with. Pass an empty proxyURL for a direct launch.
func LaunchOptions(proxyURL string) ([]chromedp.ExecAllocatorOption, error) {
	flags, err := launchFlags(proxyURL)
	if err != nil {
		return nil, err
	}
	// len == cap on the package array's slice, so the first append copies rather than
	// writing into chromedp's own defaults.
	opts := chromedp.DefaultExecAllocatorOptions[:]
	for _, f := range flags {
		opts = append(opts, chromedp.Flag(f.name, f.value))
	}
	return opts, nil
}
