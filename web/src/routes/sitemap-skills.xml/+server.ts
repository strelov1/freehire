import { skillGlossaryPaths, urlsetXml, xmlResponse } from '$lib/sitemap';
import { loadSkillDescriptions } from '$lib/skillDescriptions';
import type { RequestHandler } from './$types';

// The glossary's crawl path. The chip's reveal only opens on interaction and the index
// is one page deep, so without this the definitions would be reachable but not findable.
//
// Listed from the catalog itself, which is the same thing /skills/<slug> reads to decide
// whether it has a page — so a slug this file names can never 404, and a described skill
// can never be missing from it. One file: the whole glossary is under a thousand URLs
// and enumerating it costs no read, unlike the role landings, which shard per category
// because each shard pays a facet call.
export const GET: RequestHandler = async ({ url }) => {
  const catalog = await loadSkillDescriptions();
  const paths = skillGlossaryPaths(Object.keys(catalog).sort());

  return xmlResponse(urlsetXml(paths.map((path) => ({ loc: `${url.origin}${path}` }))));
};
