package config

import "testing"

// setDiscordEnv sets every value the paid-channel feature needs. Tests that want a
// partial configuration clear one afterwards.
func setDiscordEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DISCORD_CLIENT_ID", "app-1")
	t.Setenv("DISCORD_CLIENT_SECRET", "shh")
	t.Setenv("DISCORD_BOT_TOKEN", "bot-token")
	t.Setenv("DISCORD_GUILD_ID", "900000000000000001")
	t.Setenv("DISCORD_PAID_ROLE_ID", "900000000000000002")
}

func TestLoad_DiscordPaidAccessFromEnv(t *testing.T) {
	setDiscordEnv(t)

	c := Load()
	if c.DiscordClientID != "app-1" || c.DiscordClientSecret != "shh" {
		t.Errorf("client credentials = %q/%q, want the env values", c.DiscordClientID, c.DiscordClientSecret)
	}
	if c.DiscordBotToken != "bot-token" {
		t.Errorf("bot token = %q, want the env value", c.DiscordBotToken)
	}
	if c.DiscordGuildID != "900000000000000001" || c.DiscordPaidRoleID != "900000000000000002" {
		t.Errorf("guild/role = %q/%q, want the env values", c.DiscordGuildID, c.DiscordPaidRoleID)
	}
	if !c.DiscordPaidAccessConfigured() {
		t.Error("a fully configured deployment must report the feature as on")
	}
}

// Every value is load-bearing, so any one of them missing has to mean the feature is off.
// A deployment holding four of five would otherwise start the OAuth flow and then fail
// somewhere the user can see, rather than never offering it.
func TestLoad_DiscordPaidAccessIsOffWhenAnyValueIsMissing(t *testing.T) {
	for _, missing := range []string{
		"DISCORD_CLIENT_ID",
		"DISCORD_CLIENT_SECRET",
		"DISCORD_BOT_TOKEN",
		"DISCORD_GUILD_ID",
		"DISCORD_PAID_ROLE_ID",
	} {
		t.Run("without "+missing, func(t *testing.T) {
			setDiscordEnv(t)
			t.Setenv(missing, "")

			if Load().DiscordPaidAccessConfigured() {
				t.Errorf("feature reports on with %s unset", missing)
			}
		})
	}
}

// The digest webhook is a different Discord integration with a different credential, and
// neither may switch the other on.
func TestLoad_DiscordDigestWebhookDoesNotEnablePaidAccess(t *testing.T) {
	t.Setenv("DISCORD_DIGEST_WEBHOOK_URL", "https://discord.example/api/webhooks/1/abc")

	if Load().DiscordPaidAccessConfigured() {
		t.Error("the digest webhook alone must not enable paid channels")
	}
}
