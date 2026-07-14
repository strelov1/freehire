<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { Marked } from 'marked';
  import DOMPurify from 'isomorphic-dompurify';
  import { Bot, AlertTriangle, Terminal, ChevronRight, ArrowUp, Loader2, Trash2 } from '@lucide/svelte';
  import { currentUser } from '$lib/auth.svelte';
  import { createSession, assistantWsUrl } from '$lib/assistant/api';
  import { RoyClient } from '$lib/assistant/client';
  import { initChat, reduceTurnEvent, type ChatState } from '$lib/assistant/chat';
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

  // The assistant chat. Auth is UNIFIED with freehire: the /my shell already
  // gates the freehire session, and the agent backend verifies that same
  // `hire_token` cookie (shared JWT secret) — so there is no separate agent
  // login. On mount the page spawns a claude session, opens the WebSocket relay
  // (the browser sends the freehire cookie on the same-origin upgrade), attaches,
  // and streams frames through the pure `reduceTurnEvent` reducer. Everything
  // WebSocket-shaped runs client-side (onMount / handlers), so SSR renders only
  // the empty shell.

  type Phase = 'connecting' | 'ready';

  // Assistant is a moderators-only rollout. The nav hides it for others, but
  // guard the route directly too — the agent backend only checks the freehire
  // cookie, not the role, so a non-moderator reaching this URL must be stopped
  // here (never connect / spawn a session for them).
  const isModerator = $derived(currentUser()?.role === 'moderator');

  let phase = $state<Phase>('connecting');
  let error = $state<string | null>(null);
  // Set when the socket drops after we were live — the composer disables and
  // prompts a reload, instead of silently accepting sends that can't go out.
  let connectionLost = $state(false);

  // Session/transport (client-only; created in the connect flow).
  let client: RoyClient | null = null;
  let session = '';
  let unsubscribers: (() => void)[] = [];

  // Chat.
  let chat = $state<ChatState>(initChat());
  let draft = $state('');
  let turnActive = $state(false);
  // The optimistically-shown user text, so we drop the backend's echoed
  // `user_prompt` frame for it instead of rendering the message twice.
  let pendingEcho: string | null = null;

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
  function renderMarkdown(text: string): string {
    const html = md.parse(text, { async: false }) as string;
    return DOMPurify.sanitize(html);
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

  // Persist the session id so a page refresh re-attaches to the same daemon
  // session and replays its journal (the chat history) instead of starting over.
  // Keyed by freehire user so a different account on the same browser never
  // adopts the previous user's session (the relay also enforces ownership).
  const sessionKey = () => `freehire.assistant.session.${currentUser()?.id ?? 'anon'}`;
  function storeSession(id: string) {
    try {
      localStorage.setItem(sessionKey(), id);
    } catch {
      /* private mode / storage disabled — history just won't persist */
    }
  }
  function loadStoredSession(): string | null {
    try {
      return localStorage.getItem(sessionKey());
    } catch {
      return null;
    }
  }
  function clearStoredSession() {
    try {
      localStorage.removeItem(sessionKey());
    } catch {
      /* ignore */
    }
  }

  async function connect() {
    phase = 'connecting';
    error = null;
    connectionLost = false;
    try {
      const url = assistantWsUrl();
      client = new RoyClient();
      // Surface a mid-session disconnect instead of stranding the UI: once we're
      // live, a socket close/error stops the turn and disables the composer.
      unsubscribers.push(
        client.onStatus((s) => {
          if ((s === 'closed' || s === 'error') && phase === 'ready') {
            connectionLost = true;
            error = 'Connection to the assistant was lost. Reload the page to reconnect.';
            endTurn();
          }
        }),
      );
      // A turn that fails with an `error` event (agent crash, tool failure)
      // instead of a terminal `result` would otherwise hang the turn forever.
      unsubscribers.push(
        client.onError((e) => {
          error = `The assistant hit an error: ${e.message}`;
          endTurn();
        }),
      );
      await client.connect(url);

      // Re-attach to the previous session (replaying its journal = the history)
      // when we still have a live, owned one; otherwise start a fresh session.
      const stored = loadStoredSession();
      if (!(stored && (await tryAttach(stored)))) {
        clearStoredSession();
        chat = initChat();
        session = await createSession();
        storeSession(session);
        unsubscribers.push(client.subscribeFrames(session, (entry) => onFrame(entry.event)));
        await client.call({ op: 'attach', session }, 'attached');
      }
      phase = 'ready';
      void scrollToBottom();
    } catch (err) {
      error = err instanceof Error ? err.message : 'Could not reach the assistant backend.';
      teardown();
    }
  }

  // Subscribe to frames BEFORE attaching so the journal replay isn't missed,
  // then attach. Returns false (leaving no subscription) if the stored session
  // is gone or not ours — the caller then starts a fresh one.
  async function tryAttach(id: string): Promise<boolean> {
    if (!client) return false;
    session = id;
    const off = client.subscribeFrames(id, (entry) => onFrame(entry.event));
    try {
      await client.call({ op: 'attach', session: id }, 'attached');
      unsubscribers.push(off);
      return true;
    } catch {
      off();
      chat = initChat(); // discard any partial replay before falling back
      return false;
    }
  }

  // End the current turn: clear the in-flight flag, then either drain the next
  // queued message (reusing the held lease) or release the lease when idle.
  // Shared by the terminal `result` frame and abnormal turn-enders (error /
  // disconnect).
  function endTurn() {
    turnActive = false;
    if (connectionLost) {
      inputAcquired = false;
      return;
    }
    if (queue.length > 0) {
      const [next, ...rest] = queue;
      queue = rest;
      if (next) void dispatch(next.text);
    } else if (inputAcquired) {
      inputAcquired = false;
      void client?.call({ op: 'release_input', session }, 'input_released').catch(() => {});
    }
  }

  function onFrame(event: TurnEvent) {
    // Suppress the echoed user_prompt for a message we already showed optimistically.
    if (event.type === 'user_prompt' && pendingEcho !== null && event.text === pendingEcho) {
      pendingEcho = null;
      return;
    }
    chat = reduceTurnEvent(chat, event);
    if (event.type === 'result') endTurn();
    void scrollToBottom();
  }

  // Composer submit: while a turn is in flight (or the queue is non-empty) the
  // message is queued and drained later; otherwise it dispatches immediately.
  function submit(e: SubmitEvent) {
    e.preventDefault();
    const text = draft.trim();
    if (!text || !client || phase !== 'ready' || connectionLost) return;
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
    if (!client) return;
    error = null;
    turnActive = true;
    try {
      if (!inputAcquired) {
        const acquired = await client.call({ op: 'acquire_input', session }, 'input_acquired');
        if (!acquired.acquired) {
          // Another client (e.g. this chat open in a second tab) holds the
          // lease. Don't silently queue forever — surface it and restore the
          // text so the user can retry, rather than deadlocking the queue.
          turnActive = false;
          error = 'The assistant is busy (is it open in another tab?). Try again in a moment.';
          if (!draft.trim()) draft = text;
          return;
        }
        inputAcquired = true;
      }
      pendingEcho = text;
      chat = reduceTurnEvent(chat, { type: 'user_prompt', text });
      client.fire({ op: 'send', session, text });
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
    for (const off of unsubscribers) off();
    unsubscribers = [];
    client?.close();
    client = null;
  }

  onMount(() => {
    if (!isModerator) return;
    void connect();
    return teardown;
  });
</script>

<svelte:head>
  <title>Assistant — freehire</title>
</svelte:head>

<div class="mb-6 flex flex-col gap-1">
  <h1 class="flex items-center gap-2 text-2xl font-semibold tracking-tight">
    <Bot class="size-6 text-brand" />
    Assistant
  </h1>
  <p class="text-sm text-muted-foreground">
    Chat with a Claude agent inside freehire.
  </p>
</div>

{#if error}
  <div
    class="mb-4 flex items-start gap-2 rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive"
    role="alert"
  >
    <AlertTriangle class="mt-0.5 size-4 shrink-0" />
    <span>{error}</span>
  </div>
{/if}

{#if !isModerator}
  <div class="rounded-xl border border-border bg-card p-8 text-center text-sm text-muted-foreground">
    The assistant is a limited rollout and isn't available for your account yet.
  </div>
{:else}
  <div class="flex h-[calc(100vh-16rem)] min-h-100 flex-col rounded-xl border border-border bg-card">
    <!-- Message list -->
    <div bind:this={scroller} class="flex-1 overflow-y-auto p-4">
      <div class="mx-auto flex max-w-3xl flex-col gap-3">
        {#if phase === 'connecting'}
          <p class="text-sm text-muted-foreground">Connecting to the assistant…</p>
        {:else if chat.messages.length === 0}
          <p class="text-sm text-muted-foreground">Ask the assistant anything to get started.</p>
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
              <article class="self-start max-w-[88%] px-1 py-1 text-sm leading-relaxed text-foreground">
                <div class="md">
                  <!-- eslint-disable-next-line svelte/no-at-html-tags -- DOMPurify-sanitized markdown -->
                  {@html renderMarkdown(message.text)}
                </div>
              </article>
            {/if}

            {#if message.errored}
              <p class="self-start text-xs text-destructive">The assistant ended the turn with an error.</p>
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
            placeholder="Message the assistant — Enter to send, Shift+Enter for newline"
            disabled={phase !== 'ready' || connectionLost}
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
            disabled={phase !== 'ready' || connectionLost || !draft.trim()}
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
