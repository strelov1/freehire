## Context

`normalizeHeading` (`internal/job/reqextract/reqextract.go`) already runs every heading through `unidecode.Unidecode` before matching — the same transliteration `normalize.Slug` applies elsewhere — so a non-Latin heading is not a new code path, only new vocabulary entries in the spelling `unidecode` produces for it. Hungarian already proved this: "Előnyt jelent" is entered as "elonyt jelent".

The local dev database holds 2002 real `tbank.ru` (Russian) postings. Sampling 500 of them (excluding a heavily-duplicated courier-role template that skews raw counts) found 369 postings with varied roles and real heading usage. Counting `<h1>`–`<h6>` text across them found "Требования" 683 times and "Мы предлагаем" 683 times — a strong, real signal, not a guess.

Checking what follows each "Требования" heading — the thing that actually decides whether `Derive` extracts anything — found `<p>` in all 683 cases and `<ul>`/`<ol>` in none. `Derive`'s walk (reqextract.go) only reads list items from `atom.Ul`/`atom.Ol`; when a `<p>` too long to be a heading candidate is reached first, it closes the section as prose (the same rule that keeps a benefits list two paragraphs below "Requirements" from being read as a requirement). This is not specific to Russian — an English posting shaped the same way (heading, then paragraphs, no list) already yields nothing today.

## Goals / Non-Goals

**Goals:**
- Recognize the measured, real Russian headings, the same way Hungarian ones are already recognized — closing the specific gap `AGENTS.md` names.
- Be honest, in the proposal, in `AGENTS.md`, and in the test suite, about what this does and does not change for the one real large source measured.

**Non-Goals:**
- Making `Derive` extract from a `<p>`-per-item section. That's a change to the shared walk every language and source goes through — larger, riskier, and a different problem than vocabulary coverage. Left as an open limitation.
- Adding Ukrainian, or any language beyond what was actually measured. `AGENTS.md`'s own new limitations bullet says to measure before adding, not to translate the four entries here into other languages speculatively.
- Adding "Обязанности" (duties/responsibilities) to `requiredHeadings`, despite it being the single most common heading in the sample after "Требования"/"Мы предлагаем". It answers "what will you do", not "what is required of you" — the same category the vocabulary already excludes for English ("what you'll do" is not in `requiredHeadings` either).

## Decisions

**Get the exact transliterated spelling by running `unidecode.Unidecode` itself, not by hand-transliterating.** Different transliteration schemes disagree on some Cyrillic letters (e.g. "я" as "ia" vs "ya"); a vocabulary entry spelled by a different scheme than the one `normalizeHeading` actually uses would silently never match — a dictionary miss produces no error, just a heading that quietly fails to open a section. `unidecode.Unidecode("Требования")` was run directly (a throwaway `go run`, not committed) and produced `"Trebovaniia"` — that exact lowercase form, "trebovaniia", is what's in the vocabulary.

**Add "Мы ждём от вас" despite the specific postings it was measured on not being where the fix has most value.** It comes from the heavily-templated courier-role posting type, whose own "Требования" analog is followed by MORE headings, not a list either — so this specific entry doesn't yield anything on the sample it was measured against, same as the others. It is still real, observed, high-frequency (631 occurrences) usage, semantically equivalent to the existing English "what we expect" entry already in the vocabulary, and plausible on other Russian-language sources this session had no local sample of (djinni, earcu) that may use a list under it. Not fabricated — measured, just measured on a posting shape that doesn't benefit yet.

**Document the `<p>`-vs-list gap as its own new `AGENTS.md` Limitations bullet, not folded into the language bullet.** They read as one discovery but are two different problems with two different fixes: widening a vocabulary is additive and low-risk; teaching `Derive` to read `<p>`-per-item sections touches the shared walk. Keeping them as separate bullets keeps a future reader from assuming "add more languages" would also close the structural gap.

## Risks / Trade-offs

- **[Risk]** A commit message or PR description that only says "add Russian support" would overclaim, since the one large real source checked gets zero benefit today. → **Mitigation**: proposal.md, `AGENTS.md`, and the test suite's own negative case all state this plainly; nothing here is +document-then-hope.
- **[Trade-off]** The value of "Мы ждём от вас" specifically is unverified beyond "it's real text" — its own sample doesn't demonstrate a list following it. → Accepted: it's semantically sound, matches an existing English entry's shape, and costs nothing to have in the vocabulary (an unmatched heading is simply never reached; a matched one that finds no list already degrades safely, per the new spec requirement's second scenario).
