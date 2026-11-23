<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { createSession, listSessions, getSession, deleteSession, type InterviewReport } from '#lib/assistant/api.js';
  import { track } from '#lib/analytics.js';
  import type { Attachment, TurnStart } from '#lib/assistant/client.js';
  import { chatFromTranscript, initChat, reduceTurnEvent, type ChatState } from '#lib/assistant/chat.js';
  import SessionRail from '#lib/assistant/SessionRail.svelte';
  import ChatInputArea from './ChatInputArea.svelte';
  import RehearsalInput from './RehearsalInput.svelte';
  import ChatGone from './ChatGone.svelte';
  import ChatDeleteConfirm from './ChatDeleteConfirm.svelte';
  import ChatSwitchConfirm from './ChatSwitchConfirm.svelte';
  import ChatTranscript from './ChatTranscript.svelte';
  import { PaneFollow } from './paneFollow.svelte';
  import { ApplicationSync } from './applicationSync';
  import { MessageQueue } from './messageQueue.svelte';
  import { addressStep, autostartTurn, bootTarget, openFirst, type Arrival } from './chatRouting';
  import { ChatAlerts } from './chatAlerts.svelte';
  import AssistantAlerts from './AssistantAlerts.svelte';
  import type { AssistantChatProps } from './AssistantChat.props';
  import ChatSessionBar from './ChatSessionBar.svelte';
  import { ChatTurns } from './chatTurns.svelte';
  import { TranscriptCatchUp } from './transcriptCatchUp';
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
  // A plain chat with nothing to resume opens blank, with no session yet: the first message
  // creates one (minting on open left most sessions empty).
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

  let sidebarOpen = $state(true);
  // Whether the transcript follows the stream (see PaneFollow).
  const follow = new PaneFollow();

  const queue = new MessageQueue();

  // Voice mode: a hands-free call on an `interview` session (see ChatInputArea).
  let voiceCallOpen = $state(false);
  let elevenLabsVoice = $state(false);
  let interviewReport = $state<InterviewReport>();

  function closeVoiceCall() {
    voiceCallOpen = false;
    if (activeId) void catchUp.reload(activeId);
  }

  const NEW_CHAT_LABEL = $derived(s.newChat);

  // What the URL this component mounted on asked for (see Arrival).
  const arrival: Arrival = { preset, kickoff };
  const isSupport = untrack(() => preset) === 'support';

  // The open conversation's turn (see ChatTurns).
  const turns: ChatTurns = new ChatTurns({
    isSupport,
    alerts,
    activeId: () => activeId,
    disposed: () => disposed,
    text: () => s,
    event: (id, event) => onEvent(id, event),
    reduce: (event) => (chat = reduceTurnEvent(chat, event)),
    reload: (id) => catchUp.reload(id),
    giveBack: (start) => giveBack(start),
    ended: () => endTurn(),
    followBottom: () => void follow.toBottom(true),
    beforeTurn: () => beforeTurn,
    onTurnStateChange: () => onTurnStateChange,
    onRunStateChange: () => onRunStateChange,
  });

  /** Whether a turn may be started right now: the pane is settled and nothing is running. */
  const canStartTurn = () =>
    phase === 'ready' && !turns.active && !switching && !creating && (!!activeId || deferredNew);
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

      // What to open first (see bootTarget).
      const target = bootTarget(session, arrival.preset);
      if (target === 'create') {
        await createAndOpen();
      } else if (target === 'newest') {
        if (!(await openNewestOpenable())) startBlankChat();
      } else {
        // The tailoring host opens a conversation the rail never lists, so seed it
        // into the local list to give it a name; the chat host's sessions all come
        // from the list already.
        if (!showSessionRail) {
          sessions = upsertSession(sessions, { id: target, label: sessionLabel ?? NEW_CHAT_LABEL, preset: 'tailor' });
        }
        await openSession(target);
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
      alerts.report(err, s.assistantUnreachable);
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
      openSession(requested, true).catch((err: unknown) => alerts.report(err, s.openChatFailed));
    }
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
    // Wait for confirmation before dropping unsent messages.
    if (queue.length > 0 && !discardQueued) {
      switchConfirm?.ask(id, fromAddress);
      return;
    }
    switching = true;
    cancelTurn();
    // End the call before switching its transcript.
    voiceCallOpen = false;
    try {
      chat = initChat();
      queue.clear();
      alerts.bulletCap = null;
      activeId = id;
      activePreset = null;
      elevenLabsVoice = false;
      interviewReport = undefined;
      // The URL-following effect must see this before the await.
      navigatingTo = fromAddress ? null : id;
      const { session: meta, messages: transcriptMessages, elevenlabs_voice, interview_report } = await getSession(id);
      if (disposed) return;
      if (showSessionRail && !opensInRail(meta.preset)) {
        alerts.notFound = true;
        return;
      }
      activePreset = meta.preset;
      elevenLabsVoice = elevenlabs_voice === true;
      interviewReport = interview_report;
      chat = chatFromTranscript(transcriptMessages);
      if (showSessionRail && !sessions.some((item) => item.id === id)) {
        sessions = upsertSession(sessions, fromSummary(meta, NEW_CHAT_LABEL));
      }
      if (meta.label?.trim()) sessions = setLabel(sessions, id, labelFromMessage(meta.label));
      onSessionChange?.(id);
    } finally {
      switching = false;
    }
    void follow.toBottom(true);
  }

  /** Open the most recent conversation that will actually open, newest first (see openFirst). */
  function openNewestOpenable(): Promise<boolean> {
    return openFirst(
      sessions.map((c) => c.id),
      async (id) => {
        await openSession(id);
        if (!alerts.notFound) return true;
        alerts.notFound = false;
        return false;
      },
    );
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
      alerts.report(err, s.openChatFailed);
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
  function endTurn(completed = false) {
    turns.finish();
    // A completed turn may have edited an artifact (e.g. the tailored CV) — let the host refresh.
    onTurnComplete?.({ completed });
    drainQueue();
  }

  // Send the next queued message — but not while the "switch and discard?" dialog is up:
  // the visitor is deciding whether that queue should exist at all, and a message sent in
  // the meantime could start work that confirming the switch can no longer take back.
  // Declining the switch drains it (see the effect beside confirmSwitch).
  function drainQueue() {
    if (confirmSwitchOpen || turns.active || queue.length === 0) return;
    const next = queue.shift();
    if (next) void dispatch({ kind: 'message', text: next.text, attachments: next.attachments });
  }

  /** Stop an in-flight turn (see ChatTurns.cancel). */
  function cancelTurn() {
    turns.cancel();
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
      endTurn(event.stop_reason === 'end_turn' && !event.is_error);
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
    if (turns.active || queue.length > 0) {
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
        alerts.report(err, s.openChatFailed);
        return;
      }
      if (activeId !== created.id) {
        giveBack(start);
        return;
      }
    }
    const id = activeId;
    if (!id || disposed) return;
    await turns.run(id, start);
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
    busy: () => turns.busy,
    began: (started) => turns.adopt(started),
    event: onEvent,
    reload: (id) => catchUp.reload(id),
    ended: () => {
      endTurn();
      void follow.toBottom(true);
    },
  });

  $effect(() => {
    if (!keepInSync || phase !== 'ready' || !activeId) return;
    return appSync.watch(activeId);
  });

  const catchUp: TranscriptCatchUp = new TranscriptCatchUp({
    isSupport,
    activeId: () => activeId,
    streaming: () => turns.active,
    show: (next, report) => {
      chat = next;
      interviewReport = report;
    },
  });

  onMount(() => {
    void boot();
    const stopListening = catchUp.listen();
    return () => {
      disposed = true;
      stopListening();
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
          turnActive={turns.active}
          {switching}
          canStart={canStartTurn}
          onsubmit={submitText}
          onrunopening={runOpening}
          {isSupport}
          onretry={() => startTurn({ kind: 'retry' })}
        />

        {#snippet input(busy: boolean)}
          <ChatInputArea
            bind:draft
            bind:attachments
            bind:voiceCallOpen
            sessionId={activeId}
            queue={queue.items}
            turnActive={turns.active}
            support={isSupport}
            disabled={phase !== 'ready' || switching || busy}
            interview={activePreset === 'interview'}
            {elevenLabsVoice}
            transcript={chat.messages}
            onSubmit={submitText}
            onRemoveQueued={(id) => queue.remove(id)}
            onCancel={cancelTurn}
            onClose={closeVoiceCall}
          />
        {/snippet}
        {#if activePreset === 'interview' && activeId}
          {@const id = activeId}
          {#key id}
            <RehearsalInput
              id={activeId}
              report={interviewReport}
              onReport={(report) => {
                if (activeId === id) interviewReport = report;
              }}
              transcript={chat.messages}
              callOpen={voiceCallOpen}
              ready={canStartTurn() && queue.length === 0}
              onReplay={(next) => (activeId === id ? openSession(next) : Promise.resolve())}
            >
              {#snippet children(busy)}{@render input(busy)}{/snippet}
            </RehearsalInput>
          {/key}
        {:else}
          {@render input(false)}
        {/if}
      </div>
    </div>
  {/if}
</div>

<ChatSwitchConfirm
  bind:this={switchConfirm}
  bind:open={confirmSwitchOpen}
  onConfirm={(target) =>
    openSession(target.id, target.fromAddress, true).catch((err: unknown) => alerts.report(err, s.openChatFailed))}
  onDecline={declineSwitch}
/>

<ChatDeleteConfirm bind:this={deleteConfirm} {sessions} newChatLabel={NEW_CHAT_LABEL} onRemove={removeChat} />
