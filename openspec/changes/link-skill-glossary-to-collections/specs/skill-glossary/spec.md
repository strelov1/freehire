## MODIFIED Requirements

### Requirement: The glossary is reachable and crawlable

The frontend SHALL serve an index of every described skill, and SHALL list every
glossary page in a sitemap registered in the sitemap index, so the surface is
discoverable without depending on a reveal that only opens on interaction.

The sitemap SHALL list exactly the pages the route serves: a slug the route would 404
MUST NOT appear in it. **Every link the glossary publishes is held to the same rule** —
the neighbouring-skills block links only to skills that have a page, because a block of
404s on a page whose claim is that it is worth linking to is worse than no block.

That rule covers where a link LANDS, not only whether it resolves. Where an indexable page
exists for the set a glossary link describes, the glossary SHALL link to that page rather
than to a URL that canonicalizes elsewhere. Concretely: a skill's postings links — the open-count
sentence and the "see all" link — SHALL address the filter collection that pins exactly that
one skill when one exists, and SHALL fall back to the `/jobs` facet URL when none does. A link
to a URL whose own canonical names a different page is not a 404, but it hands a crawler an
address the site has said is not the address, which is the same failure quieter.

The fallback is not a defect. Most described skills have no collection, the facet URL is a
legitimate live filter for a reader, and its canonical is correct for a crawler; what the rule
forbids is preferring it when a better-addressed page exists.

While the glossary was being written it was NOT advertised — no footer link, no sitemap
shard, `noindex` on both pages — because each of those promises a glossary and a handful
of words is not one. That threshold was removed when the vocabulary was covered. A future
programme that grows the surface in stages should reach for the same shape rather than
publish a partial one.

#### Scenario: A neighbour the dictionary no longer emits

- **WHEN** the facet distribution still carries a slug the vocabulary has retired
- **THEN** it is absent from the neighbours block rather than linked to a page that 404s

#### Scenario: The index lists the glossary

- **WHEN** a reader opens the glossary index
- **THEN** every described skill is listed and links to its page

#### Scenario: The sitemap matches the routes

- **WHEN** a crawler fetches the skills sitemap and follows every URL in it
- **THEN** none of them 404

#### Scenario: A skill whose collection pins exactly that skill

- **WHEN** a reader opens the glossary page for a skill that has a filter collection pinning
  only that skill, such as `kotlin`
- **THEN** both the open-count sentence and the "see all" link address
  `/collections/kotlin`, the self-canonical page for that same set

#### Scenario: A skill with no collection

- **WHEN** a reader opens the glossary page for a described skill no collection pins, as most
  of them are
- **THEN** its postings links address the `/jobs` facet URL for that skill, unchanged

#### Scenario: A skill whose name collides with a collection of a different kind

- **WHEN** a reader opens the glossary page for a skill whose slug also names a collection, but
  that collection pins a category rather than the skill — `data-science`, `data-engineering`,
  `devops`, `machine-learning`
- **THEN** its postings links address the `/jobs` facet URL, NOT the same-named collection,
  because a category feed and a skill facet are different sets and the link would promise the
  one while delivering the other
