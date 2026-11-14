import type { Snippet } from 'svelte';
import type { ChatPreset, OpeningAction } from './presets';

/** AssistantChat's props; what each one does is listed beside the component's `$props()`. */
export interface AssistantChatProps {
  session?: string;
  kickoff?: string;
  sessionLabel?: string;
  onTurnComplete?: () => void;
  /** Told after each successful `cv_edit` tool call resolves — not just at the end of the
   *  turn — so the tailoring host can refresh the CV preview as a run writes it instead of
   *  only once the whole turn finishes. `onTurnComplete` still fires at turn end regardless
   *  (the safety net for anything this per-call signal missed); this is additive. */
  onDocumentEdited?: () => void;
  /** Told when an UNATTENDED run starts and ends. The tailoring host locks its editor for
   *  the duration: a run rewrites the same document the editor holds and saves on a
   *  debounce, and whoever writes last would silently win. */
  onRunStateChange?: (running: boolean) => void;
  /** Told when ANY turn starts and ends, so a host can disable what a running turn would
   *  make a no-op. `onRunStateChange` is the narrower signal: only unattended runs. */
  onTurnStateChange?: (active: boolean) => void;
  /** Awaited before a turn is started. The tailoring host writes its pending edit here: a
   *  debounce armed a moment ago would otherwise fire mid-turn and overwrite what the agent
   *  wrote — and the run's pre-run snapshot would be taken without that edit in it. */
  beforeTurn?: () => Promise<void>;
  onSessionChange?: (id: string) => void;
  showSessionRail?: boolean;
  /** What an empty conversation offers instead of a blank prompt. Shown only while the
   *  session has no messages: once it has any, the composer is the way in. A host that
   *  passes these should NOT also pass a kickoff — the whole point is that the first turn
   *  is the caller's choice rather than ours. */
  openingActions?: OpeningAction[];
  /** Which unbound conversation a NEW session starts as. An existing session keeps
   *  whatever preset it was created with. `support` is the help agent: it runs on its own
   *  routes, which only create, send and cancel, so it never lists, reopens or deletes a
   *  conversation and offers no attachments, dictation or retry. */
  preset?: ChatPreset;
  /** Start an unattended run the moment the session is ready, instead of waiting for the
   *  candidate to pick an opening action. The tailoring host sets this on a cold-start
   *  bootstrap (a vacancy tailored for the first time) — only on an empty session, and only
   *  once, the same guard `kickoff` already uses. A host that passes this should not also
   *  pass `openingActions`: there is nothing left to offer once the run is already starting. */
  autoRun?: boolean;
  /** The host's own control for opening the account navigation below `lg`, where
   *  `AccountNavRail collapsible` folds into a drawer the host has to open. Rendered at
   *  the start of the chat's header row — the one row this surface has — so the host does
   *  not stack a second bar above it. Only shown where a header row exists, which is
   *  wherever `showSessionRail` is on. */
  navTrigger?: Snippet;
  /** The conversation is held against one automatic application: catch it up on what
   *  happened to the application when it opens and every few seconds while it is shown.
   *  A check with nothing new costs nothing and paints nothing. */
  syncApplication?: boolean;
}
