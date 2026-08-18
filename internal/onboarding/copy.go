package onboarding

import (
	"html/template"

	"github.com/strelov1/freehire/internal/mailtpl"
)

// spec is one mail's copy: what the inbox shows, what the card says, and the
// plain-text alternative.
type spec struct {
	subject   string
	preheader string
	heading   string
	body      *template.Template
	// text builds the plain-text alternative. It takes the site origin because the
	// text body spells its links out rather than hiding them behind labels.
	text func(baseURL string) string
}

// signature is the sign-off every mail in the sequence ends with. It is appended
// once, in body(), rather than pasted into three templates: it is a property of the
// sequence — these are letters from a person — not of any one letter.
//
// The portrait is a plain <img> with a border-radius rather than a pre-cut circular
// PNG: Outlook ignores the radius and shows a square, which is a fine degradation,
// while transparency there renders as a black box in the corners. The alt text is
// the name, so an images-off client still shows who is writing.
const signature = `
<table role="presentation" cellpadding="0" cellspacing="0" border="0" style="margin-top:22px;">
  <tr>
    <td width="52" valign="top" style="width:52px;padding-right:12px;">
      <img src="{{.PortraitURL}}" width="44" height="44" alt="Ilya" style="display:block;border:0;border-radius:22px;">
    </td>
    <td valign="middle">
      <div class="m-ink" style="font-size:14px;font-weight:600;color:#070707;">Ilya Strelov</div>
      <div class="m-muted" style="font-size:13px;color:#505050;padding-top:2px;">
        building freehire ·
        <a href="{{.LinkedInURL}}" class="m-muted" style="color:#505050;text-decoration:none;"><img src="{{.LinkedInIcon}}" width="14" height="14" alt="" class="m-logo" style="vertical-align:-2px;border:0;padding-right:4px;">LinkedIn</a>
      </div>
    </td>
  </tr>
</table>`

// body parses one mail's markup with the sign-off appended.
func body(name, markup string) *template.Template {
	return template.Must(mailtpl.Partials().New(name).Parse(markup + signature))
}

