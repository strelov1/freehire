package main

import (
	"testing"

	"github.com/strelov1/freehire/internal/engage/socialdigest"
)

func TestConfiguredPublishers(t *testing.T) {
	t.Run("a configured webhook yields the discord channel", func(t *testing.T) {
		got := configuredPublishers("https://discord.example/api/webhooks/1/abc", "https://freehire.me")
		if len(got) != 1 {
			t.Fatalf("got %d publishers, want 1", len(got))
		}
		if got[0].Name() != socialdigest.ChannelDiscord {
			t.Errorf("name = %q, want %q", got[0].Name(), socialdigest.ChannelDiscord)
		}
	})

	// An absent credential means the channel was never turned on. That is not an
	// error and must not read like one, or the worker looks broken every night on
	// every deployment that has not adopted the feature.
	t.Run("an absent webhook yields no channel and no error", func(t *testing.T) {
		if got := configuredPublishers("", "https://freehire.me"); len(got) != 0 {
			t.Errorf("got %d publishers, want 0", len(got))
		}
	})
}

func TestRenderOnlyTouchesNothing(t *testing.T) {
	day, err := parseDay("2026-09-03")
	if err != nil {
		t.Fatal(err)
	}
	digest := socialdigest.Digest{
		Day: day,
		Items: []socialdigest.Posting{
			{JobID: 1, Slug: "acme-go-1", Title: "Senior Go Engineer", Company: "Acme", CompanySlug: "acme", PageUniques: 42},
		},
	}
	// Pointed at a port nothing is listening on: were renderOnly to publish rather
	// than render, this would fail instead of passing quietly.
	publishers := configuredPublishers("http://127.0.0.1:1/hook", "https://freehire.me")

	if got := renderOnly(digest, publishers); got != 0 {
		t.Errorf("exit code = %d, want 0", got)
	}
}

func TestParseDay(t *testing.T) {
	t.Run("empty means discover the freshest day", func(t *testing.T) {
		got, err := parseDay("")
		if err != nil {
			t.Fatal(err)
		}
		if !got.IsZero() {
			t.Errorf("got %s, want the zero time", got)
		}
	})

	t.Run("a valid day parses", func(t *testing.T) {
		got, err := parseDay("2026-09-03")
		if err != nil {
			t.Fatal(err)
		}
		if got.Format("2006-01-02") != "2026-09-03" {
			t.Errorf("got %s", got)
		}
	})

	// Rejected, not silently ignored: a typo and a quiet day must not look the same.
	t.Run("a malformed day is rejected", func(t *testing.T) {
		for _, in := range []string{"03-09-2026", "2026-9-3", "yesterday", "2026-13-01"} {
			if _, err := parseDay(in); err == nil {
				t.Errorf("parseDay(%q) should have failed", in)
			}
		}
	})
}
