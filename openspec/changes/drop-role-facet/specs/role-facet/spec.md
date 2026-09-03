## REMOVED Requirements

### Requirement: A deterministic dictionary derives a job's roles

**Reason**: The vocabulary it derives is a cross-product of two facets the site already
has. Measured against the live catalogue, its 1,200 values are 47 that repeat a
specialization identically (all 47 return the same posting count to the digit — `design`
40,769 both ways, `sales` 191,352 both ways, zero divergence), 979 that are
specialization × seniority, 8 that are a bare grade, and 166 that carry their own name.

**Migration**: `category` and `seniority` express the first 1,034 between them, on the
same request, and more precisely: a role slug fuses two axes into one value, so
`senior_backend` cannot be widened to "any grade of backend" without changing the
filter. The 166 named roles are already in the suggestion dictionary as mined posting
titles, written the way the market writes them.

### Requirement: Roles are derived at index time, not stored or backfilled

**Reason**: Nothing derives them any more.

**Migration**: None needed — the attribute leaves the index document, and the next
rebuild drops it.

### Requirement: The role catalog is the source of truth for picker labels

**Reason**: There is no picker. `ROLE_LABELS` and `ROLE_ALIASES` leave the generated
web contracts with it.

**Migration**: A suggestion's display text comes from the dictionary document, which
already carries it.

### Requirement: Roles are served with live facet counts

**Reason**: The facet is not served.

**Migration**: `category` and `seniority` are both served with live counts, and were
before this.

### Requirement: Forward Deployed Engineer resolves from FDE and its synonym titles

**Reason**: A named role, and the last one added — which is the argument against the
dictionary rather than for it: every new job title the market invents needed a curated
entry, and mined titles need none.

**Migration**: "Forward Deployed Engineer" is a posting title, so the suggestion
dictionary carries it if enough postings use it and drops it if they stop — which is
the honest answer to whether it is a role people search for.
