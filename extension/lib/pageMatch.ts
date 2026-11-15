import { browser } from 'wxt/browser';
import { offersAddJob } from './addJob';
import {
  findJob,
  freehireSlugFromUrl,
  getJob,
  getMatch,
  resolveJob,
  resolveNotice,
  type FreehireJob,
  type JobMatch,
} from './freehire';
import type { RuntimeMessage } from './protocol';

/** What the side panel's Match tab shows for a page: the catalogue job and the reader's
 *  match with it, or nothing (offering "Add this job" when the page is plausibly a
 *  vacancy). */
export type PageMatch =
  | { status: 'ready'; job: FreehireJob; match: JobMatch }
  | { status: 'empty'; canAddJob: boolean };

/** Recognise the page at `url` as a catalogue job and read the reader's match with it.
 *  `isStale` says a newer lookup has started, and a stale one answers null rather than
 *  overwrite it. */
export async function matchPage(url: string, token: string, isStale: () => boolean): Promise<PageMatch | null> {
  const catalogMatch = async (slug: string): Promise<PageMatch | null> => {
    const [job, match] = await Promise.all([getJob(slug, token), getMatch(slug, token)]);
    return isStale() ? null : { status: 'ready', job, match };
  };
  // Freehire's own job page → curated slug directly.
  const directSlug = freehireSlugFromUrl(url);
  if (directSlug) return await catalogMatch(directSlug);

  // Any other page: recognise it as a catalog job from its URL (curated card),
  // else there is nothing to show. No ad-hoc scrape-and-match call here — that
  // used to run automatically on every tab the panel happened to be open on,
  // job posting or not, which is both wasted work and a wasted LLM call for
  // the vast majority of pages. `contributePage` (the "Add vacancy" action on
  // the empty state) is the explicit, on-demand equivalent.
  const found = await findJob(sanitizeForLookup(url), token);
  if (isStale()) return null;
  if (found.slug) return await catalogMatch(found.slug);
  const offer = offersAddJob({
    atsBoard: found.atsBoard,
    // The page is asked only when the URL alone did not settle it.
    jobPostingMarkup: !found.atsBoard && (await readJobPostingMarkup()),
  });
  if (isStale()) return null;
  return { status: 'empty', canAddJob: offer };
}

/** Strips the fragment and any embedded credentials from a URL before it goes to
 *  the server for a catalog lookup — neither affects which job a page is, and a
 *  fragment can carry an SSO token on some ATS redirects. Query parameters are
 *  kept: Greenhouse's embedded application form encodes the board and job id
 *  there (`for`, `token`), which `sources.RefFromURL` depends on. */
function sanitizeForLookup(url: string): string {
  try {
    const u = new URL(url);
    return `${u.origin}${u.pathname}${u.search}`;
  } catch {
    return url;
  }
}

async function readJobPostingMarkup(): Promise<boolean> {
  const reply = (await browser.runtime.sendMessage({
    kind: 'GET_JOB_POSTING_MARKUP',
  } satisfies RuntimeMessage)) as RuntimeMessage | undefined;
  return reply?.kind === 'JOB_POSTING_MARKUP' && reply.present;
}

/** Hands the page at `url` to freehire, which imports the vacancy when a link-source
 *  adapter can read it and queues the link for a maintainer when none can. Answers what to
 *  tell the user, and the posting's slug when the page resolved to one. */
export async function contributePageUrl(url: string, token: string): Promise<{ notice: string; slug: string | null }> {
  const resolved = await resolveJob(url, token);
  return { notice: resolveNotice(resolved.status), slug: resolved.public_slug ?? null };
}
