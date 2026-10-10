package limitnudge

import (
	"html/template"

	"github.com/strelov1/freehire/internal/application/mailtpl"
)

// body is the letter's markup, written once — every recipient hit the same kind of
// wall (a daily ceiling on some feature), and the letter never says which one: the
// pricing page already treats every feature the same way ("every feature is on every
// plan, what changes is how much"), and naming one here would promise the nudge knows
// more about the reader's day than it does.
var body = template.Must(mailtpl.Partials().New("limit-nudge").Parse(`
{{template "p" "Hi — I'm Ilya, I built freehire. I noticed you ran into today's limit on one of the AI features."}}
{{template "lead" "Pro removes the daily ceiling — $5 a month, cancel any time."}}
{{template "p" "Free gives you a trial-sized amount of everything each day, on purpose: enough to see whether a feature is worth using before you pay for it. If you already know it is, there is no reason to wait for the limit to reset."}}
{{template "button" (mailLink .PricingURL "See the plans")}}
{{template "muted" "Replying to this email reaches me directly — tell me if something about the limits feels off."}}
` + "\n" + `{{template "signature" .}}`))

// text is the plain-text alternative, spelling the link out the way every other
// personal letter in this codebase does.
func text(pricingURL string) string {
	return "Hi — I'm Ilya, I built freehire. I noticed you ran into today's limit on one of\n" +
		"the AI features.\n\n" +
		"Pro removes the daily ceiling — $5 a month, cancel any time.\n\n" +
		"Free gives you a trial-sized amount of everything each day, on purpose: enough to\n" +
		"see whether a feature is worth using before you pay for it. If you already know it\n" +
		"is, there is no reason to wait for the limit to reset.\n\n" +
		"See the plans: " + pricingURL + "\n\n" +
		"Replying to this email reaches me directly — tell me if something about the limits\n" +
		"feels off.\n\n" +
		"— Ilya Strelov, building freehire\n" + mailtpl.LinkedInURL + "\n"
}
