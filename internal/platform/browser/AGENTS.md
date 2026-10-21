# Headless browser launch

How to launch a headless Chrome that does not announce itself as automated. Transport, not a
feature: this package knows nothing about jobs, candidates or application forms, in the same
sense `platform/llm` and `platform/aigateway` know nothing about what they carry bytes for.

`api/atsapply` is its one caller; it drives the browser to fill an application form.
`ingest/sources` used to be the second (a chromedp `Session` that read echojobs.io behind
Vercel's challenge, freehire#2588). That reader was deleted in 2026-10 when the challenge began
refusing it, and the source now goes through the Scrapling fetch service
([`platform/scraplingfetch`](../scraplingfetch/AGENTS.md)), which is where a crawl that needs a
browser belongs.

**Not to be confused with `platform/browseruse`**, its neighbour: that is an HTTP client for
somebody else's *hosted* browser (browser-use.com). This one launches a process on our own
host.

## What it owns

- **`LaunchOptions`** — the stealth launch flags, and the single home for them in the
  repository. A second copy would mean the next anti-bot fix lands in one caller and silently
  not the other.

The pair (`headless` + `disable-blink-features=AutomationControlled`) is what the 2026-09-02
auto-apply spike measured against bot.sannysoft.com: it flipped `navigator.webdriver` from true
to false, matching a dedicated stealth framework at no extra dependency. It is not a claim of
undetectability.
