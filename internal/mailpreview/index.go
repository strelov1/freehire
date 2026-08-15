package mailpreview

import (
	"bytes"
	"html/template"
)

// DefaultDir is where the previews are written. They live inside the design system
// because that is what serves them to Storybook, and Storybook can only load files
// under its own root.
const DefaultDir = "design-system/static/email-previews"

// Index renders a contact sheet linking every preview: one iframe per mail, at the
// width a phone gives it. Reviewing the set side by side is the point — a mail that
// has drifted from the others is obvious in a row and invisible one file at a time.
func Index(samples []Sample) string {
	var buf bytes.Buffer
	// Trusted template over our own sample data; a failure is a template bug.
	_ = indexTemplate.Execute(&buf, samples)
	return buf.String()
}

// indexTemplate is the contact sheet. It is a plain browser page, not an email, so
// it is free to use the CSS and the script the mails cannot.
//
// The light/dark toggle swaps each frame's src between the pinned copies rather
// than restyling anything: a page cannot tell a framed document to ignore the OS
// preference, and rewriting the frame's markup would need to read it first, which
// the browser forbids when the sheet is opened from disk.
//
// Each frame is sandboxed: the previews are trusted, but a sandbox costs nothing
// and keeps a preview from navigating the sheet away if a link is ever clicked.
var indexTemplate = template.Must(template.New("index").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>freehire — email previews</title>
<style>
  :root { color-scheme: light; }
  body {
    margin: 0; padding: 40px 32px; background: #fafafa; color: #070707;
    font-family: ui-sans-serif, system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif;
  }
  h1 { margin: 0 0 6px; font-size: 22px; font-weight: 650; letter-spacing: -0.02em; }
  .lede { margin: 0 0 36px; font-size: 14px; color: #505050; max-width: 60ch; line-height: 1.6; }
  .grid { display: grid; gap: 28px; grid-template-columns: repeat(auto-fill, minmax(400px, 1fr)); }
  .card { background: #fff; border: 1px solid #e4e4e4; border-radius: 12px; overflow: hidden; }
  .meta { padding: 14px 16px; border-bottom: 1px solid #e4e4e4; }
  .name { font-size: 13px; font-weight: 650; }
  .subject { margin-top: 3px; font-size: 12px; color: #505050; }
  iframe { display: block; width: 100%; height: 560px; border: 0; background: #f0f0f0; }
  .text { padding: 12px 16px; border-top: 1px solid #e4e4e4; background: #fafafa; }
  .text summary { font-size: 12px; color: #505050; cursor: pointer; }
  .text pre { margin: 10px 0 0; font-size: 12px; line-height: 1.5; white-space: pre-wrap; color: #070707; }

  .toolbar { display: flex; gap: 8px; align-items: center; margin: 0 0 32px; }
  .toolbar span { font-size: 13px; color: #505050; margin-right: 4px; }
  .toolbar button {
    font: inherit; font-size: 13px; padding: 6px 14px; cursor: pointer;
    background: #fff; color: #070707; border: 1px solid #e4e4e4; border-radius: 8px;
  }
  .toolbar button[aria-pressed='true'] { background: #5b6f00; border-color: #5b6f00; color: #f3f6e2; }
</style>
</head>
<body>
<h1>Email previews</h1>
<p class="lede">
  Every mail freehire sends, rendered from the real templates against sample data.
  Regenerate with <code>make mail-preview</code>.
</p>

<div class="toolbar">
  <span>Theme</span>
  <button type="button" data-scheme="light" aria-pressed="true">Light</button>
  <button type="button" data-scheme="dark" aria-pressed="false">Dark</button>
  <button type="button" data-scheme="auto" aria-pressed="false">Your system</button>
</div>

<div class="grid">
{{range .}}
  <div class="card">
    <div class="meta">
      <div class="name">{{.Title}}</div>
      <div class="subject">{{.Subject}}</div>
    </div>
    <iframe data-mail="{{.Name}}" src="{{.Name}}.light.html" title="{{.Title}}" sandbox="allow-same-origin" loading="lazy"></iframe>
    <div class="text">
      <details><summary>Plain-text alternative</summary><pre>{{.Text}}</pre></details>
    </div>
  </div>
{{end}}
</div>

<script>
  // "auto" points at the unpinned file — the one that actually gets sent — so the
  // third button is not a third design but a way to check the media query itself.
  const suffix = { light: '.light.html', dark: '.dark.html', auto: '.html' };
  const buttons = document.querySelectorAll('.toolbar button');
  const frames = document.querySelectorAll('iframe[data-mail]');

  for (const button of buttons) {
    button.addEventListener('click', () => {
      const scheme = button.dataset.scheme;
      for (const other of buttons) other.setAttribute('aria-pressed', String(other === button));
      for (const frame of frames) frame.src = frame.dataset.mail + suffix[scheme];
      document.body.style.background = scheme === 'dark' ? '#141414' : '#fafafa';
      document.body.style.color = scheme === 'dark' ? '#fafafa' : '#070707';
    });
  }
</script>
</body>
</html>`))
