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

/**
 * One way to open a conversation that has not started yet: send a message we wrote, or start
 * the unattended run — whose brief the server owns, so there is no text to carry.
 *
 * It lives here rather than in the chat component for the same reason `AssistantEntry` does:
 * a surface decides what it offers, the chat only renders and runs it.
 */
export type OpeningAction =
  | { label: string; hint?: string; kind: 'message'; text: string }
  | { label: string; hint?: string; kind: 'autopilot' };

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
 * Whether this surface may open a conversation running under `preset`.
 *
 * Stated as a refusal, not a whitelist, and that is the point: `ListAssistantSessions`
 * decides what the rail carries (chat, profile and browse today), and a client that
 * whitelists presets goes stale the moment the backend gains one. It did — `browse`
 * arrived with the extension's side panel and this check did not follow, so anyone whose
 * newest conversation came from the extension landed on the dead-link panel, which
 * replaces the rail they would have escaped through.
 *
 * Only a conversation BOUND to another artifact is refused: a tailoring chat belongs to
 * the CV that owns it, is reached through the tailoring workspace, and its tools close
 * over ids this surface knows nothing about.
 */
export function opensInRail(preset: string): boolean {
  return preset !== 'tailor';
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
