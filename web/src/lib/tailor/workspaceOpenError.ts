import { notInPlan, resetsAtLabel } from '#lib/allowance.js';
import { ApiError } from '#lib/api.js';
import type { Allowance } from '#lib/types.js';

/** What the tailoring workspace says when it could not open (`baseMode`: a base CV rather
 *  than a vacancy's tailored copy). */
export function workspaceOpenError(e: unknown, baseMode: boolean): string {
  if (e instanceof ApiError && e.status === 402) {
    // Today's allowance is spent: surface what ran out plus when it starts over. The
    // instant comes from the refusal itself rather than being computed here — the
    // server owns when the day rolls, and a second opinion about it would be wrong
    // for anybody whose clock disagrees.
    //
    // A plan that includes none of the feature gets no reset time at all. The server's
    // own message already says the plan excludes this, and a reset appended under it
    // would name a moment at which the allowance is still zero.
    const at = e.body?.allowance as Allowance | undefined;
    const more = at?.resets_at && !notInPlan(at) ? ` More at ${resetsAtLabel(at)}.` : '';
    return `${e.message}${more}`;
  }
  return e instanceof ApiError
    ? e.message
    : baseMode
      ? 'Could not open this CV.'
      : 'Could not open the tailoring workspace.';
}