// specs is the sequence. Runner selects by Step; nothing else indexes this map.
var specs = map[Step]spec{
	StepWelcome: {
		subject:   "What are you looking for?",
		preheader: "I’m Ilya — I built freehire. One question.",
		heading:   "Hi, I’m Ilya",
		body: body("welcome", `
{{template "p" "I built freehire because job boards kept wasting my time — the same posting reposted for months, “remote” that turned out to be hybrid, roles that were already filled before I applied."}}
{{template "p" "So I built an aggregator that pulls from hundreds of company boards, merges the duplicates, and flags postings that look dead. The whole thing is open source."}}
{{template "p" "One question — a one-line reply is plenty:"}}
{{template "lead" "What role are you looking for, and where?"}}
{{template "p" "I read every reply myself. If we don’t cover your search well yet, tell me — that is usually what I work on next."}}
{{template "button" (mailLink .AlertsURL "Set up a job alert")}}`),
		text: func(base string) string {
			return "I built freehire because job boards kept wasting my time — the same posting reposted for months,\n" +
				"\"remote\" that turned out to be hybrid, roles that were already filled before I applied.\n\n" +
				"So I built an aggregator that pulls from hundreds of company boards, merges the duplicates,\n" +
				"and flags postings that look dead. The whole thing is open source.\n\n" +
				"One question — a one-line reply is plenty:\n\n" +
				"What role are you looking for, and where?\n\n" +
				"I read every reply myself. If we don't cover your search well yet, tell me — that is usually\n" +
				"what I work on next.\n\n" +
				"Set up a job alert: " + base + "/my/notifications\n\n" +
				"— Ilya Strelov, building freehire\n" + linkedInURL + "\n"
		},
	},

	StepAdvancedSearch: {
		subject:   "The filters go deeper than you’d think",
		preheader: "Exclude a value, not just include one — and save the search.",
		heading:   "The filters go deeper",
		body: body("advanced_search", `
{{template "p" "Most job boards give you a handful of dropdowns. freehire has twenty — role, seniority, stack, region, company type, salary currency, even whether a posting looks real."}}
{{template "p" "The one people miss: almost every filter can also mean “not this.” Click a value once to require it, again to rule it out — a company you already applied to, a stack you’re done with, a source you don’t trust."}}
{{template "lead" "Save the search once, and freehire keeps running it for you — Telegram, email or push, your choice."}}
{{template "button" (mailLink .AdvancedSearchURL "See how the filters work")}}`),
		text: func(base string) string {
			return "Most job boards give you a handful of dropdowns. freehire has twenty — role, seniority,\n" +
				"stack, region, company type, salary currency, even whether a posting looks real.\n\n" +
				"The one people miss: almost every filter can also mean \"not this.\" Click a value once to\n" +
				"require it, again to rule it out — a company you already applied to, a stack you're done\n" +
				"with, a source you don't trust.\n\n" +
				"Save the search once, and freehire keeps running it for you — Telegram, email or push,\n" +
				"your choice.\n\n" +
				"See how the filters work: " + base + "/features/advanced-search\n\n" +
				"— Ilya Strelov, building freehire\n" + linkedInURL + "\n"
		},
	},

	StepNoAlert: {
		subject:   "Want me to set up your search?",
		preheader: "Tell me the role — I’ll tell you honestly if we cover it.",
		heading:   "Want me to set it up?",
		body: body("no_alert", `
{{template "p" "You signed up a few days ago but haven’t set up a job alert yet. That is the part that does the work for you: describe the search once, and new matches come to you instead of you checking."}}
{{template "p" "If it is easier, just reply with the role and where you want to work. I’ll tell you honestly whether we cover it well — and if we don’t, I’d rather you knew now than checked an empty page for a week."}}
{{template "button" (mailLink .AlertsURL "Set up an alert")}}`),
		text: func(base string) string {
			return "You signed up a few days ago but haven't set up a job alert yet. That is the part that does the\n" +
				"work for you: describe the search once, and new matches come to you instead of you checking.\n\n" +
				"If it is easier, just reply with the role and where you want to work. I'll tell you honestly\n" +
				"whether we cover it well — and if we don't, I'd rather you knew now than checked an empty page\n" +
				"for a week.\n\n" +
				"Set up an alert: " + base + "/my/notifications\n\n" +
				"— Ilya Strelov, building freehire\n" + linkedInURL + "\n"
		},
	},

	StepOpenSource: {
		subject:   "freehire is open source",
		preheader: "Where the code lives, and where I hang out.",
		heading:   "Why this thing is open",
		body: body("open_source", `
{{template "p" "Some background you might not know: freehire is fully open source. Every parser, every dedup rule, the ranking — all of it is public. Nothing is hidden about how a job gets in, or why it sits where it sits."}}
{{template "p" "I built it in the open because job search runs on trust, and “trust me” is a poor answer from a site that decides what you get to see."}}
{{template "p" "If it has been useful, a star is how most people end up finding the project:"}}
{{template "icon-button" (mailIconLink .RepoURL "Star it on GitHub" .GitHubIcon)}}
{{template "p" "And the Discord is small enough that you will actually be heard — ask anything, or argue with where this is going."}}
{{template "icon-button" (mailIconLink .DiscordURL "Join the Discord" .DiscordIcon)}}
{{template "p" "Replying to this mail reaches me too."}}`),
		text: func(string) string {
			return "Some background you might not know: freehire is fully open source. Every parser, every dedup\n" +
				"rule, the ranking — all of it is public. Nothing is hidden about how a job gets in, or why it\n" +
				"sits where it sits.\n\n" +
				"I built it in the open because job search runs on trust, and \"trust me\" is a poor answer from\n" +
				"a site that decides what you get to see.\n\n" +
				"If it has been useful, a star is how most people end up finding the project:\n" +
				repoURL + "\n\n" +
				"And the Discord is small enough that you will actually be heard:\n" +
				discordURL + "\n\n" +
				"Replying to this mail reaches me too.\n\n" +
				"— Ilya Strelov, building freehire\n" + linkedInURL + "\n"
		},
	},
}
