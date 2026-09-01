import { error, redirect } from '@sveltejs/kit';
import { SKILL_ALIASES } from '$lib/generated/skillAliases';
import { skillLabel } from '$lib/facets';
import { serverApi } from '$lib/server/api';
import { MIN_SKILL_OPEN, displayAliases, showsPostings, topNeighbours } from '$lib/skillGlossary';
import { loadSkillDescriptions } from '$lib/skillDescriptions';
import type { PageServerLoad } from './$types';

// One skill's glossary entry: what it is, what else it is called, what it is named
// alongside, and who is hiring for it.
//
// The page exists for every DESCRIBED skill, not for every skill with postings — the
// opposite of the /roles gate, and deliberately. A /roles pair page is about its
// postings; this one is about the definition, and gating it on hiring volume would take
// the "what is X?" link away from exactly the obscure skills a reader does not
// recognise. Only the postings block is gated.
//
// The alias table is imported here rather than in the component: it is 28 KB covering
// the whole vocabulary, and this page renders one row of it. Reading it server-side
// keeps all of it off the wire.
const NEIGHBOUR_LIMIT = 8;
const POSTINGS_SHOWN = 10;

// The generated table is `as const`, so its keys are 863 string literals and a lookup by
// an arbitrary slug does not type-check. Widened once here, the way facets.ts widens
// SKILL_LABELS for exactly the same reason — the slug comes from a URL, not from the
// vocabulary's type.
const aliasTable: Readonly<Record<string, readonly string[]>> = SKILL_ALIASES;

export const load: PageServerLoad = async ({ params, url, fetch, setHeaders }) => {
  const slug = params.slug.toLowerCase();
  // The lookup is case-insensitive but the URL is not: /skills/DBT would render and then
  // build its canonical and links from the raw param — a second URL for one page. Same
  // 308 as the roles routes, and it carries the query string for the same reason they
  // do: a campaign parameter must survive the normalisation.
  if (params.slug !== slug) redirect(308, `/skills/${slug}${url.search}`);

  // The catalog itself, not the fail-soft `skillDescription`. That accessor answers ""
  // for a fetch it could not make, which is right for a chip and wrong here: it would
  // turn a broken build into a 404 and tell crawlers to drop every glossary page rather
  // than showing that something is wrong.
  const catalog = await loadSkillDescriptions();
  const description = catalog[slug] ?? '';
  // No description means no page. An entry that says only the label is not a glossary
  // entry, and the sitemap lists exactly what this route serves.
  if (!description) error(404, 'Not found');

  // Two calls, run together. The neighbours read the skills distribution under this
  // skill; the gate and the sentence above it read `total` from that same response, so
  // the two cannot state different numbers.
  const filter = new URLSearchParams({ skills: slug });
  const api = serverApi(fetch);
  const [counts, postings] = await Promise.all([
    api.facetCounts(filter, { facets: ['skills'] }),
    api.searchJobs(filter, POSTINGS_SHOWN, 0),
  ]);

  const label = skillLabel(slug);
  const showPostings = showsPostings(counts.total);
  setHeaders({ 'cache-control': 'public, max-age=0, s-maxage=3600' });

  return {
    slug,
    label,
    description,
    aliases: displayAliases(aliasTable[slug] ?? [], slug, label),
    // Filtered against the catalogue, not against the dictionary: a facet value left
    // behind by a skill the dictionary no longer emits would otherwise be published as
    // a link to a 404, from the page whose whole claim is that it is worth linking to.
    neighbours: topNeighbours(
      counts.facets.skills ?? {},
      slug,
      NEIGHBOUR_LIMIT,
      (s) => catalog[s] !== undefined
    ).map((s) => ({ slug: s, label: skillLabel(s) })),
    total: counts.total,
    // The block, not the count: the count is stated either way, because "3 open
    // postings" is a fact. What the gate withholds is a list that would read as a
    // sample of a market.
    showPostings,
    minPostings: MIN_SKILL_OPEN,
    postings: showPostings ? postings.items : [],
  };
};
