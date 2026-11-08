-- Skill co-occurrence snapshot: for each skill, the skills most often listed beside it
-- on open postings. Recomputed daily by cmd/rollup-facets as an atomic
-- delete-and-reinsert, and read by the /skills/<slug> glossary page for its "related
-- skills" links.
--
-- It exists because that page used to ask Meilisearch for the whole skills
-- distribution under one skill on every render — 100-700 ms of engine time each, since
-- the attribute holds ~1,100 values — and a crawler opening thousands of cold glossary
-- URLs in one minute (2026-11-07 and 08, ~01:12 UTC) starved the engine and turned
-- every facet call into a 500. Neighbours move over days, not seconds.
CREATE TABLE public.insights_skill_neighbours (
    skill     text   NOT NULL,
    neighbour text   NOT NULL,
    count     bigint NOT NULL,
    PRIMARY KEY (skill, neighbour)
);

-- The one read: a skill's neighbours, most frequent first.
CREATE INDEX insights_skill_neighbours_skill_count_idx
    ON public.insights_skill_neighbours (skill, count DESC);
