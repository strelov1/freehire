package broadcast

import (
	"html/template"

	"github.com/strelov1/freehire/internal/application/mailtpl"
)

// signature is the sign-off, appended to every campaign: these are letters from a
// person, and the portrait and the profile link are what make that read as true
// rather than as a pose. It matches the onboarding sequence's sign-off line for
// line — a reader who gets both should be looking at the same person, not at two
// slightly different ones.
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

// alertsPath is where a campaign sends someone to set a search up, matching the
// onboarding sequence's own link so the two letters land on the same page.
const alertsPath = "/my/notifications?utm_source=email"

// linkedInURL is the profile in the sign-off, mirrored from the sequence: the
// signature is the same person's, so it points at the same profile.
const linkedInURL = "https://www.linkedin.com/in/istrelov/"

func body(name, markup string) *template.Template {
	return template.Must(mailtpl.Partials().New(name).Parse(markup + signature))
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
				"— Ilya Strelov, building freehire\n" + linkedInURL + "\n"
		},
	},
}
