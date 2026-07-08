package location

import "strings"

// usOnlyPhrases are anchored, US-specific eligibility statements that mark a role
// as restricted to the United States. Like descriptionWorkModePhrases, they are
// tuned for PRECISION over recall in long prose: only phrases that essentially
// never appear outside a genuinely US-restricted posting are listed, so a bare
// "citizen", "secret", or "security" token cannot misfire. Citizenship phrases are
// unambiguous; the clearance phrases are US-specific terms of art ("Secret
// clearance", "TS/SCI") that other countries' vetting schemes do not use (the UK
// says "SC"/"DV", not "Secret clearance"), so generic "security clearance" is
// deliberately excluded to avoid mislabeling a UK/AU role as US.
var usOnlyPhrases = []string{
	"u.s. citizen", "us citizen", "united states citizen", "citizen of the united states",
	"u.s. citizenship", "us citizenship",
	"secret clearance", "ts/sci",
}

// USOnlyFromDescription reports whether a job description carries a hard US-only
// eligibility signal (US citizenship or a US security clearance). It reads prose,
// so it is a lowest-priority geography hint used only to rescue a job the location
// dictionary could not pin to a country (see jobderive): a bare-"Remote" posting
// that resolved to the global bucket but requires US citizenship is US-restricted,
// not open-anywhere. It never guesses — an absent phrase yields false.
func USOnlyFromDescription(desc string) bool {
	lower := strings.ToLower(desc)
	for _, p := range usOnlyPhrases {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}
