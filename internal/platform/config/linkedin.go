package config

// LinkedInOrganizationURN is the company page the daily digest posts to, in the Posts API's
// form. Empty means the channel is off. Built from the numeric id (readable off the page URL)
// so nobody retypes the prefix; a typo there is a 400 at post time, not a startup refusal.
func (s Settings) LinkedInOrganizationURN() string {
	if s.LinkedInOrganizationID == "" {
		return ""
	}
	return "urn:li:organization:" + s.LinkedInOrganizationID
}

// LinkedInDigestConfigured reports whether the digest's LinkedIn channel has everything it needs
// to sign in and publish. It is the one place that decides, so the three commands cannot disagree.
// All four or none; absence is not an error (it is how the channel ships and rolls back).
//
// The access token is deliberately excluded: whether a credential is stored is a database
// question, and answering it here would hide the warning that tells someone to sign in.
func (s Settings) LinkedInDigestConfigured() bool {
	return s.LinkedInClientID != "" &&
		s.LinkedInClientSecret != "" &&
		s.LinkedInRedirectURI != "" &&
		s.LinkedInOrganizationID != ""
}
