package mailtpl

import (
	"html/template"
	neturl "net/url"
	"unicode"
	"unicode/utf8"
)

// Partials returns a fresh template set carrying the shared building blocks every
// mail body is assembled from. A feature package parses its own body template into
// this set and calls the blocks by name:
//
//	var tpl = template.Must(mailtpl.Partials().New("verify").Parse(`
//	  {{template "p" "Confirm your email address with this code:"}}
//	  {{template "code" .Code}}
//	`))
//
// The set is returned by value (a fresh clone per call) because parsing into a
// shared set would let one package's body template become visible to another's.
//
// Why named partials rather than exported helper functions returning template.HTML:
// a helper has to be called from Go, which pushes markup back out of the templates
// and into the render functions — exactly the shape the six mails were in before.
func Partials() *template.Template {
	t, err := partials.Clone()
	if err != nil {
		// Clone fails only on an already-executed template, and `partials` is never
		// executed — only cloned. Unreachable outside a stdlib change.
		panic("mailtpl: cloning partials: " + err.Error())
	}
	return t
}

// partials holds the block definitions. Cloned, never executed.
//
// Each block takes the dot as its whole argument, so a call site reads
// {{template "code" .Code}} rather than needing a wrapper struct — except the two
// button blocks, which need two values and take a Link.
//
// "button-right" nests a second table inside a right-aligned cell rather than
// setting text-align on one: a table sized to its content is what keeps the button
// hugging its label, and align= on the cell is the only right-alignment Outlook
// honours reliably.
var partials = template.Must(template.New("partials").Funcs(funcs).Parse(`
{{define "signature"}}<table role="presentation" cellpadding="0" cellspacing="0" border="0" style="margin-top:22px;">
  <tr>
    <td width="52" valign="top" style="width:52px;padding-right:12px;">
      <img src="{{.PortraitURL}}" width="44" height="44" alt="Ilya" style="display:block;border:0;border-radius:22px;">
    </td>
    <td valign="middle">
      <div class="m-ink" style="font-size:14px;font-weight:600;color:` + colorInk + `;">Ilya Strelov</div>
      <div class="m-muted" style="font-size:13px;color:` + colorMuted + `;padding-top:2px;">
        building freehire ·
        <a href="{{.LinkedInURL}}" class="m-muted" style="color:` + colorMuted + `;text-decoration:none;"><img src="{{.LinkedInIcon}}" width="14" height="14" alt="" class="m-logo" style="vertical-align:-2px;border:0;padding-right:4px;">LinkedIn</a>
      </div>
    </td>
  </tr>
</table>{{end}}

{{define "p"}}<p class="m-ink" style="margin:0 0 14px 0;font-size:15px;line-height:1.6;color:` + colorInk + `;">{{.}}</p>{{end}}

{{define "muted"}}<p class="m-muted" style="margin:0 0 14px 0;font-size:14px;line-height:1.6;color:` + colorMuted + `;">{{.}}</p>{{end}}

{{define "code"}}<table role="presentation" cellpadding="0" cellspacing="0" border="0" style="margin:0 0 16px 0;"><tr><td class="m-code" style="background:` + colorBrandMuted + `;border-radius:8px;padding:16px 24px;font-size:30px;letter-spacing:8px;font-weight:700;color:` + colorBrandStrong + `;">{{.}}</td></tr></table>{{end}}

{{define "button"}}<table role="presentation" cellpadding="0" cellspacing="0" border="0" style="margin:4px 0 14px 0;"><tr><td class="m-btn" style="background:` + colorBrand + `;border-radius:8px;"><a href="{{.URL}}" class="m-btn-a" style="display:inline-block;padding:11px 20px;font-size:15px;font-weight:600;color:` + colorBrandFg + `;text-decoration:none;">{{.Label}}</a></td></tr></table>{{end}}

{{define "button-right"}}<table role="presentation" cellpadding="0" cellspacing="0" border="0" width="100%" style="margin:4px 0 14px 0;"><tr><td align="right"><table role="presentation" cellpadding="0" cellspacing="0" border="0"><tr><td class="m-btn" style="background:` + colorBrand + `;border-radius:8px;"><a href="{{.URL}}" class="m-btn-a" style="display:inline-block;padding:11px 20px;font-size:15px;font-weight:600;color:` + colorBrandFg + `;text-decoration:none;">{{.Label}}</a></td></tr></table></td></tr></table>{{end}}

{{define "quote"}}<blockquote class="m-quote" style="margin:0 0 14px 0;padding:2px 0 2px 14px;border-left:3px solid ` + colorBorder + `;font-size:15px;line-height:1.6;color:` + colorMuted + `;">{{.}}</blockquote>{{end}}

{{define "p-link"}}<p class="m-ink" style="margin:0 0 14px 0;font-size:15px;line-height:1.6;color:` + colorInk + `;">{{.Text}} <a href="{{.URL}}" class="m-link" style="color:` + colorBrandStrong + `;text-decoration:underline;">{{.Label}}</a></p>{{end}}

{{define "lead"}}<p class="m-ink" style="margin:0 0 14px 0;font-size:17px;line-height:1.5;font-weight:600;color:` + colorInk + `;">{{.}}</p>{{end}}

{{define "icon-button"}}<table role="presentation" cellpadding="0" cellspacing="0" border="0" style="margin:4px 0 14px 0;"><tr><td class="m-btn" style="background:` + colorBrand + `;border-radius:8px;"><a href="{{.URL}}" class="m-btn-a" style="display:inline-block;padding:11px 20px;font-size:15px;font-weight:600;color:` + colorBrandFg + `;text-decoration:none;"><img src="{{.IconURL}}" width="18" height="18" alt="" class="m-logo" style="vertical-align:-3px;border:0;padding-right:9px;">{{.Label}}</a></td></tr></table>{{end}}

{{define "job-row"}}<table role="presentation" cellpadding="0" cellspacing="0" border="0" width="100%"><tr>
{{if .LogoURL}}<td width="44" valign="top" style="width:44px;padding-right:12px;">
  <table role="presentation" cellpadding="0" cellspacing="0" border="0" width="40" style="width:40px;">
    <tr><td align="center" valign="middle" height="40" class="m-tile" style="height:40px;background:` + colorBrandMuted + `;border-radius:8px;font-size:15px;font-weight:700;color:` + colorBrandStrong + `;">
      <img src="{{.LogoURL}}" width="40" height="40" alt="{{.Monogram}}" style="display:block;border:0;border-radius:8px;">
    </td></tr>
  </table>
</td>{{end}}
<td valign="top">
  <a href="{{.URL}}" class="m-ink" style="font-size:16px;font-weight:600;color:` + colorInk + `;text-decoration:underline;">{{.Title}}</a>
  {{if or .Company .Salary}}<div class="m-muted" style="font-size:14px;line-height:1.5;color:` + colorMuted + `;padding-top:3px;">{{.Company}}{{if and .Company .Salary}} · {{end}}{{.Salary}}</div>{{end}}
</td>
</tr></table>{{end}}
`))

