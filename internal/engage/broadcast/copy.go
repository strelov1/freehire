package broadcast

import (
	"html/template"

	"github.com/strelov1/freehire/internal/application/mailtpl"
)

// body parses one campaign's markup with the shell's sign-off appended: a campaign
// is a letter from the same person the sequence's letters are from, and the two
// signatures were byte-identical copies until the shell took the block.
func body(name, markup string) *template.Template {
	return template.Must(mailtpl.Partials().New(name).Parse(markup + "\n" + `{{template "signature" .}}`))
}

// campaigns is the registry. A campaign is removed once it has been sent: the ledger
// in broadcast_emails is what records that it went out, so keeping the copy here
// after the fact only offers a letter about a date that has passed to whoever reads
// the worker's usage line. The two Product Hunt letters were dropped on 2026-09-01
// for that reason; git holds them.
var campaigns = map[string]Campaign{
	// The Discord letter asks for one thing and links to one place. It could also
	// have sold the alerts, the CV tools and the extension in the same breath — every
	// campaign is tempted to — but a letter with four buttons is a letter nobody acts
	// on, and what this one is for is the room filling up.
	//
	// It promises an answer from a person, and deliberately not a same-day one: a
	// deadline in a letter is a promise about behaviour, and the letter keeps going out
	// long after the week it was written in.
	"discord-invite": {
		Name:      "discord-invite",
		Subject:   "There’s a Discord for this",
		Preheader: "One room: ask me anything, and everyone else job hunting right now.",
		Heading:   "There’s a Discord for this",
		body: body("discord-invite", `
{{template "p" "Hi — short one. freehire has a Discord, and I’d like you in it."}}
{{template "p" "It is where the questions go now. A filter that misbehaves, a company board we don’t cover yet, a posting that smells fake, a feature you wish existed — ask there and I answer myself. The whole project is open source, so there is nothing about how it works that is off limits to ask."}}
{{template "lead" "And it isn’t only me on the other end."}}
{{template "p" "It is a community of job seekers, and what we do there is share how the search is actually going: what got a reply and what got silence, a CV rewritten until it started landing interviews, what a company’s interview loop really looks like, what the offer came in at. I share mine too — I am reading the same market you are."}}
{{template "p" "That half I can’t build. Whether a company is really hiring or just collecting CVs, which recruiters answer and which never do — you only get that from other people running the same search. Job hunting alone is the worst way to do it, and that is really what the room is for."}}
{{template "icon-button" (mailIconLink .DiscordURL "Join the Discord" .DiscordIcon)}}
{{template "muted" "It’s free, it’s small, and lurking is fine. If you’d rather just tell me something, reply to this — I read every one."}}`),
		text: func(string) string {
			return "Hi — short one. freehire has a Discord, and I'd like you in it.\n\n" +
				"It is where the questions go now. A filter that misbehaves, a company board we don't cover\n" +
				"yet, a posting that smells fake, a feature you wish existed — ask there and I answer myself.\n" +
				"The whole project is open source, so there is nothing about how it works that is off limits\n" +
				"to ask.\n\n" +
				"And it isn't only me on the other end.\n\n" +
				"It is a community of job seekers, and what we do there is share how the search is actually\n" +
				"going: what got a reply and what got silence, a CV rewritten until it started landing\n" +
				"interviews, what a company's interview loop really looks like, what the offer came in at.\n" +
				"I share mine too — I am reading the same market you are.\n\n" +
				"That half I can't build. Whether a company is really hiring or just collecting CVs, which\n" +
				"recruiters answer and which never do — you only get that from other people running the same\n" +
				"search. Job hunting alone is the worst way to do it, and that is really what the room is for.\n\n" +
				"Join the Discord: " + mailtpl.DiscordURL + "\n\n" +
				"It's free, it's small, and lurking is fine. If you'd rather just tell me something, reply to\n" +
				"this — I read every one.\n\n" +
				"— Ilya Strelov, building freehire\n" + mailtpl.LinkedInURL + "\n"
		},
	},
}
