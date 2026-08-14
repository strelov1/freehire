# OCF (Open Career Format) export/import — research pass

Research pass for [#1873](https://github.com/strelov1/freehire/issues/1873). The ticket asks
for a decision on whether OCF export is worth building, not code.

**Decision: not yet. Note the seam, write down the mapping, and don't build.** Two named
triggers below are what should reopen it. The interesting part of this pass is not the mapping —
that is small — but that OCF's provenance model and freehire's are philosophically incompatible,
and the resolution turns out to be cheap and worth recording before anyone starts.

## What OCF is, verified

`opencareerformat.org`, v0.3, last site update 2026-08-04. The schema is real and reasonably
thought through: a `person` + `meta` root, then `skills`, `competencies`, `experience`,
`projects`, `education`, `certifications`, `languages`, `goals`, `sourceArtifacts`, and a long
tail (`governance`, `teaching`, `speaking`, `patents`, `publications`, `awards`, …). Experience
nests positions under one organization/period so a promotion inside one employer stays one
tenure. Skills carry a category, proficiency, date range, aliases and taxonomy URIs (O*NET /
ESCO / SFIA).

It models the same thing `internal/experience` does — the career behind the CV rather than a
rendered CV — so the conceptual fit is genuinely good. That is what makes this worth a written
answer rather than a one-line decline.

## Adoption, measured

The ticket flags "near-zero adoption today" as a hunch. It checks out, and the numbers are worse
than the word "near-zero" implies:

| Signal | Value (checked 2026-08-14) |
|---|---|
| Repo | `opencareerformat/opencareerformat` |
| Stars | 5 |
| **Forks** | **0** |
| Watchers | 2 |
| Created | 2026-05-22 (under three months old) |
| Last push | 2026-08-04 |
| Open issues | 1 |

Stars are a bookmark. **Zero forks is the number that matters**: nobody has copied the schema,
which means nobody has implemented against it. There is no published list of adopting tools. So
building an exporter today means being the first implementation of a three-month-old v0.3 schema
with one author — and the first implementation of a format nobody reads is a file nobody opens.

The site states no stability or deprecation policy, which is the other half of the risk: v0.3 → v0.4
carries no compatibility promise anyone has written down.

## The finding: two incompatible provenance philosophies

This is the part worth keeping regardless of what gets built.

**freehire's provenance is a binary, enforced, fail-closed gate.**
`experience.Provenance.Publishable()` is the single predicate the whole capability turns on:
`cv_import` / `stated_in_chat` / `manual` may reach a CV, `agent_inferred` may not, and an
unknown value is never publishable so a value escaping validation fails closed. The check lives
in the service path rather than in a system prompt, deliberately.

**OCF's provenance is advisory annotation, and it is spread across four fields**: `meta.source.kind`
(authored/imported/converted/merged/translated) at file level, a per-item `provenance` object
that is *open-shape* (tool, date, source, confidence — no fixed vocabulary), a per-item
`reviewStatus` (unreviewed/reviewed/needs-review/superseded), and `visibility`
(public/shared/private), with `sourceArtifactId` pointing into a `sourceArtifacts` registry. The
ticket's own quote is the crux: the schema "defines them but does not enforce how tools honor
them."

There is no faithful mapping between these, and the near-miss is the dangerous part.
`reviewStatus` looks like the counterpart and is not: it records *whether a human checked an
item*, where freehire's enum records *who originated it*. An `agent_inferred` atom the candidate
never confirmed is fairly described as `unreviewed` — but so is a `cv_import` atom, which nobody
reviewed either and which is fully publishable. Mapping the gate onto `reviewStatus` therefore
loses the distinction in both directions at once.

### The resolution: enforce by omission, not by annotation

**Export only publishable atoms.** Do not export `agent_inferred` material with a warning
attached; leave it out of the document entirely.

The reason is structural rather than fussy. Any annotation-based scheme relies on the consumer
honoring an advisory field the spec explicitly does not require them to honor — so exporting a
model-originated claim with `"reviewStatus": "unreviewed"` ships exactly the harm the gate exists
to prevent, merely relocated outside our process, where a downstream tool renders it into a CV
with no gate at all.

This is also already the house style. `ghost.ContributorGate` withholds a contributor count below
the threshold rather than serving it redacted, and the reasoning written at that constant applies
here verbatim: *absence is what makes the guarantee structural — with a single witness there is
no count to serve, so no later caller can forget to redact one.* Same shape. A claim that is not
in the file cannot be mis-rendered by a consumer who ignored an advisory field.

Cost, stated plainly: the export is then not a complete copy of the candidate's career memory,
which is arguably what OCF is *for*. That is the right trade — an incomplete honest document
beats a complete one whose safety depends on strangers.

When it is built, freehire's exact enum should still ride along inside the open-shape `provenance`
object (it is open-shape precisely so tools can do this) as a lossless record for any consumer
that does care. It just must not be the thing the guarantee rests on.

## Mapping sketch

Small, and mostly mechanical. Recorded so the next person does not re-derive it.

| OCF | freehire source | Note |
|---|---|---|
| `person` | `resumeextract.Structured` contacts | Subject to the contact-layer precedence in `internal/resume` — owned block, then current extract, then provisional |
| `experience[]` | `experience.Employment` (`kind = job`) + its atoms | OCF nests positions under one org; the bank is already one row per (company, role) |
| `projects[]` | `experience.Employment` (`kind = project`) | The bank's kind split lines up exactly |
| `experience[].achievements` | `experience.Atom.Claim` / `.Context` | **Publishable atoms only** |
| `skills[]` | `Atom.Skills`, canonical `internal/skilltag` slugs | Slugs are already a controlled vocabulary; OCF's `taxonomies` field is where an O*NET/ESCO mapping would go if ever wanted |
| `education[]`, `certifications[]`, `languages[]` | `resumeextract.Structured` | Per the boundary table in `internal/experience/AGENTS.md`, these belong to `resumeextract`, not the bank |
| `goals` | `internal/userprofile` | Not mentioned in the ticket but the obvious source: specializations, wanted/excluded skills, location preferences |
| `sourceArtifacts[]` | — | Nothing to map. The bank records *how* an atom entered, not *which file* it came from; there is no artifact registry to point at |
| `meta.source.kind` | — | `"authored"` for a freehire-generated document |
| `competencies` | — | No counterpart. Cross-career narrative clusters are not something freehire derives |

Two gaps worth noting: `sourceArtifacts` has no counterpart at all (our provenance is an enum, not
a pointer to a retained artifact), and `competencies` would have to be invented. Neither blocks an
export — both fields are optional — but it means an OCF document from freehire is a partial one,
which is a second reason not to lead with it as an interop story.

## Import

Materially harder than export, and it should not ship with it.

Every atom in an inbound OCF document arrives with unknown origination as far as our gate is
concerned, and our enum has no value for it. Each existing option is a false statement:
`cv_import` claims the candidate asserted it, `agent_inferred` claims our model originated it. So
import needs a fifth provenance value that is **not** publishable, plus a confirmation flow to
promote items out of it — which is the same shape as the existing agent-inferred confirmation
path, and roughly the same amount of work.

The tempting shortcut is to trust the inbound `reviewStatus`: an item marked `reviewed` with a
`reviewedBy` was, on its face, confirmed by the candidate. That must be refused, and for exactly
the reason the export section gives — an advisory field carries no guarantee, and trusting it on
the way in while distrusting it on the way out would be incoherent. Symmetric skepticism: on
import, everything lands unpublishable until the candidate confirms it here.

Import is also where the ticket's non-goal has teeth: OCF's `goals` and the career-ops mapping's
"operational overlays" (comp ranges, urgency, location preference) are exactly the operational
state we said we would not take back. Since `goals` maps to `internal/userprofile`, an import path
would have to deliberately drop the half of `goals` that is targeting state.

## Why not build the export anyway, given it is cheap

The mapping is a day's work, so the case for shipping it opportunistically is real. Three things
outweigh it:

1. **AGENTS.md answers this directly** — "don't build infrastructure before there's a concrete
   need (note the seam for later instead)". Zero forks is the absence of a concrete need, stated
   as a number.
2. **The user-facing need is already met.** `GET /me/resume` and `GET /me/profile` already serve
   the candidate their own structured data, and the talent-network page already renders a
   shareable public projection. OCF export is not new data portability; it is one specific
   third-party encoding of portability we have.
3. **The stated motivation points at a party that has twice declined to integrate.** The format
   side needs nobody's permission, which is true and is why the ticket is open — but the *value*
   of the format side is largely "feed career-ops", and career-ops' own mapping describes a
   one-way bootstrap a user could perform by hand from an existing export today.

## Triggers to reopen

Named so this is a decision rather than a deferral:

- **A second independent implementation appears** — a fork, or any tool that reads OCF that is
  not the spec's own author. That is the cheapest possible evidence that a file we emit would be
  read by something.
- **A user asks for it.** One real request beats any amount of format-watching, and the mapping
  above means the answer can be "a day", not "a project".

Absent either, revisit when OCF reaches a version with a written compatibility policy. Pin `v0.3`
in whatever gets built when it does — the ticket is right about that — and treat the schema URL
as a version-pinned constant rather than fetching it.

## Related

- `internal/experience/AGENTS.md` — the bank's boundary table, which is what makes the mapping mechanical
- `internal/experience/experience.go` — `Provenance`, `Valid()`, `Publishable()`, the fail-closed rule
- `internal/resumeextract/structured.go` — `Structured` / `Professional`, the contact-free projection
- `internal/resume/AGENTS.md` — the three contact layers and their read-time precedence
- `internal/ghost/AGENTS.md` — `ContributorGate`, the precedent for guaranteeing by absence
- Issue #1873, discussion #1342, and #1082 / #1197 for the listings-side history
