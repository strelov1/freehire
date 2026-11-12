package handler

import (
	"bytes"
	"html/template"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// consentScreen is what one server-rendered approval screen says. The OAuth
// authorize flow (ChatGPT, Claude, a CLI on a loopback port) and the
// browser-extension connect share it, so the screens a person approves an agent
// on cannot drift apart in look or in what they warn. html/template escapes every
// field, so a caller cannot forget to.
type consentScreen struct {
	Title       string // e.g. "ChatGPT wants to access your freehire account"
	SignedIn    string // the account's email; empty omits the line
	Allows      []string
	Denies      []string
	Destination string // where approving sends the credential; empty omits the line
	Caution     string // the line a person must read before allowing
	Action      string // the POST target
	Hidden      []hiddenField
	Origin      string // the web app, for the privacy and terms links
}

type hiddenField struct{ Name, Value string }

// decisionSwitch is the consent form's third answer: "this is not my account".
const decisionSwitch = "switch"

// switchAccount ends the session and sends the person to sign in again, back to
// the same request. Origin-gated like /auth/logout: the consent routes are not, and
// a cross-site form post must not be able to sign someone out.
func (h *authHandlers) switchAccount(c *fiber.Ctx, signInURL string) error {
	if err := h.checkSameOrigin(c); err != nil {
		return err
	}
	if err := h.endSession(c); err != nil {
		return err
	}
	return c.Redirect(signInURL, fiber.StatusFound)
}

// noticeScreen is the same card without a decision: a dead end that tells the
// visitor what to do instead.
type noticeScreen struct {
	Title, Message string
	LinkURL        string
	LinkLabel      string
}

// bearerAllows and bearerDenies describe what a Bearer credential reaches. Both
// carriers these screens mint — an OAuth grant (a full-scope key) and the
// extension's session JWT — pass RequireAuthOrKey and fail the cookie-only gate
// (handler.go's cookieAuth), so one honest list serves both.
var (
	bearerAllows = []string{
		"Search jobs and read your profile, CV and tracked applications.",
		"Read the mail in your freehire inbox.",
		"Make changes as you: save and track jobs, edit your profile and CV.",
		"Use your plan's daily AI allowances, such as the assistant.",
	}
	bearerDenies = []string{
		"No access to your password, API keys, connected apps, billing, or deleting your account.",
	}
)

// consentCSS carries the design system's light and dark tokens by value
// (design-system/dist/tokens-*.css): this page is served by the API, outside the
// SPA build that would otherwise hand them over.
const consentCSS = `
:root{--bg:oklch(.997 .003 130);--fg:oklch(.13 0 0);--card:oklch(1 0 0);--muted:oklch(.43 0 0);--border:oklch(.92 0 0);--brand:#5b6f00;--brand-fg:#fff;--tile:oklch(.96 0 0);color-scheme:light dark}
@media (prefers-color-scheme:dark){:root{--bg:oklch(.16 .006 110);--fg:oklch(.985 0 0);--card:oklch(.2 0 0);--muted:oklch(.72 0 0);--border:oklch(1 0 0/10%);--brand:oklch(.72 .095 121);--brand-fg:oklch(.16 .006 110);--tile:oklch(1 0 0/6%)}}
*{box-sizing:border-box}
body{margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;padding:1.5rem;background:var(--bg);color:var(--fg);font:15px/1.5 ui-sans-serif,system-ui,-apple-system,"Segoe UI",Roboto,"Helvetica Neue",Arial,sans-serif}
.card{width:100%;max-width:28rem;background:var(--card);border:1px solid var(--border);border-radius:.875rem;overflow:hidden}
.brand{display:flex;align-items:center;justify-content:center;gap:.5rem;padding:1rem;border-bottom:1px solid var(--border);font-weight:600;letter-spacing:-.01em}
.mark{width:1.375rem;height:1.375rem;fill:currentColor}
.body{padding:1.75rem 2rem 1.5rem}
.lock{width:2.75rem;height:2.75rem;margin:0 auto 1rem;display:flex;align-items:center;justify-content:center;border-radius:.625rem;background:var(--tile)}
h1{margin:0;text-align:center;font-size:1.375rem;line-height:1.3;letter-spacing:-.02em}
.who,.msg{margin:.375rem 0 0;text-align:center;color:var(--muted);font-size:.875rem}
.msg{margin-top:.75rem;font-size:.9375rem}
.section{margin:1.5rem 0 0;padding:1.25rem 0;border-top:1px solid var(--border);border-bottom:1px solid var(--border)}
.label{margin:0 0 .75rem;font-size:.75rem;font-weight:600;letter-spacing:.06em;text-transform:uppercase;color:var(--muted)}
ul{list-style:none;margin:0;padding:0;display:grid;gap:.625rem}
li{display:flex;gap:.625rem;align-items:flex-start}
li svg{flex:none;width:1.125rem;height:1.125rem;margin-top:.15rem}
.ok{stroke:var(--brand)}
.no{stroke:var(--muted)}
.caution{margin:1rem 0 0;color:var(--muted);font-size:.875rem}
.caution strong{color:var(--fg)}
form,.actions{display:grid;grid-template-columns:1fr 1fr;gap:.75rem;margin:1.5rem 0 0}
.actions{grid-template-columns:1fr}
button,.btn{font:inherit;font-weight:500;height:2.75rem;border-radius:.625rem;cursor:pointer;border:1px solid var(--border);background:transparent;color:var(--fg);display:flex;align-items:center;justify-content:center;text-decoration:none}
button:hover,.btn:hover{background:var(--tile)}
.primary{background:var(--brand);border-color:var(--brand);color:var(--brand-fg)}
.primary:hover{background:var(--brand);filter:brightness(1.08)}
button:focus-visible,.btn:focus-visible{outline:2px solid var(--brand);outline-offset:2px}
button.link{display:inline;height:auto;border:0;padding:0;font-size:inherit;font-weight:inherit;color:var(--fg);text-decoration:underline}
button.link:hover{background:none}
.foot{margin:1.25rem 0 0;text-align:center;color:var(--muted);font-size:.8125rem}
.foot a{color:inherit}
`

// The mark is web/static/favicon.svg, inlined so the page needs no second request
// and its colour follows the page's own theme.
const consentLayout = `{{define "page"}}<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="robots" content="noindex"><title>{{.Title}}</title><link rel="icon" href="/favicon.svg"><style>` + consentCSS + `</style></head>
<body>
<main class="card">
  <div class="brand"><svg class="mark" viewBox="0 0 512 512" aria-hidden="true"><path fill-rule="evenodd" clip-rule="evenodd" d="M256 56C366.457 56 456 145.543 456 256C456 366.457 366.457 456 256 456C145.543 456 56 366.457 56 256C56 145.543 145.543 56 256 56ZM256 166L346 256L256 346L166 256L256 166Z"/></svg><span>freehire</span></div>
  <div class="body">
    <div class="lock"><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="4" y="11" width="16" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 8 0v4"/><circle cx="12" cy="16" r="1"/></svg></div>
    <h1>{{.Title}}</h1>
{{template "content" .}}
  </div>
</main>
</body>
</html>{{end}}`

const consentContent = `{{define "content"}}{{if .SignedIn}}    <p class="who">Signed in as {{.SignedIn}} · <button type="submit" form="consent" name="decision" value="switch" class="link">Not you? Switch account</button></p>
{{end}}    <div class="section">
      <p class="label">This allows</p>
      <ul>
{{range .Allows}}        <li><svg class="ok" viewBox="0 0 24 24" fill="none" stroke-width="2.25" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M20 6 9 17l-5-5"/></svg><span>{{.}}</span></li>
{{end}}{{range .Denies}}        <li><svg class="no" viewBox="0 0 24 24" fill="none" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 3 4 6v6c0 5 3.5 8 8 9 4.5-1 8-4 8-9V6z"/><path d="m9.5 9.5 5 5M14.5 9.5l-5 5"/></svg><span>{{.}}</span></li>
{{end}}      </ul>
      <p class="caution">{{if .Destination}}Access will be sent to <strong>{{.Destination}}</strong>. {{end}}{{.Caution}}</p>
    </div>
    <form id="consent" method="post" action="{{.Action}}">
{{range .Hidden}}      <input type="hidden" name="{{.Name}}" value="{{.Value}}">
{{end}}      <button type="submit" name="decision" value="deny">Cancel</button>
      <button type="submit" name="decision" value="allow" class="primary">Connect</button>
    </form>
    <p class="foot">You can disconnect at any time. See the freehire <a href="{{.Origin}}/privacy">privacy policy</a> and <a href="{{.Origin}}/terms">terms</a>.</p>
{{end}}`

const noticeContent = `{{define "content"}}    <p class="msg">{{.Message}}</p>
    <div class="actions"><a class="btn primary" href="{{.LinkURL}}">{{.LinkLabel}}</a></div>
{{end}}`

var (
	consentTemplate = template.Must(template.Must(template.New("consent").Parse(consentLayout)).Parse(consentContent))
	noticeTemplate  = template.Must(template.Must(template.New("notice").Parse(consentLayout)).Parse(noticeContent))
)

// renderConsentScreen returns the approval page. Either POST handler treats any
// decision but "allow" as a refusal, so Cancel posts the same "deny" for both.
func renderConsentScreen(s consentScreen) (string, error) {
	s.Origin = strings.TrimSuffix(s.Origin, "/")
	return executePage(consentTemplate, s)
}

func renderNoticeScreen(s noticeScreen) (string, error) {
	return executePage(noticeTemplate, s)
}

func executePage(t *template.Template, data any) (string, error) {
	var b bytes.Buffer
	if err := t.ExecuteTemplate(&b, "page", data); err != nil {
		return "", err
	}
	return b.String(), nil
}
