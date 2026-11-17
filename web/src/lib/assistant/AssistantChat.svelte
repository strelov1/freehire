<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import {
    createSession,
    listSessions,
    getSession,
    deleteSession,
    SessionNotFound,
    SUPPORT_BASE,
  } from '#lib/assistant/api.js';
  import { track } from '#lib/analytics.js';
  import {
    TurnRefused,
    startTurnFor,
    StreamInterrupted,
    type Attachment,
    type Turn,
    type TurnStart,
  } from '#lib/assistant/client.js';
  import { chatFromTranscript, initChat, reduceTurnEvent, turnBegins, type ChatState } from '#lib/assistant/chat.js';
  import SessionRail from '#lib/assistant/SessionRail.svelte';
  import ChatInputArea from './ChatInputArea.svelte';
  import ChatGone from './ChatGone.svelte';
  import ChatDeleteConfirm from './ChatDeleteConfirm.svelte';
  import ChatSwitchConfirm from './ChatSwitchConfirm.svelte';
  import ChatTranscript from './ChatTranscript.svelte';
  import { PaneFollow } from './paneFollow.svelte';
  import { ApplicationSync } from './applicationSync';
  import { MessageQueue } from './messageQueue.svelte';
  import { addressStep, autostartTurn, type Arrival } from './chatRouting';
  import { ChatAlerts } from './chatAlerts.svelte';
  import AssistantAlerts from './AssistantAlerts.svelte';
  import type { AssistantChatProps } from './AssistantChat.props';
  import ChatSessionBar from './ChatSessionBar.svelte';
  import {
    fromSummary,
    upsertSession,
    removeSession,
    setLabel,
    activeAfterDelete,
    labelFromMessage,
    type SessionItem,
  } from '#lib/assistant/sessions.js';
  import type { TurnEvent } from '#lib/assistant/wire.js';
  import { opensInRail, type OpeningAction } from '#lib/assistant/presets.js';
  import { messages } from './AssistantChat.messages';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';

  // The agent chat. The agent runs inside the freehire backend, so this is an
  // ordinary authenticated API surface: the session list, one session's stored
  // transcript, and a turn that streams its events over SSE. There is no
  // connection to hold, nothing to install, and no separate agent login.
  // Everything streaming-shaped runs client-side, so SSR renders only the shell.

  type Phase = 'loading' | 'ready';

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
    navTrigger = undefined,
    syncApplication: keepInSync = false,
  }: AssistantChatProps = $props();

  const s = $derived(t(messages, locale()));

  let phase = $state<Phase>('loading');
  // Startup requests may finish after the host has replaced this conversation.
  let disposed = false;
  // What the banners over the transcript say (see ChatAlerts).
  const alerts = new ChatAlerts();

  // Sidebar: the caller's conversations, newest activity first. `switching`
  // disables the composer and list while a session load is in flight.
  let sessions = $state<SessionItem[]>([]);
  let activeId = $state<string | null>(null);
  // A plain chat with nothing to resume opens blank, with no session behind it yet: the first
  // message creates one. Minting it on page open left most chat sessions empty — new accounts
  // land here and leave without typing (2026-11-09: 143 of 152 a week).
  let deferredNew = $state(false);
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
  // Documents read for the next message. Held here beside the draft, and handed back with it
  // when a turn never starts, for the same reason: the composer cleared them on send.
  let attachments = $state<Attachment[]>([]);
  let turnActive = $state(false);
  // The turn in flight, so it can be cancelled. Cancelling asks the server to stop the work;
  // aborting the fetch only stops this client reading it.
  let turn: Turn | null = null;

  let sidebarOpen = $state(true);
  // Whether the transcript follows the stream (see PaneFollow).
  const follow = new PaneFollow();

  const queue = new MessageQueue();

  // Voice mode: a hands-free call on an `interview` session (see ChatInputArea).
  let voiceCallOpen = $state(false);

  function closeVoiceCall() {
    voiceCallOpen = false;
    if (activeId) void reloadTranscript(activeId);
  }

  const NEW_CHAT_LABEL = $derived(s.newChat);

  // What the URL this component mounted on asked for (see Arrival).
  const arrival: Arrival = { preset, kickoff };
  const isSupport = untrack(() => preset) === 'support';

  // Whether the turn in flight is an unattended run rather than a reply to something typed.
  let runActive = $state(false);

  /** Whether a turn may be started right now: the pane is settled and nothing is running. */
  const canStartTurn = () => phase === 'ready' && !turnActive && !switching && !creating && (!!activeId || deferredNew);
  function startTurn(start: TurnStart) {
    if (canStartTurn()) void dispatch(start);
  }

  /**
   * Start an unattended run from outside the chat — the workspace's "Run again" beside the
   * run report. Exported rather than driven by a prop because it is an ACTION, not state:
   * a boolean the host toggles would re-run the moment anything else re-rendered.
   */
  export function startRun() {
    startTurn({ kind: 'autopilot' });
  }

  /** Run an opening action — the first turn of a conversation nobody has typed into yet. */
  function runOpening(action: OpeningAction) {
    startTurn(action.kind === 'message' ? { kind: 'message', text: action.text } : { kind: 'autopilot' });
  }

  /** The conversation the host is currently being navigated to, or null. Held until the
   *  `session` prop catches up, so a switch we started is never undone by the stale URL. */
  let navigatingTo = $state<string | null>(null);

  // --- Session orchestration -----------------------------------------------

  async function boot() {
    phase = 'loading';
    alerts.error = null;
    try {
      if (isSupport) {
        await createAndOpen();
        if (disposed) return;
        phase = 'ready';
        if (arrival.kickoff) {
          const opening = arrival.kickoff;
          arrival.kickoff = undefined;
          void dispatch({ kind: 'message', text: opening });
        }
        return;
      }
      let summaries: Awaited<ReturnType<typeof listSessions>> = [];
      try {
        summaries = await listSessions();
      } catch {
        alerts.fail(s.loadChatsFailed);
      }
      if (disposed) return;
      sessions = summaries.map((summary) => fromSummary(summary, NEW_CHAT_LABEL));

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
        startBlankChat();
      }
      if (disposed) return;
      phase = 'ready';

      // Autostart, once, on an empty conversation (see autostartTurn).
      const auto = autostartTurn({
        kickoff: arrival.kickoff,
        empty: chat.messages.length === 0,
        preset: activePreset,
        autoRun,
      });
      if (auto?.kind === 'message') arrival.kickoff = undefined;
      if (auto) void dispatch(auto);
    } catch (err) {
      report(err, s.assistantUnreachable);
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

  /** Reconcile the pane with the address the host is showing (see addressStep). */
  function followAddress(requested: string | undefined) {
    if (isSupport) return;
    const step = addressStep(requested, { navigatingTo, deferredNew, activeId });
    if (step.clearGuard) navigatingTo = null;
    if (step.action === 'boot') {
      alerts.notFound = false;
      void boot();
    } else if (step.action === 'open' && requested) {
      openSession(requested, true).catch((err: unknown) => report(err, s.openChatFailed));
    }
  }

  /** Surface a failure. A conversation the caller cannot open is a dead link, not
   *  a broken assistant, so it gets the explanation panel rather than an error. */
  function report(err: unknown, fallback: string) {
    if (err instanceof SessionNotFound) {
      alerts.notFound = true;
      return;
    }
    alerts.fail(err instanceof Error ? err.message : fallback);
  }

  // The discard dialog a switch waits on while messages are queued (see ChatSwitchConfirm).
  let confirmSwitchOpen = $state(false);
  let switchConfirm: ReturnType<typeof ChatSwitchConfirm> | undefined = $state();

  // A switch that came from the address — Back, Forward, a link — has already moved the URL
  // to the declined chat, so put it back on the one still on screen; otherwise the address
  // and the pane disagree.
  function declineSwitch(declined: { fromAddress: boolean }) {
    // The guard is raised only when a navigation will follow, or it outlives it (see
    // followAddress).
    if (declined.fromAddress && activeId && onSessionChange) {
      navigatingTo = activeId;
      onSessionChange(activeId);
    }
    // A turn may have finished while the dialog held the queue; send what waited.
    drainQueue();
  }

  // Open a session and repaint its stored transcript (see chatFromTranscript).
  //
  // `fromAddress` says the address ALREADY names this chat — we are following it, not
  // asking the host to change it. That distinction is the guard's whole life: raising it
  // when no navigation will follow leaves it raised forever, and the pane then ignores
  // every later address change, which is Forward appearing to do nothing.
  async function openSession(id: string, fromAddress = false, discardQueued = false) {
    if (activeId === id && chat.messages.length > 0) return;
    // A message typed while the current turn was streaming sits in `queue`, waiting
    // for the turn to finish so endTurn() can drain it — cancelTurn() below stops the
    // turn but never drains the queue, so switching here would otherwise wipe out
    // whatever the user just typed with no trace and no way back. The switch waits on
    // the dialog: this call returns as a refusal would, and confirming re-enters it.
    if (queue.length > 0 && !discardQueued) {
      switchConfirm?.ask(id, fromAddress);
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
      queue.clear();
      alerts.bulletCap = null;
      activeId = id;
      activePreset = null;
      // Raise the guard HERE, not after the fetch below: the URL-following effect reruns
      // the moment activeId changes, which is during the await — set it late and the
      // effect has already reopened the conversation we just left.
      navigatingTo = fromAddress ? null : id;
      const { session: meta, messages: transcriptMessages } = await getSession(id);
      if (disposed) return;
      // A tailoring conversation is reachable by id but belongs to a CV and only makes
      // sense beside it. Opening one here would show a conversation the rail cannot list
      // and the user cannot get back to. The unbound presets — chat and the experience
      // interviewer — are both at home here.
      if (showSessionRail && !opensInRail(meta.preset)) {
        alerts.notFound = true;
        return;
      }
      activePreset = meta.preset;
      chat = chatFromTranscript(transcriptMessages);
      if (meta.label?.trim()) sessions = setLabel(sessions, id, labelFromMessage(meta.label));
      onSessionChange?.(id);
    } finally {
      switching = false;
    }
    void follow.toBottom(true);
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
      if (!alerts.notFound) return true;
      alerts.notFound = false;
    }
    return false;
  }

  function startBlankChat() {
    activeId = null;
    activePreset = 'chat';
    chat = initChat();
    deferredNew = true;
  }

  async function createAndOpen() {
    const created = await createSession(arrival.preset);
    // Only once a session exists: a failed create leaves a blank chat still able to send.
    deferredNew = false;
    if (isSupport) {
      // The support routes cannot replay a transcript, so the new session is opened as is.
      activeId = created.id;
      activePreset = 'support';
      // A host that started over while this was being created has moved on to another chat.
      if (!disposed) onSessionChange?.(created.id);
      return;
    }
    // Spent: the next chat minted here — "New chat", or a later boot() — is a plain one.
    arrival.preset = 'chat';
    sessions = upsertSession(sessions, fromSummary(created, NEW_CHAT_LABEL));
    await openSession(created.id);
  }

  async function newChat() {
    if (creating || switching || phase !== 'ready') return;
    creating = true;
    alerts.error = null;
    try {
      await createAndOpen();
    } catch (err) {
      alerts.fail(err instanceof Error ? err.message : s.newChatFailed);
    } finally {
      creating = false;
    }
  }

  async function selectSession(id: string) {
    if (switching || id === activeId) return;
    alerts.error = null;
    try {
      await openSession(id);
    } catch (err) {
      report(err, s.openChatFailed);
    }
  }

  let deleteConfirm: ReturnType<typeof ChatDeleteConfirm> | undefined = $state();

  function requestRemoveChat(id: string) {
    if (switching) return;
    deleteConfirm?.request(id);
  }

  async function removeChat(id: string) {
    if (switching) return;
    alerts.error = null;
    const wasActive = id === activeId;
    try {
      await deleteSession(id);
    } catch {
      alerts.fail(s.deleteChatFailed);
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
        // Not a blank chat: the address still names the deleted one, and only a new id moves it.
        if (next) await openSession(next);
        else await createAndOpen();
      } catch (err) {
        alerts.fail(err instanceof Error ? err.message : s.openAChatFailed);
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
    drainQueue();
  }

  // Send the next queued message — but not while the "switch and discard?" dialog is up:
  // the visitor is deciding whether that queue should exist at all, and a message sent in
  // the meantime could start work that confirming the switch can no longer take back.
  // Declining the switch drains it (see the effect beside confirmSwitch).
  function drainQueue() {
    if (confirmSwitchOpen || turnActive || queue.length === 0) return;
    const next = queue.shift();
    if (next) void dispatch({ kind: 'message', text: next.text, attachments: next.attachments });
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
      // A message that is only a document is labelled by its name, as the server does.
      sessions = setLabel(sessions, sessionId, labelFromMessage(event.text || event.attachments?.[0] || ''));
    }
    alerts.noteEvent(event);
    if (event.type === 'tool_result' && event.name === 'cv_edit' && !event.is_error) {
      onDocumentEdited?.();
    }
    chat = reduceTurnEvent(chat, event);
    if (event.type === 'result') {
      endTurn();
    }
    void follow.toBottom();
  }

  // Composer submit: while a turn is in flight the message is queued and drained
  // later; otherwise it is sent immediately.
  function submitText(raw: string, docs: Attachment[] = []) {
    const text = raw.trim();
    if ((!text && docs.length === 0) || phase !== 'ready' || switching || creating || (!activeId && !deferredNew))
      return;
    if (alerts.error?.cutOff) alerts.error = null;
    // Counted once per message the user sends, queued or dispatched — never the text
    // itself. This measures usage only; what a turn costs is recorded server-side.
    track('assistant_message');
    draft = '';
    attachments = [];
    if (turnActive || queue.length > 0) {
      queue.add(text, docs);
      void follow.toBottom(true);
      return;
    }
    void dispatch({ kind: 'message', text, attachments: docs });
  }

  async function dispatch(start: TurnStart) {
    if (!activeId && deferredNew && !disposed) {
      // Held as a switch while the session is created: the composer refuses a second send
      // (openSession, which ends this, would clear a queued one anyway).
      switching = true;
      let created: Awaited<ReturnType<typeof createSession>>;
      try {
        created = await createSession('chat');
      } catch (err) {
        switching = false;
        giveBack(start);
        alerts.fail(err instanceof Error ? err.message : s.newChatFailed);
        return;
      }
      sessions = upsertSession(sessions, fromSummary(created, NEW_CHAT_LABEL));
      // The caller opened another chat while this one was being created. Leave them there,
      // and hand the message back rather than send it somewhere it was not written for.
      if (disposed || activeId !== null) {
        giveBack(start);
        return;
      }
      deferredNew = false;
      try {
        await openSession(created.id);
      } catch (err) {
        // The session exists and is active, so a resend goes to it; only this send is lost.
        giveBack(start);
        report(err, s.openChatFailed);
        return;
      }
      if (activeId !== created.id) {
        giveBack(start);
        return;
      }
    }
    const id = activeId;
    if (!id || disposed) return;
    if (!alerts.error?.cutOff) alerts.error = null;
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
    if (disposed) return;
    // Whether the turn ever began. A message can queue behind the session's running turn and
    // then never start — the wait runs out, or it is stopped — and the composer cleared the
    // draft the moment it was sent. Without this the user's words would simply vanish.
    // A retry never emits user_prompt (that is the point), so "began" is not meaningful for it.
    let began = start.kind === 'retry';
    const watch = (event: TurnEvent) => {
      if (turnBegins(event)) began = true;
      onEvent(id, event);
    };

    if (start.kind === 'autopilot') {
      runActive = true;
      onRunStateChange?.(true);
    }
    const started = startTurnFor(id, start, watch, isSupport ? SUPPORT_BASE : undefined);
    turn = started;
    void follow.toBottom(true);
    try {
      await started.done;
      if (!began && start.kind === 'message' && id === activeId) {
        // The turn never ran, so the message was never recorded. Give it back rather than
        // losing it, and say why — the composer is empty and nothing else would explain it.
        giveBack(start);
        alerts.fail(s.messageNotSent);
      }
    } catch (err) {
      if (err instanceof StreamInterrupted) {
        // The stream broke, not the turn: it runs on the server under its own bounds and
        // stores everything it does. Marking the message as failed would tell the user
        // their CV edits were lost when they were not — so we re-read the session and show
        // whatever the agent has managed so far. A support session cannot be re-read that
        // way, so the rest of its answer is lost: say so rather than leave a silent cut. The
        // open answer is closed as an ordinary one (not errored — the server turn did not
        // fail), so it stops showing as streaming.
        if (isSupport && id === activeId) {
          chat = reduceTurnEvent(chat, { type: 'result', stop_reason: 'end_turn' });
          alerts.cutOff(s.supportInterrupted);
        } else await reloadTranscript(id);
        endTurn();
        return;
      }
      if (err instanceof TurnRefused) {
        // Neither remedy below ever started the turn, so the message was never recorded —
        // give it back to the composer rather than losing it while the candidate decides
        // what to do next.
        giveBack(start);
        if (err.canExtend) {
          // The session can be continued right now by spending another of the day's
          // tailoring sessions.
          alerts.refusal = err;
        } else {
          // A spent daily allowance: Retry would only repeat the same refusal, and there is
          // no assistant frame to mark errored (the turn never started), so this skips the
          // generic error result entirely rather than fabricating a dead-end Retry button.
          alerts.limit(err.message);
        }
        endTurn();
        return;
      }
      alerts.fail(err instanceof Error ? err.message : s.sendFailed);
      chat = reduceTurnEvent(chat, { type: 'result', stop_reason: 'error', is_error: true });
      endTurn();
    }
  }

  /** Return an unsent message to the composer — its text when the draft is empty, and its
   *  documents when none have been attached since. */
  function giveBack(start: TurnStart) {
    if (start.kind !== 'message') return;
    if (draft.trim() === '') draft = start.text;
    if (attachments.length === 0) attachments = start.attachments ?? [];
  }

  // An apply conversation catches up on its application while it is shown.
  const appSync = new ApplicationSync({
    busy: () => turnActive || turn !== null,
    began: (started) => {
      turnActive = true;
      turn = started;
      onTurnStateChange?.(true);
    },
    event: onEvent,
    reload: reloadTranscript,
    ended: () => {
      endTurn();
      void follow.toBottom(true);
    },
  });

  $effect(() => {
    if (!keepInSync || phase !== 'ready' || !activeId) return;
    return appSync.watch(activeId);
  });

  /** Re-read a session's stored transcript into the view. The server holds the truth about a
   *  turn whose stream we lost, so this is how a returning client catches up. */
  async function reloadTranscript(id: string) {
    if (isSupport || id !== activeId) return;
    try {
      const { messages: transcriptMessages } = await getSession(id);
      const next = chatFromTranscript(transcriptMessages);
      if (id === activeId) chat = next;
    } catch {
      /* the transcript stays as it is; the next visit will catch up */
    }
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
    document.addEventListener('visibilitychange', catchUpOnReturn);
    return () => {
      disposed = true;
      document.removeEventListener('visibilitychange', catchUpOnReturn);
      cancelTurn();
    };
  });
