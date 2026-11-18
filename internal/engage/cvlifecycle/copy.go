package cvlifecycle

import (
	"fmt"
	"html/template"
	"strings"

	"github.com/strelov1/freehire/internal/application/mailtpl"
)

// spec is one letter's copy.
type spec struct {
	subject   func(data) string
	preheader func(data) string
	heading   string
	body      *template.Template
	text      func(data) string
	// footer says why the account is getting the letter; the CV letters' reason when empty.
	footer template.HTML
}

// body parses a letter's markup with the founder's signature appended.
func body(name, markup string) *template.Template {
	return template.Must(mailtpl.Partials().New(name).Parse(markup + "\n" + `{{template "signature" .}}`))
}

const listStyle = `margin:0 0 14px 0;padding-left:20px;font-size:15px;line-height:1.6;`

var specs = map[Step]spec{
	StepPaywall: paywallSpec,

	StepATS: {
		subject:   func(d data) string { return fmt.Sprintf("Your CV scored %d/100 in an ATS check", d.Overall) },
		preheader: func(d data) string { return "What is costing your CV the most points." },
		heading:   "What an ATS makes of your CV",
		body: body("ats", `
{{template "p" "You uploaded your CV to freehire, so I ran the check an applicant tracking system would run on it. The score is a plain read of the document, and nobody read it but a program."}}
{{template "lead" (printf "Your CV scores %d out of 100." .Overall)}}
{{if .Gaps}}{{template "p" "What is costing you the most points:"}}
<ul style="`+listStyle+`">{{range .Gaps}}<li>{{.}}</li>{{end}}</ul>{{end}}
{{if .Keywords}}{{template "p" "Skills that postings for your role ask for and your CV does not show:"}}
<ul style="`+listStyle+`">{{range .Keywords}}<li>{{.}}</li>{{end}}</ul>{{end}}
{{template "button" (mailLink .ReportURL "See the full report")}}
{{template "p" "If a result looks wrong to you, reply and tell me which one. I read every reply myself."}}`),
		text: func(d data) string {
			var b strings.Builder
			fmt.Fprintf(&b, "You uploaded your CV to freehire, so I ran the check an applicant tracking system would run on it.\n\nYour CV scores %d out of 100.\n\n", d.Overall)
			writeList(&b, "What is costing you the most points:", d.Gaps)
			writeList(&b, "Skills that postings for your role ask for and your CV does not show:", d.Keywords)
			fmt.Fprintf(&b, "See the full report: %s\n\nIf a result looks wrong to you, reply and tell me which one. I read every reply myself.\n", d.ReportURL)
			return b.String()
		},
	},

	StepPotential: {
		subject:   func(d data) string { return fmt.Sprintf("Your CV could score %d, not %d", d.Potential, d.Overall) },
		preheader: func(d data) string { return "The same experience, worded for one posting." },
		heading:   "What tailoring adds",
		body: body("potential", `
{{template "p" (printf "Your CV scored %d out of 100 in the ATS check. Written against one specific posting, with its wording and the skills it asks for, the same experience can reach %d." .Overall .Potential)}}
{{template "p" "That is what the checker sees, not a promise of an interview. But a CV that matches the posting is read by a person more often than one that does not."}}
{{template "p" "Tailoring does this for you: it takes a posting and rewrites your CV for it, using only what you have already told it about yourself. It is part of Pro."}}
{{template "button" (mailLink .TailorURL "See how tailoring works")}}`),
		text: func(d data) string {
			return fmt.Sprintf("Your CV scored %d out of 100 in the ATS check. Written against one specific posting, with its wording and the skills it asks for, the same experience can reach %d.\n\n", d.Overall, d.Potential) +
				"That is what the checker sees, not a promise of an interview. But a CV that matches the posting is read by a person more often than one that does not.\n\n" +
				"Tailoring does this for you: it takes a posting and rewrites your CV for it, using only what you have already told it about yourself. It is part of Pro.\n\n" +
				"See how tailoring works: " + d.TailorURL + "\n"
		},
	},

	StepOffer: {
		subject:   func(d data) string { return fmt.Sprintf("%d%% off your first month of Pro", d.Percent) },
		preheader: func(d data) string { return "A code for tailoring your CV to the postings you want." },
		heading:   "A code for your first month",
		body: body("offer", `
{{template "p" (printf "If tailoring your CV sounded useful, here is %d%% off your first month of Pro." .Percent)}}
{{template "code" .Code}}
{{template "button" (mailLink .OfferURL "Use the code")}}
{{template "p" "The link fills the code in for you. If you would rather not, ignore this letter: nothing changes and I will not send another about it."}}`),
		text: func(d data) string {
			return fmt.Sprintf("If tailoring your CV sounded useful, here is %d%% off your first month of Pro.\n\nCode: %s\n\nUse the code: %s\n\n", d.Percent, d.Code, d.OfferURL) +
				"If you would rather not, ignore this letter: nothing changes and I will not send another about it.\n"
		},
	},
}

var paywallSpec = spec{
	subject:   func(d data) string { return "You wanted to " + d.AttemptGeneric },
	preheader: func(d data) string { return "It needs Pro, and your account is on the free plan." },
	heading:   "About what you just tried",
	footer:    "You’re getting this because you tried a Pro feature on freehire.",
	body: body("paywall", `
{{template "p" (printf "You wanted to %s. It did not run: that is part of Pro, and your account is on the free plan." .Attempt)}}
{{template "p" "If it is still on your list, Pro unlocks it along with the rest of the AI tools. If something was unclear, or you pressed it by accident, reply and tell me. I read every reply myself."}}
{{template "button" (mailLink .PricingURL "See what Pro includes")}}
{{if .JobURL}}{{template "p-link" (mailTextLink "Or go back to the posting:" .JobURL .JobLabel)}}{{end}}`),
	text: func(d data) string {
		var b strings.Builder
		fmt.Fprintf(&b, "You wanted to %s. It did not run: that is part of Pro, and your account is on the free plan.\n\n", d.Attempt)
		b.WriteString("If it is still on your list, Pro unlocks it along with the rest of the AI tools. If something was unclear, or you pressed it by accident, reply and tell me. I read every reply myself.\n\n")
		fmt.Fprintf(&b, "See what Pro includes: %s\n", d.PricingURL)
		if d.JobURL != "" {
			fmt.Fprintf(&b, "Or go back to the posting (%s): %s\n", d.JobLabel, d.JobURL)
		}
		return b.String()
	},
}

func writeList(b *strings.Builder, lead string, items []string) {
	if len(items) == 0 {
		return
	}
	b.WriteString(lead + "\n")
	for _, it := range items {
		b.WriteString("- " + it + "\n")
	}
	b.WriteString("\n")
}
