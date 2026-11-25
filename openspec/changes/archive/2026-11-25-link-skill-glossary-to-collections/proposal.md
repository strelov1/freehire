## Why

For 45 skills the site publishes two indexable pages about the same subject and no link
between them.

`/skills/kotlin` is the glossary entry. `/collections/kotlin` is the feed, titled "N Kotlin
jobs", self-canonical, already in the sitemap. The glossary page's two job links — the
"N open Kotlin jobs" sentence and "See all N →" — both point at `/jobs?skills=kotlin`, and
that URL's canonical is `https://freehire.me/jobs`. So the page's most traffic-shaped link
hands reader and crawler a URL that declares itself not canonical, while the page built for
exactly that set sits one slug away, unlinked. `/collections/kotlin` does not link back
either. Verified on prod 2026-09-08 for `kotlin`, `terraform`, `kubernetes` and `python`.

The glossary spec already holds the principle this violates: *"Every link the glossary
publishes is held to the same rule — the neighbouring-skills block links only to skills that
have a page, because a block of 404s on a page whose claim is that it is worth linking to is
worse than no block."* A link to a canonicalized-away URL is the same failure in a quieter
form: not a 404, but an address the site itself says is not the address.

Measured against the registry and `sitemap-skills.xml`:

| | Count |
|---|---:|
| Glossary pages | 875 |
| …whose skill has a collection pinning **only** that skill | **45** |
| …whose slug matches a collection that pins something else | 4 |
| …with no collection at all | 830 |

The 45 are the commercially valuable ones — `python`, `java`, `react`, `aws`, `kubernetes`,
`typescript`, `terraform`. The 4 are why this cannot be a slug match: `data-science`,
`data-engineering`, `devops` and `machine-learning` are each both a skill and a collection,
but the collection pins a **category**, so it is a different set and linking to it would send
the reader somewhere other than the page promised.

## What Changes

- **`collectionForSkill(skill)` joins `collectionForCategory` in `web/src/lib/collections.ts`**,
  same shape and same guard: it matches only a collection whose params pin exactly one key,
  `skills`, to this slug as a scalar. The single-key guard is what excludes the four
  category-pinning collisions; the scalar guard is because `params` values may be lists, and a
  list means "any of these skills", which is not this skill.
- **The glossary's job links prefer the collection.** `/skills/:slug` builds its `jobsHref`
  from the collection when one exists and keeps `/jobs?skills=:slug` for the other 830. Both
  the count sentence and "See all N →" read that one value, so this is one change, not two.
- **The collection links back to the definition**, mirroring the `marketLink` the same page
  already carries for `/roles/[category]`. That pair states the division of labour in its own
  comment — "this page answers 'show me the jobs', that one answers 'where are they, and what
  do they pay'" — and this is the third side of it: what the thing actually is.

## Impact

- 45 glossary pages stop linking to a canonicalized-away URL and start passing their weight to
  an indexable page about the same skill; 45 collection pages gain an outbound contextual link
  to a page that currently has no inbound link but the glossary index and the sitemap.
- No route, no data, no API and no schema changes. The reverse link needs no catalog read:
  `skillDescriptions.ts` records that every canonical carries an entry, and all 45 target
  slugs were confirmed present in `sitemap-skills.xml`.

## Non-Goals

- **The 830 glossary pages with no collection.** Their job links keep pointing at
  `/jobs?skills=:slug`. There is no indexable page for those sets and inventing 830 of them is
  a different, much larger change; the URL is a legitimate live filter for a reader, and its
  canonical is doing the right thing for a crawler.
- **Retitling the glossary.** An SEO audit proposed leading `/skills/python` with its job
  count. That is already `/collections/python`'s title, word for word — adopting it would pit
  two of the site's own pages against one query. For the 830 with no collection, "What is X?"
  is the query the page can actually win, which is what `+page.server.ts` already argues.
- **Whether `/jobs?skills=` should canonicalize to `/jobs` at all.** It should; that is not
  what this change is about.