</script>

<!-- Single flex-1 root so a host can compose the chat beside another pane (e.g. the tailor
     artifact panel); the host supplies the outer height + flex row. -->
<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <AssistantAlerts {alerts} />

  {#if alerts.notFound}
    <ChatGone />
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
      <div class="flex min-h-0 min-w-0 flex-1 flex-col">
        {#if showSessionRail}
          <ChatSessionBar
            {sessions}
            {activeId}
            {switching}
            newDisabled={creating || switching || phase !== 'ready'}
            bind:sidebarOpen
            {navTrigger}
            onNew={newChat}
            onSelect={selectSession}
            onDelete={requestRemoveChat}
          />
        {/if}

        <!-- Message list -->
        <ChatTranscript
          {follow}
          {phase}
          {chat}
          {openingActions}
          {autoRun}
          {activePreset}
          {turnActive}
          {switching}
          canStart={canStartTurn}
          onsubmit={submitText}
          onrunopening={runOpening}
          {isSupport}
          onretry={() => startTurn({ kind: 'retry' })}
        />

        <ChatInputArea
          bind:draft
          bind:attachments
          bind:voiceCallOpen
          sessionId={activeId}
          queue={queue.items}
          {turnActive}
          support={isSupport}
          disabled={phase !== 'ready' || switching}
          interview={activePreset === 'interview'}
          onSubmit={submitText}
          onRemoveQueued={(id) => queue.remove(id)}
          onCancel={cancelTurn}
          onClose={closeVoiceCall}
        />
      </div>
    </div>
  {/if}
</div>

<ChatSwitchConfirm
  bind:this={switchConfirm}
  bind:open={confirmSwitchOpen}
  onConfirm={(target) =>
    openSession(target.id, target.fromAddress, true).catch((err: unknown) => report(err, s.openChatFailed))}
  onDecline={declineSwitch}
/>

<ChatDeleteConfirm bind:this={deleteConfirm} {sessions} newChatLabel={NEW_CHAT_LABEL} onRemove={removeChat} />
