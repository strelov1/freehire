import type { BuddyCompany, BuddySimilar } from './types.js';

// The network page's arithmetic, pure and `$app`-free so it is unit-tested.

/** The companies the viewer shares with the members listed, each with those members —
 *  "who else applied to GitLab", the people worth asking what that loop is like. Busiest
 *  company first, ties by name so the order is stable as more members load. A shared
 *  company is the similar list's own (both applied in the last 90 days). */
export function buddiesByCompany(members: readonly BuddySimilar[]): { company: BuddyCompany; members: BuddySimilar[] }[] {
  const groups = new Map<string, { company: BuddyCompany; members: BuddySimilar[] }>();
  for (const m of members) {
    for (const c of m.shared_companies) {
      const g = groups.get(c.slug);
      if (g) g.members.push(m);
      else groups.set(c.slug, { company: c, members: [m] });
    }
  }
  return [...groups.values()].sort((a, b) => b.members.length - a.members.length || a.company.name.localeCompare(b.company.name));
}
