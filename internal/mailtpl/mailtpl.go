// Package mailtpl is the shared chrome every outgoing freehire email is rendered
// into: the branded header, the white card, and the fine-print footer. Feature
// packages (emailnotify, reminder, referral, report, nudge) supply only the part
// that differs — a heading and a body fragment — and this package supplies
// everything a recipient recognises as "a mail from freehire".
//
// Why one package rather than a shell per feature: the six transactional mails
// were written independently and drifted, from a fully laid-out digest down to
// bare <p> tags with no wrapper at all. Branding is not a per-feature decision,
// so it does not live in per-feature code.
//
// Email rendering constraints this file obeys, and why each one matters:
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

// Palette, converted from design-system/tokens/color.tokens.json. The tokens are
// authored in oklch and mail clients cannot parse that function, so the values are
// converted to sRGB hex here. Keep this block in step with the tokens by hand: a
// build-time conversion would buy nothing, because the palette changes about once
// a year and a silent drift is caught by the golden previews.
const (
	colorPage   = "#f0f0f0" // --muted: the canvas behind the card
	colorCard   = "#ffffff" // --card
	colorInk    = "#070707" // --foreground
	colorMuted  = "#505050" // --muted-foreground
	colorBorder = "#e4e4e4" // --border

	// The brand tokens are authored in hex already — they are a named palette
	// (oats green), not a computed scale — so these are copies, not conversions.
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

// fontStack mirrors the --font-sans token. Mail clients get no web fonts, so the
// system stack is the whole typographic decision.
//
// The multi-word families are quoted with apostrophes, not double quotes: this
// string is interpolated into a style="..." attribute, and html/template parses
// the result as HTML, where an inner double quote ends the attribute. CSS accepts
// either quote character, so the apostrophes cost nothing.
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
// package's own html/template, which escaped the user- and source-derived values
// inside them. Passing a string built by concatenation would defeat that escaping,
// so the type is the reminder.
type Body struct {
	// Preheader is the one-line summary mail clients show next to the subject in
	// the inbox list. Left empty, the client shows whatever text comes first —
	// usually the logo's alt text or a stray link.
	Preheader string
	// Heading is the <h1>. Plain text; escaped here.
	Heading string
	// Content is the mail's own markup, rendered by the feature package.
	Content template.HTML
	// Footer is the fine print under the card: why this person is receiving this.
	// Optional.
	Footer template.HTML
	// Essential marks a mail the recipient cannot turn off — a verification or
	// password-reset code, sent because they just asked for it. Those mails get no
	// notification-settings link, because there is no setting that would stop them
	// and offering one invites a click that changes nothing.
	//
	// Everything else is a notification and gets the link. The default is therefore
	// the safe one: a new mail that forgets to think about this is opt-out-able.
	Essential bool
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
// Pinning exists for review, not for sending: a preview has to show both designs
// side by side, and the reviewer's own OS setting is not a control. Nothing in the
// send path passes anything but SchemeAuto.
//
// It returns no error. The only way Execute can fail here is a malformed shell —
// the data cannot cause it — and that is ruled out at package load by the check
// below, so a caller would have nothing to do with an error but ignore it.
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
// It exists for previews. The mails are rendered by their own packages, each of
// which builds its Layout internally, so a reviewer has no seam to pass a scheme
// through — and threading one through six constructors would put a review-only
// concern into the send path. Rewriting afterwards keeps it out.
//
// The rewrite is exact-literal, not a parse: this package emitted the document, so
// it knows the byte-for-byte strings it is replacing. A miss is reported rather
// than ignored, because a silently unpinned preview is a preview that quietly shows
// the wrong thing.
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
// Light emits nothing at all: the layout is already light inline, so the override
// has nothing to say. Dark emits the same rules unconditionally — the media query
// is the only difference between "when the reader prefers dark" and "always".
var darkCSS = map[Scheme]template.HTML{
	SchemeAuto:  template.HTML(`<style>@media (prefers-color-scheme: dark) {` + darkRules + `}</style>`), //nolint:gosec // a package constant, no data reaches it
	SchemeLight: "",
	SchemeDark:  template.HTML(`<style>` + darkRules + `</style>`), //nolint:gosec // same
}

// colorSchemeMeta tells the client which schemes the document supports. Claiming
// "light dark" while pinned to one would invite the client to adapt a design that
// cannot adapt.
var colorSchemeMeta = map[Scheme]string{
	SchemeAuto:  "light dark",
	SchemeLight: "light",
	SchemeDark:  "dark",
}

// Package load runs the shell once so a malformed template is a startup panic
// rather than a silently empty mail body.
//
// This is not paranoia: html/template does its HTML parse at first Execute, not at
// Parse, and Execute reports the failure by returning an error with nothing
// written. A shell that ended an attribute early — a double quote inside a
// style="..." value, say — would therefore mail every recipient a blank page while
// every send call reported success.
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

// logoPath is the mark served for mail. It is a copy rather than a reuse of
// /pwa-192x192.png because a PWA icon is free to gain a maskable safe-area or
// change shape, while a mail already in someone's archive keeps loading this URL
// forever.
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

// settingsPath is the one page that governs every notification channel, so it is
// the honest destination for "turn these off" regardless of which mail asks.
const settingsPath = "/my/notifications"

// darkStyles repaints the mail when the reader's client is in dark mode.
//
// What works where, because it decides the whole shape of this block:
//
//   - Apple Mail (macOS, iOS) and Outlook.com honour prefers-color-scheme. They are
//     what this block is for.
//   - Gmail — web and app — does not support prefers-color-scheme at all. In dark
//     mode it applies its own colour inversion, which nothing in a message can
//     steer. Gmail readers therefore see either the light design or Gmail's
//     algorithmic take on it; that is a client limitation, not a gap here.
//   - Outlook on Windows ignores the whole <style> block, so it stays light —
//     which is correct, since it has no dark mode to match.
//
// Every rule needs !important: the layout is styled inline, and an inline style
// beats a stylesheet rule of any specificity without it.
//
// The logo is inverted rather than swapped for a second file. The usual trick — two
// images, one hidden per scheme — misfires in clients that ignore display:none and
// shows both. An invert() only ever applies where the media query matched, and the
// mark is a two-tone circle, so inverting it produces exactly the light-on-dark
// version we would otherwise have to draw and ship.
//
// The rules are held without their wrapper so RenderScheme can emit them either
// inside the media query (what gets sent) or bare (a pinned dark preview).
const darkRules = `
  .m-page   { background: ` + darkPage + ` !important; }
  .m-card   { background: ` + darkCard + ` !important; border-color: ` + darkBorder + ` !important; }
  .m-ink    { color: ` + darkInk + ` !important; }
  .m-muted  { color: ` + darkMuted + ` !important; }
  .m-row    { border-top-color: ` + darkBorder + ` !important; }
  .m-quote  { color: ` + darkMuted + ` !important; border-left-color: ` + darkBorder + ` !important; }
  .m-code   { background: ` + darkBrandMuted + ` !important; color: ` + darkBrandStrong + ` !important; }
  .m-tile   { background: ` + darkBrandMuted + ` !important; color: ` + darkBrandStrong + ` !important; }
  .m-btn    { background: ` + darkBrand + ` !important; }
  .m-btn-a  { color: ` + darkBrandFg + ` !important; }
  .m-logo   { filter: invert(1); }
`

// shell is the chrome. The logo carries an empty alt and the wordmark is live text
// beside it, so a client with images disabled shows a clean "freehire" rather than
// a broken-image placeholder spelling the same word twice.
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
        {{if not .Body.Essential}}<br><a href="{{.SettingsURL}}" class="m-muted" style="color:` + colorMuted + `;text-decoration:underline;">Turn off these notifications</a>{{end}}
      </td></tr>

    </table>
  </td></tr>
</table>
</body>
</html>`))
