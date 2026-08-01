## Context

`resumeextract.Structured` is the full parse of a stored CV. `resumeextract.Professional` is its
contact-free projection, and `structured.go:57-64` states the design rule in place:

> The field set is a whitelist, deliberately: a field later added to `Structured` is withheld
> until it is added here too. A blacklist — dropping the four known contact keys — would
> disclose that new field by default, which is the wrong way round for a projection whose whole
> job is to hold personal data back.

Three consumers act on that rule, and they do not agree on how it is held:

| Consumer | Producer marshals | Consumer receives | Enforcement |
|---|---|---|---|
| `/me/profile` (`me_profile.go:92`) | — | `*resumeextract.Professional` | the type |
| fit chain (`match_analysis.go:207`) | `resumeextract.Professional` | `string` | producer discipline, plus a defensive re-projection |
| ATS review (`ats_report.go:212`) | **`resumeextract.Structured`** | `string` | a four-key blacklist in `atscheck.stripContacts` |

The complement of `Professional` within `Structured` is exactly `full_name`, `email`, `phone`,
`links` — the four keys the blacklist deletes — so nothing leaks today and no output changes.
That coincidence is the whole hazard: the two mechanisms agree until somebody adds a field, and
the one that then diverges is the one whose own package documents why it must not exist.

The fit chain's defence is worth naming precisely, because it is not wrong, only expensive:
`candidateContext` unmarshals the string into `Structured` and calls `.Professional()` again.
Since `candidateProfileJSON` marshalled a `Professional` and `Professional`'s JSON keys are a
subset of `Structured`'s with identical names, the second projection can only ever be a no-op.
It is a round trip that exists because the field type cannot state what the field holds.

## Goals / Non-Goals

**Goals:**

- One typed seam. A model-facing consumer receives `resumeextract.Professional`, not a string it
  has to trust or re-filter.
- Delete the blacklist rather than fix it. `stripContacts` is the mechanism `resumeextract`
  argues against; a correct blacklist is still a blacklist.
- Identical bytes to both models before and after. This change removes a latent fail-open; it is
  not a behaviour change, and its tests should say so.

**Non-Goals:**

- A new package or interface for "de-identified CV". There are three producers and two model
  consumers; a shared type already exists and is the right one.
- Touching the raw-CV path. Extraction is the one place the raw text reaches a model, it is
  masked by the PII detector, and `cv-pii-masking` owns that rule.
- Changing what the ATS review judges. It reads the structure OF THE UPLOADED FILE, deliberately
  not the experience bank; that stays.

## Decisions

**`Analyze` takes `resumeextract.Professional`, and `internal/atscheck` imports
`internal/resumeextract`.** The alternative — keep the `string` and simply have the handler pass
`Professional` marshalled — leaves the signature saying "any JSON will do" while a comment says
otherwise, which is the state being fixed. `internal/matchanalysis` already imports
`resumeextract` for exactly this, so the dependency direction is established, not new.

**"No résumé" is the caller's decision, expressed as `ok bool`.** Today `Analyze("")` returns
`(nil, nil)` and the handler cannot tell "unconfigured" from "no structure". With a typed
argument the analyzer would have to test a zero value, which is a third way of asking the same
question. Instead the reader returns `(resumeextract.Professional, bool)` and `PostATSReport`
skips the model call when `!ok`, so the analyzer keeps only its own nil-client guard.

*Alternative considered — `*resumeextract.Professional`, nil meaning absent.* Equivalent in
effect, but a pointer invites a nil dereference at every use, and the codebase already spells
this shape as `(value, ok)` in `resume.Store.Structured`.

**`candidateContext` collapses to marshal + truncate.** With the field typed, the unmarshal and
the second `.Professional()` have nothing left to defend against. Keeping them "just in case"
would be a guard against a caller the type no longer permits.

**The spec text stops enumerating the four keys.** `cv-ats-score`, `job-fit-analysis` and
`assistant-agent-runtime` each name `full_name`, `email`, `phone`, `links` in prose. That is the
blacklist restated three more times: adding a field to the projection means editing three
specs, and forgetting leaves three documents asserting a set that is no longer the set. The two
requirements this change touches name the projection instead. `assistant-agent-runtime` is left
alone — it is not in this change's blast radius, and is worth a follow-up rather than a drive-by.

## Risks / Trade-offs

- **A reviewer may read "no behaviour change" as "no test needed".** → The change is proved by a
  test that adds a field to a `Structured` fixture and asserts it reaches neither model's prompt.
  That test fails against the old `stripContacts` and passes after, which is the only honest way
  to demonstrate a latent fail-open was closed.
- **`internal/atscheck` gains a domain dependency.** → It is one import, in the direction
  `matchanalysis` already goes, and it replaces `encoding/json` map surgery. If `atscheck` ever
  needs to serve a non-CV document, that is when the seam becomes an interface — not before.
- **The `ok` flag adds a branch to `PostATSReport`.** → It replaces an empty-string sentinel
  threaded through two packages. The branch is where the decision actually belongs, next to the
  other two "serve the deterministic report" exits.

## Migration Plan

None. No schema, no wire format, no cached-value shape: the ATS review is cached as the derived
`Review`, not as its input, and the fit analysis likewise caches its output. Rollback is the
revert.
