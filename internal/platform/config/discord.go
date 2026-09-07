package config

// DiscordPaidAccessConfigured reports whether the paid-channel feature has everything it
// needs. It is the ONE place that decides, so the routes, the SPA's card and the sync
// worker cannot disagree about whether the feature exists — a route that answered "on"
// while the worker answered "off" would leave a linked user permanently without the role
// and nothing would say why.
//
// All five or none: see the field comments for why a partial configuration is treated as
// absence rather than as an error. Absence is not an error at all here — it is how the
// change deploys before the Discord application exists, and how it is rolled back.
func (s Settings) DiscordPaidAccessConfigured() bool {
	return s.DiscordClientID != "" &&
		s.DiscordClientSecret != "" &&
		s.DiscordBotToken != "" &&
		s.DiscordGuildID != "" &&
		s.DiscordPaidRoleID != ""
}
