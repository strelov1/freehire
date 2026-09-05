package socialdigest

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func sampleDigest() Digest {
	return Digest{
		Day: day("2026-09-03"),
		Items: []Posting{
			{JobID: 1, Slug: "acme-go-1", Title: "Senior Go Engineer", Company: "Acme", CompanySlug: "acme", Location: "Berlin", PageUniques: 142},
			{JobID: 2, Slug: "globex-rust-2", Title: "Rust Developer", Company: "Globex", CompanySlug: "globex", Remote: true, PageUniques: 90},
		},
	}
}

func TestDiscordRender(t *testing.T) {
	out, err := NewDiscordPublisher("https://example.invalid/hook", "https://freehire.me").Render(sampleDigest())
	if err != nil {
		t.Fatal(err)
	}

	var got discordPayload
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("render must produce valid JSON: %v", err)
	}
	if len(got.Embeds) != 1 {
		t.Fatalf("got %d embeds, want 1", len(got.Embeds))
	}
	e := got.Embeds[0]

	if !strings.Contains(e.Title, "3 September 2026") {
		t.Errorf("title should name the digest day, got %q", e.Title)
	}
	for _, want := range []string{
		"Senior Go Engineer",
		"https://freehire.me/jobs/acme-go-1?utm_source=discord",
		"Acme",
		"Berlin",
		"Rust Developer",
		"Remote",
	} {
		if !strings.Contains(e.Description, want) {
			t.Errorf("description missing %q:\n%s", want, e.Description)
		}
	}
	// Remote wins over a city, so a remote posting must not also print a location.
	if strings.Contains(e.Description, "Remote · ") {
		t.Errorf("remote posting should carry one place, got:\n%s", e.Description)
	}
}

// A title full of markdown is ordinary in this catalogue; unescaped it would either
// reformat the message or break the link wrapped around it.
func TestDiscordRenderEscapesMarkdown(t *testing.T) {
	d := Digest{
		Day:   day("2026-09-03"),
		Items: []Posting{{Slug: "s", Title: "C++ *urgent* [remote]", Company: "a_b_c"}},
	}
	out, err := NewDiscordPublisher("https://example.invalid/hook", "https://freehire.me").Render(d)
	if err != nil {
		t.Fatal(err)
	}
	var got discordPayload
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	desc := got.Embeds[0].Description
	for _, want := range []string{`\*urgent\*`, `\[remote\]`, `a\_b\_c`} {
		if !strings.Contains(desc, want) {
			t.Errorf("expected %q to be escaped, got:\n%s", want, desc)
		}
	}
}

func TestDiscordPublish(t *testing.T) {
	t.Run("posts the payload as JSON", func(t *testing.T) {
		var gotBody []byte
		var gotContentType string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotBody, _ = io.ReadAll(r.Body)
			gotContentType = r.Header.Get("Content-Type")
			w.WriteHeader(http.StatusNoContent)
		}))
		defer srv.Close()

		p := NewDiscordPublisher(srv.URL, "https://freehire.me").WithHTTPClient(srv.Client())
		if err := p.Publish(context.Background(), sampleDigest()); err != nil {
			t.Fatal(err)
		}
		if gotContentType != "application/json" {
			t.Errorf("content-type = %q", gotContentType)
		}
		var payload discordPayload
		if err := json.Unmarshal(gotBody, &payload); err != nil {
			t.Fatalf("body was not JSON: %v", err)
		}
		if len(payload.Embeds) != 1 {
			t.Errorf("got %d embeds, want 1", len(payload.Embeds))
		}
	})

	t.Run("a rejected post is an error carrying the reason", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"message":"Invalid Webhook Token","code":50027}`))
		}))
		defer srv.Close()

		p := NewDiscordPublisher(srv.URL, "https://freehire.me").WithHTTPClient(srv.Client())
		err := p.Publish(context.Background(), sampleDigest())
		if err == nil {
			t.Fatal("want an error")
		}
		// The body distinguishes a retired webhook from a payload Discord disliked,
		// and a bare "status 400" tells the operator neither.
		if !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "Invalid Webhook Token") {
			t.Errorf("error should carry status and reason, got %v", err)
		}
	})

	t.Run("an unreachable webhook is an error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		client := srv.Client()
		srv.Close()

		p := NewDiscordPublisher(srv.URL, "https://freehire.me").WithHTTPClient(client)
		if err := p.Publish(context.Background(), sampleDigest()); err == nil {
			t.Error("want an error")
		}
	})
}

func TestDiscordName(t *testing.T) {
	if got := NewDiscordPublisher("", "").Name(); got != ChannelDiscord {
		t.Errorf("name = %q, want %q", got, ChannelDiscord)
	}
}
