package atsapply

import (
	"bytes"
	"fmt"
	"os"
	"testing"
)

// The two platforms this package can drive, and the values it drives them by.
//
// Every number and string here was read off a live posting, which is the point: a submit
// click cannot be withdrawn, so what triggers one is measured rather than inferred. See
// layout.go's own comment for the two occasions this package inferred instead.
func TestLayoutFor_KnowsTheTwoProvidersWithAFillPath(t *testing.T) {
	gh, ok := layoutFor("greenhouse")
	if !ok {
		t.Fatal("greenhouse has no layout")
	}
	want := formLayout{formSelector: "application-form", submitSelector: "#submit_app", addressBy: byID}
	if gh != want {
		t.Errorf("greenhouse layout = %+v, want %+v", gh, want)
	}

	// Measured on jobs.lever.co/coderio/6ce0e52b-e7bc-462f-ac9e-1d31c8c0e037/apply,
	// 2026-09-09: the form carries the same id Greenhouse's does, the button a person
	// clicks is #btn-submit, and the inputs carry no id at all — only name.
	lv, ok := layoutFor("lever")
	if !ok {
		t.Fatal("lever has no layout")
	}
	wantLever := formLayout{formSelector: "application-form", submitSelector: "#btn-submit", addressBy: byName}
	if lv != wantLever {
		t.Errorf("lever layout = %+v, want %+v", lv, wantLever)
	}
}

// A platform nobody measured is never driven. Its attempts park for the reason that is true
// of them — no fill path exists — rather than reaching a browser on a page this package
// knows nothing about.
func TestLayoutFor_RefusesAProviderWithNoFillPath(t *testing.T) {
	for _, provider := range []string{"ashby", "workable", "recruitee", ""} {
		if _, ok := layoutFor(provider); ok {
			t.Errorf("layoutFor(%q) returned a layout; no fill path exists for it", provider)
		}
	}
}

// The registry and fillProviders answer one question in two files, and must not drift: a
// platform Submit will try to fill, with no layout to fill it by, would reach a submit
// click with selectors matching nothing.
//
// This passes trivially while Greenhouse is the only entry in fillProviders. It starts
// carrying weight when a platform is added THERE — adding one to layouts alone creates no
// risk, since the containment runs in one direction only.
func TestLayoutFor_CoversEveryFillProvider(t *testing.T) {
	for provider := range fillProviders {
		if _, ok := layoutFor(provider); !ok {
			t.Errorf("%q is in fillProviders but has no layout", provider)
		}
	}
}

// The selector must address a field the way its platform names it — and a name carrying
// brackets (Lever's `urls[LinkedIn]`, and every employer question as
// `cards[<uuid>][field0]`) must survive quoted intact, since brackets are selector syntax
// when unquoted.
func TestFieldSelector_AddressesAFieldTheWayItsPlatformNamesIt(t *testing.T) {
	if got := fieldSelector("first_name", byID); got != "#first_name" {
		t.Errorf("byID selector = %q, want #first_name", got)
	}
	if got := fieldSelector("name", byName); got != `[name="name"]` {
		t.Errorf("byName selector = %q, want [name=\"name\"]", got)
	}
	if got := fieldSelector("urls[LinkedIn]", byName); got != `[name="urls[LinkedIn]"]` {
		t.Errorf("byName selector for a bracketed name = %q, want it quoted intact", got)
	}
}

// The selector and the way chromedp interprets it come from one value, so they cannot
// disagree: a `[name=…]` selector handed to a by-id lookup finds nothing, silently.
func TestAddressing_QueryKindMatchesTheSelectorItProduces(t *testing.T) {
	if byID.queryKind() == nil || byName.queryKind() == nil {
		t.Fatal("an addressing produced no query kind")
	}
	if fmt.Sprintf("%p", byID.queryKind()) == fmt.Sprintf("%p", byName.queryKind()) {
		t.Error("both addressings resolve to the same chromedp query kind; one of them cannot be right")
	}
}

// Lever can be filled. This is the switch the whole change exists to flip, and it is
// asserted separately from the layout because the two live in different files — which is
// exactly what TestLayoutFor_CoversEveryFillProvider guards from the other side.
func TestFillProviders_IncludesLever(t *testing.T) {
	if !fillProviders["lever"] {
		t.Error("lever is not in fillProviders; its resolved plans still park as not-implemented")
	}
}

// Every chromedp call that acts on `sel` must carry the layout's own query kind, never a
// literal.
//
// This reads the source rather than exercising the code because the failure it guards is
// invisible to any test that does not drive a real browser: chromedp.ByID silently rewrites
// a `[name=…]` selector to `#[name=…]`, which is invalid CSS matching nothing. A review
// found exactly that in the file-upload branch — under byName it would have meant no Lever
// application carrying a résumé could ever be submitted, on pages where the résumé is
// required, with every unit test still green.
//
// It checks lines acting on `sel` specifically, not the whole function: the checkbox_group
// branch builds its own `optSel` from name+value, which is a query selector under either
// addressing, and naming ByQuery there is correct.
//
// Reading source is an unusual test. It is the honest one here: the rule is a property of
// the text, and the alternative is trusting every branch to be edited correctly forever.
func TestFillOne_EveryActionOnTheSharedSelectorCarriesTheLayoutsQueryKind(t *testing.T) {
	src, err := os.ReadFile("fill.go")
	if err != nil {
		t.Fatalf("read fill.go: %v", err)
	}
	start := bytes.Index(src, []byte("func fillOne("))
	if start < 0 {
		t.Fatal("fillOne not found in fill.go — this test is pinned to that function")
	}
	end := bytes.Index(src[start:], []byte("\nfunc "))
	if end < 0 {
		end = len(src) - start
	}

	checked := 0
	for _, line := range bytes.Split(src[start:start+end], []byte("\n")) {
		trimmed := bytes.TrimSpace(line)
		if bytes.HasPrefix(trimmed, []byte("//")) || !bytes.Contains(trimmed, []byte("(sel,")) {
			continue
		}
		checked++
		if !bytes.Contains(trimmed, []byte("kind")) {
			t.Errorf("this line acts on sel without the layout's query kind:\n\t%s", trimmed)
		}
	}
	if checked == 0 {
		t.Error("no line acting on sel was found; this test is no longer pinned to anything")
	}
}
