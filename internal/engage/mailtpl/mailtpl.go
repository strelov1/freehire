// Package mailtpl is the shared chrome every outgoing freehire email is rendered
// into: the branded header, the white card, and the fine-print footer. Feature
// packages (emailnotify, reminder, referral, report, nudge) supply only the part
// that differs — a heading and a body fragment — and this package supplies
// everything a recipient recognises as "a mail from freehire". Branding is not a
// per-feature decision, so it does not live in per-feature code.
//
// Email rendering constraints this file obeys:
//   - Layout is a nested <table>, not flexbox or grid. Outlook's Word rendering
//     engine supports neither.
//   - Every style is inline. The one <style> block is the dark-mode override,
//     which cannot be expressed inline; it is additive, so a client that drops it
//     still renders the mail correctly in light.
//   - No JavaScript, no web fonts, no external CSS.
//   - Colours are hex. The design-system tokens are oklch(), which no mail
//     client parses; see the const block for the conversion.
package mailtpl

import (
	"bytes"
	"errors"
	"html/template"
	"io"
	"strings"
)

// DiscordURL is the community invite. It lives in the shell because the footer
// carries it into every mail and letters that name it in their copy must name the
// same room.
//
// It must be an invite that never expires, has no use cap, and opens on a channel a
// stranger can read (not #github, which is a stream of webhook bot messages). A mail
// is read weeks after it is sent, and a default invite dies after seven days.
const DiscordURL = "https://discord.gg/79cgB9RRfx"

// Palette, converted from design-system/tokens/color.tokens.json. The tokens are
// oklch, which mail clients cannot parse, so the values are sRGB hex. Keep this block
// in step with the tokens by hand; the golden previews catch drift.
const (
	colorPage   = "#f0f0f0" // --muted: the canvas behind the card
	colorCard   = "#ffffff" // --card
	colorInk    = "#070707" // --foreground
	colorMuted  = "#505050" // --muted-foreground
	colorBorder = "#e4e4e4" // --border

	// The brand tokens are already hex, so these are copies, not conversions.
	colorBrand       = "#5b6f00" // --brand: CTA fill
	colorBrandFg     = "#f3f6e2" // --brand-foreground: text on the CTA
	colorBrandStrong = "#4c5c00" // --brand-strong: links, which need the contrast
	colorBrandMuted  = "#e5eacd" // --brand-muted: soft tint behind a code block
)

// Dark palette, converted the same way from color-dark.tokens.json. Used only by
// the prefers-color-scheme block in the shell.
const (
	darkPage        = "#0d0e0b" // --background
	darkCard        = "#161616" // --card, lifted off the page so the card still reads as a card
	darkInk         = "#fafafa" // --foreground
	darkMuted       = "#a4a4a4" // --muted-foreground
	darkBorder      = "#2a2a2a" // --border is rgba white 8% in the tokens; email needs an opaque value
	darkBrand       = "#9cae69" // --brand
	darkBrandFg     = "#101309" // --brand-foreground
	darkBrandStrong = "#b5c478" // --brand-strong
	darkBrandMuted  = "#2e3318" // --brand-muted
)

// fontStack mirrors the --font-sans token; mail clients get no web fonts.
//
// Multi-word families use apostrophes because this string is interpolated into a
// style="..." attribute, where an inner double quote would end the attribute.
const fontStack = `-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,'Helvetica Neue',Arial,sans-serif`

// Layout renders bodies into the shared shell. It is immutable and safe to share.
type Layout struct {
	baseURL string
}

// New builds a Layout whose logo and footer links point at baseURL, the site origin.
func New(baseURL string) *Layout {
	return &Layout{baseURL: strings.TrimRight(baseURL, "/")}
}

// Body is the per-mail content the shell wraps.
//
// Content and Footer are template.HTML: they arrive already rendered by the calling
// package's html/template, which escaped user- and source-derived values. Passing a
// concatenated string would defeat that escaping.
type Body struct {
	// Preheader is the one-line summary mail clients show next to the subject in
	// the inbox list; left empty, the client shows whatever text comes first.
	Preheader string
	// Heading is the <h1>. Plain text; escaped here.
	Heading string
	// Content is the mail's own markup, rendered by the feature package.
	Content template.HTML
	// Footer is the fine print under the card: why this person is receiving this.
	// Optional.
	Footer template.HTML
	// UnsubscribeURL is where this recipient turns THIS kind of mail off, without
	// signing in. Set on every non-essential mail; empty on an essential one (a
	// verification or password-reset code they just asked for).
	//
	// The shell renders it and does not build it: signing the token needs
	// internal/engage/emailprefs, a layer above, and one caller minting it once keeps
	// the List-Unsubscribe header and the footer in agreement.
	//
	// This field does not ENFORCE the rule: the transport refuses a silenceable,
	// linkless mail, which also covers mails that never touch this shell.
	UnsubscribeURL string
}

