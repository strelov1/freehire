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
		subject:   "If something gets in your way, tell me",
		preheader: "I’m Ilya — I built freehire, and I read every reply.",
		heading:   "Hi, I’m Ilya",
		body: body("welcome", `
{{template "p" "I built freehire because job boards kept wasting my time — the same posting reposted for months, “remote” that turned out to be hybrid, roles that were already filled before I applied."}}
{{template "p" "So I built an aggregator that pulls from hundreds of company boards, merges the duplicates, and flags postings that look dead. The whole thing is open source."}}
{{template "lead" "If something gets in your way, reply to this email."}}
{{template "p" "A filter you can’t find, a search that comes back empty, a posting that turned out to be dead. I read every reply myself, and the more specific it is, the sooner it gets fixed — that is usually what I work on next."}}
{{template "button" (mailLink .AlertsURL "Set up a job alert")}}`),
		text: func(base string) string {
			return "I built freehire because job boards kept wasting my time — the same posting reposted for months,\n" +
				"\"remote\" that turned out to be hybrid, roles that were already filled before I applied.\n\n" +
				"So I built an aggregator that pulls from hundreds of company boards, merges the duplicates,\n" +
				"and flags postings that look dead. The whole thing is open source.\n\n" +
				"If something gets in your way, reply to this email.\n\n" +
				"A filter you can't find, a search that comes back empty, a posting that turned out to be dead.\n" +
				"I read every reply myself, and the more specific it is, the sooner it gets fixed — that is\n" +
				"usually what I work on next.\n\n" +
				"Set up a job alert: " + base + "/my/notifications\n\n" +
				"— Ilya Strelov, building freehire\n" + mailtpl.LinkedInURL + "\n"
		},
	},

	StepAdvancedSearch: {
		subject:   "20 filters. Most boards give you 5.",
		preheader: "Exclude a value, not just include one — and save the search.",
		heading:   "The filters go deeper",
		body: body("advanced_search", `
{{template "p" "Most job boards give you a handful of dropdowns. freehire has twenty — role, seniority, stack, region, company type, salary currency, even whether a posting looks real."}}
{{template "p" "The one people miss: almost every filter can also mean “not this.” Click a value once to require it, again to rule it out — a company you already applied to, a stack you’re done with, a source you don’t trust."}}
{{template "lead" "Save the search once, and freehire keeps running it for you — Telegram, email or push, your choice."}}
{{template "button" (mailLink .AdvancedSearchURL "See how the filters work")}}
{{template "p" "One question: which filter did you look for and not find? Or did a search come back emptier than it should have? Reply with what you were trying to search for — I read every reply myself."}}`),
		text: func(base string) string {
			return "Most job boards give you a handful of dropdowns. freehire has twenty — role, seniority,\n" +
				"stack, region, company type, salary currency, even whether a posting looks real.\n\n" +
				"The one people miss: almost every filter can also mean \"not this.\" Click a value once to\n" +
				"require it, again to rule it out — a company you already applied to, a stack you're done\n" +
				"with, a source you don't trust.\n\n" +
				"Save the search once, and freehire keeps running it for you — Telegram, email or push,\n" +
				"your choice.\n\n" +
				"See how the filters work: " + base + "/features/advanced-search\n\n" +
				"One question: which filter did you look for and not find? Or did a search come back emptier\n" +
				"than it should have? Reply with what you were trying to search for — I read every reply myself.\n\n" +
				"— Ilya Strelov, building freehire\n" + mailtpl.LinkedInURL + "\n"
		},
	},

	StepNoAlert: {
		subject:   "You never set up your search",
		preheader: "Something got in the way? Tell me what — I read every reply.",
		heading:   "What stopped you?",
		body: body("no_alert", `
{{template "p" "You signed up a few days ago but haven’t set up a job alert yet. That is the part that does the work for you: describe the search once, and new matches come to you instead of you checking."}}
{{template "p" "If something stopped you, reply and tell me what: a filter you needed and couldn’t find, too few matches for your search, or it just wasn’t clear how. A one-line reply is plenty — it is usually what I fix next."}}
{{template "button" (mailLink .AlertsURL "Set up an alert")}}`),
		text: func(base string) string {
			return "You signed up a few days ago but haven't set up a job alert yet. That is the part that does the\n" +
				"work for you: describe the search once, and new matches come to you instead of you checking.\n\n" +
				"If something stopped you, reply and tell me what: a filter you needed and couldn't find, too\n" +
				"few matches for your search, or it just wasn't clear how. A one-line reply is plenty — it is\n" +
				"usually what I fix next.\n\n" +
				"Set up an alert: " + base + "/my/notifications\n\n" +
				"— Ilya Strelov, building freehire\n" + mailtpl.LinkedInURL + "\n"
		},
	},

	// The extension letter claims only what the feature page claims — reads the page,
	// scores it against the CV, fills the form. It is the one letter in the sequence
	// about software the reader has to install, and an overstated sentence here is
	// found out within a minute of installing it.
	StepExtension: {
		subject:   "It fills out the application for you",
		preheader: "It reads the posting, scores it against your CV, and fills the form from your profile.",
		heading:   "A side panel on any job page",
		body: body("extension", `
{{template "p" "One more thing worth knowing about: freehire has a browser extension, and it works on job pages that have nothing to do with us."}}
{{template "p" "Open the side panel on a posting — Greenhouse, Lever, Workday, Ashby, or a career page nobody has heard of. It reads the page itself, scores the role against your CV, and fills the application form out of your profile. You read what it wrote and press Submit."}}
{{template "lead" "The point is that it works where you already are."}}
{{template "p" "It fills from your freehire account — your CV, your profile, and your screening answers: work authorization, visa sponsorship, notice period, salary expectations, relocation. Fill those in once and every form gets them. A question you have answered before is answered the same way again, so you stop typing your notice period into the fortieth form."}}
{{template "p-link" (mailTextLink "Two minutes that pay off on every application:" .ScreeningURL "add your screening answers")}}
{{template "icon-button" (mailIconLink .StoreURL "Add it to Chrome" .ChromeIcon)}}
{{template "p-link" (mailTextLink "On Firefox?" .FirefoxURL "get it for Firefox")}}
{{template "p-link" (mailTextLink "What it does, in more detail:" .ExtensionURL "see how the extension works")}}
{{template "muted" "If it stumbled on a form, reply with the link — that is exactly what I fix next."}}`),
		text: func(base string) string {
			return "One more thing worth knowing about: freehire has a browser extension, and it works on job\n" +
				"pages that have nothing to do with us.\n\n" +
				"Open the side panel on a posting — Greenhouse, Lever, Workday, Ashby, or a career page\n" +
				"nobody has heard of. It reads the page itself, scores the role against your CV, and fills\n" +
				"the application form out of your profile. You read what it wrote and press Submit.\n\n" +
				"The point is that it works where you already are.\n\n" +
				"It fills from your freehire account — your CV, your profile, and your screening answers:\n" +
				"work authorization, visa sponsorship, notice period, salary expectations, relocation. Fill\n" +
				"those in once and every form gets them. A question you have answered before is answered the\n" +
				"same way again, so you stop typing your notice period into the fortieth form.\n\n" +
				"Add your screening answers: " + base + "/my/profile/screening\n\n" +
				"Add it to Chrome: " + storeURL + "\n" +
				"On Firefox: " + firefoxURL + "\n\n" +
				"What it does, in more detail: " + base + "/features/extension\n\n" +
				"If it stumbled on a form, reply with the link — that is exactly what I fix next.\n\n" +
				"— Ilya Strelov, building freehire\n" + mailtpl.LinkedInURL + "\n"
		},
	},

	StepOpenSource: {
		subject:   "Every line of freehire is public",
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

	// The tailoring letter claims only what the feature page claims: it starts from a
	// match analysis, reframes what the candidate already has, asks about what they do
	// not, and exports a PDF. "Invents nothing" is the promise the page is built on, so
	// it is the promise the letter leads with. It names Pro plainly — a reader who
	// clicks through to a paywall they were not told about has been tricked, not sold.
	StepTailor: {
		subject:   "One CV for every job is why you don’t hear back",
		preheader: "Tailor your CV to one vacancy — without inventing a line.",
		heading:   "A CV for this job, not every job",
		body: body("tailor", `
{{template "p" "A generic CV loses to a specific one. Most people know that and still send the same CV everywhere, because rewriting it for every application takes an evening."}}
{{template "p" "freehire does that rewrite with you. Start from a job: run the match analysis, then tailor from its result. It copies your CV for that one vacancy — your original is never touched — and pulls forward the experience you already have but buried three roles down."}}
{{template "lead" "It does not invent anything."}}
{{template "p" "When the job asks for something your history doesn’t support, it asks you instead of writing it in. Every change is one edit you can see and undo, and the result is a clean PDF an ATS can actually read."}}
{{template "p" "Tailoring is part of Pro."}}
{{template "button" (mailLink .TailorURL "See how tailoring works")}}
{{template "p-link" (mailTextLink "What Pro includes:" .PricingURL "see the plans")}}
{{template "muted" "If you tried it and something got in the way, reply — I read every reply myself."}}`),
		text: func(base string) string {
			return "A generic CV loses to a specific one. Most people know that and still send the same CV\n" +
				"everywhere, because rewriting it for every application takes an evening.\n\n" +
				"freehire does that rewrite with you. Start from a job: run the match analysis, then tailor\n" +
				"from its result. It copies your CV for that one vacancy — your original is never touched —\n" +
				"and pulls forward the experience you already have but buried three roles down.\n\n" +
				"It does not invent anything.\n\n" +
				"When the job asks for something your history doesn't support, it asks you instead of writing\n" +
				"it in. Every change is one edit you can see and undo, and the result is a clean PDF an ATS\n" +
				"can actually read.\n\n" +
				"Tailoring is part of Pro.\n\n" +
				"See how tailoring works: " + base + "/features/tailor\n\n" +
				"What Pro includes: " + base + "/pricing\n\n" +
				"If you tried it and something got in the way, reply — I read every reply myself.\n\n" +
				"— Ilya Strelov, building freehire\n" + mailtpl.LinkedInURL + "\n"
		},
	},
}
