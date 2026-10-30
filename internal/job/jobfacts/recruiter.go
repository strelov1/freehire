package jobfacts

import (
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/platform/htmltext"
)

// reViaRecruiter matches a posting's own statement that it is written by an
// intermediary hiring for someone else: a staffing agency, a recruiter, a job portal
// relaying a client's vacancy. Tuned for precision, measured against a 29,466-row
// random sample of open postings (2026-10-30), where these phrases marked ~0.5%.
//
// What the sample excluded, and why each alternative is shaped as it is:
//   - "our client" is anchored to a following "is <a|looking|seeking|…>" or ", a/an":
//     the bare pair is how consultancies describe their own work ("depending on our
//     client's needs"), and the plural "our clients" is every product company's copy.
//   - "confidential client" matched only "confidential client information" — 4 of 4
//     false — so it is absent.
//   - "for a client"/"for our client" read as caregiving and asset management ("for a
//     client base") more often than as a recruiter, so they are absent.
//   - "our client's" counts only inside "join our client's team".
var reViaRecruiter = regexp.MustCompile(`(?i)` + strings.Join([]string{
	`\bon behalf of (?:our|a|one of our) (?:valued |esteemed |reputed )?client\b`,
	`\bour (?:valued |esteemed |reputed )?client is (?:a|an|looking|seeking|hiring|currently|expanding|interested|recruiting)\b`,
	`\bour (?:valued |esteemed |reputed )?client, (?:a|an|one)\b`,
	`\babout our client\b`,
	`\brecruiting (?:\w+ ){0,3}on behalf of\b`,
	`\b(?:a|one) (?:[\w-]+ ){0,4}client of ours\b`,
	`\bjoin our client['’]s team\b`,
	`\bour customer is hiring for\b`,
}, "|"))

// ViaRecruiter reports whether the posting states that it is published by an
// intermediary on a client's behalf. Like the other matchers here it reads explicit
// statements only: a recruiter who never says so is not detected, and the caller
// stores the absence as unknown rather than as "posted by the employer".
func ViaRecruiter(description string) bool {
	// ToText renders bold and italic as asterisks ("our *client* is a"), which would
	// split a phrase the markup merely emphasised.
	text := strings.ReplaceAll(htmltext.ToText(description), "*", "")
	text = strings.Join(strings.Fields(text), " ")
	return reViaRecruiter.MatchString(text)
}
