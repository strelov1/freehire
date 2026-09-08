package config

// LinkedInOrganizationURN is the company page the daily digest posts to, in the form the Posts
// API wants it. Empty when no organization is configured, which every caller reads as "this
// channel is off".
//
// Built here from a plain numeric id rather than configured as a whole URN: the id is what a
// person can read off the company page's own URL, while the prefix is a literal nobody should
// be asked to retype — a typo in it produces a 400 INVALID_URN_TYPE at 06:45 UTC rather than a
// refusal at startup.
func (s Settings) LinkedInOrganizationURN() string {
	if s.LinkedInOrganizationID == "" {
		return ""
	}
	return "urn:li:organization:" + s.LinkedInOrganizationID
}

// LinkedInDigestConfigured reports whether the digest's LinkedIn channel has everything it
// needs to be signed in and published to. It is the ONE place that decides, so cmd/social-digest,
// cmd/linkedin-auth and cmd/linkedin-token-refresh cannot disagree about whether the channel
// exists.
//
// All four or none, for the reason the Discord bot beside it gives: a deployment holding three
// of four would start a sign-in flow it cannot finish, or mint a credential with nowhere to
// post it. Absence is not an error — it is how this ships before the LinkedIn application is
// approved, and what rolling it back looks like.
//
// The access token is deliberately not part of this. Configuration says the channel MAY be
// used; whether a valid credential is stored is a question for the database, and answering it
// here would make a worker that has not been signed in yet look unconfigured — hiding the one
// warning that tells somebody to sign in.
func (s Settings) LinkedInDigestConfigured() bool {
	return s.LinkedInClientID != "" &&
		s.LinkedInClientSecret != "" &&
		s.LinkedInRedirectURI != "" &&
		s.LinkedInOrganizationID != ""
}
