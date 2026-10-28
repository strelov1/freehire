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
		// Name is the ledger key and stays "discord-invite" whatever the subject line
		// says: renaming it would mail the whole list a second time.
		Name:      "discord-invite",
		Subject:   "Join a community of job seekers",
		Preheader: "A room of people job hunting right now — and me, answering.",
		Heading:   "Join a community of job seekers",
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

	// The interview-buddies letter, like the Discord one, asks for one thing and links
	// to one place: the feature's landing. It is honest that the network is new and
	// small — a reader who joins expecting a crowd and finds a handful has been sold
	// something — and honest about what the anonymity covers: others see the handle,
	// the role titles and the progress. It says the feature is free.
	"interview-buddies": {
		// Name is the ledger key and stays "interview-buddies" whatever the subject line
		// says: renaming it would mail the whole list a second time. The onboarding
		// sequence reads this key too, to skip anyone this letter already reached.
		Name:      "interview-buddies",
		Subject:   "Find people interviewing where you are",
		Preheader: "New on freehire: an anonymous network of people running the same search.",
		Heading:   "You don’t have to interview alone",
		body: body("interview-buddies", `
{{template "p" "Hi — something new I’d like you to try."}}
{{template "image-link" (mailImageLink .BuddiesURL .BuddiesDemoURL "An example interview buddies feed: members applying and interviewing at the same companies, then a message asking how the interview went" 480)}}
{{template "p" "Job hunting is mostly done alone, and the people who could help most — the ones going through the same interview loops at the same companies — have been invisible to you."}}
{{template "p" "Interview buddies changes that. Join under a random handle and you’ll see members looking for the same kind of role, in the same place, who applied to the same companies in the last 90 days — and how far they got: applied, interview, offer."}}
{{template "lead" "Then message the one who just had the interview you’re preparing for."}}
{{template "p" "Your name, email and CV stay hidden — others see your handle, your role titles and your progress, never a rejection. Block, report or leave at any time. It’s free for everyone."}}
{{template "button" (mailLink .BuddiesURL "Find your interview buddies")}}
{{template "muted" "It’s new, so the network is small and grows with every person who joins. If you’d rather just tell me what you think, reply — I read every one."}}`),
		text: func(base string) string {
			return "Hi — something new I'd like you to try.\n\n" +
				"Job hunting is mostly done alone, and the people who could help most — the ones going\n" +
				"through the same interview loops at the same companies — have been invisible to you.\n\n" +
				"Interview buddies changes that. Join under a random handle and you'll see members looking\n" +
				"for the same kind of role, in the same place, who applied to the same companies in the last\n" +
				"90 days — and how far they got: applied, interview, offer.\n\n" +
				"Then message the one who just had the interview you're preparing for.\n\n" +
				"Your name, email and CV stay hidden — others see your handle, your role titles and your\n" +
				"progress, never a rejection. Block, report or leave at any time. It's free for everyone.\n\n" +
				"Find your interview buddies: " + base + "/features/interview-buddies\n\n" +
				"It's new, so the network is small and grows with every person who joins. If you'd rather\n" +
				"just tell me what you think, reply — I read every one.\n\n" +
				"— Ilya Strelov, building freehire\n" + mailtpl.LinkedInURL + "\n"
		},
	},
}
