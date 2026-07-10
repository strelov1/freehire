## Context

`POST /api/v1/me/resume/extract` (`internal/handler/resume.go` `ExtractResumeSkills`, cookie-auth) reads a PDF or `{text}` and returns `{skills}` via `skilltag.Parse` — a deterministic dictionary, no LLM. It also stores the résumé and background-embeds it (feeding `/me/recommendations`). Three sibling dictionaries already exist and speak the same controlled vocabularies (`enrich.SeniorityValues`/`CategoryValues`): `classify.Parse(title) → {Seniority, Category}` (ordered aliases → deterministic pick, whole-word via `wordmatch`), and `roletag.Derive(seniority, category, title) → []roleSlug`. The onboarding wizard (`OnboardingWizard.svelte`) captures `sel.{specialization(category), seniority, workMode, region, stack(skills)}` from pill/typeahead pickers and has no CV path. `ProfileForm.svelte` already calls `extractResumeSkills` and merges the returned skills into its `skills` field (but never touches `specializations`).

This change widens `extract` to return the dictionary-derived seniority + categories alongside skills, and consumes it in the wizard (a new CV path, with Focus and Seniority made multi-select) and the profile form (specialization pre-fill). `roletag` is not used — the wizard maps to the category/seniority facets, and roles remain the full FilterModal's job.

## Goals / Non-Goals

**Goals:**
- Pre-fill the wizard's focus/seniority/stack from a résumé, deterministically, reusing the existing dictionaries — no LLM, no new endpoint, no DB change.
- Keep it best-effort and non-blocking: unresolved fields stay empty, failures fall back to manual.
- One refactor improves two surfaces (wizard + profile form).

**Non-Goals:**
- No LLM CV parser (deliberately — the deterministic dictionaries are the project's doctrine and stay instant/free).
- No work-mode/region from résumés (not stated on a CV). No `user_profiles` auto-write beyond the profile form's existing merge. No new résumé formats (PDF/text only, as today). No change to résumé storage/embedding or auth.

## Decisions

**D1 — Dictionaries, not an LLM.**
`skilltag` (skills, whole text) and `classify` (seniority + categories) — deterministic, vocabulary-aligned, "never guess." Skills scan the whole CV; seniority/categories scan a `resumeHeadline` slice, not the full text, so a grade word buried in the career history can't over-promote the current grade (and a generic title simply yields no category rather than a guess). *Alternative:* an LLM CV parser (more robust on messy CVs). Rejected — it adds cost, latency (~2s), a prompt/contract/sanitize layer, and contradicts the dict-only facet doctrine; emitting nothing on an unresolved field is the on-brand behavior.

**D1a — Headline extraction, robust to PDF quirks.**
`resumeHeadline` collapses all whitespace (PDF text extraction often emits one token per line, so a line-based scan stalls on the name), drops contact/metadata tokens (email, phone, profile URL, bare numbers, `|`/punctuation), and keeps a bounded leading window (`headlineRunes`). Wide enough to clear the name/contact preamble and reach the title + a few summary words; tight enough to exclude the experience section below.

**D1b — Multi-category, not a single primary.**
A résumé can span several functions (backend + ML), so `classify.Categories` returns *every* category the headline names (distinct, precedence order), not just the strongest. `classify.Parse` still supplies the single seniority grade.

**D2 — Widen `extract` additively, don't add an endpoint.**
`ExtractResumeProfile` (renamed from `ExtractResumeSkills`) runs `classify` over the headline and returns `{skills, categories, seniority?}` (`skills`/`categories` always arrays, `seniority` omitted when unresolved). Existing `{skills}` consumers keep working (the extra fields are additive). One endpoint stays the single résumé-derivation path. *Alternative:* a parallel `/me/resume/parse`. Rejected as redundant — same input, same side effects.

**D3 — CV path is auth-gated, reusing the existing endpoint.**
The résumé endpoint is cookie-only (it stores + embeds to the account). The wizard's CV button therefore prompts sign-in first (`openAuthDialog`), then opens the file picker — so the file is chosen already-authenticated and no file-across-redirect handoff is needed (an OAuth redirect just reopens the wizard). *Alternative:* an anonymous parse-only endpoint. Rejected — chosen for the persistence bonus (résumé feeds recommendations) and to avoid an unauthenticated upload surface.

**D4 — Pre-fill in place, multi-select, only the resolved fields.**
On a successful extraction the wizard merges `categories`/`seniority`/`stack` into `sel` (dedup, keeping any manual picks) and **stays on the current step** with a note — so the user reviews and corrects the pre-filled pills rather than being advanced past the "What do you do?" step. Focus and Seniority are **multi-select** (a person can be several specializations; a search can span a grade range), so the extraction's several categories all light up. A generation counter drops a parse that resolves after the wizard was reset. *Earlier alternative:* advancing to step 2 on success — rejected, it skips review of the fields just filled.

**D5 — Component boundaries.**
- Backend: `resumeHeadline` + `looksLikeContactToken` + `resumeProfile(text) → {skills, seniority, categories}` in `internal/handler` composing `skilltag` + `classify` (incl. the new `classify.Categories`); `ExtractResumeProfile` calls it and serializes. Unit-tested against sample text (contact-first, one-token-per-line, multi-category, buried-grade).
- Frontend: `api.ts` `extractResumeProfile` returns the widened type; `OnboardingWizard.svelte` owns the CV affordance + `parsing`/`error`/note state, the multi-select Focus/Seniority (`toggleIn`/`multiPills` over `specializations[]`/`seniorities[]`), and the result→`sel` merge; `ProfileForm.svelte` merges `categories` into specializations. The wizard's manual flow is otherwise untouched.

## Risks / Trade-offs

- **[Dictionary picks the wrong role from a multi-role CV]** → It's a reviewable pre-fill on a step the user confirms; `classify`'s ordered aliases give a stable, explainable pick, and the user can change any field before applying.
- **[Résumé names no recognizable role/skills]** → Best-effort nulls; the wizard simply pre-fills less and the user continues manually — no failure.
- **[Widening a shared endpoint breaks a caller]** → Additive only; `ProfileForm` (the sole caller) reads `skills` and now also `category`; a round-trip/shape check covers it.
- **[Auth friction on the CV path]** → Deliberate (D3); the manual wizard path stays fully anonymous, so only the opt-in CV shortcut asks for sign-in.
- **[Non-PDF résumé / large file]** → Unchanged from today: PDF/text only, bounded by the server `BodyLimit`; a bad upload returns an error the wizard surfaces as its fallback.

## Migration Plan

Additive backend refactor + frontend feature — no schema, no new endpoint, no data migration. If the extract response type is generated into `web/src/lib/generated/contracts`, regenerate it; otherwise the widened shape is a hand-written TS type. Rollback is reverting the handler + web changes; the extra response fields are ignored by any client that doesn't read them.

## Open Questions

_Resolved during implementation:_
- Full text vs. a leading slice for the title dictionaries → a `resumeHeadline` slice (D1a), after full-text scanning over-promoted the grade from career-history mentions.
- One category vs. several → several (D1b); the wizard's Focus (and Seniority) became multi-select (D4).
- Advance after pre-fill vs. stay for review → stay on the step (D4).
- CV affordance placement → a step-1 "Upload CV — autofill" button above the manual pickers.
