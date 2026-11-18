import type { ExperienceAtom, ExperienceBank } from './types';

export const isUnconfirmed = (atom: ExperienceAtom) => atom.provenance === 'agent_inferred';

/** The bank-order-first unconfirmed achievement, and where it lives — an employment or
 *  `unplaced`. Bank order is `employments` in the order the server returned them, then
 *  `unplaced`; this is what the unconfirmed-achievements banner jumps to. */
export function findFirstUnconfirmed(bank: ExperienceBank): { employmentId: string | null; atomId: string } | null {
  for (const employment of bank.employments) {
    const found = employment.atoms.find(isUnconfirmed);
    if (found) return { employmentId: employment.id, atomId: found.id };
  }
  const unplaced = bank.unplaced.find(isUnconfirmed);
  if (unplaced) return { employmentId: null, atomId: unplaced.id };
  return null;
}

/** Unconfirmed first. The banner tells the owner something needs a decision; leaving
 *  those entries wherever the server happened to return them makes that a scavenger
 *  hunt at eleven achievements and useless at two hundred. */
export function sortNeedsAttentionFirst(atoms: ExperienceAtom[]): ExperienceAtom[] {
  return [...atoms].sort((a, b) => Number(isUnconfirmed(b)) - Number(isUnconfirmed(a)));
}
