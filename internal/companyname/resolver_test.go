package companyname

import (
	"context"
	"testing"
)

type fakeText map[string]string

func (f fakeText) GetText(_ context.Context, url string) (string, error) { return f[url], nil }

func TestTitleResolver(t *testing.T) {
	getter := fakeText{
		"https://lbresearch.pinpointhq.com": `<html><head><title>Jobs at Centellic | Centellic Careers</title></head></html>`,
		"https://empty.pinpointhq.com":      `<html><head><title>Just a moment...</title></head></html>`,
	}
	r := newTitleResolver(getter, "https://%s.pinpointhq.com")

	if got, _ := r.Name(context.Background(), "lbresearch"); got != "Centellic" {
		t.Errorf("Name(lbresearch) = %q, want Centellic", got)
	}
	if got, _ := r.Name(context.Background(), "empty"); got != "" {
		t.Errorf("Name(empty) = %q, want empty", got)
	}
}

func TestRegistryLookup(t *testing.T) {
	reg := NewRegistry(fakeText{})
	for _, src := range []string{"pinpoint", "bamboohr", "lever", "ashby"} {
		if _, ok := reg[src]; !ok {
			t.Errorf("registry missing %s resolver", src)
		}
	}
	// Greenhouse job URLs are vanity domains, so it has no URL-derivable board.
	if _, ok := reg["greenhouse"]; ok {
		t.Error("registry should not have a greenhouse resolver")
	}
	if _, ok := reg["nonexistent-ats"]; ok {
		t.Error("registry should not have a resolver for an unknown source")
	}
}
