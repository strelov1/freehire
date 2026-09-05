// Derives which Kanban column a tracked job belongs to. The column is NOT stored
// — it is a view over the application's saved_at / applied_at / stage. A row the board
// cannot place is one the user has neither saved nor tracked; a saved job is a card in
// the first column, because saving one is taking it on.
//
// The columns and their membership are the generated ones (internal/userjob →
// cmd/gen-contracts). This file used to restate both, which is how the board and the
// pipeline came to call the same settled application by two different names.
import { STAGE_GROUPS } from './generated/contracts';
import type { MyJob } from './types';
import { must } from './utils';

export type BoardColumnId = (typeof STAGE_GROUPS)[number]['id'];

export const BOARD_COLUMNS: { id: BoardColumnId; label: string }[] = STAGE_GROUPS.map((g) => ({
  id: g.id,
  label: g.label,
}));

const STAGE_COLUMN: Record<string, BoardColumnId> = Object.fromEntries(
  STAGE_GROUPS.flatMap((g) => g.stages.map((stage) => [stage, g.id])),
);

/** The column a tracked job currently sits in, or `null` when it is neither saved nor
 *  tracked and so has nothing to sit in. Priority: precise stage, then a legacy
 *  applied-without-stage row, then the saved mark, else off-board. */
export function columnOf(item: MyJob): BoardColumnId | null {
  const col = item.stage ? STAGE_COLUMN[item.stage] : undefined;
  if (col) return col;
  if (item.applied_at) return 'applied';
  // A bookmark is a board card. Saving a job is taking it on, and the board is where a
  // candidate looks to see what they have taken on — a saved row that showed up nowhere
  // there meant the product had two places to keep a job and told you about one.
  //
  // Read-side only, deliberately: nothing writes a stage on the saver's behalf, so this
  // needed no migration and applies to every job saved before the rule as well as after.
  // A stage, when there is one, still decides — this is the fallback, not an override.
  if (item.saved_at) return 'preparing';
  return null;
}

// The terminal stages live behind the single "Closed" column; the user picks which one in the
// drawer after dropping there. Derived rather than listed, for the reason at the top of this
// file: a hand-written copy went stale the moment a settled stage was added, leaving the new
// stage unreachable from the board while every type check still passed — the generated-stage
// check binds Stage to STAGE_GROUPS and says nothing about a literal array.
export const CLOSED_OUTCOMES = must(
  STAGE_GROUPS.find((g) => g.id === 'closed'),
  "STAGE_GROUPS's closed group",
).stages;
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
