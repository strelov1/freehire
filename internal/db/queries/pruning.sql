-- Catalogue pruning: the queries that find, and eventually remove, jobs that do not
-- belong on an IT job board. See the catalog-pruning capability spec.

-- name: ResidualTitleGroups :many
-- The residual unclassified mass grouped into clusters an operator can act on: the
-- word groups occurring most often in the titles of live jobs that carry no is_tech
-- signal. Grouping is by word group, not by whole title, because boards append
-- location, schedule and requisition detail — half the unclassified mass has a title
-- that occurs exactly once, so whole-title grouping splits one role into singletons.
-- A word group is also the unit the non-tech dictionary accepts, so a reported
-- cluster is directly usable as an anchored term.
--
-- A title is first split at punctuation into runs, and groups are formed only WITHIN
-- a run. Without that, adjacency spans separators and invents phrases that never
-- occur: "Personal Care Aide - Honolulu" would yield "aide honolulu", and a term
-- copied from that cluster into the dictionary would match none of the jobs that
-- produced it, because the dictionary matches contiguous text. It also inflated the
-- counts of legitimate clusters by every occurrence that straddled punctuation.
--
-- Two-word groups carry the ordinary case. Three-word groups exist only to bridge a
-- connector: in Portuguese and Spanish the preposition is part of the role name
-- ("operador de caixa"), while the two-word fragments around it ("analista de") are
-- noise, so a connector is allowed mid-group and never at an edge. Every other edge
-- token must be at least three characters and non-numeric, which keeps requisition
-- numbers and shredded schedule notation ("m w", "req 12345") out of the ranking.
-- The vocabularies are COALESCEd: a NULL parameter would make every <> ALL(...)
-- comparison NULL and silently return zero rows — indistinguishable from the
-- campaign having exhausted the residual mass, which is the one wrong answer this
-- report must never give.
--
-- Tokenizing on [^[:alnum:]]+ is Unicode-aware, so accented Latin and Cyrillic
-- survive whole — an ASCII class would split "Técnico" and lose the cluster.
-- Closed and duplicate rows are excluded: only a live, canonical posting is worth a
-- dictionary term.
WITH runs AS (
    SELECT j.id,
           j.source,
           regexp_split_to_table(lower(btrim(j.title)), '[^[:alnum:] ]+') AS run
    FROM jobs j
    WHERE j.closed_at IS NULL
      AND j.duplicate_of IS NULL
      AND j.is_tech IS NULL
),
tok AS (
    SELECT id, source, regexp_split_to_array(btrim(run), ' +') AS a
    FROM runs
    WHERE btrim(run) <> ''
),
pos AS (
    SELECT tok.id, tok.source, tok.a, i
    FROM tok, generate_subscripts(tok.a, 1) AS i
),
grp AS (
    -- Two-word groups: both tokens must be role-bearing edges.
    SELECT DISTINCT id, source, (a[i] || ' ' || a[i + 1])::text AS grp
    FROM pos
    WHERE i + 1 <= cardinality(a)
      AND length(a[i])     >= 3 AND a[i]     !~ '^[0-9]+$'
      AND length(a[i + 1]) >= 3 AND a[i + 1] !~ '^[0-9]+$'
      AND a[i]     <> ALL(COALESCE(sqlc.arg(stop_words)::text[], '{}'))
      AND a[i + 1] <> ALL(COALESCE(sqlc.arg(stop_words)::text[], '{}'))
      AND a[i]     <> ALL(COALESCE(sqlc.arg(connectors)::text[], '{}'))
      AND a[i + 1] <> ALL(COALESCE(sqlc.arg(connectors)::text[], '{}'))
    UNION ALL
    -- Three-word groups: role-bearing edges bridging one connector.
    SELECT DISTINCT id, source, (a[i] || ' ' || a[i + 1] || ' ' || a[i + 2])::text
    FROM pos
    WHERE i + 2 <= cardinality(a)
      AND length(a[i])     >= 3 AND a[i]     !~ '^[0-9]+$'
      AND length(a[i + 2]) >= 3 AND a[i + 2] !~ '^[0-9]+$'
      AND a[i]     <> ALL(COALESCE(sqlc.arg(stop_words)::text[], '{}'))
      AND a[i + 2] <> ALL(COALESCE(sqlc.arg(stop_words)::text[], '{}'))
      AND a[i]     <> ALL(COALESCE(sqlc.arg(connectors)::text[], '{}'))
      AND a[i + 2] <> ALL(COALESCE(sqlc.arg(connectors)::text[], '{}'))
      AND a[i + 1]  = ANY(COALESCE(sqlc.arg(connectors)::text[], '{}'))
)
-- count(*) is the job count, not an approximation of it: the two branches cannot
-- emit the same group (a group's space count fixes its branch, and tokens hold no
-- spaces), and each branch already emits DISTINCT (id, source, group) — which is
-- what stops a group repeated inside one verbose title from outranking a real
-- cluster. A second count(DISTINCT id) would only add a per-group sort.
SELECT grp,
       count(*)                           AS jobs,
       array_agg(DISTINCT source)::text[] AS sources
FROM grp
GROUP BY grp
ORDER BY 2 DESC, 1
LIMIT sqlc.arg(row_limit);

