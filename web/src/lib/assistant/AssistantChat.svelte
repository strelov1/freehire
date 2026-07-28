<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { Marked } from 'marked';
  import DOMPurify from 'isomorphic-dompurify';
  import { AlertTriangle, PanelLeft, Plus, Trash2 } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { currentUser } from '$lib/auth.svelte';
  import {
    createSession,
    listSessions,
    getSession,
    deleteSession,
    SessionNotFound,
  } from '$lib/assistant/api';
  import { sendTurn, type Turn } from '$lib/assistant/client';
  import { initChat, reduceTurnEvent, type ChatState } from '$lib/assistant/chat';
  import { splitPresentingCalls } from '$lib/assistant/deck';
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
  import type { ChatPreset } from '$lib/assistant/presets';

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
  //  - requireBeta: kept so an embedder can gate the chat in the UI as well as at the API.
  let {
    session = undefined,
    kickoff = undefined,
    sessionLabel = undefined,
    onTurnComplete = undefined,
    onSessionChange = undefined,
    showSessionRail = true,
    requireBeta = false,
    preset = 'chat',
  }: {
    session?: string;
    kickoff?: string;
    sessionLabel?: string;
    onTurnComplete?: () => void;
    onSessionChange?: (id: string) => void;
    showSessionRail?: boolean;
    requireBeta?: boolean;
    /** Which unbound conversation a NEW session starts as. An existing session keeps
     *  whatever preset it was created with. */
    preset?: ChatPreset;
  } = $props();

  // The API gates the assistant to the restricted rollout; this mirrors it in the
  // UI so a non-member sees an explanation instead of a wall of 403s.
  const allowed = $derived(!requireBeta || (currentUser()?.beta_tester ?? false));

  let phase = $state<Phase>('loading');
  let error = $state<string | null>(null);
  // Distinct from `error`: the URL names a conversation that is not the caller's to
  // open — deleted, someone else's, or a tailoring chat that belongs to a CV. It is
  // a dead link, not a failure, so it gets an explanation and a way out.
  let notFound = $state(false);

  // Sidebar: the caller's conversations, newest activity first. `switching`
  // disables the composer and list while a session load is in flight.
  let sessions = $state<SessionItem[]>([]);
  let activeId = $state<string | null>(null);
  let switching = $state(false);
  // Creating a chat is a round trip before `switching` is ever set, and each
  // click landing in that window would create a real session on the server.
  let creating = $state(false);

  // Chat (active session).
  let chat = $state<ChatState>(initChat());
  let draft = $state('');
  let turnActive = $state(false);
  // The turn in flight, so it can be cancelled. Aborting the request is what
  // tells the backend to stop the loop.
  let turn: Turn | null = null;

  let sidebarOpen = $state(true);
  let scroller = $state<HTMLDivElement | null>(null);
  let textareaEl = $state<HTMLTextAreaElement | null>(null);

  // Messages typed while a turn is in flight, drained one by one when it ends.
  let queue = $state<{ id: string; text: string }[]>([]);
  let queueCounter = 0;

  function enqueue(text: string) {
    queue = [...queue, { id: `q${++queueCounter}`, text }];
  }

  const NEW_CHAT_LABEL = 'New chat';

  /** The conversation the host is currently being navigated to, or null. Held until the
   *  `session` prop catches up, so a switch we started is never undone by the stale URL. */
  let navigatingTo = $state<string | null>(null);

  // Auto-grow the composer textarea up to a cap (px).
  const COMPOSER_CAP = 200;
  $effect(() => {
    void draft;
    const el = textareaEl;
    if (!el) return;
    el.style.height = 'auto';
    el.style.height = `${Math.min(el.scrollHeight, COMPOSER_CAP)}px`;
  });

  // --- Markdown rendering (sanitized) --------------------------------------
  // freehire is a PUBLIC app and model output is untrusted, so marked's output is
  // run through DOMPurify before it reaches `{@html}`. `isomorphic-dompurify` is
  // SSR-safe, so the guard holds even though this page paints client-side only.
  const md = new Marked({ gfm: true, breaks: true });
  // Open links the model writes in a new tab so clicking one never navigates the
  // chat away. Scoped to this sanitize call so it never affects DOMPurify elsewhere.
  function openLinksInNewTab(node: Element) {
    if (node.tagName === 'A') {
      node.setAttribute('target', '_blank');
      node.setAttribute('rel', 'noopener noreferrer');
    }
  }
  function renderMarkdown(text: string): string {
    const html = md.parse(text, { async: false }) as string;
    DOMPurify.addHook('afterSanitizeAttributes', openLinksInNewTab);
    const clean = DOMPurify.sanitize(html);
    DOMPurify.removeHook('afterSanitizeAttributes');
    return clean;
  }


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

  async function scrollToBottom() {
    await tick();
    if (scroller) scroller.scrollTop = scroller.scrollHeight;
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
      if (!session && preset !== 'chat') {
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
      } else if (sessions[0]) {
        await openSession(sessions[0].id);
      } else {
        await createAndOpen();
      }
      phase = 'ready';

      // Autostart: the host can pass a kickoff prompt so the agent begins
      // immediately instead of waiting for the user to type. Only on an empty session.
      if (kickoff && chat.messages.length === 0) {
        void dispatch(kickoff);
      }
    } catch (err) {
      report(err, 'Could not reach the assistant.');
      phase = 'ready';
    }
  }

  // The host navigates between conversations, so the requested id arrives as a prop
  // change — including on Back and Forward. Follow it without re-booting the list.
  //
  // Except while a switch WE started is still in flight. Opening a conversation asks the
  // host to navigate, and that navigation is async: for the moment between setting
  // activeId and the URL catching up, this effect sees a `session` that disagrees with
  // activeId and would "follow" the URL straight back to the conversation just left.
  // That is what made a new chat snap back to the old one — the chat was created, and the
  // stale URL immediately evicted it.
  $effect(() => {
    const requested = session;
    if (!requested || phase !== 'ready' || requested === activeId) return;
    if (navigatingTo) {
      // The host has caught up; stop holding the effect off.
      if (requested === navigatingTo) navigatingTo = null;
      return;
    }
    openSession(requested).catch((err: unknown) => report(err, 'Could not open that chat.'));
  });

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
  async function openSession(id: string) {
    if (activeId === id && chat.messages.length > 0) return;
    switching = true;
    cancelTurn();
    try {
      chat = initChat();
      queue = [];
      activeId = id;
      // Raise the guard HERE, not after the fetch below: the URL-following effect reruns
      // the moment activeId changes, which is during the await — set it late and the
      // effect has already reopened the conversation we just left.
      navigatingTo = id;
      const { session: meta, messages } = await getSession(id);
      // A tailoring conversation is reachable by id but belongs to a CV and only makes
      // sense beside it. Opening one here would show a conversation the rail cannot list
      // and the user cannot get back to. The unbound presets — chat and the experience
      // interviewer — are both at home here.
      if (showSessionRail && meta.preset !== 'chat' && meta.preset !== 'profile') {
        notFound = true;
        return;
      }
      let next = initChat();
      for (const event of eventsFromTranscript(messages)) next = reduceTurnEvent(next, event);
      chat = next;
      if (meta.label?.trim()) sessions = setLabel(sessions, id, labelFromMessage(meta.label));
      onSessionChange?.(id);
    } finally {
      switching = false;
    }
    void scrollToBottom();
  }

  async function createAndOpen() {
    const created = await createSession(preset);
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

  async function removeChat(id: string) {
    if (switching) return;
    const named = sessions.find((s) => s.id === id)?.label !== NEW_CHAT_LABEL;
    // A chat that has been named has real history — confirm before deleting.
    if (named && !confirm('Delete this chat? This cannot be undone.')) return;
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
    // A completed turn may have edited an artifact (e.g. the tailored CV) — let the host refresh.
    onTurnComplete?.();
    if (queue.length > 0) {
      const [next, ...rest] = queue;
      queue = rest;
      if (next) void dispatch(next.text);
    }
  }

  /** Stop an in-flight turn. The backend notices its next write fail and stops the
   *  loop before spending another model call. */
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
    }
    chat = reduceTurnEvent(chat, event);
    if (event.type === 'result') endTurn();
    void scrollToBottom();
  }

  // Composer submit: while a turn is in flight the message is queued and drained
  // later; otherwise it is sent immediately.
  function submitText(raw: string) {
    const text = raw.trim();
    if (!text || phase !== 'ready' || switching || !activeId) return;
    draft = '';
    if (turnActive || queue.length > 0) {
      enqueue(text);
      void scrollToBottom();
      return;
    }
    void dispatch(text);
  }

  async function dispatch(text: string) {
    const id = activeId;
    if (!id) return;
    error = null;
    turnActive = true;
    const started = sendTurn(id, text, (event) => onEvent(id, event));
    turn = started;
    void scrollToBottom();
    try {
      await started.done;
    } catch (err) {
      error = err instanceof Error ? err.message : 'Could not send the message.';
      chat = reduceTurnEvent(chat, { type: 'result', stop_reason: 'error', is_error: true });
      endTurn();
    }
  }

  function removeQueued(id: string) {
    queue = queue.filter((q) => q.id !== id);
  }

  onMount(() => {
    if (!allowed) return;
    void boot();
    return cancelTurn;
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
{:else if !allowed}
  <div class="m-3 rounded-xl border border-border bg-card p-8 text-center text-sm text-muted-foreground">
    The agent is a limited beta and isn't available for your account yet.
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
        onDelete={removeChat}
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
            onclick={() => removeChat(activeId as string)}
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
      <div bind:this={scroller} class="flex-1 overflow-y-auto p-4">
        <div class="mx-auto flex max-w-3xl flex-col gap-3">
          {#if phase === 'loading'}
            <p class="text-sm text-muted-foreground">Connecting to the agent…</p>
          {:else if chat.messages.length === 0}
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
              <ToolGroupList calls={rest} />
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
                <p class="self-start text-xs text-destructive">The agent ended the turn with an error.</p>
              {/if}
            {/if}
          {/each}

          {#if turnActive}
            <div class="self-start inline-flex items-baseline gap-2 px-2 py-1 text-xs text-muted-foreground">
              <span class="star-glow font-mono text-[0.85rem] font-semibold">
                {SPINNER_GLYPHS[spinnerIdx]}
              </span>
              <span class="shimmer font-medium">{currentVerb}…</span>
              <span class="font-mono text-[0.7rem] text-muted-foreground/70">({elapsedSec}s)</span>
            </div>
          {/if}
        </div>
      </div>

      <Composer
        bind:draft
        {queue}
        {turnActive}
        disabled={phase !== 'ready' || switching}
        onSubmit={submitText}
        onRemoveQueued={removeQueued}
        onCancel={cancelTurn}
      />
    </div>
  </div>
{/if}
</div>

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
