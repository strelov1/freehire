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

/** Whether an application answers the search query, matching on the employer and the
 *  role. Shared by the board and the list — one field narrows whichever view is open.
 *
 *  A blank query keeps everything. An application whose posting the catalogue no
 *  longer holds has no `job` to read: its employer survives only as `company_slug`
 *  (which is punctuated, e.g. `acme-corp`) and its role as `role_title`. */
export function matchesQuery(item: MyJob, query: string): boolean {
  const tokens = query.toLowerCase().split(/\s+/).filter(Boolean);
  if (tokens.length === 0) return true; // a blank query narrows nothing
  // Every token must appear, but anywhere and in any order: somebody typing what
  // they remember of a role does not recall its word order, so "go senior" has to
  // find "Senior Go Engineer". A substring test over the joined text cannot do that.
  const haystack = normalize(`${item.job?.company || item.company_slug} ${item.job?.title || item.role_title}`);
  return tokens.every((t) => haystack.includes(normalize(t)));
}

/** Lowercases and turns a slug's punctuation into the spaces a person types — the
 *  dashes in `acme-corp` are how we store the employer, not how anyone recalls it,
 *  and on an application whose posting is gone the slug is the only name we hold. */
function normalize(s: string): string {
  return s.toLowerCase().replace(/[-_/.]+/g, ' ');
}

/** How the board addresses one application's row, given what a timeline event knows
 *  about it — or null when the event names no application at all.
 *
 *  The listing mints two forms and neither is the bare application id: a row backed by a
 *  posting is named by that posting's public slug, and only an application whose posting
 *  cmd/prune removed is named `a<id>` (see ListInteractions in
 *  internal/jobtracking/repository.go and the applicationRef contract in internal/handler).
 *  `/my/tracking/[id]` opens its drawer by matching this id against the loaded rows, so a
 *  link built from the bare integer matches nothing and fails silently — the drawer simply
 *  does not open, with no error to notice. */
export function boardRefFor(e: {
  job_slug?: string;
  application_id?: number;
}): string | null {
  if (e.job_slug) return e.job_slug;
  return e.application_id ? `a${e.application_id}` : null;
}
