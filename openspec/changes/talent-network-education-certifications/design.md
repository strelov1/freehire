## Context

See `proposal.md` - Why/What Changes for motivation and scope. Two facts shape this
design:

- `internal/dict/vocab.EducationLevelValues` (`internal/dict/vocab/vocab.go:46`) already
  defines the closed vocabulary — `{"none", "bachelor", "master", "phd"}` — shared across
  every consumer that needs education-level enums. There is no "associate" value; adding
  one is out of scope (it would change a vocabulary other consumers, e.g. job-posting
  requirements, already rely on).
- The only existing text→level resolver, `internal/job/jobfacts.EducationLevel`
  (`internal/job/jobfacts/jobfacts.go:112-125`), lives in the `job` block (layer 5).
  `internal/candidate/talentnetwork` is in the `candidate` block (layer 4) and, per the
  layering table in the root `AGENTS.md`, may import `dict` (layer 2) but not `job`. That
  resolver is also shaped for job-POSTING requirement prose (its regexes require
  possessive/degree framing — "bachelor's degree", "master's", "phd" — via
  `hardRequirementText`, `internal/job/jobfacts/optional.go:60-79`, which strips
  "nice-to-have" clauses using `internal/job/reqextract`), not CV shorthand like "BSc" or
  "MSc Computer Science".

## Goals / Non-Goals

**Goals:**
- Reuse `vocab.EducationLevelValues` as the only source of truth for education-level
  strings; do not introduce a second, competing vocabulary.
- Give `candidate`-layer code a dictionary it is allowed to import, without duplicating the
  job-posting resolver's regex set or breaking its one existing caller.
- Add a certification dictionary structured the same way as the existing
  `internal/dict/skilltag` (alias → canonical, unresolved input dropped).

**Non-Goals:**
- No day/month precision, issuer, or field-of-study capture — the source data
  (`resumeextract.Structured.Education`/`.Certifications`) doesn't carry them, and
  extending CV extraction is a separate change.
- No institution name in any form, anywhere in the public card.
- No change to `resumeextract`'s LLM extraction schema or prompt.

## Decisions

### `internal/dict/edulevel`: two entry points over one vocabulary, following the `location` precedent

`internal/dict/location` already solves the "same string space, two different callers"
problem: `Parse` (job/work location) and `ParseResidence` (candidate location) live in one
package because, per its own `AGENTS.md`, "the same string means opposite things on the two
sides." Education level has the same shape — a job posting's requirement text and a CV's
degree line are both "text that mentions a degree," but tuned for very different framing —
so `edulevel` follows the identical pattern:

- `ForRequirement(text string) string` — the pure regex/alias matching extracted verbatim
  from `jobfacts.go`'s current `matchBachelor`/`matchMaster`/`matchPhD`-style checks (the
  possessive-framed forms: "bachelor's degree", "b.sc", "master's", "mba", "phd",
  "doctorate", etc., deliberately excluding bare "bs"/"ms" per the existing comment about
  "MS Office"/"scrum master" collisions).
