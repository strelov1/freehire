package config

// DiscordPaidAccessConfigured reports whether the paid-channel feature has everything it needs.
// It is the one place that decides, so routes, the SPA card and the sync worker cannot disagree.
// All five or none; absence is not an error (it is how the feature ships and rolls back).
func (s Settings) DiscordPaidAccessConfigured() bool {
	return s.DiscordClientID != "" &&
		s.DiscordClientSecret != "" &&
		s.DiscordBotToken != "" &&
		s.DiscordGuildID != "" &&
		s.DiscordPaidRoleID != ""
}