// Scheme selects how the mail responds to the reader's colour preference.
type Scheme int

const (
	// SchemeAuto follows the reader's client: light by default, dark where the
	// client reports a dark preference. Every mail actually sent uses this.
	SchemeAuto Scheme = iota
	// SchemeLight pins the mail to light regardless of preference.
	SchemeLight
	// SchemeDark pins the mail to dark regardless of preference.
	SchemeDark
)

// Render wraps body in the shell and returns a complete HTML document that adapts
// to the reader's colour preference.
func (l *Layout) Render(b Body) string { return l.RenderScheme(b, SchemeAuto) }

// RenderScheme is Render with the colour scheme pinned.
//
// Pinning is for review previews, not sending: nothing in the send path passes
// anything but SchemeAuto.
//
// It returns no error: Execute can only fail on a malformed shell, which the check
// in init rules out at package load.
func (l *Layout) RenderScheme(b Body, s Scheme) string {
	var buf bytes.Buffer
	_ = shell.Execute(&buf, shellData{
		Body:        b,
		LogoURL:     l.baseURL + logoPath,
		SiteURL:     l.baseURL,
		SettingsURL: l.baseURL + settingsPath,
		ColorScheme: colorSchemeMeta[s],
		DarkCSS:     darkCSS[s],
	})
	return buf.String()
}

// PinScheme rewrites an already-rendered document to a fixed colour scheme.
//
// It exists for previews: each mail package builds its own Layout internally, and
// threading a scheme through them would put a review-only concern into the send path.
//
// The rewrite is exact-literal, not a parse, since this package emitted the document.
// A miss is reported, because a silently unpinned preview shows the wrong thing.
func PinScheme(document string, s Scheme) (string, error) {
	if s == SchemeAuto {
		return document, nil
	}
	autoStyle := string(darkCSS[SchemeAuto])
	if !strings.Contains(document, autoStyle) {
		return "", errors.New("mailtpl: document was not rendered by this package's shell")
	}
	out := strings.Replace(document, autoStyle, string(darkCSS[s]), 1)
	return strings.ReplaceAll(out,
		`content="`+colorSchemeMeta[SchemeAuto]+`"`,
		`content="`+colorSchemeMeta[s]+`"`), nil
}

// darkCSS is the stylesheet each scheme emits.
//
// Light emits nothing: the layout is already light inline. Dark emits the same rules
// unconditionally; the media query is the only difference from Auto.
var darkCSS = map[Scheme]template.HTML{
	SchemeAuto:  template.HTML(`<style>@media (prefers-color-scheme: dark) {` + darkRules + `}</style>`), //nolint:gosec // a package constant, no data reaches it
	SchemeLight: "",
	SchemeDark:  template.HTML(`<style>` + darkRules + `</style>`), //nolint:gosec // same
}

// colorSchemeMeta tells the client which schemes the document supports; claiming
// "light dark" while pinned would invite it to adapt a design that cannot adapt.
var colorSchemeMeta = map[Scheme]string{
	SchemeAuto:  "light dark",
	SchemeLight: "light",
	SchemeDark:  "dark",
}

// Package load runs the shell once so a malformed template is a startup panic.
// html/template parses HTML at first Execute, not at Parse, so a shell that ended an
// attribute early (a double quote inside style="...") would otherwise mail every
// recipient a blank page while every send reported success.
func init() {
	probe := shellData{
		Body:        Body{Heading: "h", Content: "<p></p>", Footer: "f"},
		ColorScheme: colorSchemeMeta[SchemeAuto],
		DarkCSS:     darkCSS[SchemeAuto],
	}
	if err := shell.Execute(io.Discard, probe); err != nil {
		panic("mailtpl: shell template is malformed: " + err.Error())
	}
}

// logoPath is the mark served for mail. It is a copy of, not a reuse of,
// /pwa-192x192.png: that icon may change shape, while mail in an archive keeps
// loading this URL forever.
const logoPath = "/email-logo.png"

// shellData is what the shell template renders from.
type shellData struct {
	Body        Body
	LogoURL     string
	SiteURL     string
	SettingsURL string
	// ColorScheme is the value of the color-scheme meta tags.
	ColorScheme string
	// DarkCSS is the <style> element for this scheme, empty for a pinned light mail.
	DarkCSS template.HTML
}

// settingsPath is the one page that governs every notification channel.
const settingsPath = "/my/notifications"

