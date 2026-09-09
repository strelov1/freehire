package atsapply

import "testing"

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
