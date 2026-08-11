## Context

`internal/atscheck` scores a CV deterministically from its extracted plain text. One line item in the Format Compliance category covers contact details:

```go
item(emailRE.MatchString(cv) && phoneRE.MatchString(cv), 4, "Contact info present (email and phone)",
    "Add both an email and a phone number near the top", StatusWarn),
```

`phoneRE` has three branches: a `+`-prefixed international run, `(NNN) NNN-NNNN`, and `NNN-NNN-NNNN`. The second and third are US grouping conventions; the first is the only one that travels.

Verified against a real CV that produced the complaint: `emailRE` matches, `linkedinRE` matches, `phoneRE` does not. The number is present as a bare 11-digit run — a Brazilian mobile with no `+`, no parentheses and no separators — which no branch accepts. The candidate is then docked 4 points and told to add an email they already have, because the `&&` collapses two signals into one message.

Both halves of that sentence are defects, and they are independent: widening the pattern alone would still leave a check that misreports which detail is missing; splitting alone would still miss the number.

## Goals / Non-Goals

**Goals**
- Recognize a phone number written in the conventions candidates outside the US actually use.
- Report email and phone independently, so the advice names the detail that is absent.
- Keep contact information's total weight at 4 points, so no CV's score inflates.

**Non-Goals**
- Validating that a number is dialable, or parsing it into country/area components. The check asks "does this document present a phone number to a recruiter", not "is this number real". No phone-number library is warranted for a boolean.
- Extracting the contact details for use elsewhere. `internal/resumeextract` already gets contact fields from deterministic PII detection; this check must not become a second source of truth for them.
- Touching `linkedinRE`, `emailRE`, or any other line item. Both matched correctly on the reproducing CV.

## Decisions

### Decision: Find candidates with a regex, then validate by digit count — not one larger regex

A single pattern that accepts a bare 11-digit run while rejecting the digit runs a CV is full of would have to encode "8 to 15 digits once separators are removed", which a regex expresses only by enumerating every grouping. The rejections are also semantic rather than lexical: a year range and a phone number differ by how many digits they carry, not by their shape.

So: a permissive pattern finds *candidates* (a digit group, optionally repeated with single separators between groups, optionally prefixed by `+` or `00` and allowing one parenthesized group), and a small validator decides. The validator strips everything but digits, drops a leading country prefix, and accepts a length in the plausible band.

**Alternatives considered**
- *Add a fourth branch for the bare 11-digit case.* Fixes this one CV and leaves the next convention broken. The reported bug is that the pattern encodes US conventions, not that it is missing one entry.
- *Adopt a phone-number library.* Correct for parsing, disproportionate for a boolean line item, and a new dependency for a check that must stay pure and I/O-free.

### Decision: Accept 10–15 normalized digits

15 is E.164's maximum, so anything longer is an identifier, not a number. The lower bound is set by what it must *exclude* rather than what it must include: an employment range like `2019 - 2024` normalizes to 8 digits, and a date like `01/15/2024` to 8. A bound of 10 clears both, and still covers a US 10-digit number, a Brazilian 11-digit mobile, and international forms carrying a country code (11–13).

The cost is a bare local number of 8 or 9 digits — a Brazilian landline written without its area code, say — which stays undetected. That is the right side to err on: a false negative costs 2 points and prints an actionable fix, while a false positive tells a candidate their CV has a phone number when it does not, which is the failure mode this change exists to remove.

### Decision: Split 4 points into 2 (email) + 2 (phone)

`ScoreCategory.Max` is summed from its items, so the split needs no constant change — Format Compliance stays at 20 and the report still sums to 100. The doc comment enumerating Format's weights (`8+4+2+2+2+2=20`) becomes `8+2+2+2+2+2+2=20` and must be updated with the code, since it is the only place the arithmetic is written down.

`LineItem` carries no id — only points, text and status — so nothing downstream is keyed on the combined item, and `atscheck.Compare` joins on *category* id. The split is therefore invisible to `delta.go` and to the SPA, which renders items as a list.

Even weights, rather than weighting email higher: an ATS that cannot find a phone number and one that cannot find an email fail a candidate the same way.

## Risks / Trade-offs

- **A broader pattern will match something that is not a phone.** Bounded by the digit-count band above, and pinned by tests for the specific runs a CV carries: years, year ranges, dates, currency amounts, percentages, and long identifiers. These belong in the test table as named cases, not as an afterthought.
- **Scores move for existing users.** Only upward, and only for CVs whose phone was previously invisible. Nothing is persisted — the deterministic report is recomputed per request — so there is no stored score to migrate and no backfill. A cached LLM review is merged on top of a freshly computed report and does not carry these items.

## Open Questions

- (none)