// Link is the argument to the button partials.
type Link struct {
	URL   string
	Label string
}

// IconLink is the argument to "icon-button": a call to action wearing a brand mark.
//
// The icon must be a light-on-transparent PNG, because the partial tags it with
// m-logo — the class the dark stylesheet inverts. That inversion is not decoration:
// in dark mode the button fill lightens and its label darkens, so a mark that
// stayed light would disappear. One asset, tracking the label, instead of two that
// can drift.
type IconLink struct {
	URL     string
	Label   string
	IconURL string
}

// LinkedInURL is the profile the sign-off points at. It lives beside DiscordURL for
// the same reason: the letters that carry the signature are from one person, and two
// copies of his profile are two things that can disagree.
const LinkedInURL = "https://www.linkedin.com/in/istrelov/"

// TextLink is the argument to "p-link": a sentence that ends in a link.
//
// It exists because a mail gets exactly one button. A second filled button beside
// the first does not read as "and also" — it reads as two equal choices, and the
// reader makes neither. The secondary ask therefore goes out as a line of prose
// with the link inside it, which is also the only shape a partial can offer: "p"
// escapes its argument, so a link cannot be smuggled into one.
type TextLink struct {
	// Text is the lead-in, rendered before the link and escaped.
	Text string
	URL  string
	// Label is the linked words.
	Label string
}

// Job is the argument to the "job-row" partial: one vacancy as every mail shows it.
//
// It is a mailtpl type rather than each feature's own because a job looks the same
// in a digest, a reminder and a nudge, and it stopped looking the same the moment
// three packages each described it in their own markup.
type Job struct {
	Title   string
	Company string
	// Salary is optional; the row omits the separator when it is empty.
	Salary string
	URL    string
	// LogoURL and Monogram are filled by NewJob. Empty Company means no tile.
	LogoURL  string
	Monogram string
}

// companyLogoBase is our logo proxy, the same origin the SPA uses (see
// web/src/lib/logo.ts). It resolves a company mark by name and 404s on a miss.
const companyLogoBase = "https://logo.freehire.me/"

// NewJob builds a Job, resolving the company's logo URL and its fallback monogram.
//
// The monogram rides in the image's alt text, which is the only fallback email
// affords: there is no onerror hook to swap in a placeholder the way the SPA does,
// so a blocked or missing image has to degrade into something readable by itself.
func NewJob(title, company, salary, url string) Job {
	j := Job{Title: title, Company: company, Salary: salary, URL: url}
	if company != "" {
		first, _ := utf8.DecodeRuneInString(company)
		j.LogoURL = companyLogoBase + neturl.PathEscape(company)
		j.Monogram = string(unicode.ToUpper(first))
	}
	return j
}

// funcs lets a body template build a Link inline — {{template "button" (mailLink
// .URL "Open")}} — instead of the calling package pre-assembling one in Go and
// threading it through its data struct just to reach a partial.
var funcs = template.FuncMap{
	"mailLink": func(url, label string) Link { return Link{URL: url, Label: label} },
	"mailIconLink": func(url, label, iconURL string) IconLink {
		return IconLink{URL: url, Label: label, IconURL: iconURL}
	},
	"mailTextLink": func(text, url, label string) TextLink {
		return TextLink{Text: text, URL: url, Label: label}
	},
}
