package onboarding

import (
	"html/template"

	"github.com/strelov1/freehire/internal/application/mailtpl"
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

// body parses one mail's markup with the shell's sign-off appended. It is appended
// here rather than pasted into five templates: these are letters from a person, so
// the signature is a property of the sequence, not of any one letter.
func body(name, markup string) *template.Template {
	return template.Must(mailtpl.Partials().New(name).Parse(markup + "\n" + `{{template "signature" .}}`))
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
				"— Ilya Strelov, building freehire\n" + mailtpl.LinkedInURL + "\n"
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
				"— Ilya Strelov, building freehire\n" + mailtpl.LinkedInURL + "\n"
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
				"— Ilya Strelov, building freehire\n" + mailtpl.LinkedInURL + "\n"
		},
	},

	// The extension letter claims only what the feature page claims — reads the page,
	// scores it against the CV, fills the form. It is the one letter in the sequence
	// about software the reader has to install, and an overstated sentence here is
	// found out within a minute of installing it.
	StepExtension: {
		subject:   "A freehire panel on any job page",
		preheader: "It reads the posting, scores it against your CV, and fills the form.",
		heading:   "A side panel on any job page",
		body: body("extension", `
{{template "p" "One more thing worth knowing about: freehire has a browser extension, and it works on job pages that have nothing to do with us."}}
{{template "p" "Open the side panel on a posting — Greenhouse, Lever, Workday, Ashby, or a career page nobody has heard of. It reads the page itself, scores the role against your CV, and fills the application form out of your profile. You read what it wrote and press Submit."}}
{{template "lead" "The point is that it works where you already are."}}
{{template "p" "It signs in with the same account, so it is your CV and your profile it fills from — nothing to set up a second time."}}
{{template "icon-button" (mailIconLink .StoreURL "Add it to Chrome" .ChromeIcon)}}
{{template "p-link" (mailTextLink "What it does, in more detail:" .ExtensionURL "see how the extension works")}}
{{template "muted" "It is Chrome for now. If you are on Firefox or Safari, reply and say so — that is how I decide what to build next."}}`),
		text: func(base string) string {
			return "One more thing worth knowing about: freehire has a browser extension, and it works on job\n" +
				"pages that have nothing to do with us.\n\n" +
				"Open the side panel on a posting — Greenhouse, Lever, Workday, Ashby, or a career page\n" +
				"nobody has heard of. It reads the page itself, scores the role against your CV, and fills\n" +
				"the application form out of your profile. You read what it wrote and press Submit.\n\n" +
				"The point is that it works where you already are.\n\n" +
				"It signs in with the same account, so it is your CV and your profile it fills from —\n" +
				"nothing to set up a second time.\n\n" +
				"Add it to Chrome: " + storeURL + "\n\n" +
				"What it does, in more detail: " + base + "/features/extension\n\n" +
				"It is Chrome for now. If you are on Firefox or Safari, reply and say so — that is how I\n" +
				"decide what to build next.\n\n" +
				"— Ilya Strelov, building freehire\n" + mailtpl.LinkedInURL + "\n"
		},
	},

	StepOpenSource: {
		subject:   "Open code, and one place to ask",
		preheader: "freehire is open source — and the Discord is where the questions go.",
		heading:   "Open code, one place to ask",
		body: body("open_source", `
{{template "p" "Some background you might not know: freehire is fully open source. Every parser, every dedup rule, the ranking — all of it is public. Nothing is hidden about how a job gets in, or why it sits where it sits."}}
{{template "p" "I built it in the open because job search runs on trust, and “trust me” is a poor answer from a site that decides what you get to see."}}
{{template "lead" "The Discord is where the rest of it happens."}}
{{template "p" "It is the main place to ask anything — a filter that misbehaves, a board we don’t cover yet, a posting that smells fake, a feature you want. I read it and answer myself, and it is small enough that you will actually be heard."}}
{{template "p" "It is also a community of job seekers sharing how the search actually goes: what got a reply and what got silence, a CV rewritten until it started landing interviews, whether a company is really hiring or just collecting CVs, what the offer came in at. That half I can’t build — it only exists because people show up."}}
{{template "icon-button" (mailIconLink .DiscordURL "Join the Discord" .DiscordIcon)}}
{{template "p-link" (mailTextLink "And if the project has been useful, a star is how most people end up finding it —" .RepoURL "star it on GitHub")}}
{{template "muted" "Replying to this mail reaches me too."}}`),
		text: func(string) string {
			return "Some background you might not know: freehire is fully open source. Every parser, every dedup\n" +
				"rule, the ranking — all of it is public. Nothing is hidden about how a job gets in, or why it\n" +
				"sits where it sits.\n\n" +
				"I built it in the open because job search runs on trust, and \"trust me\" is a poor answer from\n" +
				"a site that decides what you get to see.\n\n" +
				"The Discord is where the rest of it happens.\n\n" +
				"It is the main place to ask anything — a filter that misbehaves, a board we don't cover yet,\n" +
				"a posting that smells fake, a feature you want. I read it and answer myself, and it is small\n" +
				"enough that you will actually be heard.\n\n" +
				"It is also a community of job seekers sharing how the search actually goes: what got a reply\n" +
				"and what got silence, a CV rewritten until it started landing interviews, whether a company\n" +
				"is really hiring or just collecting CVs, what the offer came in at. That half I can't build —\n" +
				"it only exists because people show up.\n\n" +
				"Join the Discord: " + mailtpl.DiscordURL + "\n\n" +
				"And if the project has been useful, a star is how most people end up finding it:\n" +
				repoURL + "\n\n" +
				"Replying to this mail reaches me too.\n\n" +
				"— Ilya Strelov, building freehire\n" + mailtpl.LinkedInURL + "\n"
		},
	},
}