-- name: PruneJobs :many
-- Permanently remove a batch of jobs and record what was removed, in ONE statement.
-- Splitting the two would let the archive drift from the deletion, and the archive is
-- the only way to answer, after an irreversible removal, whether something was taken
-- that should have been kept.
--
-- The batch extends to each target's whole duplicate CHAIN, not just its immediate
-- duplicates. jobs.duplicate_of is the one foreign key to jobs that restricts, and it
-- chains: RecomputeRoleDuplicatesForCompany and the aggregator suppression both scope
-- to open jobs, so a closed row's pointer is never repointed when its parent later
-- becomes a duplicate itself — and the prune scan reaches closed rows by design. A
-- one-level extension would leave the tail pointing at a deleted row and the whole
-- batch would abort on the foreign key. UNION (not UNION ALL) terminates a cycle.
--
-- Semantically the chain belongs together anyway: the duplicates of a cook posting are
-- cook postings, and they inherit the rule that named their root.
--
-- DISTINCT ON collapses a row reachable more than once, which would otherwise violate
-- the archive's primary key and take the batch down. It orders direct matches first so
-- a row named outright keeps its own rule rather than an arbitrary one — the archive's
-- purpose is auditing a rule in isolation, which a nondeterministic rule defeats.
--
-- A user's marks on the posting — the view, the bookmark, the dismissal, the vote — cascade
-- away with it. That is an accepted cost of the campaign: what is lost is a bookmark.
--
-- Their APPLICATION does not, and the distinction is deliberate. It lives in its own table
-- (0064), holds a date, a stage, free-text notes and a mail history, and references the
-- posting with ON DELETE SET NULL: the link clears, the record stands. An application is a
-- record of something a person did, and no catalogue-hygiene campaign has the standing to
-- delete it. The sentence this replaces weighed a bookmark and, through one shared cascade,
-- silently applied the same verdict to applications.
--
-- Returns the ids actually deleted, which the caller mirrors into the search index.
-- The two parameter arrays are positionally paired. They are unnested separately and
-- rejoined on ordinality because sqlc cannot infer the types of a multi-argument
-- unnest over query parameters.
WITH RECURSIVE target_id AS (
    SELECT * FROM unnest(sqlc.arg(ids)::bigint[]) WITH ORDINALITY AS t(id, n)
),
target_rule AS (
    SELECT * FROM unnest(sqlc.arg(rules)::text[]) WITH ORDINALITY AS t(rule, n)
),
target AS (
    SELECT i.id, r.rule FROM target_id i JOIN target_rule r USING (n)
),
walk AS (
    SELECT j.id, t.rule, 1 AS direct
    FROM jobs j JOIN target t ON j.id = t.id
    UNION
    SELECT j.id, w.rule, 0
    FROM jobs j JOIN walk w ON j.duplicate_of = w.id
),
cluster AS (
    SELECT DISTINCT ON (id) id, rule
    FROM walk
    ORDER BY id, direct DESC, rule
),
deleted AS (
    DELETE FROM jobs
    WHERE id IN (SELECT id FROM cluster)
    RETURNING id, source, external_id, title, company_slug
),
-- A pruned job's document has to leave the facet index too, and this is the only place that
-- knows it existed: after this statement the row is gone, so nothing downstream could work
-- out that it was ever indexed. search_delete_outbox deliberately carries no foreign key to
-- jobs, which is what lets this entry outlive the row it names — see migration 0113.
queued AS (
    INSERT INTO search_delete_outbox (job_id)
    SELECT id FROM deleted
    ON CONFLICT (job_id) DO NOTHING
)
INSERT INTO pruned_jobs (id, source, external_id, title, company_slug, rule)
SELECT d.id, d.source, d.external_id, d.title, d.company_slug, c.rule
FROM deleted d JOIN cluster c ON c.id = d.id
RETURNING id;

-- name: CompanyTechEvidence :many
-- Whether each company has EVER shown technical evidence, over its entire history
-- including closed and duplicate rows. "This company never posts anything technical"
-- is the premise of the company-scoped pruning rules, and it has to rest on the
-- maximum available evidence — restricting it to open jobs would let a company whose
-- one engineering role closed last month read as having none.
--
-- any_skills is the weaker second signal: the skill dictionary firing on a description
-- means the posting had technical content even when neither the title nor the category
-- resolved. A company with neither signal has shown nothing technical at all.
-- Postings with no company are excluded: they would otherwise pool into one
-- pseudo-company per source whose combined evidence then decides every one of them.
--
-- This is an uncapped full-table GROUP BY over the whole jobs table, returning a row
-- per company. It is computed once per prune run, not per batch.
SELECT source,
       company_slug,
       bool_or(is_tech IS TRUE)              AS any_tech,
       bool_or(cardinality(skills) > 0)      AS any_skills
FROM jobs
WHERE company_slug <> ''
GROUP BY source, company_slug;

-- name: PruneCandidates :many
-- One keyset page of rows the prune rule evaluates, ordered by id.
--
-- Closed rows are included deliberately. Once ingest rejects a board's non-technical
-- postings, the 48-hour unseen sweep closes the ones already in the catalogue — so a
-- scan restricted to open jobs would leave exactly the rows the campaign is about to
-- stop replacing, permanently. Duplicates are included too: one may match a rule while
-- its canonical does not, and nothing references a duplicate, so removing it alone is
-- safe.
--
-- external_id carries the board: the write path namespaces it as "<board>:<native id>",
-- and the board is what the source files are keyed on. Matching on it is exact, where
-- matching on company_slug is not — many adapters take the company name from the
-- posting payload rather than the board entry, so the two spellings diverge.
-- skills comes back whole rather than as a cardinality test: whether a posting is
-- technical depends on WHICH skills it carries, and only skilltag knows that. The
-- dictionary covers the recruiting, HR, finance, legal and operations craft a technical
-- company hires for, so "has any skill" answers a different question than the caller
-- is asking.
SELECT id, source, external_id, company_slug, title, category, is_tech, skills
FROM jobs
WHERE id > sqlc.arg(after_id)
ORDER BY id
LIMIT sqlc.arg(page_size);
