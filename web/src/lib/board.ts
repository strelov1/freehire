// Derives which Kanban column a tracked job belongs to. The column is NOT stored
// — it is a view over the user_jobs row's applied_at / stage. The board shows only
// jobs in an active application state; a saved-only row (saved but never applied,
// no stage) has no column and lives in the Activity → Saved list instead.
import type { MyJob } from './types';

export type BoardColumnId = 'applied' | 'interview' | 'offer' | 'closed';

export const BOARD_COLUMNS: { id: BoardColumnId; label: string }[] = [
  { id: 'applied', label: 'Applied' },
  { id: 'interview', label: 'Interview' },
  { id: 'offer', label: 'Offer' },
  { id: 'closed', label: 'Closed' },
];

// Precise backend stage → column group. Stages not listed fall through to the
// applied_at fallback in columnOf.
const STAGE_COLUMN: Record<string, BoardColumnId> = {
  applied: 'applied',
  screening: 'applied',
  responded: 'applied',
  interview: 'interview',
  offer: 'offer',
  accepted: 'closed',
  rejected: 'closed',
  withdrawn: 'closed',
};

/** The column a tracked job currently sits in, or `null` when it is saved-only and
 *  therefore not on the board. Priority: precise stage, then a legacy
 *  applied-without-stage row, else off-board. */
export function columnOf(item: MyJob): BoardColumnId | null {
  const col = item.stage ? STAGE_COLUMN[item.stage] : undefined;
  if (col) return col;
  if (item.applied_at) return 'applied';
  return null;
}

// The three terminal stages live behind the single "Closed" column; the user
// picks which one in the drawer after dropping there.
export const CLOSED_OUTCOMES = ['accepted', 'rejected', 'withdrawn'] as const;
export type ClosedOutcome = (typeof CLOSED_OUTCOMES)[number];

// svelte-dnd-action keys each draggable by a top-level `id`; MyJob has none, so
// the board wraps each row with id = the job's public_slug.
export type BoardItem = MyJob & { id: string };
