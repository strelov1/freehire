## Context

Three surfaces describe a skill, and each answers a different question:

| Surface | Question | Title today |
|---|---|---|
| `/skills/kotlin` | what is it | "What is Kotlin? · freehire" |
| `/collections/kotlin` | who is hiring for it | "N Kotlin jobs · freehire" |
| `/jobs?skills=kotlin` | the live filter | canonical → `/jobs` |

The division is right and predates this change. What is missing is that the first two do not
know about each other, and the first links to the third.

The pattern to copy already exists in the same file. `collectionForCategory` links
`/roles/[category]` to its curated feed, and the collection page carries `marketLink` back —
a reciprocal pair with the guard written into it: *"Matches only a collection whose ONLY pin
is the category. A feed pinning the category alongside anything else is narrower than 'all
&lt;category&gt; jobs', and offering it under that label would send the reader somewhere
smaller than promised."* This change is the same pair for the skill axis.

## Goals / Non-Goals

**Goals:**

- The glossary's job links go to an indexable page when one exists.
- The collection page names what its subject actually is.
- The mapping is by params, never by slug, so a name collision cannot mislink.

**Non-Goals:** anything in proposal.md's Non-Goals — the 830 collection-less skills, the
glossary's title, and `/jobs?skills=`'s canonical.

## Decisions

### Match on params, not on slug

A slug match looks obviously correct and is wrong for four of the fifty candidates:

| Slug | Is a skill | Collection pins | Link? |
|---|---|---|---|
| `python`, `kotlin`, `terraform`, … (45) | yes | `{ skills: <slug> }` | **yes** |
| `data-science` | yes | `{ category: 'data_science' }` | no |
| `data-engineering` | yes | `{ category: 'data_engineering' }` | no |
| `devops` | yes | `{ category: 'devops' }` | no |
| `machine-learning` | yes | `{ category: 'machine_learning' }` | no |

A category feed and a skill facet are different sets. `/collections/devops` is every posting
the classifier filed under DevOps; `skills=devops` is every posting that names DevOps as a
skill, including postings filed elsewhere. Offering the first under a link that reads "the
open DevOps jobs" from the DevOps *skill* page states something untrue about which set the
reader is about to see.

Two guards, both load-bearing:

- **Exactly one param.** Same reason `collectionForCategory` has it: a feed pinning the skill
  alongside a region is narrower than "the open Kotlin jobs".
- **The value is a scalar.** `FilterCollection.params` is `Record<string, string | string[]>`
  and a list expands to repeated keys with OR semantics. `{ skills: ['go', 'rust'] }` is not
  the Go page's set. No current entry is a list; the guard is for the next one.

### The reverse link needs no catalog read

The obvious worry is that the collection page would have to learn whether a glossary entry
exists, which means reaching for the description catalog — a dynamic `import()` that page has
no other reason to pay for.

It does not. `skillDescriptions.ts` records the invariant: *"There is no 'is this described?'
reader any more. Every canonical carries an entry."* The `skills` value a filter collection
pins is a canonical facet value, so its glossary page exists by construction. Confirmed rather
than assumed: all 45 target slugs appear in `sitemap-skills.xml`, whose own spec requires it to
list exactly the pages the route serves.

So both helpers are pure and synchronous, like `collectionForCategory` — no new fetch, no new
await, nothing added to the page's critical path.

### The reverse direction cannot read `collection.params`

The obvious implementation of the reverse link is to test `collection.params` in the landing
page's load, the way `marketLink` beside it tests `params.category`. It is wrong, and finding
out why is the reason `skillForCollection` exists as a second helper rather than three lines
inline.

`ResolvedCollection.params` is `Record<string, string>`, not `Record<string, string | string[]>`:
`scopeParams` has already flattened any list-valued pin, **taking its first value**. So a
`{ skills: ['go', 'rust'] }` feed arrives at the load looking exactly like a `{ skills: 'go' }`
one, and the scalar guard that works on the raw registry cannot be written there at all — there
is nothing left to test. The page would link a two-skill feed to the Go glossary entry it is
not about.

`marketLink` carries the same latent hole for `category`. It has never bitten because no
category pin is list-valued, and this change does not fix it — noting it here is cheaper than
a drive-by edit to code this change is not otherwise touching.

`skillForCollection(slug)` therefore reads `FILTER_COLLECTIONS` directly, where the list is
still a list. Both directions then live beside each other in one file, which is where a reader
comparing them will look.

### Where each side computes its link

The glossary page computes it in the component, because `jobsHref` is already derived there
and both consumers read that one value. The collection page computes it in `+page.server.ts`,
because `marketLink` is computed there and a sibling link belongs beside its sibling. Neither
placement is a judgement call; each follows what the file already does.

## Risks / Trade-offs

- **A skill's collection could later gain a second pin**, at which point the guard silently
  drops the link and the glossary page falls back to `/jobs?skills=`. That is the correct
  failure — the same one `collectionForCategory` chooses — and it is silent by design: a
  narrower feed under a broader promise is worse than no link.
- **45 collection pages gain a line of markup.** Small, and it sits with an existing optional
  line rather than introducing a new region.
- **Nothing here helps the 830.** The change makes the best-connected skills better connected
  and leaves the long tail exactly as it is. That is a real limit, stated rather than papered
  over: closing it means deciding whether those sets deserve indexable pages at all, which is
  a much larger question than a link.

## Migration Plan

None. Frontend links only; no schema, no index, no data. Reverting is reverting the commit.
