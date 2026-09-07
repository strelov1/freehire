// Pure logic for the homepage's live "recently added jobs" feed. Kept out of the
// component so the list bookkeeping is unit-testable without a DOM — the same split
// matchAnalysis.ts draws for its own SSE reducer. The wire shape is the JSON one
// event payload from GET /api/v1/feed/recent carries (internal/job/recentfeed.Entry).

/** One event as it arrives over the wire. */
export interface RecentFeedEvent {
  kind: 'single' | 'aggregate';
  title: string;
  company_name: string;
  /** Present on a `single` event; links the card to its posting. */
  job_slug?: string;
  /** Present on an `aggregate` event: how many postings it represents. */
  count?: number;
}

/** A feed event plus the client-assigned id {#each} keys the card list on — the
 *  wire payload carries none, since nothing downstream needs one beyond display. */
export interface RecentFeedEntry extends RecentFeedEvent {
  id: number;
}

/** Prepends event as a new entry (newest first) and drops entries past max. The
 *  caller supplies id (a simple incrementing counter) so this stays pure and
 *  testable without hidden shared state. */
export function pushFeedEntry(
  entries: RecentFeedEntry[],
  event: RecentFeedEvent,
  id: number,
  max: number,
): RecentFeedEntry[] {
  return [{ ...event, id }, ...entries].slice(0, max);
}

/** The card copy for an aggregated entry — explicit that the sample company shown
 *  is one of several, never a claim that all `count` postings came from it (see
 *  openspec/changes/add-homepage-recent-jobs-feed/design.md, "Aggregated entries do
 *  not attribute a single company"). */
export function aggregateLabel(entry: Pick<RecentFeedEvent, 'count'>): string {
  return `+${entry.count} more at other companies`;
}
