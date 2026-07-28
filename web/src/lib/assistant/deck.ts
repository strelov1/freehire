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

export interface JobDeck {
  heading?: string;
  entries: DeckEntry[];
}

/** A presenting call's render state. A call is `pending` until its result comes
 *  back; a call whose result is an error produces no slot at all, so a deck the
 *  backend rejected is never briefly shown and then replaced. */
export type DeckSlot = { status: 'pending' } | { status: 'ready'; deck: JobDeck };

/** Split a message's tool calls into the decks to render and the calls that
 *  belong in the tool-activity list. A presenting call never appears in both: a
 *  progress chip above the deck it produced is noise. */
export function splitPresentingCalls(calls: readonly ToolCall[]): {
  decks: DeckSlot[];
  rest: ToolCall[];
} {
  const decks: DeckSlot[] = [];
  const rest: ToolCall[] = [];
  for (const call of calls) {
    if (call.name !== PRESENT_JOBS_TOOL) {
      rest.push(call);
      continue;
    }
    const slot = deckSlot(call);
    if (slot) decks.push(slot);
  }
  return { decks, rest };
}

/** The slot one presenting call renders as, or null when it renders nothing. */
function deckSlot(call: ToolCall): DeckSlot | null {
  if (call.result === undefined) return { status: 'pending' };
  if (call.isError) return null;

  const authored = authoredDeck(call.input);
  const accepted = acceptedSlugs(call.result);
  if (!authored || !accepted) return null;

  // Driven by the backend's list: it decides which slugs exist, and it preserves
  // the model's ranking. An entry it dropped has no card even though the model
  // wrote a rationale for it.
  const entries = accepted
    .map((slug) => authored.entries.get(slug))
    .filter((e): e is DeckEntry => e !== undefined);
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
    if (typeof slug !== 'string' || slug === '') continue;
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

function strings(value: unknown): string[] {
  return Array.isArray(value) ? value.filter((v): v is string => typeof v === 'string') : [];
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}