- `ForDegree(text string) string` — a new, separate alias table tuned for CV-style
  shorthand: bare "BSc"/"B.Sc"/"BS" (in a degree-field context, not general prose, so the
  "MS Office" collision risk that justified excluding bare forms in `ForRequirement` does
  not apply — a CV's `Education[].Degree` field is never free-running prose), "MSc"/"M.Sc",
  "MBA", "PhD"/"Ph.D", "Bachelor of Science", "Master of ...", "Doctorate", etc.

Both return one of `bachelor`/`master`/`phd`, or `""` when nothing matches — no `"none"`,
which is a job-requirement-only concept (a posting can require no degree; a candidate
either has one that resolves or doesn't).

**Alternative considered:** keep `jobfacts.EducationLevel` where it is and give
`talentnetwork` its own independent CV-only resolver with no shared package. Rejected: it
would fork the vocabulary's matching logic into two places with no shared home, the exact
problem the `location` package was already built to avoid, and the `card.go` header
comment already commits to "moving the dictionary down" as the fix.

`internal/job/jobfacts.EducationLevel` becomes:
```go
func EducationLevel(description string) string {
    return edulevel.ForRequirement(hardRequirementText(description))
}
```
Same signature, same single caller (`internal/job/jobderive/jobderive.go:234`), same
behavior — `hardRequirementText`'s required/optional clause split stays in `job` since it
depends on `internal/job/reqextract`, a `job`-layer package `dict` may not import.

### `internal/dict/certification`: curated alias table, `skilltag`-shaped

`internal/dict/skilltag.Canonicalize(tokens []string, opts ...Option) []string` is the
existing pattern for "candidate-claimed free tokens → whitelisted canonical set,
unresolved dropped" (already used by `card.go:119` for skills). `certification` exposes
the same shape:
```go
func Canonicalize(tokens []string) []string
```
backed by a hand-curated alias map (AWS, Azure, GCP, PMP, CKA/CKAD, CISSP, Scrum
certifications, CompTIA family, etc. — seeded from what's observably common in CVs, grown
over time the way `skilltag`'s own list has). No acronym-ambiguity option like skilltag's
`WithResumeAcronyms` is needed: certification names are domain-specific enough that a
single alias table suffices at this scale.

### Card projection: drop, don't keep-under-empty-label

`cardRoles` (`card.go:89-109`) keeps a role whose title resolves to nothing, under empty
seniority/category, because a work-history gap reads worse than an unlabelled job. An
education entry has no equivalent argument: a candidate's set of degrees isn't a
chronological sequence a reader expects to be complete, so an entry that resolves to no
level is simply omitted, matching how an unresolved skill or certification is already
handled (dropped, not shown empty).

```go
type EducationEntry struct {
    Level string                 `json:"level,omitempty"`
    Year  *perioddate.PeriodDate `json:"year,omitempty"`
}
```

`ProjectCard` gains:
```go
Education:      cardEducation(s.Education),
Certifications: certification.Canonicalize(s.Certifications),
```
where `cardEducation` maps each `resumeextract.Education`, calls
`edulevel.ForDegree(e.Degree)`, and skips the entry when that returns `""`.

### Frontend: same chip pattern as Skills

New "Education" and "Certifications" sections in
`web/src/routes/talent/[handle]/+page.svelte`, following the existing `{#if
card.skills.length}` / `Chip` pattern. `EDUCATION_LEVEL_LABELS` (`bachelor` → "Bachelor's
degree", etc.) and `CERTIFICATION_LABELS` (canonical code → display name) join
`CATEGORY_LABELS` in `web/src/lib/labels.ts`. An education chip reads "Bachelor's degree ·
2019" when a year is present, "Bachelor's degree" otherwise.

## Risks / Trade-offs

- [`ForDegree`'s bare-acronym matching ("BSc", "MBA") is looser than `ForRequirement`'s
  possessive-framed regexes, so it carries a higher false-positive/-negative risk on the
  handful of degree titles that don't cleanly map to bachelor/master/phd (e.g. a
  professional diploma, a bare "Certificate")] → acceptable: an unresolved degree is
  dropped, never mis-labeled, so the failure mode is "this education entry doesn't appear"
  rather than a wrong level being shown.
- [The certification alias table starts small and will miss real certifications early on]
  → same trade-off `skilltag` already accepts today; the list grows from what curators
  observe missing, not upfront.
- [Modifying `talent-network-catalog`'s existing requirement text touches a spec another
  team may be relying on for the current withholding behavior] → the delta spec is
  additive to what's disclosed (institution/issuer/date/field-of-study stay withheld), and
  the modified requirement's own scenarios make the new boundary explicit.

## Migration Plan

No schema migration. Deploy order: `internal/dict/edulevel` and
`internal/dict/certification` land first (pure functions, no callers yet), then the
`jobfacts.EducationLevel` refactor (behavior-preserving, covered by its existing tests),
then `card.go`'s new fields, then the frontend sections. Each step ships independently
green; the card's new JSON fields are additive (`omitempty`), so no frontend/backend
version coupling is required.
