## Context

#2074 treated `companies.industries` and `companies.domains` as two equal answers to
one question. Verified against aggregates — coverage of companies with open jobs rose
27% → 66% — and not against individual companies, which is where it fails.

`companies.domains` is a union over every open job. Uber's is `{adtech, cybersecurity,
ecommerce, edtech, fintech, gamedev, govtech, hrtech, logistics, media, other, saas,
travel}`; on production `?industries=gaming` returns it. The array was never wrong —
under the label "Domain" it meant *posts jobs in these areas*. #2074 reinterpreted it
as *is in these industries*.

Distribution of domain count over companies with open jobs (production, 2026-08-17):

| domains | companies | of those, no curated industry | mean open jobs |
|---|---|---|---|
| 1 | 42,400 | 34,480 | |
| 2 | 16,551 | 12,811 | 5 (1–2 combined) |
| 3+ | ~14,000 | 9,989 | 34 |

A focused company carries one or two. Three or more correlates with volume, not with
breadth of business.

## Goals / Non-Goals

**Goals:**

- Stop asserting industries a classified company is not in.
- Keep the reach the derived arm was added for.
- No migration, no reindex — the defect is live.

**Non-Goals:**

- The domain-count threshold. It is the other half of the fix and needs a reindex; see
  Risks.
- Changing `RefreshCompanyFacets`, the `domains` column, or the jobs catalogue's own
  domains facet.

## Decisions

### Precedence, not equality

The derived arm applies only where `industries` is empty. Two reasons, and the second
is the one that generalises:

1. It costs no reach. A company with a curated industry is already matched by it; the
   derived arm can only *add* industries to a company someone has already classified.
2. The sources differ in kind. Curated is a statement about the company. Derived is an
   inference from what the company advertises, and inference should not overrule a
   statement — the same precedence `internal/experience` applies between what a
   candidate asserts and what a model infers.

Expressible on both backends over existing attributes: `cardinality(industries) = 0` in
Postgres, `industries IS EMPTY` in Meilisearch. That is what makes it shippable today.

### `media` → `entertainment`, `mobility` → `transportation`

Both were left unmapped in #2074 for want of an honest target. Both have one.

`media` — the curated dictionary already routes `digital-media`,
`media-and-entertainment`, `media-and-communications` and `content-creation` to
`entertainment`. The domain was being held to a stricter standard than the aliases
beside it. Crunchbase groups the two as one industry group for the same reason. The
bare `media`, `publishing`, `digital-publishing`, `social-media` and `creator-economy`
labels had no alias at all — a gap, not a decision — and get one here.

`mobility` — resolved against NAICS rather than by judgement. Ride-hailing is 485310,
under 485 *Transit and Ground Passenger Transportation*, explicitly distinct from 3361
*Motor Vehicle Manufacturing*. Crunchbase has no "mobility" category at all:
Transportation is the umbrella group, with Ride Sharing, Car Sharing and Automotive as
children. So `transportation` covers the domain without asserting anything false,
where `automotive` would have put taxi platforms under vehicle manufacturing. Both
classifiers agree, so this is recorded in the dictionary as the tie-break rule for
future disputes: contested placements are settled against NAICS/Crunchbase, not by
argument.

## Risks / Trade-offs

- **The 3+ domain tail still produces false industries** → 9,989 companies with no
  curated industry and three or more domains, averaging 34 open jobs. Precedence does
  not touch them. The threshold that would is not expressible in a Meilisearch filter
  over existing attributes; it needs a materialized column and a companies reindex,
  and a reindex cannot be scheduled while the jobs rebuild is running. Shipping the
  half that needs no reindex now, against a live defect, and tracking the rest.
- **Forcing these requests onto Postgres was considered and rejected** → it would make
  the threshold expressible immediately, but the filtered count costs 1.2s there today
  and 2.6s with the threshold, against a Meilisearch count that is effectively free.
  Measured with `EXPLAIN ANALYZE` on production before rejecting.
- **Reach drops from the number #2074 advertised** → that number counted the noise.
  The honest figure is lower and will drop again when the threshold lands.

## Migration Plan

An ordinary deploy. Rollback is a code revert.

## Open Questions

None.
