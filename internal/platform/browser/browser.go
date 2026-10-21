// Package browser holds how to launch a headless Chrome that does not announce itself as
// automated. It is transport, not a feature: it knows nothing about jobs, candidates or
// applications, in the same sense platform/llm and platform/aigateway know nothing about the
// domain they carry bytes for.
//
// api/atsapply is its one caller today; it drives the browser to fill an application form.
// ingest/sources used to be the second, reading a source behind a JavaScript challenge, and
// moved to the Scrapling fetch service (internal/platform/scraplingfetch) in 2026-10.
package browser

import "github.com/chromedp/chromedp"

// flag is one Chrome command-line switch, kept as data so the flag set can be asserted on:
// chromedp's own ExecAllocatorOption is an opaque func over an unexported struct, so a test
// cannot read one back.
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

// LaunchOptions are the chromedp allocator options for a stealth headless browser.
func LaunchOptions() []chromedp.ExecAllocatorOption {
	// len == cap on the package array's slice, so the first append copies rather than
	// writing into chromedp's own defaults.
	opts := chromedp.DefaultExecAllocatorOptions[:]
	for _, f := range stealthFlags() {
		opts = append(opts, chromedp.Flag(f.name, f.value))
	}
	return opts
}
