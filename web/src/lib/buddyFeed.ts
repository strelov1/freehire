import type { BuddyFeedItem } from './types.js';

// How the buddy feed reads as posts. Pure and `$app`-free, so the grouping is unit-tested
// and the page only draws.

/** One post in the feed: a single event, or one member's applications on one day folded
 *  together — four "Applied to" cards in a row say less than one card naming four
 *  companies. Interviews and offers always stand alone: each is news in itself. */
export type BuddyPost =
  | { kind: 'event'; key: string; item: BuddyFeedItem }
  | { kind: 'applied'; key: string; handle: string; card_summary: BuddyFeedItem['card_summary']; day: string; items: BuddyFeedItem[] };

/** The feed's items (newest first) as posts, in the same order. A member's applications on
 *  one day collect into the post placed where the first of them appeared; one application
 *  alone stays an event post. */
export function buddyFeedPosts(items: readonly BuddyFeedItem[]): BuddyPost[] {
  const applied = new Map<string, BuddyFeedItem[]>();
  for (const it of items) {
    if (it.type !== 'applied') continue;
    const k = `${it.handle}|${it.day}`;
    const group = applied.get(k);
    if (group) group.push(it);
    else applied.set(k, [it]);
  }
  const posts: BuddyPost[] = [];
  const placed = new Set<string>();
  for (const it of items) {
    const group = it.type === 'applied' ? applied.get(`${it.handle}|${it.day}`) : undefined;
    if (!group || group.length === 1) {
      posts.push({ kind: 'event', key: `e${it.id}`, item: it });
      continue;
    }
    const k = `${it.handle}|${it.day}`;
    if (placed.has(k)) continue;
    placed.add(k);
    // Keyed by the first event's id (this one: the group is met at its first item), not by
    // handle and day — a later page can bring more of the same day, and the id keeps the
    // key unique and stable as the list grows.
    posts.push({ kind: 'applied', key: `a${it.id}`, handle: it.handle, card_summary: it.card_summary, day: it.day, items: group });
  }
  return posts;
}

/** The posts under their day, in feed order: the "Today / Yesterday" separators. */
export function buddyFeedDays(posts: readonly BuddyPost[]): { day: string; posts: BuddyPost[] }[] {
  const out: { day: string; posts: BuddyPost[] }[] = [];
  for (const p of posts) {
    const day = p.kind === 'event' ? p.item.day : p.day;
    const last = out[out.length - 1];
    if (last && last.day === day) last.posts.push(p);
    else out.push({ day, posts: [p] });
  }
  return out;
}
