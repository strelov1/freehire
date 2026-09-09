package atsapply

// addressing is how a platform identifies a control on its own application form — which
// attribute names it, and therefore which attribute selects it.
//
// It is ONE setting rather than two because the two must agree. If the scan identified a
// field by one attribute while the fill selected it by another, nothing would report an
// error: every field would resolve, the plan would call itself complete, and no control
// would be found on the page — at the last step, after the model spend and after the
// candidate approved the application.
type addressing int

const (
	// byID identifies a control by its `id`. Greenhouse names every one of them.
	byID addressing = iota
	// byName identifies a control by its `name`. Lever's inputs carry no id at all —
	// `<input type="text" data-qa="name-input" name="name" required>` — measured on
	// jobs.lever.co/coderio/6ce0e52b-e7bc-462f-ac9e-1d31c8c0e037/apply, 2026-09-09.
	byName
)

// formLayout is what this package needs to know about one platform's application page:
// three values, each of which somebody read off a real posting.
//
// A table rather than heuristics, deliberately. Finding the form as "the element with the
// most inputs", or the button as "the one whose text says Apply", is the shape of the two
// failures this package had in a single day: a captcha marker that fired on the mere WORD
// "recaptcha" and so parked every Greenhouse posting there is, and a per-provider captcha
// list that decided what a page said before anyone had loaded the page. A submit click
// cannot be withdrawn, so what drives one is measured.
type formLayout struct {
	// formSelector is the element id the application form renders under — the bare id,
	// no leading '#', because renderedHTML waits on it with chromedp.ByID and the DOM
	// scan finds it by id.
	formSelector string
	// submitSelector is the CSS selector for the button a PERSON clicks. On Lever that is
	// #btn-submit, a type="button" whose own JS performs the invisible hCaptcha when the
	// employer enabled one and then clicks the real, hidden #hcaptchaSubmitBtn. This
	// package clicks the visible one and does not model the rest: driving the hidden
	// button directly would bypass the page's own submission logic.
	submitSelector string
	// addressBy is how a control on this platform is named, and so how it is selected.
	addressBy addressing
}

// layouts is every platform this package can drive a browser against.
//
// Nothing consults it yet — Submit still gates on fillProviders alone, and the two are
// wired together later in this same change. Until then this table is the description, not
// the gate.
//
// Both entries happen to name the form `application-form`. That is written out per
// platform rather than shared: the third platform will not share it, and a shared constant
// would have to be un-shared under time pressure.
var layouts = map[string]formLayout{
	"greenhouse": {formSelector: "application-form", submitSelector: "#submit_app", addressBy: byID},
	"lever":      {formSelector: "application-form", submitSelector: "#btn-submit", addressBy: byName},
}

// layoutFor returns the platform's page description, or false when this package cannot
// drive it.
func layoutFor(provider string) (formLayout, bool) {
	l, ok := layouts[provider]
	return l, ok
}