// darkStyles repaints the mail when the reader's client is in dark mode.
//
// Client support:
//
//   - Apple Mail and Outlook.com honour prefers-color-scheme; this block is for them.
//   - Gmail does not support it and applies its own inversion, which nothing in a
//     message can steer.
//   - Outlook on Windows ignores the whole <style> block and stays light.
//
// Every rule needs !important because inline styles beat any stylesheet rule.
//
// The logo is inverted rather than swapped for a second image: two images with one
// hidden per scheme misfire in clients that ignore display:none, and the mark is a
// two-tone circle.
//
// The rules are held without their wrapper so RenderScheme can emit them inside the
// media query (sent mail) or bare (pinned dark preview).
const darkRules = `
  .m-page   { background: ` + darkPage + ` !important; }
  .m-card   { background: ` + darkCard + ` !important; border-color: ` + darkBorder + ` !important; }
  .m-ink    { color: ` + darkInk + ` !important; }
  .m-muted  { color: ` + darkMuted + ` !important; }
  .m-row    { border-top-color: ` + darkBorder + ` !important; }
  .m-quote  { color: ` + darkMuted + ` !important; border-left-color: ` + darkBorder + ` !important; }
  .m-shot   { border-color: ` + darkBorder + ` !important; }
  .m-code   { background: ` + darkBrandMuted + ` !important; color: ` + darkBrandStrong + ` !important; }
  .m-tile   { background: ` + darkBrandMuted + ` !important; color: ` + darkBrandStrong + ` !important; }
  .m-link   { color: ` + darkBrandStrong + ` !important; }
  .m-btn    { background: ` + darkBrand + ` !important; }
  .m-btn-a  { color: ` + darkBrandFg + ` !important; }
  .m-logo   { filter: invert(1); }
`

// shell is the chrome. The logo has an empty alt and the wordmark is live text, so
// a client with images disabled shows "freehire" once, not a broken-image placeholder.
var shell = template.Must(template.New("shell").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="color-scheme" content="{{.ColorScheme}}">
<meta name="supported-color-schemes" content="{{.ColorScheme}}">
<title>{{.Body.Heading}}</title>
{{.DarkCSS}}
</head>
<body class="m-page" style="margin:0;padding:0;background:` + colorPage + `;">
<div style="display:none;max-height:0;overflow:hidden;opacity:0;">{{.Body.Preheader}}</div>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" class="m-page" style="background:` + colorPage + `;">
  <tr><td align="center" style="padding:32px 12px;">
    <table role="presentation" width="600" cellpadding="0" cellspacing="0" border="0" style="max-width:600px;width:100%;font-family:` + fontStack + `;">

      <tr><td style="padding:0 4px 16px 4px;">
        <a href="{{.SiteURL}}" style="text-decoration:none;color:` + colorInk + `;">
          <img src="{{.LogoURL}}" width="28" height="28" alt="" class="m-logo" style="vertical-align:middle;border:0;display:inline-block;">
          <span class="m-ink" style="vertical-align:middle;padding-left:10px;font-size:17px;font-weight:700;letter-spacing:-0.01em;color:` + colorInk + `;">freehire</span>
        </a>
      </td></tr>

      <tr><td class="m-card" style="background:` + colorCard + `;border:1px solid ` + colorBorder + `;border-radius:12px;padding:28px;">
        {{if .Body.Heading}}<h1 class="m-ink" style="margin:0 0 16px 0;font-size:20px;line-height:1.3;font-weight:600;color:` + colorInk + `;">{{.Body.Heading}}</h1>{{end}}
        {{.Body.Content}}
      </td></tr>

      {{if .Body.Footer}}
      <tr><td class="m-muted" style="padding:20px 8px 0 8px;font-size:12px;line-height:1.6;color:` + colorMuted + `;">
        {{.Body.Footer}}
      </td></tr>
      {{end}}

      <tr><td class="m-muted" style="padding:16px 8px 0 8px;font-size:12px;line-height:1.6;color:` + colorMuted + `;">
        <a href="{{.SiteURL}}" class="m-muted" style="color:` + colorMuted + `;text-decoration:none;">freehire.me</a> — job search without the noise.
        <br><a href="` + DiscordURL + `" class="m-muted" style="color:` + colorMuted + `;text-decoration:underline;">Job hunting with us on Discord</a>
        {{if .Body.UnsubscribeURL}}<br><a href="{{.Body.UnsubscribeURL}}" class="m-muted" style="color:` + colorMuted + `;text-decoration:underline;">Unsubscribe</a> · <a href="{{.SettingsURL}}" class="m-muted" style="color:` + colorMuted + `;text-decoration:underline;">Manage settings</a>{{end}}
      </td></tr>

    </table>
  </td></tr>
</table>
</body>
</html>`))
