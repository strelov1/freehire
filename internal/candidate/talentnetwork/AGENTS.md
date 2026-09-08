# AGENTS.md — internal/candidate/talentnetwork

The public catalogue of candidates who asked to be found: the membership handle, the
projection that decides what a stranger may see, and the filtered list served over it.

Block `candidate`, layer 4. It reads `dict` (classify, skilltag) and `platform` (db), and
nothing above. The recruiter-facing half of this product — approved accounts that see
names and employers — will live in `engage`, layer 7, and will import this. The reverse is
what the layering guard exists to prevent, so **do not** reach upward from here for
anything.

## The one rule

> Everything this package publishes is a value a DICTIONARY resolved, a number, or a
> date. Nothing a candidate typed reaches a public response.

That is stricter than masking the fields which identify somebody, and the difference is
why `ProjectCard` exists instead of reusing `resumeextract.Anonymous`. `Anonymous`
replaces the current role's employer column and leaves everything else alone — right for
a link a candidate hands to one person, wrong for a page a crawler reads. The employer's
name is usually sitting in the prose beside the column that was masked:

- `summary`: "at &lt;employer&gt; I rebuilt the billing pipeline"
- a role's `summary` or `highlights`
- the job title: "Backend Engineer @ &lt;employer&gt;"
- a project's `name` (usually the employer's product) or `highlights`
- `institution`, and the free-text `location`

The whitelist also fails in the safe direction. Masking named fields means the next field
the extraction contract grows is published by default; a whitelist means it is withheld
until somebody adds it deliberately.

`card_test.go` is the guard: one CV, run once per hiding place, asserting a distinctive
employer token appears nowhere in the marshalled card. **Add a case to it whenever the
extraction contract grows a field.**

## What the rule costs

Languages, certifications and education are absent. All three are free text, and no
dictionary this block can reach resolves them: `vocab.EducationLevelValues` exists, but
the text→level resolver lives in `internal/job/jobfacts` — block `job`, layer 5, which
`candidate` may not import. Publishing them means moving that dictionary down into `dict`
first, which is a change of its own.

## The handle

`/talent/<handle>` is a member's only public address. Minted on the first join and never
recomputed: a URL that moves is a URL somebody has already shared.

It is deliberately **not** the account's `username`. `internal/identity/username`'s doc
comment anticipates this use, but `Suggest` derives that name from the email's local part,
so for most accounts it *is* the person's name — and the hosted mailbox adopts the same
string as a live address.

The readable part is the category the candidate's current role resolves to; the suffix
carries the uniqueness. The SENIORITY is left out on purpose: the handle is frozen at
mint, and a grade is the part of a title most likely to change.

Minting is `MintHandle`; the claim is idempotent by its own `talent_handle IS NULL`
predicate, which is what makes leaving and rejoining keep the same URL. A collision
re-mints a suffix, the shape `internal/identity/accounts` uses for usernames.

## The snapshot, and its seam

`Catalogue` reads the whole membership, projects it, and serves filtered pages from an
in-memory snapshot refreshed on a TTL.

That is not a shortcut around SQL — it is what the data allows. Category and seniority do
not exist as columns: they are derived from a job title by a dictionary that changes
weekly, so `WHERE category = ?` would first need them stored, backfilled, and kept in
step. At a membership in the hundreds the whole catalogue is a few megabytes.

**The seam:** when the membership outgrows a snapshot that fits comfortably in memory —
order of thousands — the projection moves to a table a worker writes and the filtering
moves to SQL. Neither the handler nor the wire shape has to change; only `Catalogue`'s
internals do.

Three behaviours worth knowing before changing it:

- A refresh that fails while a snapshot exists is **swallowed**. A stale list beats an
  empty one, because an empty one does not read as an outage — it reads as "nobody is in
  the network". A cold catalogue that cannot read reports the error.
- Concurrent readers refresh **once**. This reads the whole membership off `users`, the
  table every authenticated request already touches.
- `ByHandle` reads the **database**, never the snapshot, so leaving takes effect on the
  next request rather than when the snapshot expires.

## Wire shapes are generated

`CandidateCard`, `CandidateRole` and `CatalogueMember` live in `card.go` and are generated
into TypeScript by `cmd/gen-contracts`. Generated rather than hand-kept because of what
they are for: the card decides what a stranger sees about a person, and a copy in
`types.ts` would drift the day somebody adds a field.

They carry those names, rather than `Card`/`Role`/`Member`, because every generated
contract lands in ONE TypeScript file and `Card` already belongs to jobview's job card.
**Renaming one of them means checking that file for a collision first.**
