<script lang="ts">
  import type { ChatMessage } from '#lib/assistant/chat.js';
  import { splitPresentingCalls } from '#lib/assistant/deck.js';
  import JobDeck from '#lib/assistant/JobDeck.svelte';
  import ToolGroupList from '#lib/assistant/ToolGroupList.svelte';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { renderMarkdown } from '#lib/markdown.js';
  import { messages } from './AssistantChat.messages';

  // One assistant message in AssistantChat's transcript: its thinking, the vacancy decks it
  // presented, the rest of its tool calls, the reply and, when the turn failed, the retry.
  // `active` is the message still streaming at the end of the transcript; `glyph` and
  // `elapsedSec` are the host's turn timer, which its thinking header shows while active.
  let {
    message,
    active,
    glyph,
    elapsedSec,
    onConfirm,
    disabled,
    canRetry,
    retryDisabled,
    onRetry,
  }: {
    message: ChatMessage;
    active: boolean;
    glyph: string | undefined;
    elapsedSec: number;
    onConfirm: (text: string) => void;
    disabled: boolean;
    canRetry: boolean;
    retryDisabled: boolean;
    onRetry: () => void;
  } = $props();

  const s = $derived(t(messages, locale()));
  // The presented decks and the rest of the calls (see the note above ToolGroupList below).
  const split = $derived(splitPresentingCalls(message.tools, message.streaming));
</script>

{#if message.thinking}
  <details class="self-start max-w-[88%] text-xs text-muted-foreground" open={active}>
    <summary
      class="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1 hover:bg-muted/50 [&::-webkit-details-marker]:hidden [&::marker]:hidden"
    >
      <span class={['font-mono text-[0.85rem] font-semibold', active ? 'star-glow' : 'text-muted-foreground/60']}>
        {active ? glyph : '✶'}
      </span>
      <span class={['font-medium', active && 'shimmer']}>{s.verbs.thinking}</span>
      {#if active}
        <span class="font-mono text-[0.7rem] text-muted-foreground">({elapsedSec}s)</span>
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
<ToolGroupList calls={split.rest} {onConfirm} {disabled} />
{#each split.decks as slot, di (di)}
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
    <p class="text-destructive">{s.turnErrored}</p>
    {#if canRetry}
      <button
        type="button"
        class="rounded-md border border-border bg-background px-2 py-0.5 font-medium text-foreground transition-colors hover:bg-muted disabled:opacity-50"
        disabled={retryDisabled}
        onclick={onRetry}
      >
        {s.retry}
      </button>
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
    0% {
      background-position: 200% 0;
    }
    100% {
      background-position: -200% 0;
    }
  }

  .star-glow {
    color: var(--color-foreground);
    animation: star-pulse 1.8s ease-in-out infinite;
  }
  @keyframes star-pulse {
    0%,
    100% {
      opacity: 0.7;
    }
    50% {
      opacity: 1;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .star-glow {
      animation: none;
      opacity: 1;
    }
    .shimmer {
      animation: none;
      background: none;
      color: var(--color-foreground);
      -webkit-text-fill-color: currentColor;
    }
  }

  /* Markdown rendering — scoped, applied with :global to reach @html output. */
  .md :global(*:first-child) {
    margin-top: 0;
  }
  .md :global(*:last-child) {
    margin-bottom: 0;
  }
  .md :global(p) {
    margin: 0 0 0.5rem;
    line-height: 1.55;
  }
  .md :global(h1),
  .md :global(h2),
  .md :global(h3),
  .md :global(h4) {
    margin: 0.8rem 0 0.35rem;
    line-height: 1.3;
    font-weight: 600;
  }
  .md :global(h1) {
    font-size: 1.1rem;
  }
  .md :global(h2) {
    font-size: 1rem;
  }
  .md :global(h3),
  .md :global(h4) {
    font-size: 0.95rem;
  }
  .md :global(ul),
  .md :global(ol) {
    margin: 0 0 0.5rem;
    padding-left: 1.25rem;
  }
  .md :global(li) {
    margin: 0.1rem 0;
  }
  .md :global(li > p) {
    margin: 0;
  }
  .md :global(a) {
    color: oklch(0.6 0.18 280);
    text-decoration: underline;
    text-underline-offset: 2px;
  }
  .md :global(strong) {
    font-weight: 600;
  }
  .md :global(em) {
    font-style: italic;
  }
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
