package prowelcome

import (
	"html/template"

	"github.com/strelov1/freehire/internal/application/mailtpl"
)

// body parses the letter's markup with the shell's sign-off appended, like
// internal/engage/onboarding's copy.go.
var body = template.Must(mailtpl.Partials().New("pro-welcome").Parse(`
{{template "p" "Hi — I'm Ilya, I built freehire. Thank you for subscribing, and congratulations on being one of freehire's earliest paying members!"}}
{{template "lead" (printf "You're now on %s, running through %s." .TierLabel .UntilLabel)}}
{{template "p" "Payment for the AI features is brand new, so some of it may still be rough around the edges. Feel free to ask about anything you run into — I read every message myself."}}
{{template "p" "Just reply to this email, or reach me on LinkedIn or Discord, whichever is easiest for you."}}
{{template "icon-button" (mailIconLink .DiscordURL "Join the Discord" .DiscordIcon)}}
{{template "p-link" (mailTextLink "Or connect on" .LinkedInURL "LinkedIn")}}
` + "\n" + `{{template "signature" .}}`))

// text builds the plain-text alternative, spelling links out the way onboarding's own
// text bodies do.
func text(tierLabel, untilLabel string) string {
	return "Hi — I'm Ilya, I built freehire. Thank you for subscribing, and congratulations on\n" +
		"being one of freehire's earliest paying members!\n\n" +
		"You're now on " + tierLabel + ", running through " + untilLabel + ".\n\n" +
		"Payment for the AI features is brand new, so some of it may still be rough around\n" +
		"the edges. Feel free to ask about anything you run into — I read every message myself.\n\n" +
		"Just reply to this email, or reach me on LinkedIn or Discord, whichever is easiest\n" +
		"for you.\n\n" +
		"Join the Discord: " + mailtpl.DiscordURL + "\n\n" +
		"Or connect on LinkedIn: " + mailtpl.LinkedInURL + "\n\n" +
		"— Ilya Strelov, building freehire\n" + mailtpl.LinkedInURL + "\n"
}
