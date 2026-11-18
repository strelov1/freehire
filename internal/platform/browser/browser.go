// Package browser holds how to launch a headless Chrome that does not announce itself as
// automated. It is transport and knows nothing about the domain.
//
// api/atsapply is its one caller; it drives the browser to fill an application form.
package browser

import "github.com/chromedp/chromedp"

// flag is one Chrome command-line switch, kept as data so tests can assert on it;
// chromedp's ExecAllocatorOption is opaque.
type flag struct {
	name  string
	value any
}

// stealthFlags make a headless Chrome resemble an ordinary one. Keep them in this one place
// so an anti-bot fix reaches every caller.
//
// disable-blink-features=AutomationControlled alone flips navigator.webdriver to false. It
// does not make the browser undetectable.
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
