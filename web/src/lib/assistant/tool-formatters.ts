// Per-tool header/line formatting for the assistant chat transcript, so the
// renderer stays free of name-dispatch ladders.
//
// The agent's tools are now our own typed functions rather than shell commands,
// so a call is a known name plus a structured argument object. That removes every
// shell affordance this module used to carry (parsing a command string out of an
// ACP terminal call, filtering pending no-command notifications) and replaces it
// with a label per tool.

export interface ToolCall {
  name: string;
  input: unknown;
  /** The tool's result payload, once it has come back. */
  result?: string;
  /** Whether that result is an error envelope. */
  isError?: boolean;
}

/** Truncate `s` to at most `n` characters, appending an ellipsis when cut. */
function truncate(s: string, n: number): string {
  return s.length <= n ? s : s.slice(0, n - 1) + '…';
}

// What each tool is doing, in the user's terms. The transcript reads as intent
// ("Searching jobs") rather than as a function name.
const LABELS: Record<string, string> = {
  facets: 'Loading filters',
  search_jobs: 'Searching jobs',
  get_job: 'Reading a job posting',
  get_company: 'Reading a company',
  market_fit: 'Analysing market fit',
  save_job: 'Saving a job',
  unsave_job: 'Removing a bookmark',
  apply_job: 'Marking as applied',
  track_job: 'Updating your board',
  my_jobs: 'Reading your tracked jobs',
  cv_context: 'Reading the match analysis',
  cv_get: 'Reading your CV',
  cv_edit: 'Updating your CV',
};

/** The intent label for one call, falling back to the tool's own name so a tool
 *  added on the backend still renders sensibly before this map catches up. */
export function toolLabel(call: ToolCall): string {
  return LABELS[call.name] ?? call.name;
}

/** Title shown in the collapsed header: the distinct intents in the group, capped
 *  so the header stays short. */
export function groupTitle(calls: readonly ToolCall[]): string {
  const distinct = [...new Set(calls.map(toolLabel))];
  if (distinct.length === 0) return '';
  if (distinct.length <= 2) return distinct.join(' · ');
  return `${distinct.slice(0, 2).join(' · ')} · +${distinct.length - 2}`;
}

/** One line in the expanded list: the intent plus the argument that identifies
 *  this particular call. */
export function callLine(call: ToolCall): string {
  const label = toolLabel(call);
  const detail = callDetail(call);
  return detail ? `${label}: ${detail}` : label;
}

/** The one argument worth showing beside a call's label. */
function callDetail(call: ToolCall): string | null {
  switch (call.name) {
    case 'search_jobs':
      return readField(call.input, 'query') ?? filterSummary(call.input);
    case 'get_job':
    case 'get_company':
    case 'save_job':
    case 'unsave_job':
    case 'apply_job':
    case 'track_job':
      return readField(call.input, 'slug');
    case 'market_fit':
      return readList(call.input, 'skills');
    case 'my_jobs':
      return readField(call.input, 'filter');
    case 'cv_edit':
      return editCount(call.input);
    default:
      return null;
  }
}

/** A compact rendering of the facet filter, for a search with no keyword. */
function filterSummary(input: unknown): string | null {
  if (!input || typeof input !== 'object') return null;
  const filters = (input as Record<string, unknown>).filters;
  if (!filters || typeof filters !== 'object') return null;
  const parts = Object.entries(filters as Record<string, unknown>).map(([key, value]) =>
    Array.isArray(value) ? `${key}=${value.join('|')}` : `${key}`,
  );
  return parts.length > 0 ? truncate(parts.join(', '), 60) : null;
}

export function nonEmptyInput(input: unknown): boolean {
  if (input === null || input === undefined) return false;
  if (typeof input === 'object' && Object.keys(input as object).length === 0) return false;
  return true;
}

export function previewToolInput(input: unknown): string {
  try {
    return truncate(JSON.stringify(input), 200);
  } catch {
    return String(input);
  }
}

/** The message a failed call should show. The backend renders a tool failure as
 *  `{"error": "..."}`, which is what the model reads; the user gets the same
 *  sentence rather than a JSON blob. */
export function toolErrorMessage(call: ToolCall): string | null {
  if (!call.isError || !call.result) return null;
  try {
    const parsed = JSON.parse(call.result) as Record<string, unknown>;
    return typeof parsed.error === 'string' ? parsed.error : call.result;
  } catch {
    return call.result;
  }
}

/** The tool whose calls the renderer shows as a claim plus Yes/No buttons instead of
 *  the generic collapsed line. */
export const REQUEST_CONFIRMATION_TOOL = 'request_confirmation';

/** What No sends, verbatim — a fixed decline, never the claim text, so declining
 *  never accidentally reads as confirming it. */
export const CONFIRMATION_DECLINE_TEXT = "No, that's not right — don't add it.";

export interface ConfirmationRequest {
  claim: string;
  question: string;
}

/** Reads a `request_confirmation` call's arguments into what the button UI needs, or
 *  null when the call is a different tool or its claim is missing or blank — the one
 *  thing the renderer actually replays on Yes. */
export function parseConfirmationRequest(call: ToolCall): ConfirmationRequest | null {
  if (call.name !== REQUEST_CONFIRMATION_TOOL) return null;
  if (!call.input || typeof call.input !== 'object') return null;
  const { claim, question } = call.input as Record<string, unknown>;
  if (typeof claim !== 'string' || claim.trim() === '') return null;
  return { claim, question: typeof question === 'string' ? question : '' };
}

/** Whether the group's body adds anything beyond its title — used to decide
 *  between a flat chip and an expandable `<details>` in the renderer. */
export function isExpandable(calls: readonly ToolCall[]): boolean {
  if (calls.length > 1) return true;
  const only = calls[0];
  if (!only) return false;
  return nonEmptyInput(only.input) || only.isError === true;
}

function readField(input: unknown, ...keys: string[]): string | null {
  if (!input || typeof input !== 'object') return null;
  const obj = input as Record<string, unknown>;
  for (const k of keys) {
    const v = obj[k];
    if (typeof v === 'string' && v.length > 0) return v;
  }
  return null;
}

function readList(input: unknown, key: string): string | null {
  if (!input || typeof input !== 'object') return null;
  const v = (input as Record<string, unknown>)[key];
  if (!Array.isArray(v)) return null;
  const parts = v.filter((s): s is string => typeof s === 'string');
  return parts.length > 0 ? truncate(parts.join(', '), 60) : null;
}


/** How many edits one cv_edit call carried — the tool takes a batch, not a single patch. */
function editCount(input: unknown): string | null {
  if (!input || typeof input !== 'object') return null;
  const ops = (input as Record<string, unknown>).ops;
  if (!Array.isArray(ops) || ops.length === 0) return null;
  return ops.length === 1 ? '1 edit' : `${ops.length} edits`;
}
