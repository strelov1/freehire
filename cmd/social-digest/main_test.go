package main

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/engage/socialdigest"
	"github.com/strelov1/freehire/internal/platform/config"
)

// stubTokens stands in for the LinkedIn credential store. The publisher list is built before
// anything is published, so what it returns never matters here — only that it exists.
type stubTokens struct{}

func (stubTokens) AccessToken(context.Context) (string, error) { return "token", nil }

// linkedInSettings is a fully configured LinkedIn channel. Spelled out rather than built by a
// helper with defaults, so a reader can see that all four values are what "configured" means.
func linkedInSettings() config.Settings {
	return config.Settings{
		FrontendOrigin:         "https://freehire.me",
		LinkedInClientID:       "client",
		LinkedInClientSecret:   "secret",
		LinkedInRedirectURI:    "https://freehire.me/oauth/linkedin",
		LinkedInOrganizationID: "130854077",
	}
}

func TestConfiguredPublishers(t *testing.T) {
	t.Run("a configured webhook yields the discord channel", func(t *testing.T) {
		cfg := config.Settings{
			DiscordDigestWebhookURL: "https://discord.example/api/webhooks/1/abc",
			FrontendOrigin:          "https://freehire.me",
		}
		got := configuredPublishers(cfg, nil)
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
		cfg := config.Settings{FrontendOrigin: "https://freehire.me"}
		if got := configuredPublishers(cfg, nil); len(got) != 0 {
			t.Errorf("got %d publishers, want 0", len(got))
		}
	})

	t.Run("a configured application yields the linkedin channel", func(t *testing.T) {
		got := configuredPublishers(linkedInSettings(), stubTokens{})
		if len(got) != 1 {
			t.Fatalf("got %d publishers, want 1", len(got))
		}
		if got[0].Name() != socialdigest.ChannelLinkedIn {
			t.Errorf("name = %q, want %q", got[0].Name(), socialdigest.ChannelLinkedIn)
		}
	})

	// The whole reason configuration excludes the token: a channel that is set up but not yet
	// signed in must still be ATTEMPTED, so the run reports the missing credential. Dropping it
	// here would turn "nobody has signed in" into silence.
	t.Run("a configured application with no stored token is still a channel", func(t *testing.T) {
		got := configuredPublishers(linkedInSettings(), stubTokens{})
		if len(got) != 1 {
			t.Fatalf("got %d publishers, want 1", len(got))
		}
	})

	// Three of four values is a half-configured channel, and half-configured must read as off:
	// a publisher built without an organization would post to urn:li:organization: and fail at
	// 06:45 UTC rather than at deploy.
	t.Run("a partial application yields no channel", func(t *testing.T) {
		for _, missing := range []string{"id", "secret", "redirect", "org"} {
			cfg := linkedInSettings()
			switch missing {
			case "id":
				cfg.LinkedInClientID = ""
			case "secret":
				cfg.LinkedInClientSecret = ""
			case "redirect":
				cfg.LinkedInRedirectURI = ""
			case "org":
				cfg.LinkedInOrganizationID = ""
			}
			if got := configuredPublishers(cfg, stubTokens{}); len(got) != 0 {
				t.Errorf("missing %s: got %d publishers, want 0", missing, len(got))
			}
		}
	})

	t.Run("both channels configured yields both", func(t *testing.T) {
		cfg := linkedInSettings()
		cfg.DiscordDigestWebhookURL = "https://discord.example/api/webhooks/1/abc"
		if got := configuredPublishers(cfg, stubTokens{}); len(got) != 2 {
			t.Fatalf("got %d publishers, want 2", len(got))
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
	publishers := configuredPublishers(config.Settings{
		DiscordDigestWebhookURL: "http://127.0.0.1:1/hook",
		FrontendOrigin:          "https://freehire.me",
	}, nil)

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
