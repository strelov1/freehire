/**
 * What a URL into the assistant asks for: which conversation to start, and whether to
 * open it by saying something.
 *
 * Kept apart from the page so the mapping is testable without rendering a route, and so
 * the two questions stay separate — a preset selects the agent's prompt and tools on the
 * backend, a kickoff only decides whether a turn runs before the caller types.
 */

/** The conversations a URL may mint. `tailor` binds to a CV and a vacancy, so it is
 *  created by the tailoring page with those ids and can never be asked for by address. */
export type ChatPreset = 'chat' | 'profile';

export type AssistantEntry = {
  preset: ChatPreset;
  /** Sent as the caller's first message, into an empty session only. */
  kickoff?: string;
};

/**
 * Deliberately short: it is not a second system prompt. The interviewer's method — read
 * the bank, find the thinnest spot, ask one question at a time — already lives in
 * `profilePrompt` on the backend. This exists because a turn does not start until a user
 * message arrives, and someone who clicked "add an achievement" has already said what
 * they want.
 */
const PROFILE_KICKOFF =
  'Walk through my experience with me — start with whatever is thinnest, and help me fill in the achievements that are missing.';

export function entryFromQuery(params: URLSearchParams): AssistantEntry {
  if (params.get('preset') === 'profile') return { preset: 'profile', kickoff: PROFILE_KICKOFF };
  return { preset: 'chat' };
}

/**
 * How the address should follow the chat that just opened. Arriving without an id is a
 * redirect to where the caller belongs, so it replaces the entry rather than becoming a
 * step they can press Back into; choosing another chat is a real move and pushes one.
 */
export function historyModeFor(
  currentId: string | undefined,
  nextId: string,
): 'replace' | 'push' | 'none' {
  if (currentId === nextId) return 'none';
  return currentId ? 'push' : 'replace';
}
