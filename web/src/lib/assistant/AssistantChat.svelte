<script lang="ts">
  import { onMount, tick, untrack } from 'svelte';
  import { AlertTriangle, ArrowDown, MessagesSquare, PanelLeft, Phone, Plus, Trash2, WandSparkles, X } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import {
    createSession,
    listSessions,
    getSession,
    deleteSession,
    SessionNotFound,
  } from '$lib/assistant/api';
  import VoiceCall from '$lib/assistant/VoiceCall.svelte';
  import { canUseVoiceCall } from '$lib/assistant/voiceCall';
  import { AUDIO_ENABLED } from '$lib/assistant/audioAvailability';
  import { track } from '$lib/analytics';
  import {
    openRehearsal,
    retryTurn,
    sendTurn,
    extendSession,
    TurnRefused,
    startAutopilot,
    StreamInterrupted,
    type Turn,
  } from '$lib/assistant/client';
  import { initChat, reduceTurnEvent, type ChatState } from '$lib/assistant/chat';
  import { bulletCapUserMessage } from '$lib/assistant/bulletCapAlert';
  import { splitPresentingCalls } from '$lib/assistant/deck';
  import { atBottom } from '$lib/assistant/scrolling';
  import { renderMarkdown } from '$lib/markdown';
  import { ConfirmDialog } from '$lib/ui';
  import JobDeck from '$lib/assistant/JobDeck.svelte';
  import SessionRail from '$lib/assistant/SessionRail.svelte';
  import ToolGroupList from '$lib/assistant/ToolGroupList.svelte';
  import Composer from '$lib/assistant/Composer.svelte';
  import {
    fromSummary,
    upsertSession,
    removeSession,
    setLabel,
    activeAfterDelete,
    labelFromMessage,
    type SessionItem,
  } from '$lib/assistant/sessions';
  import { eventsFromTranscript, type TurnEvent } from '$lib/assistant/wire';
  import { opensInRail, type ChatPreset, type OpeningAction } from '$lib/assistant/presets';

  // The agent chat. The agent runs inside the freehire backend, so this is an
  // ordinary authenticated API surface: the session list, one session's stored
  // transcript, and a turn that streams its events over SSE. There is no
  // connection to hold, nothing to install, and no separate agent login.
  // Everything streaming-shaped runs client-side, so SSR renders only the shell.

  type Phase = 'loading' | 'ready';

  // Reusable chat: the host (/my/assistant or /tailor) composes the layout around it.
  //  - session: open this specific session id (else the newest, else a fresh chat).
  //  - kickoff: auto-send this as the first message of a fresh session.
  //  - sessionLabel: name the session in the rail until its first message names it.
  //  - onTurnComplete: called when a turn finishes (the tailor host refreshes the CV preview).
  //  - onSessionChange: the active conversation changed. The /my/assistant host turns this
  //    into a navigation so each chat has its own URL and Back works; the tailor host,
  //    whose chat is bound to one CV, passes nothing.
  //  - showSessionRail: whether to render the session sidebar (off on the focused /tailor surface).
  let {
    session = undefined,
    kickoff = undefined,
    sessionLabel = undefined,
    onTurnComplete = undefined,
    onDocumentEdited = undefined,
    onRunStateChange = undefined,
    onTurnStateChange = undefined,
    beforeTurn = undefined,
    onSessionChange = undefined,
    showSessionRail = true,
    preset = 'chat',
    openingActions = undefined,
    autoRun = false,
  }: {
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
     *  whatever preset it was created with. */
    preset?: ChatPreset;
    /** Start an unattended run the moment the session is ready, instead of waiting for the
     *  candidate to pick an opening action. The tailoring host sets this on a cold-start
     *  bootstrap (a vacancy tailored for the first time) — only on an empty session, and only
     *  once, the same guard `kickoff` already uses. A host that passes this should not also
     *  pass `openingActions`: there is nothing left to offer once the run is already starting. */
    autoRun?: boolean;
  } = $props();

  let phase = $state<Phase>('loading');
  let error = $state<string | null>(null);
  // A ceiling refusal is not a dead end: the session can be continued by spending another
  // of the day's tailoring sessions. Held apart from `error` because it needs an action
  // beside it, and because a plain error message would tell the candidate to give up on
  // work they can carry on with right now.
  let refusal = $state<TurnRefused | null>(null);
  let extending = $state(false);
  // Distinct from `error`: the URL names a conversation that is not the caller's to
  // open — deleted, someone else's, or a tailoring chat that belongs to a CV. It is
  // a dead link, not a failure, so it gets an explanation and a way out.
  let notFound = $state(false);
  // When cv_edit refuses an insert that would silently truncate a full role, show a
  // dismissible banner — the tool chip alone is easy to miss, and the candidate needs
  // to know their existing bullets were kept.
  let bulletCapAlert = $state<string | null>(null);

  // Sidebar: the caller's conversations, newest activity first. `switching`
  // disables the composer and list while a session load is in flight.
  let sessions = $state<SessionItem[]>([]);
  let activeId = $state<string | null>(null);
  // The open conversation's preset, as the SERVER records it — not the one this arrival
  // asked for. A rehearsal is reached by its own address as often as it is created, and
  // only the server knows which kind of conversation an id names.
  let activePreset = $state<string | null>(null);
  let switching = $state(false);
  // Creating a chat is a round trip before `switching` is ever set, and each
  // click landing in that window would create a real session on the server.
  let creating = $state(false);

  // Chat (active session).
  let chat = $state<ChatState>(initChat());
  let draft = $state('');
  let turnActive = $state(false);
  // The turn in flight, so it can be cancelled. Cancelling asks the server to stop the work;
  // aborting the fetch only stops this client reading it.
  let turn: Turn | null = null;

  let sidebarOpen = $state(true);
  let scroller = $state<HTMLDivElement | null>(null);
  // Whether arriving turn events may move the pane. Set from the pane's own scroll
  // events, so scrolling up mid-turn holds position and scrolling back resumes
  // following with no further ceremony.
  let stickToBottom = $state(true);

  // Messages typed while a turn is in flight, drained one by one when it ends.
  let queue = $state<{ id: string; text: string }[]>([]);
  let queueCounter = 0;

  // Voice mode: a hands-free call on an `interview` session, replacing the composer
  // for its duration. `voiceModeOff` latches once the server reports no Realtime
  // gateway (501) — same reasoning as Composer's `dictationOff` — so the trigger does
  // not keep offering a call that can only fail. `voiceSupported` starts false and is
  // decided after mount, the same reasoning VoiceInput.svelte's `supported` gives:
  // reading navigator during SSR would disagree with the client on the first paint.
  // It also carries AUDIO_ENABLED, which is false while the gateway migration leaves
  // the Realtime route unproven — see audioAvailability.ts.
  let voiceCallOpen = $state(false);
  let voiceModeOff = $state(false);
  let voiceSupported = $state(false);

  function closeVoiceCall() {
    voiceCallOpen = false;
    if (activeId) void reloadTranscript(activeId);
  }

  function enqueue(text: string) {
    queue = [...queue, { id: `q${++queueCounter}`, text }];
  }

  const NEW_CHAT_LABEL = 'New chat';

  /**
   * What the URL this component mounted on asked for: which conversation to mint, and the
   * opening message to send into it. Read once, and each half spent once.
   *
   * Both are properties of the ARRIVAL, not of the surface. The address is rewritten to
   * the session's own URL moments later — reading them reactively would race that rewrite
   * — and this component outlives the rewrite now that one route node serves every chat.
   * Left standing, they would mint a second interview on the next `boot()` and put the
   * same words in the caller's mouth again.
   */
  const arrival: { preset: ChatPreset; kickoff?: string } = { preset, kickoff };

  // Whether the turn in flight is an unattended run rather than a reply to something typed.
  let runActive = $state(false);

  /** Whether a turn may be started right now: the pane is settled and nothing is running. */
  const canStartTurn = () => phase === 'ready' && !turnActive && !switching && !!activeId;

  /**
   * Start an unattended run from outside the chat — the workspace's "Run again" beside the
   * run report. Exported rather than driven by a prop because it is an ACTION, not state:
   * a boolean the host toggles would re-run the moment anything else re-rendered.
   */
  export function startRun() {
    if (!canStartTurn()) return;
    void dispatch({ kind: 'autopilot' });
  }

  /** Run an opening action — the first turn of a conversation nobody has typed into yet. */
  function runOpening(action: OpeningAction) {
    if (!canStartTurn()) return;
    void dispatch(action.kind === 'message' ? { kind: 'message', text: action.text } : { kind: 'autopilot' });
  }

  /** The conversation the host is currently being navigated to, or null. Held until the
   *  `session` prop catches up, so a switch we started is never undone by the stale URL. */
  let navigatingTo = $state<string | null>(null);

  // --- Streaming spinner / thinking timers ---------------------------------
  const SPINNER_GLYPHS = ['·', '✢', '✳', '✶', '✻', '✽'] as const;
  const VERBS = ['Thinking', 'Working', 'Crunching', 'Pondering', 'Composing', 'Simmering'] as const;
  let elapsedSec = $state(0);
  let spinnerIdx = $state(0);
  let turnStartedAt = $state<number | null>(null);
  let currentVerb = $state<string>('Thinking');

  $effect(() => {
    if (turnActive && turnStartedAt === null) {
      turnStartedAt = Date.now();
      elapsedSec = 0;
      spinnerIdx = 0;
      currentVerb = VERBS[Math.floor(Math.random() * VERBS.length)] ?? 'Thinking';
    } else if (!turnActive) {
      turnStartedAt = null;
      elapsedSec = 0;
    }
  });
  $effect(() => {
    if (!turnActive || turnStartedAt === null) return;
    const id = setInterval(() => {
      elapsedSec = Math.floor((Date.now() - (turnStartedAt as number)) / 1000);
      spinnerIdx = (spinnerIdx + 1) % SPINNER_GLYPHS.length;
    }, 120);
    return () => clearInterval(id);
  });

  /**
   * Bring the newest content into view.
   *
   * `force` is the difference between a frame that ARRIVED and an act the reader
   * PERFORMED. An arriving frame defers to where they are reading; sending a message,
   * starting a run and opening a conversation do not, because the thing each produces
   * is at the bottom and leaving it off screen hides the result of the act.
   */
  async function scrollToBottom(force = false) {
    if (!force && !stickToBottom) return;
    await tick();
    // Asked again after the await: a frame that was allowed to scroll when it arrived
    // must not still scroll if the reader started reading during that tick.
    if (!force && !stickToBottom) return;
    if (!scroller) return;
    scroller.scrollTop = scroller.scrollHeight;
    // Landing at the bottom is also a decision to follow again. The pane's own scroll
    // event would say so a moment later; saying it here spares the next frame that wait.
    stickToBottom = true;
  }

  /** The pane moved. Follow the stream only while the reader is at its end. */
  function onPaneScroll() {
    if (scroller) stickToBottom = atBottom(scroller);
  }

  // --- Session orchestration -----------------------------------------------

  async function boot() {
    phase = 'loading';
    error = null;
    try {
      let summaries: Awaited<ReturnType<typeof listSessions>> = [];
      try {
        summaries = await listSessions();
      } catch {
        error = 'Could not load your chats — starting a new one.';
      }
      sessions = summaries.map((s) => fromSummary(s, NEW_CHAT_LABEL));

      // Open the requested session (the host prop), else the newest, else a fresh
      // chat. A host (the /tailor route) seeds `session` + `sessionLabel`.
      //
      // An explicitly-asked-for preset overrides "resume the newest": arriving at
      // ?preset=profile is a request to START an experience interview, not to reopen
      // whatever was last discussed. Without this the link did nothing at all for anyone
      // who already had a chat, which is everyone after their first visit.
      if (!session && arrival.preset !== 'chat') {
        await createAndOpen();
      } else if (session) {
        // The tailoring host opens a conversation the rail never lists, so seed it
        // into the local list to give it a name; the chat host's sessions all come
        // from the list already.
        if (!showSessionRail) {
          sessions = upsertSession(sessions, {
            id: session,
            label: sessionLabel ?? NEW_CHAT_LABEL,
            preset: 'tailor',
          });
        }
        await openSession(session);
      } else if (!(await openNewestOpenable())) {
        await createAndOpen();
      }
      phase = 'ready';

      // Autostart: the host can pass a kickoff prompt so the agent begins immediately
      // instead of waiting for the user to type. Only on an empty session, and only once
      // — boot() runs again whenever the caller returns to the bare address, and putting
      // the same words in their mouth a second time is not what they asked for.
      if (arrival.kickoff && chat.messages.length === 0) {
        const opening = arrival.kickoff;
        arrival.kickoff = undefined;
        void dispatch({ kind: 'message', text: opening });
      } else if (activePreset === 'interview' && chat.messages.length === 0) {
        // A rehearsal speaks first. There is no kickoff text to carry because the brief
        // is the server's — the candidate came here from an application, and the agent
        // opens by reading its context. The empty-transcript guard is the client half of
        // the backend's: a reload replays the conversation rather than restarting it.
        void dispatch({ kind: 'opening' });
      } else if (autoRun && chat.messages.length === 0) {
        // Cold start: the tailoring workspace runs the unattended pass itself instead of
        // offering the two-action menu. Same empty-transcript guard as the branches above —
        // boot() re-runs on every return to the bare address, and a session that already has
        // a message (the run already started, or the candidate answered its closing question)
        // must not be run again.
        void dispatch({ kind: 'autopilot' });
      }
    } catch (err) {
      report(err, 'Could not reach the assistant.');
      phase = 'ready';
    }
  }

  // The host navigates between conversations, so the requested id arrives as a prop
  // change — including on Back and Forward. Follow it without re-booting the list.
  //
  // The body runs untracked: it reads and writes activeId, navigatingTo and notFound, and
  // subscribing to what it also assigns is how this becomes a loop. The address and the
  // ready phase are the only things that should wake it.
  $effect(() => {
    const requested = session;
    if (phase !== 'ready') return;
    untrack(() => followAddress(requested));
  });

  /** Reconcile the pane with the address the host is showing. */
  function followAddress(requested: string | undefined) {
    // A switch WE started asked the host to navigate, and that navigation is async: for
    // the moment between setting activeId and the URL catching up, `session` disagrees
    // with activeId and following it would go straight back to the chat just left. That
    // is what evicted a newly created chat. Clear the guard the moment the host lands —
    // BEFORE any early return, or the guard outlives its navigation and the pane stops
    // following the URL at all, which is Back and Forward silently doing nothing.
    if (navigatingTo && requested === navigatingTo) navigatingTo = null;
    if (navigatingTo) return;

    // The bare address. Since one route node serves both it and a chat's own URL, getting
    // here is a prop change rather than a fresh mount: nothing re-runs on its own, so a
    // dead link's "Open your chats" and the nav rail's own entry would both leave the
    // caller staring at whatever was already on screen. Boot again, which is what a
    // remount used to do for us — open the newest chat and re-address to it.
    if (!requested) {
      notFound = false;
      void boot();
      return;
    }

    if (requested === activeId) return;
    openSession(requested, true).catch((err: unknown) => report(err, 'Could not open that chat.'));
  }

  /** Surface a failure. A conversation the caller cannot open is a dead link, not
   *  a broken assistant, so it gets the explanation panel rather than an error. */
  function report(err: unknown, fallback: string) {
    if (err instanceof SessionNotFound) {
      notFound = true;
      return;
    }
    error = err instanceof Error ? err.message : fallback;
  }

  // Open a session and repaint its stored transcript. The replay folds through the
  // same reducer live events do, so history and a running turn render identically.
  //
  // `fromAddress` says the address ALREADY names this chat — we are following it, not
  // asking the host to change it. That distinction is the guard's whole life: raising it
  // when no navigation will follow leaves it raised forever, and the pane then ignores
  // every later address change, which is Forward appearing to do nothing.
  async function openSession(id: string, fromAddress = false) {
    if (activeId === id && chat.messages.length > 0) return;
    // A message typed while the current turn was streaming sits in `queue`, waiting
    // for the turn to finish so endTurn() can drain it — cancelTurn() below stops the
    // turn but never drains the queue, so switching here would otherwise wipe out
    // whatever the user just typed with no trace and no way back.
    if (
      queue.length > 0 &&
      !confirm('You have an unsent message waiting in this chat. Switch chats and discard it?')
    ) {
      return;
    }
    switching = true;
    cancelTurn();
    // Navigating away from a session must end its call rather than silently carry it
    // over to whichever session id lands next — the {#key activeId} on VoiceCall
    // guards the same correctness issue structurally; this is the deliberate one.
    voiceCallOpen = false;
    try {
      chat = initChat();
      queue = [];
      bulletCapAlert = null;
      activeId = id;
      activePreset = null;
      // Raise the guard HERE, not after the fetch below: the URL-following effect reruns
      // the moment activeId changes, which is during the await — set it late and the
      // effect has already reopened the conversation we just left.
      navigatingTo = fromAddress ? null : id;
      const { session: meta, messages } = await getSession(id);
      // A tailoring conversation is reachable by id but belongs to a CV and only makes
      // sense beside it. Opening one here would show a conversation the rail cannot list
      // and the user cannot get back to. The unbound presets — chat and the experience
      // interviewer — are both at home here.
      if (showSessionRail && !opensInRail(meta.preset)) {
        notFound = true;
        return;
      }
      activePreset = meta.preset;
      let next = initChat();
      for (const event of eventsFromTranscript(messages)) next = reduceTurnEvent(next, event);
      chat = next;
      if (meta.label?.trim()) sessions = setLabel(sessions, id, labelFromMessage(meta.label));
      onSessionChange?.(id);
    } finally {
      switching = false;
    }
    void scrollToBottom(true);
  }

  /**
   * Open the most recent conversation that will actually open, newest first.
   *
   * Nobody asked for a SPECIFIC chat here — we picked one on their behalf — so a chat
   * that will not open is our problem to route around, not a dead link to report. Saying
   * "this chat no longer exists" for a pick they never made replaces the rail with a
   * panel, and its only way out lands on this same code, which picks the same chat: a
   * loop with no exit but a manual URL.
   *
   * Returns false when the caller has nothing openable, so boot() can start them a chat.
   */
  async function openNewestOpenable(): Promise<boolean> {
    for (const candidate of sessions) {
      // Sequential on purpose: we want the FIRST one that opens, and opening them in
      // parallel would race several transcripts into the same pane.
      await openSession(candidate.id);
      if (!notFound) return true;
      notFound = false;
    }
    return false;
  }

  async function createAndOpen() {
    const created = await createSession(arrival.preset);
    // Spent: the next chat minted here — "New chat", or a later boot() — is a plain one.
    arrival.preset = 'chat';
    sessions = upsertSession(sessions, fromSummary(created, NEW_CHAT_LABEL));
    await openSession(created.id);
  }

  async function newChat() {
    if (creating || switching || phase !== 'ready') return;
    creating = true;
    error = null;
    try {
      await createAndOpen();
    } catch (err) {
      error = err instanceof Error ? err.message : 'Could not start a new chat.';
    } finally {
      creating = false;
    }
  }

  async function selectSession(id: string) {
    if (switching || id === activeId) return;
    error = null;
    try {
      await openSession(id);
    } catch (err) {
      report(err, 'Could not open that chat.');
    }
  }

  let removeTargetId = $state<string | null>(null);
  let confirmRemoveChatOpen = $state(false);

  // A chat that has been named has real history — confirm before deleting. A
  // still-unnamed one has nothing in it worth confirming.
  function requestRemoveChat(id: string) {
    if (switching) return;
    const named = sessions.find((s) => s.id === id)?.label !== NEW_CHAT_LABEL;
    if (named) {
      removeTargetId = id;
      confirmRemoveChatOpen = true;
    } else {
      void removeChat(id);
    }
  }

  function confirmRemoveChat() {
    if (!removeTargetId) return Promise.resolve();
    return removeChat(removeTargetId);
  }

  async function removeChat(id: string) {
    if (switching) return;
    error = null;
    const wasActive = id === activeId;
    try {
      await deleteSession(id);
    } catch {
      error = 'Could not delete the chat.';
      return;
    }
    const remaining = removeSession(sessions, id);
    sessions = remaining;
    if (wasActive) {
      cancelTurn();
      voiceCallOpen = false;
      activeId = null;
      chat = initChat();
      const next = activeAfterDelete(remaining, true, null);
      try {
        if (next) await openSession(next);
        else await createAndOpen();
      } catch (err) {
        error = err instanceof Error ? err.message : 'Could not open a chat.';
      }
    }
  }

  // End the current turn and drain the next queued message, if any.
  function endTurn() {
    turnActive = false;
    turn = null;
    onTurnStateChange?.(false);
    if (runActive) {
      runActive = false;
      onRunStateChange?.(false);
    }
    // A completed turn may have edited an artifact (e.g. the tailored CV) — let the host refresh.
    onTurnComplete?.();
    if (queue.length > 0) {
      const [next, ...rest] = queue;
      queue = rest;
      if (next) void dispatch({ kind: 'message', text: next.text });
    }
  }

  /** Stop an in-flight turn: ask the server to stop the work, and stop reading it here. The
   *  backend no longer infers the first from the second — it could not tell a deliberate stop
   *  apart from a phone locking its screen. */
  function cancelTurn() {
    turn?.cancel();
    turn = null;
    turnActive = false;
  }

  function onEvent(sessionId: string, event: TurnEvent) {
    // The user may have switched sessions while the turn was streaming; frames
    // from the old one must never paint into the new chat.
    if (sessionId !== activeId) return;
    if (event.type === 'user_prompt') {
      sessions = setLabel(sessions, sessionId, labelFromMessage(event.text));
      bulletCapAlert = null;
    }
    if (event.type === 'tool_result' && event.is_error) {
      const alert = bulletCapUserMessage(event.result);
      if (alert) bulletCapAlert = alert;
    }
    if (event.type === 'tool_result' && event.name === 'cv_edit' && !event.is_error) {
      onDocumentEdited?.();
    }
    chat = reduceTurnEvent(chat, event);
    if (event.type === 'result') {
      endTurn();
    }
    void scrollToBottom();
  }

  // Composer submit: while a turn is in flight the message is queued and drained
  // later; otherwise it is sent immediately.
  function submitText(raw: string) {
    const text = raw.trim();
    if (!text || phase !== 'ready' || switching || !activeId) return;
    // Counted once per message the user sends, queued or dispatched — never the text
    // itself. This measures usage only; what a turn costs is recorded server-side.
    track('assistant_message');
    draft = '';
    if (turnActive || queue.length > 0) {
      enqueue(text);
      void scrollToBottom(true);
      return;
    }
    void dispatch({ kind: 'message', text });
  }

  // The ways a turn begins. Autopilot and opening carry no text (server-owned brief).
  // Retry continues from the existing transcript without appending another user message —
  // re-sending the prompt would duplicate it in the model's context.
  type TurnStart =
    | { kind: 'message'; text: string }
    | { kind: 'autopilot' }
    | { kind: 'opening' }
    | { kind: 'retry' };

  async function dispatch(start: TurnStart) {
    const id = activeId;
    if (!id) return;
    error = null;
    turnActive = true;
    onTurnStateChange?.(true);
    // Before anything reaches the server: the host may be holding an edit on a timer, and
    // the run is about to snapshot and rewrite the very document that edit belongs to.
    if (beforeTurn) {
      try {
        await beforeTurn();
      } catch {
        /* the host reports its own save failures; a turn is still worth starting */
      }
    }
    // Whether the turn ever began. A message can queue behind the session's running turn and
    // then never start — the wait runs out, or it is stopped — and the composer cleared the
    // draft the moment it was sent. Without this the user's words would simply vanish.
    // A retry never emits user_prompt (that is the point), so "began" is not meaningful for it.
    let began = start.kind === 'retry';
    const watch = (event: TurnEvent) => {
      if (event.type === 'user_prompt' || event.type === 'assistant_text' || event.type === 'assistant_thought' || event.type === 'tool_use') {
        began = true;
      }
      onEvent(id, event);
    };

    let started: Turn;
    if (start.kind === 'autopilot') {
      runActive = true;
      onRunStateChange?.(true);
      started = startAutopilot(id, watch);
    } else if (start.kind === 'opening') {
      started = openRehearsal(id, watch);
    } else if (start.kind === 'retry') {
      // A failed autopilot left runActive false after endTurn; if the last user brief was
      // the unattended run, treat the retry as that run again so the host can keep its
      // "run in progress" affordances. We cannot see the brief here cheaply — the server
      // already re-raises the ceiling — so only flip the flag when the host already knew
      // a run was in play (openingActions with autopilot, or prior runActive). For the
      // tailor workspace, refreshing after a failed run is enough via onTurnComplete.
      started = retryTurn(id, watch);
    } else {
      started = sendTurn(id, start.text, watch);
    }
    turn = started;
    void scrollToBottom(true);
    try {
      await started.done;
      if (!began && start.kind === 'message' && id === activeId) {
        // The turn never ran, so the message was never recorded. Give it back rather than
        // losing it, and say why — the composer is empty and nothing else would explain it.
        draft = draft.trim() === '' ? start.text : draft;
        error = 'That message was not sent: the chat was still busy. Try again.';
      }
    } catch (err) {
      if (err instanceof StreamInterrupted) {
        // The stream broke, not the turn: it runs on the server under its own bounds and
        // stores everything it does. Marking the message as failed would tell the user
        // their CV edits were lost when they were not — so we re-read the session and show
        // whatever the agent has managed so far.
        await reloadTranscript(id);
        endTurn();
        return;
      }
      if (err instanceof TurnRefused && err.canExtend) {
        // The turn never started, so the message was never recorded — give it back to the
        // composer rather than losing it while the candidate decides whether to continue.
        draft = draft.trim() === '' && start.kind === 'message' ? start.text : draft;
        refusal = err;
        endTurn();
        return;
      }
      error = err instanceof Error ? err.message : 'Could not send the message.';
      chat = reduceTurnEvent(chat, { type: 'result', stop_reason: 'error', is_error: true });
      endTurn();
    }
  }

  /** Spend another of today's tailoring sessions so this conversation can carry on.
   *
   *  It does not re-send the message: the draft was handed back when the turn was refused,
   *  so the candidate sends it themselves and can change their mind first. Extending is
   *  idempotent on the server, so a double click costs one session rather than two. */
  async function continueSession() {
    const stopped = refusal;
    if (!stopped || extending) return;
    extending = true;
    try {
      await extendSession(stopped.sessionId);
      refusal = null;
    } catch (err) {
      // Refused again means the day's tailoring allowance is gone too, and that IS a wall
      // until tomorrow — so it becomes an ordinary error rather than another offer.
      refusal = null;
      error = err instanceof Error ? err.message : 'Could not continue this session.';
    } finally {
      extending = false;
    }
  }

  function retryFailedTurn() {
    if (!canStartTurn()) return;
    void dispatch({ kind: 'retry' });
  }

  /** Re-read a session's stored transcript into the view. The server holds the truth about a
   *  turn whose stream we lost, so this is how a returning client catches up. */
  async function reloadTranscript(id: string) {
    if (id !== activeId) return;
    try {
      const { messages } = await getSession(id);
      let next = initChat();
      for (const event of eventsFromTranscript(messages)) next = reduceTurnEvent(next, event);
      if (id === activeId) chat = next;
    } catch {
      /* the transcript stays as it is; the next visit will catch up */
    }
  }

  function removeQueued(id: string) {
    queue = queue.filter((q) => q.id !== id);
  }

  /** Catch up when the tab comes back. A phone freezes a backgrounded tab, which breaks the
   *  stream while the turn keeps running on the server — so what is on screen when the user
   *  returns is whatever arrived before the freeze, and the session holds the rest. Nothing
   *  is re-read while a turn is streaming: that stream is already the newer truth. */
  function catchUpOnReturn() {
    if (document.visibilityState !== 'visible' || turnActive || !activeId) return;
    void reloadTranscript(activeId);
  }

  onMount(() => {
    void boot();
    voiceSupported =
      AUDIO_ENABLED &&
      canUseVoiceCall({
        mediaDevices: navigator.mediaDevices,
        RTCPeerConnection: typeof RTCPeerConnection === 'undefined' ? undefined : RTCPeerConnection,
      });
    document.addEventListener('visibilitychange', catchUpOnReturn);
    return () => {
      document.removeEventListener('visibilitychange', catchUpOnReturn);
      cancelTurn();
    };
  });
