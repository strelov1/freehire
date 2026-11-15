package atsapply

import (
	"strings"
)

// addressing is how a platform names a control on its application form, and therefore
// which attribute selects it. One setting, not two: if the scan and the fill disagreed,
// every field would resolve and the plan would look complete, yet no control would be found
// at the last step, after model spend and candidate approval.
type addressing int

const (
	// byID identifies a control by its `id`. Greenhouse names every one of them.
	byID addressing = iota
	// byName identifies a control by its `name`. Lever's inputs carry no id.
	byName
)

// formLayout is what this package needs to know about one platform's application page,
// each value read off a real posting.
//
// A table rather than heuristics, deliberately: guessing the form ("most inputs") or the
// button ("text says Apply") already parked every Greenhouse posting once. A submit click
// cannot be withdrawn, so what drives one is measured.
type formLayout struct {
	// formSelector is the bare element id (no '#') the application form renders under;
	// renderedHTML waits on it as a chromedp.ID selector and the DOM scan finds it by id.
	formSelector string
	// submitSelector is the CSS selector for the button a PERSON clicks. On Lever that is
	// #btn-submit, whose own JS runs the invisible hCaptcha and then clicks the hidden
	// #hcaptchaSubmitBtn. Driving the hidden button directly would bypass the page's own
	// submission logic.
	submitSelector string
	// addressBy is how a control on this platform is named, and so how it is selected.
	addressBy addressing
	// applyPath is appended to a posting's URL to reach its form page; empty when the
	// posting URL is that page. Lever's posting URL has the description but no form, which is
	// at that URL plus /apply.
	applyPath string
}

// applyURL is the page this platform's application form is on, given the posting's URL.
// Idempotent: a URL already ending in the path (with or without a trailing slash) is
// returned unchanged.
func (l formLayout) applyURL(postingURL string) string {
	if l.applyPath == "" {
		return postingURL
	}
	trimmed := strings.TrimSuffix(postingURL, "/")
	if strings.HasSuffix(trimmed, l.applyPath) {
		return postingURL
	}
	return trimmed + l.applyPath
}

// layouts is every platform this package can drive a browser against. Submit and the
// preview pass consult it to decide whether to launch a browser, and fillProviders must
// agree with it (a test asserts the containment). The form id is repeated per platform
// rather than shared because the next platform will not share it.
var layouts = map[string]formLayout{
	"greenhouse": {formSelector: "application-form", submitSelector: "#submit_app", addressBy: byID},
	"lever":      {formSelector: "application-form", submitSelector: "#btn-submit", addressBy: byName, applyPath: "/apply"},
}

// layoutFor returns the platform's page description, or false when it cannot be driven.
func layoutFor(provider string) (formLayout, bool) {
	l, ok := layouts[provider]
	return l, ok
}
