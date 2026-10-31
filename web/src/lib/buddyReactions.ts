import type { BuddyFeedItem, BuddyReactions, ReactionKind } from './types.js';

// Reactions to buddy feed events: the vocabulary, the default each kind of news offers,
// and the arithmetic of a viewer changing their mind. Pure and `$app`-free, unit-tested;
// the icons and colours are the component's (BuddyReactionBar).

/** The five reactions, in the order the picker shows them. The kinds are the API's. */
export const REACTIONS: readonly { kind: ReactionKind; label: string }[] = [
  { kind: 'congrats', label: 'Congrats' },
  { kind: 'luck', label: 'Good luck' },
  { kind: 'rooting', label: 'Rooting for you' },
  { kind: 'same', label: 'Same boat' },
  { kind: 'insight', label: 'Insightful' },
];

/** What a single click gives, by the news it answers: congratulations for an offer, luck
 *  for an interview, encouragement for an application. */
export function defaultReaction(type: BuddyFeedItem['type']): ReactionKind {
  if (type === 'offer') return 'congrats';
  if (type === 'interview') return 'luck';
  return 'rooting';
}

/** The kinds to show beside a post (the most chosen first, at most three) and the total. */
export function reactionSummary(counts: BuddyReactions['counts']): { top: ReactionKind[]; total: number } {
  const entries = Object.entries(counts) as [ReactionKind, number][];
  const order = new Map(REACTIONS.map((r, i) => [r.kind, i]));
  const top = entries
    .sort((a, b) => b[1] - a[1] || (order.get(a[0]) ?? 0) - (order.get(b[0]) ?? 0))
    .slice(0, 3)
    .map(([k]) => k);
  return { top, total: entries.reduce((n, [, c]) => n + c, 0) };
}

/** The reactions after the viewer chooses `kind` ("" clears theirs) — what the screen shows
 *  before the server answers, and the answer it should agree with. */
export function applyReaction(r: BuddyReactions, kind: ReactionKind | ''): BuddyReactions {
  const counts: BuddyReactions['counts'] = {};
  for (const [k, n] of Object.entries(r.counts) as [ReactionKind, number][]) {
    const left = k === r.mine ? n - 1 : n;
    if (left > 0) counts[k] = left;
  }
  if (kind) counts[kind] = (counts[kind] ?? 0) + 1;
  return { counts, mine: kind };
}
