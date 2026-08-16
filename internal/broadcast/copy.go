package broadcast

import (
	"html/template"

	"github.com/strelov1/freehire/internal/mailtpl"
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

// productHuntURL is the launch page. Both campaigns point at it; only what they ask
// for there differs, because Product Hunt itself changes what is possible on the day.
const productHuntURL = "https://www.producthunt.com/products/freehire?launch=freehire"

func body(name, markup string) *template.Template {
	return template.Must(mailtpl.Partials().New(name).Parse(markup + signature))
}

// campaigns is the registry. The two Product Hunt letters are deliberately separate
// campaigns rather than one mail sent twice: before launch day a vote does not
// exist, so asking for one would send people to a page where the button they were
// promised is not there.
var campaigns = map[string]Campaign{
	"ph-heads-up": {
		Name:      "ph-heads-up",
		Subject:   "We're launching on Product Hunt on 26 August",
		Preheader: "One click now means one notification then.",
		Heading:   "We launch on 26 August",
		body: body("ph-heads-up", `
{{template "p" "freehire goes up on Product Hunt on 26 August. For a small open-source project that day decides how many people ever hear about it — and more people means more boards covered, more duplicates caught, more of the work done in the open."}}
{{template "p" "Votes do not count until the day itself, so there is nothing to vote for yet. What helps now is one thing:"}}
{{template "lead" "Tap “Notify me” on the page — it takes five seconds."}}
{{template "p" "Product Hunt will remind you on the 26th. That is all — no second mail from me until launch day."}}
{{template "icon-button" (mailIconLink "`+productHuntURL+`" "Notify me on Product Hunt" .ProductHuntIcon)}}`),
		text: "freehire goes up on Product Hunt on 26 August. For a small open-source project that day\n" +
			"decides how many people ever hear about it.\n\n" +
			"Votes do not count until the day itself, so there is nothing to vote for yet. What helps\n" +
			"now is one thing: tap \"Notify me\" on the page. It takes five seconds, and Product Hunt\n" +
			"will remind you on the 26th.\n\n" + productHuntURL + "\n\n" +
			"That is all — no second mail from me until launch day.\n\n" +
			"— Ilya Strelov, building freehire\n",
	},

	"ph-live": {
		Name:      "ph-live",
		Subject:   "freehire is live on Product Hunt",
		Preheader: "Today is the day the votes count.",
		Heading:   "We’re live today",
		body: body("ph-live", `
{{template "p" "freehire is on Product Hunt today. Unlike last time I wrote, today the votes actually count — and the ranking is decided within the first few hours."}}
{{template "p" "If the project has been useful to you, an upvote is the single most effective thing you can do for it. One click, and it costs nothing."}}
{{template "icon-button" (mailIconLink "`+productHuntURL+`" "Upvote on Product Hunt" .ProductHuntIcon)}}
{{template "p" "A comment saying what you actually use it for is worth more than the vote — that is what people read before trying something new."}}
{{template "muted" "Either way: thank you for being here early."}}`),
		text: "freehire is on Product Hunt today. Unlike last time I wrote, today the votes actually\n" +
			"count — and the ranking is decided within the first few hours.\n\n" +
			"If the project has been useful to you, an upvote is the single most effective thing you\n" +
			"can do for it:\n\n" + productHuntURL + "\n\n" +
			"A comment saying what you actually use it for is worth more than the vote — that is what\n" +
			"people read before trying something new.\n\n" +
			"Either way: thank you for being here early.\n\n" +
			"— Ilya Strelov, building freehire\n",
	},
}