</script>

<!-- Single flex-1 root so a host can compose the chat beside another pane (e.g. the tailor
     artifact panel); the host supplies the outer height + flex row. -->
<div class="flex min-w-0 flex-1 flex-col">
{#if error}
  <div
    class="m-3 mb-0 flex items-start gap-2 rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive"
    role="alert"
  >
    <AlertTriangle class="mt-0.5 size-4 shrink-0" />
    <span>{error}</span>
  </div>
{/if}
{#if refusal}
  <div
    class="m-3 mb-0 flex items-start gap-3 rounded-lg border border-warning/40 bg-warning/5 px-3 py-2 text-sm"
    role="status"
  >
    <AlertTriangle class="mt-0.5 size-4 shrink-0 text-warning-strong" />
    <div class="flex min-w-0 flex-1 flex-col gap-1">
      <span>{refusal.message}</span>
      {#if refusal.ceiling > 0}
        <span class="text-xs text-muted-foreground">
          {refusal.turns} of {refusal.ceiling} messages used in this session.
        </span>
      {/if}
    </div>
    <button
      type="button"
      class="shrink-0 rounded-md border border-border px-2 py-1 text-xs font-medium transition-colors hover:bg-muted disabled:opacity-60"
      onclick={continueSession}
      disabled={extending}
    >
      {extending ? 'Continuing…' : 'Continue — uses another session'}
    </button>
  </div>
{/if}
{#if bulletCapAlert}
  <div
    class="m-3 mb-0 flex items-start gap-2 rounded-lg border border-warning/40 bg-warning/5 px-3 py-2 text-sm text-foreground"
    role="status"
  >
    <AlertTriangle class="mt-0.5 size-4 shrink-0 text-warning-strong" />
    <span class="min-w-0 flex-1">{bulletCapAlert}</span>
    <button
      type="button"
      class="shrink-0 rounded-md p-0.5 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
      aria-label="Dismiss"
      onclick={() => (bulletCapAlert = null)}
    >
      <X class="size-4" />
    </button>
  </div>
{/if}

{#if notFound}
  <div class="m-3 rounded-xl border border-border bg-card p-8 text-center">
    <p class="text-sm text-muted-foreground">
      This chat no longer exists, or it belongs to a CV you are tailoring.
    </p>
    <a
      href={resolve('/my/assistant/[[id]]', {})}
      class="mt-4 inline-flex items-center gap-2 rounded-lg border border-border px-3 py-2 text-sm font-medium transition-colors hover:bg-muted"
    >
      Open your chats
    </a>
  </div>
{:else}
  <div class="flex min-h-0 flex-1">
    <!-- Session rail (desktop): collapsible; hidden entirely when the host disables it. -->
    {#if showSessionRail && sidebarOpen}
      <SessionRail
        {sessions}
        {activeId}
        {switching}
        {creating}
        ready={phase === 'ready'}
        onNew={newChat}
        onSelect={selectSession}
        onDelete={requestRemoveChat}
      />
    {/if}

    <!-- Chat pane -->
    <div class="flex min-w-0 flex-1 flex-col">
      <!-- Desktop: collapse/expand the session list (only when a rail exists). -->
      {#if showSessionRail}
        <div class="hidden items-center gap-1 border-b border-border px-2 py-1.5 md:flex">
          <button
            type="button"
            onclick={() => (sidebarOpen = !sidebarOpen)}
            class="rounded p-1 text-muted-foreground transition-colors hover:text-foreground"
            aria-label={sidebarOpen ? 'Hide chats' : 'Show chats'}
            title={sidebarOpen ? 'Hide chats' : 'Show chats'}
          >
            <PanelLeft class="size-4" />
          </button>
          {#if !sidebarOpen}
            <button
              type="button"
              onclick={newChat}
              disabled={creating || switching || phase !== 'ready'}
              class="flex items-center gap-1.5 rounded px-1.5 py-1 text-sm text-muted-foreground transition-colors hover:text-foreground disabled:cursor-not-allowed disabled:opacity-50"
              title="New chat"
            >
              <Plus class="size-4" />New chat
            </button>
          {/if}
        </div>
      {/if}
      <!-- Mobile session switcher -->
      {#if showSessionRail}
      <div class="flex items-center gap-2 border-b border-border p-2 md:hidden">
        <select
          value={activeId}
          onchange={(e) => selectSession(e.currentTarget.value)}
          disabled={switching}
          aria-label="Select chat"
          class="min-w-0 flex-1 rounded-lg border border-border bg-background px-2 py-1.5 text-sm"
        >
          {#each sessions as s (s.id)}
            <option value={s.id}>{s.label}</option>
          {/each}
        </select>
        <!-- Starting a conversation is the primary action here and it is what people came
             to do, so on mobile it is a labelled button rather than a bare glyph, and it is
             a real 44px touch target. Delete is pushed to the far side with a gap: an
             unlabelled trash icon sitting flush against the "new" control is a mis-tap that
             destroys a conversation. -->
        <button
          type="button"
          onclick={newChat}
          disabled={creating || switching || phase !== 'ready'}
          title="New chat"
          class="flex h-11 shrink-0 items-center gap-1.5 rounded-lg bg-brand px-3 text-sm font-medium text-brand-foreground disabled:opacity-50"
        >
          <Plus class="size-4" />
          New
        </button>
        {#if activeId}
          <button
            type="button"
            onclick={() => requestRemoveChat(activeId as string)}
            aria-label="Delete chat"
            title="Delete chat"
            class="ml-1 flex size-11 shrink-0 items-center justify-center rounded-lg border border-border text-muted-foreground hover:bg-muted"
          >
            <Trash2 class="size-4" />
          </button>
        {/if}
      </div>
      {/if}

      <!-- Message list -->
      <div bind:this={scroller} onscroll={onPaneScroll} class="flex-1 overflow-y-auto p-4">
        <div class="mx-auto flex max-w-3xl flex-col gap-3">
          {#if phase === 'loading'}
            <p class="text-sm text-muted-foreground">Connecting to the agent…</p>
          {:else if chat.messages.length === 0 && openingActions?.length}
            <div class="rounded-xl border border-border bg-muted/30 p-4">
              <div class="flex flex-wrap gap-2">
                <!-- The icon comes from the action's kind, not from the action itself: the
                     surfaces that offer these are plain modules, and a Svelte component cannot
                     travel through one. The two kinds are the two rhythms — the unattended run
                     and the conversation — so the mapping is complete by construction. -->
                {#each openingActions as action (action.label)}
                  {@const Icon = action.kind === 'autopilot' ? WandSparkles : MessagesSquare}
                  <button
                    type="button"
                    class="inline-flex items-center gap-1.5 rounded-lg bg-brand px-3 py-2 text-sm font-medium text-brand-foreground transition-opacity hover:opacity-90 disabled:opacity-50"
                    disabled={turnActive || switching}
                    onclick={() => runOpening(action)}
                  >
                    <Icon class="size-4" />
                    {action.label}
                  </button>
                {/each}
              </div>
              {#each openingActions.filter((a) => a.hint) as action (action.label)}
                <p class="mt-2 text-xs text-muted-foreground">{action.hint}</p>
              {/each}
            </div>
          {:else if chat.messages.length === 0 && !autoRun}
            <!-- Never shown on a cold start (autoRun): the brief hasn't landed as a message
                 yet, but the run has already started — inviting the candidate to type here
                 would contradict the "Crunching…" indicator already on screen. -->
            <p class="text-sm text-muted-foreground">Ask the agent anything to get started.</p>
          {/if}

          {#each chat.messages as message, i (i)}
            {#if message.role === 'user'}
              <article class="self-end max-w-[80%] rounded-2xl rounded-br-md bg-secondary px-4 py-2.5 text-sm leading-relaxed text-secondary-foreground">
                <pre class="m-0 whitespace-pre-wrap break-words font-sans">{message.text}</pre>
              </article>
            {:else}
              {@const active = i === chat.messages.length - 1 && message.streaming}
              {#if message.thinking}
                <details class="self-start max-w-[88%] text-xs text-muted-foreground" open={active}>
                  <summary class="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1 hover:bg-muted/50 [&::-webkit-details-marker]:hidden [&::marker]:hidden">
                    <span
                      class={[
                        'font-mono text-[0.85rem] font-semibold',
                        active ? 'star-glow' : 'text-muted-foreground/60',
                      ]}
                    >
                      {active ? SPINNER_GLYPHS[spinnerIdx] : '✶'}
                    </span>
                    <span class={['font-medium', active && 'shimmer']}>Thinking</span>
                    {#if active}
                      <span class="font-mono text-[0.7rem] text-muted-foreground/70">({elapsedSec}s)</span>
                    {/if}
                  </summary>
                  <div class="md mt-1 max-h-56 overflow-y-auto border-l-2 border-border pl-3 py-1 text-muted-foreground">
                    <!-- eslint-disable-next-line svelte/no-at-html-tags -- DOMPurify-sanitized markdown -->
                    {@html renderMarkdown(message.thinking)}
                  </div>
                </details>
              {/if}

              <!-- A recommendation is a `present_jobs` call, not prose. Its cards
                   render as a deck here and the call itself is withheld from the
                   activity list, so no progress chip sits above the deck it
                   produced — the work that found the vacancies stays in the
                   activity list above them. `message.streaming` decides what an
                   unanswered call means: a placeholder mid-turn, nothing once the
                   turn has closed without its result. -->
              {@const { decks, rest } = splitPresentingCalls(message.tools, message.streaming)}
              <ToolGroupList calls={rest} onConfirm={submitText} disabled={turnActive || switching} />
              {#each decks as slot, di (di)}
                <div class="self-start w-full">
                  <JobDeck {slot} />
                </div>
              {/each}

              {#if message.text}
                <article class="md self-start max-w-[88%] px-1 py-1 text-sm leading-relaxed text-foreground">
                  <!-- eslint-disable-next-line svelte/no-at-html-tags -- DOMPurify-sanitized markdown -->
                  {@html renderMarkdown(message.text)}
                </article>
              {/if}

              {#if message.errored}
                <div class="self-start flex flex-wrap items-center gap-2 text-xs">
                  <p class="text-destructive">The agent ended the turn with an error.</p>
                  {#if i === chat.messages.length - 1 && !turnActive && !switching}
                    <button
                      type="button"
                      class="rounded-md border border-border bg-background px-2 py-0.5 font-medium text-foreground transition-colors hover:bg-muted disabled:opacity-50"
                      disabled={!canStartTurn()}
                      onclick={retryFailedTurn}
                    >
                      Retry
                    </button>
                  {/if}
                </div>
              {/if}
            {/if}
          {/each}

          <!-- One indicator, two states. Queued is not working: nothing is being spent yet, and
               the elapsed counter would be timing somebody else's turn. -->
          {#if chat.queued || turnActive}
            <div class="self-start inline-flex items-baseline gap-2 px-2 py-1 text-xs text-muted-foreground">
              <span class="star-glow font-mono text-[0.85rem] font-semibold">
                {SPINNER_GLYPHS[spinnerIdx]}
              </span>
              <span class="shimmer font-medium">
                {chat.queued ? 'Waiting for the current turn to finish' : currentVerb}…
              </span>
              {#if !chat.queued}
                <span class="font-mono text-[0.7rem] text-muted-foreground/70">({elapsedSec}s)</span>
              {/if}
            </div>
          {/if}
        </div>

        <!-- Jump to latest. Sticky rather than absolute so it needs no positioned
             ancestor: as the scroller's last child its natural place is past the bottom
             of the scrollport, which is exactly when `bottom-2` pins it into view. -->
        {#if !stickToBottom}
          <div class="pointer-events-none sticky bottom-2 mt-2 flex justify-center">
            <button
              type="button"
              onclick={() => scrollToBottom(true)}
              class="pointer-events-auto flex items-center gap-1.5 rounded-full border border-border bg-card px-3 py-1.5 text-xs font-medium text-muted-foreground shadow-md transition-colors hover:text-foreground"
            >
              <ArrowDown class="size-3.5" />
              Jump to latest
            </button>
          </div>
        {/if}
      </div>

      {#if voiceCallOpen && activeId}
        <!-- Keyed on activeId: switching sessions while a call is open must destroy
             this instance (its onDestroy ends the call cleanly) rather than reuse it
             with a new sessionId prop, which would silently post the OLD call's turns
             into the NEW session's transcript. -->
        {#key activeId}
          <div class="border-t border-border p-3">
            <div class="mx-auto w-full max-w-3xl">
              <VoiceCall
                sessionId={activeId}
                onClose={closeVoiceCall}
                onUnavailable={() => {
                  voiceModeOff = true;
                  closeVoiceCall();
                }}
              />
            </div>
          </div>
        {/key}
      {:else}
        <Composer
          bind:draft
          {queue}
          {turnActive}
          disabled={phase !== 'ready' || switching}
          onSubmit={submitText}
          onRemoveQueued={removeQueued}
          onCancel={cancelTurn}
        />
        {#if activePreset === 'interview' && activeId && voiceSupported && !voiceModeOff}
          <div class="flex justify-center border-t border-border/60 py-2">
            <button
              type="button"
              onclick={() => (voiceCallOpen = true)}
              disabled={phase !== 'ready' || switching || turnActive}
              class="flex items-center gap-1.5 rounded-full px-3 py-1.5 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:cursor-not-allowed disabled:opacity-50"
            >
              <Phone class="size-3.5" />
              Voice mode
            </button>
          </div>
        {/if}
      {/if}
    </div>
  </div>
{/if}
</div>

<ConfirmDialog
  bind:open={confirmRemoveChatOpen}
  title="Delete this chat?"
  description="This cannot be undone."
  confirmLabel="Delete"
  variant="destructive"
  onConfirm={confirmRemoveChat}
/>

<style>
  /* Shimmer over the spinner verb. background-clip masks the text shape onto
     a moving gradient — no JS, no repaints beyond the GPU-composited layer. */
  .shimmer {
    background: linear-gradient(
      90deg,
      var(--color-muted-foreground) 0%,
      var(--color-muted-foreground) 35%,
      var(--color-foreground) 50%,
      var(--color-muted-foreground) 65%,
      var(--color-muted-foreground) 100%
    );
    background-size: 200% 100%;
    background-clip: text;
    -webkit-background-clip: text;
    color: transparent;
    -webkit-text-fill-color: transparent;
    animation: shimmer-pan 2.4s linear infinite;
  }
  @keyframes shimmer-pan {
    0%   { background-position: 200% 0; }
    100% { background-position: -200% 0; }
  }

  .star-glow {
    color: var(--color-foreground);
    animation: star-pulse 1.8s ease-in-out infinite;
  }
  @keyframes star-pulse {
    0%, 100% { opacity: 0.7; }
    50%      { opacity: 1; }
  }

  @media (prefers-reduced-motion: reduce) {
    .star-glow { animation: none; opacity: 1; }
    .shimmer {
      animation: none;
      background: none;
      color: var(--color-foreground);
      -webkit-text-fill-color: currentColor;
    }
  }

  /* Markdown rendering — scoped, applied with :global to reach @html output. */
  .md :global(*:first-child) { margin-top: 0; }
  .md :global(*:last-child)  { margin-bottom: 0; }
  .md :global(p) { margin: 0 0 0.5rem; line-height: 1.55; }
  .md :global(h1),
  .md :global(h2),
  .md :global(h3),
  .md :global(h4) {
    margin: 0.8rem 0 0.35rem;
    line-height: 1.3;
    font-weight: 600;
  }
  .md :global(h1) { font-size: 1.1rem; }
  .md :global(h2) { font-size: 1.0rem; }
  .md :global(h3),
  .md :global(h4) { font-size: 0.95rem; }
  .md :global(ul),
  .md :global(ol) { margin: 0 0 0.5rem; padding-left: 1.25rem; }
  .md :global(li) { margin: 0.1rem 0; }
  .md :global(li > p) { margin: 0; }
  .md :global(a) {
    color: oklch(0.6 0.18 280);
    text-decoration: underline;
    text-underline-offset: 2px;
  }
  .md :global(strong) { font-weight: 600; }
  .md :global(em) { font-style: italic; }
  .md :global(code) {
    font-family: var(--font-mono);
    font-size: 0.85em;
    padding: 0.05rem 0.3rem;
    border-radius: 4px;
    background: color-mix(in oklab, currentColor 10%, transparent);
  }
  .md :global(pre) {
    background: color-mix(in oklab, currentColor 12%, transparent);
    padding: 0.65rem 0.8rem;
    border-radius: 6px;
    overflow-x: auto;
    margin: 0.4rem 0;
    font-size: 0.85em;
  }
  .md :global(pre code) {
    background: transparent;
    padding: 0;
    font-size: 1em;
  }
  .md :global(blockquote) {
    margin: 0.4rem 0;
    padding: 0.2rem 0.7rem;
    border-left: 3px solid var(--color-border);
    color: var(--color-muted-foreground);
  }
  .md :global(table) {
    border-collapse: collapse;
    margin: 0.4rem 0;
    font-size: 0.85em;
  }
  .md :global(th),
  .md :global(td) {
    border: 1px solid var(--color-border);
    padding: 0.25rem 0.55rem;
    text-align: left;
  }
  .md :global(hr) {
    border: none;
    border-top: 1px solid var(--color-border);
    margin: 0.6rem 0;
  }
</style>
