<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { replaceState } from '$app/navigation';
  import { Marked } from 'marked';
  import DOMPurify from 'isomorphic-dompurify';
  import {
    Bot,
    AlertTriangle,
    Terminal,
    ChevronRight,
    ArrowUp,
    Loader2,
    Trash2,
    Plus,
    RefreshCw,
    ExternalLink,
    X,
    FileText,
    PanelLeft,
  } from '@lucide/svelte';
  import { currentUser } from '$lib/auth.svelte';
  import { createSession, listSessions, deleteSession, assistantWsUrl } from '$lib/assistant/api';
  import { api } from '$lib/api';
  import { RoyClient } from '$lib/assistant/client';
  import { initChat, reduceTurnEvent, type ChatState } from '$lib/assistant/chat';
  import { parseJobSegments } from '$lib/assistant/unfurl';
  import JobCardUnfurl from '$lib/assistant/JobCardUnfurl.svelte';
  import {
    fromSummary,
    newestFirst,
    upsertSession,
    removeSession,
    setLabel,
    activeAfterDelete,
    labelFromMessage,
    type SessionItem,
  } from '$lib/assistant/sessions';
  import {
    classifyFamily,
    groupTitle,
    isExpandable,
    callLine,
    bashCommand,
    nonEmptyInput,
    previewToolInput,
    type ToolCall,
    type ToolFamily,
  } from '$lib/assistant/tool-formatters';
  import type { TurnEvent } from '$lib/assistant/wire';

  // The agent chat. Auth is UNIFIED with freehire: the /my shell already gates
  // the freehire session, and the agent backend verifies that same `hire_token`
  // cookie (shared JWT secret) — so there is no separate agent login. On mount
  // the page loads the caller's held sessions (owner-scoped `GET /sessions`),
  // opens ONE WebSocket relay, and attaches to the newest session with
  // `from_seq: 0` so its full journal replays through the same pure
  // `reduceTurnEvent` reducer as live frames. Switching sessions detaches and
  // re-attaches the one socket; "New chat" spawns a session; delete removes one.
  // Everything WebSocket-shaped runs client-side, so SSR renders only the shell.

  type Phase = 'connecting' | 'ready';

  // The agent is a beta-tester-only rollout. The nav hides it for others, but
  // guard the route directly too — the agent backend only checks the freehire
  // cookie, not the group, so a non-beta user reaching this URL must be stopped
  // here (never connect / spawn a session for them).
  const isBeta = $derived(currentUser()?.beta_tester ?? false);

  let phase = $state<Phase>('connecting');
  let error = $state<string | null>(null);
  // Set when the socket drops after we were live — the composer disables and
  // prompts a reload, instead of silently accepting sends that can't go out.
  let connectionLost = $state(false);

  // Transport: one RoyClient per page. `activeId` is the session currently
  // attached and shown in the chat pane; `frameUnsub` is that session's frame
  // subscription; `statusUnsubs` are page-lifetime status/error subs.
  let client: RoyClient | null = null;
  let activeId = $state<string | null>(null);
  let frameUnsub: (() => void) | null = null;
  let statusUnsubs: (() => void)[] = [];

  // Sidebar: the caller's held sessions (newest-first). `switching` disables the
  // composer and list while a detach→attach is in flight.
  let sessions = $state<SessionItem[]>([]);
  let switching = $state(false);

  // Chat (active session).
  let chat = $state<ChatState>(initChat());
  let draft = $state('');
  let turnActive = $state(false);
  // The optimistically-shown user text, so we drop the backend's echoed
  // `user_prompt` frame for it instead of rendering the message twice.
  let pendingEcho: string | null = null;

  // CV-tailoring artifact panel: when the session was opened from a tailoring CTA (?cv=<id>),
  // a collapsible panel beside the chat renders that CV's live PDF. previewVersion is bumped
  // on every completed turn to cache-bust the iframe (the agent may have just edited the CV).
  let cvPreviewId = $state<string | null>(null);
  let previewOpen = $state(false);
  let previewVersion = $state(0);
  // Session-list rail: collapsible for a full-width chat; starts collapsed in a tailoring
  // session so the chat + CV preview get the room.
  let sidebarOpen = $state(true);
  const cvPreviewUrl = $derived(
    cvPreviewId ? `${api.cvPdfUrl(Number(cvPreviewId))}?v=${previewVersion}` : '',
  );

  let scroller = $state<HTMLDivElement | null>(null);
  let textareaEl = $state<HTMLTextAreaElement | null>(null);

  // Whether we currently hold the session's input lease. Held across a queued
  // run of turns (acquired lazily on the first dispatch, released on idle when
  // the queue drains) so back-to-back messages don't fight for the lease.
  let inputAcquired = false;

  // Messages typed while a turn is in flight. Drained one-by-one when a
  // terminal `result` frame lands (see onFrame), mirroring roy-web's composer.
  let queue = $state<{ id: string; text: string }[]>([]);
  let queueCounter = 0;

  function enqueue(text: string, toFront = false) {
    const item = { id: `q${++queueCounter}`, text };
    queue = toFront ? [item, ...queue] : [...queue, item];
  }

  // --- Session labels ------------------------------------------------------
  // The backend has no `display_label` for freehire sessions, so the sidebar
  // labels a session by its first user message, cached client-side (cosmetic;
  // the session list itself is backend-backed) so it survives reloads. Until a
  // session has a message, it shows a timestamp fallback.
  const LABELS_KEY = 'hire.assistant.labels';
  let labelCache: Record<string, string> = {};

  function loadLabels() {
    try {
      labelCache = JSON.parse(localStorage.getItem(LABELS_KEY) ?? '{}') as Record<string, string>;
    } catch {
      labelCache = {};
    }
  }
  function persistLabels() {
    try {
      localStorage.setItem(LABELS_KEY, JSON.stringify(labelCache));
    } catch {
      // ignore — labels are best-effort cosmetics
    }
  }
  function fallbackLabel(createdAtSec: number): string {
    return new Intl.DateTimeFormat(undefined, {
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    }).format(new Date(createdAtSec * 1000));
  }
  // Record a session's first user message as its sidebar label (once).
  function noteFirstUserMessage(id: string, text: string) {
    if (labelCache[id]) return;
    const label = labelFromMessage(text);
    if (!label) return;
    labelCache[id] = label;
    persistLabels();
    sessions = setLabel(sessions, id, label);
  }

  // Auto-grow the composer textarea up to a cap (px), like roy-web's `autosize`.
  const COMPOSER_CAP = 200;
  $effect(() => {
    void draft;
    const el = textareaEl;
    if (!el) return;
    el.style.height = 'auto';
    el.style.height = `${Math.min(el.scrollHeight, COMPOSER_CAP)}px`;
  });

  // --- Markdown rendering (sanitized) --------------------------------------
  // freehire is a PUBLIC app and the model output is untrusted, so — unlike
  // roy-web — we never render raw model HTML: marked's output is run through
  // DOMPurify before it reaches `{@html}`. `isomorphic-dompurify` is SSR-safe,
  // so the guard holds even though this page only paints messages client-side.
  const md = new Marked({ gfm: true, breaks: true });
  // Open links the model writes in a new tab so clicking one never navigates the
  // chat away (losing the conversation). Scoped to this sanitize call — added
  // right before and removed right after — so it never affects DOMPurify use
  // elsewhere in the app.
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

  // Fold a message's flat tool-call list into consecutive same-family groups,
  // so the renderer shows one card per run of e.g. bash commands or file reads.
  function groupTools(calls: readonly ToolCall[]): { family: ToolFamily; calls: ToolCall[] }[] {
    const groups: { family: ToolFamily; calls: ToolCall[] }[] = [];
    for (const c of calls) {
      const family = classifyFamily(c.name);
      const last = groups[groups.length - 1];
      if (last && last.family === family) last.calls.push(c);
      else groups.push({ family, calls: [c] });
    }
    return groups;
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

  // --- Connection + session orchestration ----------------------------------

  async function boot() {
    phase = 'connecting';
    error = null;
    connectionLost = false;
    loadLabels();
    try {
      // Load the caller's held sessions (owner-scoped, newest-first). Degrade to
      // a single fresh session if the backend can't list them.
      let summaries: Awaited<ReturnType<typeof listSessions>> = [];
      try {
        summaries = await listSessions();
      } catch {
        error = 'Could not load your chats — starting a new one.';
      }

      client = new RoyClient();
      // Surface a mid-session disconnect instead of stranding the UI.
      statusUnsubs.push(
        client.onStatus((s) => {
          if ((s === 'closed' || s === 'error') && phase === 'ready') {
            connectionLost = true;
            error = 'Connection to the agent was lost. Reload the page to reconnect.';
            endTurn();
          }
        }),
      );
      // A turn that fails with an `error` event (agent crash, tool failure)
      // instead of a terminal `result` would otherwise hang the turn forever.
      statusUnsubs.push(
        client.onError((e) => {
          error = `The agent hit an error: ${e.message}`;
          endTurn();
        }),
      );
      await client.connect(assistantWsUrl());

      sessions = newestFirst(
        summaries.map((s) =>
          fromSummary(s, labelCache[s.session_id], fallbackLabel(s.created_at)),
        ),
      );
      const params = new URLSearchParams(location.search);
      // ?cv=<id> marks a tailoring session and opens the CV artifact panel beside the chat.
      cvPreviewId = params.get('cv');
      previewOpen = !!cvPreviewId;
      sidebarOpen = !cvPreviewId;
      // ?session=<id> opens a specific session (e.g. a tailoring session just started from
      // the fit page); ensure it shows in the list even if listSessions is momentarily stale.
      // Otherwise open the newest, or start a fresh chat when there are none.
      const requested = params.get('session');
      // ?title=<vacancy · company> names the session in the sidebar, kept separate from the
      // kickoff prompt. Seeding labelCache also stops noteFirstUserMessage from overwriting
      // the name with the first message.
      const title = params.get('title');
      if (requested) {
        if (title) {
          labelCache[requested] = title;
          persistLabels();
        }
        if (!sessions.some((s) => s.id === requested)) {
          sessions = upsertSession(sessions, {
            id: requested,
            label: title ?? 'CV tailoring',
            createdAt: Math.floor(Date.now() / 1000),
            live: true,
          });
        }
        await openSession(requested);
      } else if (sessions[0]) {
        await openSession(sessions[0].id);
      } else {
        await createAndOpen();
      }
      phase = 'ready';

      // Autostart: a CTA (e.g. CV tailoring) can pass a first prompt via ?prompt so the agent
      // begins immediately instead of waiting for the user to type. Only fire on a fresh
      // (empty) session — on a refresh the kickoff and its reply are already in the journal,
      // so chat is non-empty and we never re-send.
      const kickoff = params.get('prompt');
      if (requested && kickoff && chat.messages.length === 0) {
        // Drop ?prompt from the URL first (keeping session + cv): a refresh replays the
        // journal asynchronously, so chat is momentarily empty again — without this the
        // kickoff would re-send.
        const keep = new URLSearchParams({ session: requested });
        if (cvPreviewId) keep.set('cv', cvPreviewId);
        replaceState(`${location.pathname}?${keep}`, {});
        await dispatch(kickoff);
      }
    } catch (err) {
      error = err instanceof Error ? err.message : 'Could not reach the agent backend.';
      teardown();
    }
  }

  // Attach to `id` and replay its journal. Detaches the current session first,
  // subscribes to frames BEFORE attaching (so the `from_seq: 0` replay frames,
  // emitted right after `attached`, are captured), and resets the chat so the
  // replay repaints from scratch.
  async function openSession(id: string) {
    if (!client) return;
    if (activeId === id && frameUnsub) return;
    switching = true;
    try {
      await closeActiveSession();
      chat = initChat();
      pendingEcho = null;
      activeId = id;
      frameUnsub = client.subscribeFrames(id, (entry) => onFrame(entry.event));
      await client.call({ op: 'attach', session: id, from_seq: 0 }, 'attached');
    } finally {
      switching = false;
    }
    void scrollToBottom();
  }

  // Unwind the current attach: stop its frames, drop any in-flight turn/queue,
  // release the lease, and detach. Safe when nothing is attached.
  async function closeActiveSession() {
    frameUnsub?.();
    frameUnsub = null;
    turnActive = false;
    queue = [];
    const id = activeId;
    if (!client || !id) {
      inputAcquired = false;
      return;
    }
    if (inputAcquired) {
      inputAcquired = false;
      void client.call({ op: 'release_input', session: id }, 'input_released').catch(() => {});
    }
    await client.call({ op: 'detach', session: id }, 'detached').catch(() => {});
  }

  // Create a fresh session, add it to the top of the list, and open it.
  async function createAndOpen() {
    if (!client) return;
    const id = await createSession();
    const nowSec = Math.floor(Date.now() / 1000);
    sessions = upsertSession(sessions, { id, label: 'New chat', createdAt: nowSec, live: true });
    await openSession(id);
  }

  async function newChat() {
    if (!client || switching || phase !== 'ready') return;
    error = null;
    try {
      await createAndOpen();
    } catch (err) {
      error = err instanceof Error ? err.message : 'Could not start a new chat.';
    }
  }

  async function selectSession(id: string) {
    if (!client || switching || id === activeId) return;
    error = null;
    try {
      await openSession(id);
    } catch {
      error = 'Could not open that chat.';
    }
  }

  async function removeChat(id: string) {
    if (!client || switching) return;
    // A chat with a derived label has real history — confirm before deleting.
    if (labelCache[id] && !confirm('Delete this chat? This cannot be undone.')) return;
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
    if (labelCache[id]) {
      delete labelCache[id];
      persistLabels();
    }
    if (wasActive) {
      // The session is already closed server-side, so skip the detach (it would
      // be rejected as not-owned); just stop its frames and open the next one.
      frameUnsub?.();
      frameUnsub = null;
      inputAcquired = false;
      turnActive = false;
      queue = [];
      activeId = null;
      const next = activeAfterDelete(remaining, true, null);
      try {
        if (next) await openSession(next);
        else await createAndOpen();
      } catch (err) {
        error = err instanceof Error ? err.message : 'Could not open a chat.';
      }
    }
  }

  // End the current turn: clear the in-flight flag, then either drain the next
  // queued message (reusing the held lease) or release the lease when idle.
  function endTurn() {
    turnActive = false;
    // A completed turn may have edited the tailored CV — refresh the artifact preview.
    if (cvPreviewId) previewVersion += 1;
    if (connectionLost) {
      inputAcquired = false;
      return;
    }
    if (queue.length > 0) {
      const [next, ...rest] = queue;
      queue = rest;
      if (next) void dispatch(next.text);
    } else if (inputAcquired && activeId) {
      inputAcquired = false;
      void client?.call({ op: 'release_input', session: activeId }, 'input_released').catch(() => {});
    }
  }

  function onFrame(event: TurnEvent) {
    // Suppress the echoed user_prompt for a message we already showed optimistically.
    if (event.type === 'user_prompt' && pendingEcho !== null && event.text === pendingEcho) {
      pendingEcho = null;
      return;
    }
    // A replayed (or non-echoed) first user message names the session.
    if (event.type === 'user_prompt' && activeId) noteFirstUserMessage(activeId, event.text);
    chat = reduceTurnEvent(chat, event);
    if (event.type === 'result') endTurn();
    void scrollToBottom();
  }

  // Composer submit: while a turn is in flight (or the queue is non-empty) the
  // message is queued and drained later; otherwise it dispatches immediately.
  function submit(e: SubmitEvent) {
    e.preventDefault();
    const text = draft.trim();
    if (!text || !client || phase !== 'ready' || connectionLost || switching || !activeId) return;
    draft = '';
    if (turnActive || queue.length > 0) {
      enqueue(text);
      void scrollToBottom();
      return;
    }
    void dispatch(text);
  }

  // Acquire the lease (if not held), show the user message optimistically, and
  // fire the prompt. `turnActive` is set synchronously up front so a second
  // submit during the acquire round-trip queues instead of double-dispatching.
  async function dispatch(text: string) {
    if (!client || !activeId) return;
    const id = activeId;
    error = null;
    turnActive = true;
    try {
      if (!inputAcquired) {
        const acquired = await client.call({ op: 'acquire_input', session: id }, 'input_acquired');
        // The user may have switched sessions during the acquire round-trip (the
        // list isn't disabled by an in-flight turn). If so, this dispatch belongs
        // to the now-detached old session: release the lease we just took on it
        // and abandon — never attribute the lease to, or paint a phantom message
        // into, the newly-active session.
        if (activeId !== id) {
          if (acquired.acquired) {
            void client.call({ op: 'release_input', session: id }, 'input_released').catch(() => {});
          }
          return;
        }
        if (!acquired.acquired) {
          // Another client (e.g. this chat open in a second tab) holds the
          // lease. Don't silently queue forever — surface it and restore the
          // text so the user can retry, rather than deadlocking the queue.
          turnActive = false;
          error = 'The agent is busy (is it open in another tab?). Try again in a moment.';
          if (!draft.trim()) draft = text;
          return;
        }
        inputAcquired = true;
      }
      pendingEcho = text;
      noteFirstUserMessage(id, text);
      chat = reduceTurnEvent(chat, { type: 'user_prompt', text });
      client.fire({ op: 'send', session: id, text });
      void scrollToBottom();
    } catch (err) {
      turnActive = false;
      inputAcquired = false;
      error = err instanceof Error ? err.message : 'Could not send the message.';
    }
  }

  function removeQueued(id: string) {
    queue = queue.filter((q) => q.id !== id);
  }

  function teardown() {
    frameUnsub?.();
    frameUnsub = null;
    for (const off of statusUnsubs) off();
    statusUnsubs = [];
    client?.close();
    client = null;
  }

  onMount(() => {
    if (!isBeta) return;
    void boot();
    return teardown;
  });
</script>

<svelte:head>
  <title>Agent — freehire</title>
</svelte:head>

{#if error}
  <div
    class="mb-4 flex items-start gap-2 rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive"
    role="alert"
  >
    <AlertTriangle class="mt-0.5 size-4 shrink-0" />
    <span>{error}</span>
  </div>
{/if}

{#if !isBeta}
  <div class="rounded-xl border border-border bg-card p-8 text-center text-sm text-muted-foreground">
    The agent is a limited beta and isn't available for your account yet.
  </div>
{:else}
  <div class="flex h-[calc(100svh-3.5rem)] gap-3 p-3">
    <!-- Sidebar (desktop): session list + New chat — collapsible for a full-width chat. -->
    {#if sidebarOpen}
      <aside class="hidden w-60 shrink-0 flex-col rounded-xl border border-border bg-card md:flex">
      <div class="p-2">
        <button
          type="button"
          onclick={newChat}
          disabled={switching || phase !== 'ready'}
          class="flex w-full items-center gap-2 rounded-lg border border-border px-3 py-2 text-sm font-medium transition-colors hover:bg-muted disabled:cursor-not-allowed disabled:opacity-50"
        >
          <Plus class="size-4" />
          New chat
        </button>
      </div>
      <ul class="flex-1 space-y-1 overflow-y-auto px-2 pb-2">
        {#each sessions as s (s.id)}
          <li class="group relative">
            <button
              type="button"
              onclick={() => selectSession(s.id)}
              disabled={switching}
              class={[
                'flex w-full items-center rounded-lg py-2 pl-3 pr-9 text-left text-sm transition-colors',
                s.id === activeId ? 'bg-secondary text-secondary-foreground' : 'hover:bg-muted',
              ]}
            >
              <span class="min-w-0 flex-1 truncate">{s.label}</span>
            </button>
            <button
              type="button"
              aria-label="Delete chat"
              title="Delete chat"
              onclick={() => removeChat(s.id)}
              class="absolute right-1 top-1/2 flex size-7 -translate-y-1/2 items-center justify-center rounded-md text-muted-foreground opacity-0 transition-opacity hover:bg-muted hover:text-foreground focus-visible:opacity-100 group-hover:opacity-100"
            >
              <Trash2 class="size-4" />
            </button>
          </li>
        {/each}
      </ul>
    </aside>
    {/if}

    <!-- Chat pane -->
    <div class="flex min-w-0 flex-1 flex-col rounded-xl border border-border bg-card">
      <!-- Desktop: collapse/expand the session list for a full-width chat. -->
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
            disabled={switching || phase !== 'ready'}
            class="flex items-center gap-1.5 rounded px-1.5 py-1 text-sm text-muted-foreground transition-colors hover:text-foreground disabled:cursor-not-allowed disabled:opacity-50"
            title="New chat"
          >
            <Plus class="size-4" />New chat
          </button>
        {/if}
      </div>
      <!-- Mobile session switcher -->
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
        <button
          type="button"
          onclick={newChat}
          disabled={switching || phase !== 'ready'}
          aria-label="New chat"
          title="New chat"
          class="flex size-9 shrink-0 items-center justify-center rounded-lg border border-border text-muted-foreground hover:bg-muted disabled:opacity-50"
        >
          <Plus class="size-4" />
        </button>
        {#if activeId}
          <button
            type="button"
            onclick={() => removeChat(activeId as string)}
            aria-label="Delete chat"
            title="Delete chat"
            class="flex size-9 shrink-0 items-center justify-center rounded-lg border border-border text-muted-foreground hover:bg-muted"
          >
            <Trash2 class="size-4" />
          </button>
        {/if}
      </div>

      <!-- Message list -->
      <div bind:this={scroller} class="flex-1 overflow-y-auto p-4">
        <div class="mx-auto flex max-w-3xl flex-col gap-3">
          {#if phase === 'connecting'}
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

              {#each groupTools(message.tools) as g, t (t)}
                {@const title = groupTitle(g.family, g.calls)}
                {#if !isExpandable(g.family, g.calls)}
                  <div class="self-start flex items-center gap-2 rounded-lg border border-border bg-muted/50 px-3 py-2 text-sm text-muted-foreground">
                    <Terminal class="size-4 shrink-0" />
                    <span>{title}</span>
                  </div>
                {:else}
                  <details class="self-start max-w-[90%]">
                    <summary class="flex cursor-pointer items-center gap-2 rounded-lg border border-border bg-muted/50 px-3 py-2 text-sm text-muted-foreground hover:bg-muted/70 [&::-webkit-details-marker]:hidden [&::marker]:hidden [&[open]>span>svg.chev]:rotate-90">
                      <Terminal class="size-4 shrink-0" />
                      <span class="flex items-center gap-1.5">
                        {title}
                        <ChevronRight class="chev size-3.5 shrink-0 transition-transform" />
                      </span>
                    </summary>
                    {#if g.family === 'bash'}
                      <div class="mt-2 overflow-hidden rounded-md border border-border bg-background">
                        <div class="border-b border-border bg-muted/40 px-3 py-1.5 text-[0.65rem] font-medium uppercase tracking-wider text-muted-foreground/80">
                          Shell
                        </div>
                        {#each g.calls as c, ci (ci)}
                          <pre class={['m-0 whitespace-pre-wrap break-words px-3 py-2 font-mono text-xs', ci > 0 && 'border-t border-border']}>$ {bashCommand(c.input) ?? ''}</pre>
                        {/each}
                      </div>
                    {:else if g.family === 'fs'}
                      <ul class="mt-2 ml-6 space-y-1 text-xs text-muted-foreground">
                        {#each g.calls as c, ci (ci)}
                          <li>{callLine(c)}</li>
                        {/each}
                      </ul>
                    {:else}
                      <ul class="mt-2 ml-6 space-y-1 text-xs text-muted-foreground">
                        {#each g.calls as c, ci (ci)}
                          <li class="flex flex-wrap items-baseline gap-1.5">
                            <span class="font-medium text-foreground/80">{c.name}</span>
                            {#if nonEmptyInput(c.input)}
                              <code class="rounded bg-muted px-1.5 py-0.5 font-mono">{previewToolInput(c.input)}</code>
                            {/if}
                          </li>
                        {/each}
                      </ul>
                    {/if}
                  </details>
                {/if}
              {/each}

              {#if message.text}
                {#if message.streaming}
                  <!-- While streaming, render plain markdown — the text arrives
                       token-by-token, so a half-typed job URL would otherwise
                       match each prefix as a bogus slug (spurious 404s + card
                       flicker). Unfurl only once the reply is settled. -->
                  <article class="md self-start max-w-[88%] px-1 py-1 text-sm leading-relaxed text-foreground">
                    <!-- eslint-disable-next-line svelte/no-at-html-tags -- DOMPurify-sanitized markdown -->
                    {@html renderMarkdown(message.text)}
                  </article>
                {:else}
                  <!-- Settled reply: split into markdown + job-link segments so
                       freehire job links unfurl into real JobRow cards while the
                       prose stays sanitized markdown. Cards take the column
                       width; prose keeps a readable measure. -->
                  <div class="self-start w-full">
                    {#each parseJobSegments(message.text) as seg, si (si)}
                      {#if seg.kind === 'job'}
                        <JobCardUnfurl slug={seg.slug} />
                      {:else}
                        <article class="md max-w-[88%] px-1 py-1 text-sm leading-relaxed text-foreground">
                          <!-- eslint-disable-next-line svelte/no-at-html-tags -- DOMPurify-sanitized markdown -->
                          {@html renderMarkdown(seg.text)}
                        </article>
                      {/if}
                    {/each}
                  </div>
                {/if}
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

      <!-- Composer -->
      <div class="border-t border-border p-3">
        <div class="mx-auto w-full max-w-3xl">
          {#if queue.length > 0}
            <!-- Queued messages: sent one-by-one as each turn finishes. -->
            <div class="mb-2 overflow-hidden rounded-2xl border border-border/60 bg-card">
              <div class="px-4 py-2 text-xs font-medium text-brand">{queue.length} queued</div>
              <ul class="divide-y divide-border/40 border-t border-border/40">
                {#each queue as item (item.id)}
                  <li class="group flex items-start gap-3 px-4 py-2">
                    <span class="min-w-0 flex-1 whitespace-pre-wrap break-words text-sm text-foreground">{item.text}</span>
                    <button
                      type="button"
                      aria-label="Remove from queue"
                      title="Remove"
                      onclick={() => removeQueued(item.id)}
                      class="flex size-7 shrink-0 items-center justify-center rounded-md text-muted-foreground opacity-60 transition-opacity hover:bg-muted hover:text-foreground group-hover:opacity-100"
                    >
                      <Trash2 class="size-4" />
                    </button>
                  </li>
                {/each}
              </ul>
            </div>
          {/if}

          <!-- svelte-ignore a11y_click_events_have_key_events -->
          <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
          <form
            onsubmit={submit}
            onclick={(e) => {
              if ((e.target as HTMLElement).closest('button')) return;
              textareaEl?.focus();
            }}
            class="relative flex w-full cursor-text items-end gap-2 rounded-3xl border border-border/60 bg-card px-4 py-2.5 shadow-sm transition-colors focus-within:border-ring/60"
          >
            <textarea
              bind:this={textareaEl}
              bind:value={draft}
              rows="1"
              placeholder="Message the agent — Enter to send, Shift+Enter for newline"
              disabled={phase !== 'ready' || connectionLost || switching}
              onkeydown={(e) => {
                if (e.key === 'Enter' && !e.shiftKey) {
                  e.preventDefault();
                  (e.currentTarget.form as HTMLFormElement | null)?.requestSubmit();
                }
              }}
              class="block max-h-[200px] min-h-[1.5rem] flex-1 resize-none cursor-text bg-transparent py-1 text-sm leading-6 text-foreground placeholder:text-muted-foreground/70 focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-50"
            ></textarea>
            <button
              type="submit"
              aria-label={turnActive ? 'Queue message' : 'Send message'}
              aria-busy={turnActive}
              disabled={phase !== 'ready' || connectionLost || switching || !draft.trim()}
              class="flex size-9 shrink-0 items-center justify-center rounded-full bg-foreground text-background transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
            >
              {#if turnActive}
                <Loader2 class="size-4 animate-spin" />
              {:else}
                <ArrowUp class="size-4" strokeWidth={2.5} />
              {/if}
            </button>
          </form>
        </div>
      </div>
    </div>

    <!-- CV artifact panel: the live tailored-CV PDF, refreshed on each completed turn. -->
    {#if cvPreviewId}
      {#if previewOpen}
        <aside class="hidden w-[26rem] shrink-0 flex-col rounded-xl border border-border bg-card lg:flex">
          <div class="flex items-center justify-between border-b border-border px-3 py-2">
            <span class="text-sm font-medium">CV preview</span>
            <div class="flex items-center gap-0.5 text-muted-foreground">
              <button
                onclick={() => (previewVersion += 1)}
                class="rounded p-1 transition-colors hover:text-foreground"
                aria-label="Refresh preview"
                title="Refresh"
              >
                <RefreshCw class="size-4" />
              </button>
              <a
                href={api.cvPdfUrl(Number(cvPreviewId))}
                target="_blank"
                rel="noopener"
                class="rounded p-1 transition-colors hover:text-foreground"
                aria-label="Open PDF in a new tab"
                title="Open in new tab"
              >
                <ExternalLink class="size-4" />
              </a>
              <button
                onclick={() => (previewOpen = false)}
                class="rounded p-1 transition-colors hover:text-foreground"
                aria-label="Hide preview"
                title="Hide"
              >
                <X class="size-4" />
              </button>
            </div>
          </div>
          <iframe src={cvPreviewUrl} title="CV preview" class="min-h-0 flex-1 rounded-b-xl"></iframe>
        </aside>
      {:else}
        <button
          onclick={() => (previewOpen = true)}
          class="hidden shrink-0 flex-col items-center gap-1 rounded-xl border border-border bg-card px-2 py-3 text-xs text-muted-foreground transition-colors hover:text-foreground lg:flex"
          aria-label="Show CV preview"
          title="Show CV"
        >
          <FileText class="size-4" />CV
        </button>
      {/if}
    {/if}
  </div>
{/if}

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
