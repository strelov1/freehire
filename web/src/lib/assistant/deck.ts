// Turns the assistant's `present_jobs` calls into the decks the chat renders.
//
// A recommendation is a typed tool call, not prose: the model supplies which
// vacancies, in what order, grouped how, and why; the backend answers with the
// slugs it accepted; the client fetches each vacancy's own facts by slug. This
// module performs the join between those two halves, kept out of the Svelte
// component so it is unit-testable without a DOM — mirroring `chat.ts`.

import type { ToolCall } from './tool-formatters';

/** The tool whose calls ARE the recommendation. */
export const PRESENT_JOBS_TOOL = 'present_jobs';

/** One card: a vacancy to hydrate, plus the rationale only the model knows. */
export interface DeckEntry {
  slug: string;
  note: string;
  whyFits: string[];
  concerns: string[];
}

interface JobDeck {
  heading?: string;
  entries: DeckEntry[];
}

/** A presenting call's render state. A call is `pending` until its result comes
 *  back; a call whose result is an error produces no slot at all, so a deck the
 *  backend rejected is never briefly shown and then replaced. `count` is how many
 *  placeholders to draw — a count is not a slug, so reading it from the pending
 *  call's arguments does not show anything the backend has yet to validate. */
export type DeckSlot =
  | { status: 'pending'; count: number }
  | { status: 'ready'; deck: JobDeck };

/** Split a message's tool calls into the decks to render and the calls that
 *  belong in the tool-activity list. A presenting call never appears in both: a
 *  progress chip above the deck it produced is noise. `streaming` is the owning
 *  message's state: a call with no result yet is only worth a placeholder while
 *  the turn is still running. */
export function splitPresentingCalls(
  calls: readonly ToolCall[],
  streaming = true,
): { decks: DeckSlot[]; rest: ToolCall[] } {
  const decks: DeckSlot[] = [];
  const rest: ToolCall[] = [];
  for (const call of calls) {
    if (call.name !== PRESENT_JOBS_TOOL) {
      rest.push(call);
      continue;
    }
    const slot = deckSlot(call, streaming);
    if (slot) decks.push(slot);
  }
  return { decks, rest };
}

/** The slot one presenting call renders as, or null when it renders nothing. */
function deckSlot(call: ToolCall, streaming: boolean): DeckSlot | null {
  // No result on a settled message means the stream dropped between the call and
  // its result. A placeholder there would pulse forever under a finished reply.
  if (call.result === undefined) {
    if (!streaming) return null;
    return { status: 'pending', count: pendingCount(call.input) };
  }
  if (call.isError) return null;

  const authored = authoredDeck(call.input);
  const accepted = acceptedSlugs(call.result);
  if (!authored || !accepted) return null;

  // Driven by the backend's list: it decides which slugs exist, and it preserves
  // the model's ranking. An entry it dropped has no card even though the model
  // wrote a rationale for it.
  //
  // Deduped because the cards are keyed by slug, and Svelte throws on a repeated
  // key in production as well as in dev. The backend refuses a duplicate going
  // forward, but a transcript recorded before that check would otherwise break
  // its session on every reload.
  const seen = new Set<string>();
  const entries: DeckEntry[] = [];
  for (const slug of accepted) {
    const entry = authored.entries.get(slug);
    if (!entry || seen.has(slug)) continue;
    seen.add(slug);
    entries.push(entry);
  }
  if (entries.length === 0) return null;

  return { status: 'ready', deck: { heading: authored.heading, entries } };
}

/** The slugs the backend accepted, or null if the receipt is unreadable. */
function acceptedSlugs(result: string): string[] | null {
  let parsed: unknown;
  try {
    parsed = JSON.parse(result);
  } catch {
    return null;
  }
  if (!isRecord(parsed) || !Array.isArray(parsed.presented)) return null;
  return parsed.presented.filter((s): s is string => typeof s === 'string');
}

/** The call's arguments — the heading and the rationale per slug — or null if
 *  they are unreadable. Entries are keyed by slug because the receipt, not the
 *  argument order, decides what renders. */
function authoredDeck(
  input: unknown,
): { heading?: string; entries: Map<string, DeckEntry> } | null {
  if (!isRecord(input) || !Array.isArray(input.jobs)) return null;

  const entries = new Map<string, DeckEntry>();
  for (const job of input.jobs) {
    if (!isRecord(job)) continue;
    const { slug, note } = job;
    // First mention wins. A slug listed twice is refused by the backend now, but a
    // transcript recorded before that check should show the rationale the model
    // led with rather than whatever it repeated.
    if (typeof slug !== 'string' || slug === '' || entries.has(slug)) continue;
    entries.set(slug, {
      slug,
      note: typeof note === 'string' ? note : '',
      whyFits: strings(job.why_fits),
      concerns: strings(job.concerns),
    });
  }

  const heading = input.heading;
  return {
    heading: typeof heading === 'string' && heading.trim() !== '' ? heading : undefined,
    entries,
  };
}

/** How many placeholders a still-running call should draw, so the deck holds
 *  roughly its final height. Falls back to two when the arguments are unreadable
 *  — they may still be arriving. */
function pendingCount(input: unknown): number {
  if (!isRecord(input) || !Array.isArray(input.jobs)) return 2;
  return Math.min(Math.max(input.jobs.length, 1), 10);
}

/** The string members of a model-authored list, deduped — a repeated phrase would
 *  otherwise be a repeated key in the chips it renders as. */
function strings(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return [...new Set(value.filter((v): v is string => typeof v === 'string'))];
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}
