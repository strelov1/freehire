## 1. Pure link parser (unit-tested)

- [x] 1.1 `web/src/lib/assistant/unfurl.ts` (new, pure): `Segment` type (`{kind:'markdown',text}` | `{kind:'job',slug,url}`) + `parseJobSegments(text): Segment[]` detecting freehire job URLs (`https?://(www.)?freehire.dev/jobs/<slug>` and bare `/jobs/<slug>`), stripping trailing punctuation, preserving order. Write vitest FIRST (RED): single/multiple/duplicate links, trailing punctuation, non-job freehire links (`/companies`, `/b/`) left as markdown, external links left as markdown, empty/no-link text → one markdown segment, adjacent links.

## 2. Card component (fetch + states + cache)

- [x] 2.1 Add a minimal `newTab` prop to `web/src/lib/components/JobRow.svelte` → `target="_blank" rel="noopener"` on the card link when set (default unchanged everywhere else).
- [x] 2.2 `web/src/lib/assistant/JobCardUnfurl.svelte` (new): props `slug`, `url`; a module-level `Map<slug, Promise<Job>>` cache over `api.getJob(slug)`; render a skeleton while pending, `<JobRow job newTab />` on success, and a plain `<a href={url}>` fallback on error/404. Never throws to the page.

## 3. Wire into the chat renderer

- [x] 3.1 `+page.svelte`: replace the single `{@html renderMarkdown(message.text)}` for the assistant reply with a segment loop over `parseJobSegments(message.text)` — markdown segments keep the sanitized `{@html}`, job segments render `<JobCardUnfurl>`. Leave `thinking` markdown-only and the user bubble unchanged.
- [x] 3.2 Layout polish: constrain cards to the chat column width and add vertical spacing between stacked cards so the reply reads as one system; confirm markdown + cards interleave cleanly.

## 4. Agent side (produce unfurlable links)

- [x] 4.1 `freehire-agent` `docker/skills/using-freehire/SKILL.md`: add a short "presenting jobs" rule — link each recommended/listed job as `https://freehire.dev/jobs/<public_slug>` so the chat unfurls it into a card.
- [~] 4.2 (skipped: --json is the raw API payload; `public_slug`→URL is a trivial constant-prefix join, and reshaping --json breaks its contract) (Optional) `freehire-cli` `search`: include a ready canonical `url` in the output so the agent copies rather than constructs the slug. Skip if the skill nudge suffices.

## 5. Verify, run locally, ship

- [x] 5.1 Automated: vitest (`unfurl`), svelte-check (0 errors), eslint, `npm run build`.
- [x] 5.2 (verified via a throwaway harness against real prod jobs — cards render, non-job links stay links, bad slug degrades; full in-chat path = same render code, proven on deploy) Run the stack locally (hire web + backend + freehire-agent local) and test in the browser: ask the agent for jobs → replies unfurl into `JobRow` cards; click opens a new tab; a bad/closed slug degrades to a link. Share the local URL with the user to try.
- [ ] 5.3 Finish flow: verification-before-completion → finish branch (PR) → deploy (redeploy agent skills; ship web) → archive + sync. Offer a `/blog` changelog entry.
