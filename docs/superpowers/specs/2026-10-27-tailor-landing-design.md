# Tailor landing redesign — design

Page: `/features/tailor` (`web/src/routes/features/tailor/+page.svelte`, body in
`web/src/lib/components/TailorLandingView.svelte`).

## Goal

The page sells **Pro** directly. Tailoring is the feature people pay for, and today's page
reads like an engineering note: path syntax (`experience[i].bullets[j]`), a CLI block, long
paragraphs, and no price. It never shows a CV getting better.

The redesign shows the result, states the price, and answers the buyer's fears.
References for the target: niceresu.me (a demo of the real product, price in the hero,
comparison table, founder note, objection-handling FAQ) and enhancv.com (before and after
copy, visible scores).

## Constraints

- No testimonials or user counts we cannot prove. Proof is the product itself and the
  figures in `PRODUCT.md` (3.3M+ live postings, 3.2k GitHub stars, open source, MIT).
- No competitor names anywhere on the page or in code.
- No Sparkles icon for AI. Mark AI in words.
- "CV", not "résumé". English copy.
- Price: Pro is **$5 a month, cancel any time**. There is no refund guarantee, so do not
  write one. Read the price from the same source `/pricing` uses, so the two pages cannot
  disagree.
- `TailorWorkspacePreview.svelte` is also used by the onboarding tour. Leave it as is. The
  hero demo is a new component.
- Every number in the demo is illustrative and labelled "example".

## Page structure, top to bottom

1. **Hero.**
   - Headline: "Your CV, rewritten for this job. Nothing made up."
   - One-line subhead: freehire reads the vacancy, brings your relevant work forward in
     the job's own words, and asks you about anything it can't find in your history.
   - Primary CTA: **Get Pro — $5/mo**, which goes to the Pro checkout through the same flow
     `/pricing` uses. Secondary CTA: "See how it works", an anchor to section 5.
   - Under the CTAs, small text: "Cancel any time · Your original CV is never touched."
   - Right side: the **live demo** (see below).
2. **Fact strip.** Four items in one row: 3.3M+ live jobs · ATS-readable PDF · 9 templates ·
   Open source, 3.2k★.
3. **Before and after.** Three CV bullets shown as a "before" line and an "after" line. The
   after line highlights the words it borrowed from the vacancy. Above them, a small
   vacancy chip names the target role (example).
4. **It doesn't make things up.** The honesty split in plain words, with two columns:
   - "You have it, it's just buried": green chips. These get pulled forward.
   - "You don't have it yet": amber chips. The agent asks you and never writes them on its own.
   - One sentence underneath: the rule is enforced in code, not in a prompt.
5. **How it works.** Four steps, each with a small UI illustration rather than plain text:
   pick a job → see the match → tailor with the agent (or by hand) → download the PDF.
6. **What Pro gets you.** The tailoring-related Pro features (CV tailoring, job match and
   ATS scores, cover letters, the AI assistant, auto-apply), each linked to its page, with
   the price and CTA repeated. The list comes from the same source as `/pricing`.
7. **Comparison table.** Columns: freehire · "Pasting into a chatbot" · "A CV builder". Rows:
   works from a real live posting; won't invent experience; shows a match score before and
   after; ATS-readable PDF; edits you can see and undo; price. Every cell must be true. Use
   "—" where we don't know, never a guess.
8. **Founder note.** A short note in the first person, signed "Ilya Strelov, founder":

   > Most AI CV tools will happily write that you led a Kubernetes migration you never
   > touched. It reads well right up to the first interview question about it. So the agent
   > here works the other way round: it only uses what you actually did, and when the job
   > asks for something it can't find in your history, it asks you instead of writing it.
   > That rule lives in the code, not in a prompt, and the code is open for you to check.
   >
   > — Ilya Strelov, founder

9. **FAQ.** Rewritten around buyer objections, still rendered from `TAILOR_FAQ` so the
   FAQPage JSON-LD keeps matching. Questions to cover:
   - Will a recruiter be able to tell an AI wrote it?
   - Can it put things on my CV that aren't true?
   - What happens to my original CV?
   - Will an ATS read the PDF?
   - How do I cancel?
   - What happens to my data?
   - Can I drive it from my own agent or the CLI? Mention the CLI in one line here.
10. **Closing CTA.** "Stop sending the same CV everywhere." with the Pro button.

**Removed:** the edit-path chip list, the CLI terminal section, the separate Export section
(it becomes one FAQ answer and one fact-strip item), and the long paragraphs.

## Hero demo — `TailorHeroDemo.svelte` (new)

A small looping animation built from plain markup. It is not a video.

- Layout: a compact vacancy card (role, three requirement chips) beside a mini CV sheet
  (name line, four bullets), with a job-match score badge in the corner.
- Loop, about 8 seconds:
  1. Show the start state, with the score at 54.
  2. The agent rewrites one bullet. The old text fades out, the new text fades in, and it
     gets a brief highlight. The matching vacancy chip turns green.
  3. Repeat for a second and a third bullet. The score counts up at each step: 54 → 68 →
     79 → 86.
  4. An amber chip stays amber and gets a small "asked you" note. This shows the honesty
     rule inside the demo itself.
  5. Hold, then reset.
- `prefers-reduced-motion`: show the final state statically, with no loop.
- Pause the loop when the demo is off screen (IntersectionObserver), so it costs nothing
  while hidden.
- `aria-hidden` on the animated parts. A visually hidden sentence describes what it shows.
- Only CSS transitions and one interval. No new dependency.
- Content lives in one typed array in the component, so the copy is easy to change.

## Files

- `web/src/lib/components/TailorLandingView.svelte`: rewritten.
- `web/src/lib/components/TailorHeroDemo.svelte`: new.
- `web/src/lib/tailorFaq.ts`: new questions. Its test keeps passing.
- `web/src/routes/features/tailor/+page.svelte`: SEO title and description updated to the
  new pitch.
- If the Pro price or checkout call is not already exported from a shared module, extract
  it from `routes/pricing` into `#lib`, so both pages read one source.

## Testing

- `pnpm check` and lint (svelte-check, eslint, knip: no dead exports).
- The existing `tailorFaq` test still passes.
- A small component test for `TailorHeroDemo`: with reduced motion, it renders the final
  state.
- Look at the page in a real browser at desktop and mobile widths, in light and dark mode,
  before merging.
