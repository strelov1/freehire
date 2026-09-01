package broadcast

import (
	"html/template"

	"github.com/strelov1/freehire/internal/application/mailtpl"
)

// signature is the sign-off, appended to every campaign: these are letters from a
// person, and the portrait is what makes that read as true rather than as a pose.
const signature = `
<table role="presentation" cellpadding="0" cellspacing="0" border="0" style="margin-top:22px;">
  <tr>
    <td width="52" valign="top" style="width:52px;padding-right:12px;">
      <img src="{{.PortraitURL}}" width="44" height="44" alt="Ilya" style="display:block;border:0;border-radius:22px;">
    </td>
    <td valign="middle">
      <div class="m-ink" style="font-size:14px;font-weight:600;color:#070707;">Ilya Strelov</div>
      <div class="m-muted" style="font-size:13px;color:#505050;padding-top:2px;">building freehire</div>
    </td>
  </tr>
</table>`

// alertsPath is where a campaign sends someone to set a search up, matching the
// onboarding sequence's own link so the two letters land on the same page.
const alertsPath = "/my/notifications?utm_source=email"

func body(name, markup string) *template.Template {
	return template.Must(mailtpl.Partials().New(name).Parse(markup + signature))
}

// campaigns is the registry. A campaign is removed once it has been sent: the ledger
// in broadcast_emails is what records that it went out, so keeping the copy here
// after the fact only offers a letter about a date that has passed to whoever reads
// the worker's usage line. The two Product Hunt letters were dropped on 2026-09-01
// for that reason; git holds them.
var campaigns = map[string]Campaign{
	// The September letter says nothing about "the market waking up", though the
	// posting counts would look like evidence for it: what grew over the summer was
	// this project's own coverage — more boards, more sources — and reading that as a
	// market signal would be measuring the instrument. The figures below are only
	// claims about the catalogue, which is all they can honestly be.
	"hiring-season-september": {
		Name:      "hiring-season-september",
		Subject:   "Happy hiring season",
		Preheader: "1 September — budgets reopen. Set your search up once.",
		Heading:   "Happy hiring season",
		body: body("hiring-season-september", `
{{template "p" "Hi — it’s the 1st of September. In our line of work that is the real new year, so: happy hiring season."}}
{{template "p" "You know how this month goes. Everyone is back at once, the good roles all surface in the same fortnight, and at some point on a Tuesday night you have twenty career pages open, hoping you have not already missed the one."}}
{{template "lead" "You don’t have to do that part. Tell freehire what you’re after, once."}}
{{template "p" "It watches the boards and pings you — email, Telegram or push, whichever you actually read. And you can tell it what you don’t want, which is the half nobody offers: the company you already applied to, the stack you’re done with, the “remote” that turns out to be three days in an office."}}
{{template "button" (mailLink .AlertsURL "Set up your alert")}}
{{template "p" "There are 3.2 million open jobs in there right now, from 326,000 companies — about half a million of them showed up last week. I won’t pretend that says anything about the market. It’s just what we’re carrying, and it’s a lot to go through by hand."}}
{{template "muted" "And if it’s easier: hit reply and tell me what you’re looking for. I read every one of these myself, and it’s usually what I work on next."}}`),
		text: func(base string) string {
			return "Hi — it's the 1st of September. In our line of work that is the real new year, so:\n" +
				"happy hiring season.\n\n" +
				"You know how this month goes. Everyone is back at once, the good roles all surface in\n" +
				"the same fortnight, and at some point on a Tuesday night you have twenty career pages\n" +
				"open, hoping you have not already missed the one.\n\n" +
				"You don't have to do that part. Tell freehire what you're after, once.\n\n" +
				"It watches the boards and pings you — email, Telegram or push, whichever you actually\n" +
				"read. And you can tell it what you don't want, which is the half nobody offers: the\n" +
				"company you already applied to, the stack you're done with, the \"remote\" that turns\n" +
				"out to be three days in an office.\n\n" +
				"Set up your alert: " + base + alertsPath + "\n\n" +
				"There are 3.2 million open jobs in there right now, from 326,000 companies — about half\n" +
				"a million of them showed up last week. I won't pretend that says anything about the\n" +
				"market. It's just what we're carrying, and it's a lot to go through by hand.\n\n" +
				"And if it's easier: hit reply and tell me what you're looking for. I read every one of\n" +
				"these myself, and it's usually what I work on next.\n\n" +
				"— Ilya Strelov, building freehire\n"
		},
	},
}
