<script lang="ts">
  import { ArrowDown } from '@lucide/svelte';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import type { ChatState } from './chat';
  import type { Attachment } from './client';
  import type { PaneFollow } from './paneFollow.svelte';
  import type { OpeningAction } from './presets';
  import AssistantReply from './AssistantReply.svelte';
  import ChatStarters from './ChatStarters.svelte';
  import OpeningActions from './OpeningActions.svelte';
  import ThinkingIndicator from './ThinkingIndicator.svelte';
  import UserMessage from './UserMessage.svelte';
  import { turnTimer } from './turnTimer.svelte';
  import { messages } from './AssistantChat.messages';

  // AssistantChat's transcript: the scrolling log of the conversation, what an empty one
  // offers to start from, the turn in flight and the way back to the newest message.
  // `follow` is the chat's scroll-follow state; the actions are the chat's own.
  let {
    follow,
    phase,
    chat,
    openingActions,
    autoRun,
    activePreset,
    turnActive,
    switching,
    canStart,
    onsubmit,
    onrunopening,
    isSupport,
    onretry,
  }: {
    follow: PaneFollow;
    phase: 'loading' | 'ready';
    chat: ChatState;
    openingActions: OpeningAction[] | undefined;
    autoRun: boolean;
    activePreset: string | null;
    turnActive: boolean;
    switching: boolean;
    canStart: () => boolean;
    onsubmit: (text: string, docs?: Attachment[]) => void;
    onrunopening: (action: OpeningAction) => void;
    isSupport: boolean;
    onretry: () => void;
  } = $props();

  const s = $derived(t(messages, locale()));

  // What an empty plain chat offers to start from, so the first turn is a tap rather than a
  // blank box. Sent as the candidate's own words, in the language they are reading — unlike
  // `profileKickoff`, which is our text put in their mouth and so stays fixed, each of these
  // is exactly the label they chose. Each one maps to a tool every chat session carries
  // (search_jobs + get_profile, market_fit, my_jobs, inbox_overview).
  const starters = $derived([s.starters.findJobs, s.starters.marketFit, s.starters.applications, s.starters.inbox]);

  // The turn's elapsed seconds and spinner glyph, which the streaming reply's thinking shows.
  const turnClock = turnTimer(() => turnActive);
</script>

<div bind:this={follow.el} onscroll={follow.onScroll} class="min-h-0 flex-1 overflow-y-auto overscroll-contain p-4">
  <!-- The transcript is a log, so a screen reader hears what arrives without moving
     to it. Busy while a turn streams (and while a conversation loads): the reply is
     re-rendered token by token, and announcing each re-render would read the same
     sentence dozens of times — the finished message is read once the turn ends. -->
  <div
    class="mx-auto flex max-w-3xl flex-col gap-3"
    role="log"
    aria-label={s.transcript}
    aria-live="polite"
    aria-relevant="additions"
    aria-busy={phase === 'loading' || switching || turnActive}
  >
    {#if phase === 'loading'}
      <p class="text-sm text-muted-foreground">{s.connecting}</p>
    {:else if chat.messages.length === 0 && openingActions?.length}
      <OpeningActions actions={openingActions} disabled={turnActive || switching} onRun={onrunopening} />
    {:else if chat.messages.length === 0 && !autoRun}
      <!-- Never shown on a cold start (autoRun): the brief hasn't landed as a message
         yet, but the run has already started — inviting the candidate to type here
         would contradict the "Crunching…" indicator already on screen. -->
      <p class="text-sm text-muted-foreground">{s.askAnything}</p>
      <!-- Starters only in a plain chat: a profile interview opens with its own
         kickoff, and these questions belong to the job search, not to it. -->
      {#if activePreset === 'chat'}
        <ChatStarters {starters} disabled={!canStart()} onPick={(text) => onsubmit(text)} />
      {/if}
    {/if}

    {#each chat.messages as message, i (i)}
      {#if message.role === 'user'}
        <UserMessage {message} />
      {:else}
        {@const active = i === chat.messages.length - 1 && message.streaming}
        <AssistantReply
          {message}
          {active}
          glyph={turnClock.glyph}
          elapsedSec={turnClock.elapsedSec}
          onConfirm={onsubmit}
          disabled={turnActive || switching}
          canRetry={!isSupport && i === chat.messages.length - 1 && !turnActive && !switching}
          retryDisabled={!canStart()}
          onRetry={onretry}
        />
      {/if}
    {/each}

    <!-- One indicator, two states. Queued is not working: nothing is being spent yet, and
       the elapsed counter would be timing somebody else's turn. -->
    {#if chat.queued || turnActive}
      <ThinkingIndicator label={chat.queued ? s.waitingForTurn : undefined} />
    {/if}
  </div>

  <!-- Jump to latest. Sticky rather than absolute so it needs no positioned
     ancestor: as the scroller's last child its natural place is past the bottom
     of the scrollport, which is exactly when `bottom-2` pins it into view. -->
  {#if !follow.stick}
    <div class="pointer-events-none sticky bottom-2 mt-2 flex justify-center">
      <button
        type="button"
        onclick={() => follow.toBottom(true)}
        class="pointer-events-auto flex items-center gap-1.5 rounded-full border border-border bg-card px-3 py-1.5 text-xs font-medium text-muted-foreground shadow-md transition-colors hover:text-foreground"
      >
        <ArrowDown class="size-3.5" />
        {s.jumpToLatest}
      </button>
    </div>
  {/if}
</div>
