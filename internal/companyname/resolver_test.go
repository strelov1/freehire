package companyname

import (
	"context"
	"encoding/json"
	"testing"
)

type fakeText map[string]string

func (f fakeText) GetText(_ context.Context, url string) (string, error) { return f[url], nil }

type fakeJSON map[string]string // url -> raw JSON body

func (f fakeJSON) GetJSON(_ context.Context, url string, v any) error {
	return json.Unmarshal([]byte(f[url]), v)
}

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

func TestGreenhouseResolver(t *testing.T) {
	getter := fakeJSON{
		"https://boards-api.greenhouse.io/v1/boards/acme/": `{"name":"Acme Corp"}`,
	}
	r := newGreenhouseResolver(getter)
	if got, _ := r.Name(context.Background(), "acme"); got != "Acme Corp" {
		t.Errorf("Name(acme) = %q, want Acme Corp", got)
	}
}

func TestRegistryLookup(t *testing.T) {
	reg := NewRegistry(fakeText{}, fakeJSON{})
	if _, ok := reg["pinpoint"]; !ok {
		t.Error("registry missing pinpoint resolver")
	}
	if _, ok := reg["greenhouse"]; !ok {
		t.Error("registry missing greenhouse resolver")
	}
	if _, ok := reg["nonexistent-ats"]; ok {
		t.Error("registry should not have a resolver for an unknown source")
	}
}
