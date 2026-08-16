// The autopilot run, as the workspace sees it: the shape of a run's report, how to read it,
// and the one ordering rule that undoing a run has to obey. Kept Svelte-free so the panel
// and the page share one reading of a report and the ordering is unit-testable.

import type { AutopilotEntry, AutopilotStatus } from '$lib/generated/contracts';
import type { OpeningAction } from '$lib/assistant/presets';

/**
 * What a tailoring workspace offers while its conversation is EMPTY.
 *
 * Not "while it is fresh": a CV re-opened by id can have a conversation with nothing in it —
 * bound at bootstrap, never spoken to — and keying the offer on "resuming" left exactly that
 * case looking like a chat whose history had vanished. The chat itself only renders these while
 * there are no messages, so handing them over always is both simpler and correct.
 *
 * Nothing is sent on its own either way: the agent talking first is what left people staring at
 * a three-column workspace they had not asked anything of.
 */
export function openingActions(): OpeningAction[] {
  return [
    {
      label: 'Tailor it for me',
      kind: 'autopilot',
      hint: 'Goes through every requirement using your experience bank, then asks about whatever it could not find.',
    },
    {
      label: 'Walk me through it',
      kind: 'message',
      text: "Let's tailor my CV for this role — review the match analysis and walk me through the gaps.",
    },
  ];
}

/** What a run achieved, counted. `total` is what it considered, not what the vacancy asks. */
export interface RunSummary {
  closed: number;
  open: number;
  notReached: number;
  total: number;
}

export function summarizeRun(report: AutopilotEntry[] | undefined): RunSummary {
  const entries = report ?? [];
  let closed = 0;
  let open = 0;
  let notReached = 0;
  for (const e of entries) {
    if (e.status === 'closed_bank' || e.status === 'closed_candidate') closed++;
    else if (e.status === 'not_reached') notReached++;
    else open++;
  }
  return { closed, open, notReached, total: entries.length };
}

/** How one outcome reads in the panel. `tone` drives the colour; the label says HOW it was
 *  closed, because "from your bank" and "from what you told me" are different assurances. */
export interface StatusMeta {
  label: string;
  tone: 'closed' | 'open' | 'skipped';
}

export function statusMeta(status: AutopilotStatus): StatusMeta {
  switch (status) {
    case 'closed_bank':
      return { label: 'From your experience bank', tone: 'closed' };
    case 'closed_candidate':
      return { label: 'From what you told me', tone: 'closed' };
    case 'open':
      return { label: 'Nothing on file yet', tone: 'open' };
    case 'not_reached':
      return { label: 'Not reached', tone: 'skipped' };
    default:
      // A status this build does not know about still renders as a row. The server owns
      // the vocabulary and may grow it; a workspace that throws on an unfamiliar value
      // would take the whole CV down with it.
      return { label: String(status), tone: 'open' };
  }
}

/**
 * Undo a run in the only order that works.
 *
 * The page holds the CV document in memory and saves it on a debounce. Undoing without
 * flushing that pending save first means the timer fires a second later and writes the
 * tailored text straight back over the restored one — the undo appears to work and then
 * silently reverses itself. The re-read comes last, and only on success: a refused undo
 * (there was no run) must leave the in-memory document exactly as it is.
 */
export async function undoRun(steps: {
  flush: () => Promise<void>;
  undo: () => Promise<void>;
  refetch: () => Promise<void>;
}): Promise<void> {
  await steps.flush();
  await steps.undo();
  await steps.refetch();
}
