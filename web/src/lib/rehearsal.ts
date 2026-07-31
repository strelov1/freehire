// View logic for the board's rehearsal affordance: which applications offer a mock
// interview.
//
// The gate is a stage rule rather than the server's, because the server has none — a
// rehearsal is allowed for any application the caller holds, so somebody who wants to
// prepare a week early can. What the board does is decide when to *offer* one, and that
// is a narrower question: a card carries few controls, and one that appears everywhere
// stops meaning anything.
import type { MyJob } from './types';

// The stages where an interview is actually in play: an employer has engaged, and a
// conversation is either scheduled or already underway. `responded` is deliberately out —
// a reply is not yet a call, and it is the commonest stage on a busy board.
const REHEARSABLE_STAGES = new Set(['screening', 'interview']);

/** Whether the card offers to rehearse. Terminal stages and everything before an
 *  employer has engaged answer false: there is no interview left to prepare for on a
 *  rejection, and nothing yet to prepare for on a fresh application. */
export function canRehearse(item: MyJob): boolean {
  return !!item.stage && REHEARSABLE_STAGES.has(item.stage);
}
