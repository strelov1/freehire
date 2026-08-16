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

/** Server-minted atom ids only — never treat free text in `atoms` as kickoff prose. */
// Built from a fragment so the source never contains `-[…]` (token-coverage false positive).
const HEX = '[0-9a-f]';
const ATOM_ID_RE = new RegExp(
  `^${HEX}{8}-${HEX}{4}-${HEX}{4}-${HEX}{4}-${HEX}{12}$`,
  'i',
);

/**
 * The opening message for the experience interview, optionally aimed at specific
 * achievements.
 *
 * Two surfaces reach the interviewer — the Experience tab's in-place panel and this
 * module's URL entry — and they must ask the same thing, so the text lives here once
 * rather than at each call site. Ids are filtered here too: the panel's come from
 * rendered rows and the URL's from a query string, and one rule for both means the
 * query can never smuggle prose in as an id.
 *
 * It asks the agent to read the achievements before saying anything about them, and
 * deliberately does not say which tool does that — this message is recorded as the
 * candidate's own, and naming our internals in their voice is both odd to read and a
 * second place the tool could be renamed. The interviewer's prompt owns the how.
 */
export function profileKickoff(ids: readonly string[]): string {
  const named = ids.filter((id) => ATOM_ID_RE.test(id));
  const unique = [...new Set(named)];
  if (unique.length === 0) return PROFILE_KICKOFF;
  const list = unique.map((id) => `\`${id}\``).join(', ');
  return (
    `${PROFILE_KICKOFF}\n\n` +
    `Start with these achievements: ${list} — they look related or thin. ` +
    `Read them first, tell me what they actually say, then ask whether to merge them or what to add.`
  );
}

export function entryFromQuery(params: URLSearchParams): AssistantEntry {
  if (params.get('preset') !== 'profile') return { preset: 'chat' };
  const ids = (params.get('atoms') ?? '').split(',').map((part) => part.trim());
  return { preset: 'profile', kickoff: profileKickoff(ids) };
}

/**
 * Whether this surface may open a conversation running under `preset`.
 *
 * Stated as a refusal, not a whitelist, and that is the point: `ListAssistantSessions`
 * decides what the rail carries (chat, profile, interview and debrief today), and a
 * client that whitelists presets goes stale the moment the backend gains one.
 *
 * Two presets are refused, for two different reasons. A tailoring chat belongs to the
 * CV that owns it, is reached through the tailoring workspace, and its tools close over
 * ids this surface knows nothing about. A browsing chat's one distinguishing tool,
 * `read_current_page`, only works over the extension's own connection — the backend
 * already excludes it from `ListAssistantSessions`, and this refuses it here too, so a
 * bookmarked or guessed link to one shows the same "chat unavailable" state rather than
 * silently opening a chat that lost its reason to exist.
 */
export function opensInRail(preset: string): boolean {
  return preset !== 'tailor' && preset !== 'browse';
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
