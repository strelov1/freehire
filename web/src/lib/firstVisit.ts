// First-visit default filter: a brand-new visitor landing on a bare homepage is
// sent to the remote-worldwide slice so SSR renders the filtered feed directly —
// no post-hydration seed, no flash. The gate is a cookie (`hire_filters_touched`,
// see filterStorage) that the server can read, unlike the localStorage marker.
//
// Env-neutral and side-effect-free so it unit-tests in plain Node and imports
// cleanly into the SSR `load`. The two moving parts — "is this a crawler" and
// "should we redirect" — live here as pure functions.

import { DEFAULT_JOB_FILTERS } from './filterStorage';

// Bots that fetch the homepage for indexing or link previews. Matched loosely on
// the UA string; the cost of a miss is only that a bot gets the default slice, and
// the cost of a false positive is only that a rare human keeps the full feed — so a
// broad, maintenance-light pattern beats an exhaustive list.
//
// `bot` alone covers the indexers (Googlebot, GPTBot, PerplexityBot, ClaudeBot,
// OAI-SearchBot). It does NOT cover the fetchers an AI assistant uses when it opens a
// link *for a person* — Perplexity-User, Claude-User, ChatGPT-User, meta-externalagent
// — and those are the requests whose content gets quoted in an answer. A miss there
// costs more than a slice: the page the assistant reads, and cites, becomes the
// filtered variant rather than the homepage.
const CRAWLER_UA =
  /bot|crawl|spider|slurp|mediapartners|facebookexternalhit|embedly|quora|bitlybot|whatsapp|telegram|discord|preview|scanner|archiver|perplexity|claude|chatgpt|externalagent/i;

/** True when the User-Agent looks like a crawler/link-preview bot rather than a
 *  human browser. A missing UA reads as human — the bots we care about all send one,
 *  and redirecting UA-less clients would risk hiding the full homepage from tools we
 *  can't identify. */
export function isCrawler(ua: string | null): boolean {
  return !!ua && CRAWLER_UA.test(ua);
}

/** The path to redirect a bare-homepage request to, or null to serve it unchanged.
 *  Redirect only a first-time human on a param-less URL: a shared filter link
 *  (`search` set), a returning visitor who changed filters (`touched`), and crawlers
 *  are all left on the URL they asked for. */
export function defaultFilterTarget(opts: {
  search: string;
  touched: boolean;
  crawler: boolean;
}): string | null {
  if (opts.search !== '' || opts.touched || opts.crawler) return null;
  return `/?${DEFAULT_JOB_FILTERS}`;
}
