<script lang="ts">
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { messages } from './AssistantChat.messages';

  // The "the agent is working" line: a cycling glyph, a shimmering verb and the seconds
  // elapsed. It times itself from the moment it mounts, so a host renders it only while
  // a turn is in flight and the counter needs no reset.
  //
  // `label` replaces the random verb for a state that is not work — "waiting for your
  // turn" — and such a state shows no counter, which would time somebody else's turn.
  let { label }: { label?: string } = $props();

  const s = $derived(t(messages, locale()));
  const GLYPHS = ['·', '✢', '✳', '✶', '✻', '✽'] as const;

  const startedAt = Date.now();
  let elapsedSec = $state(0);
  let glyph = $state(0);
  // Picked once per mount: a verb that changes mid-turn reads as a status change.
  const pick = Math.random();
  const verb = $derived.by(() => {
    const verbs = Object.values(s.verbs);
    return verbs[Math.floor(pick * verbs.length)] ?? s.verbs.thinking;
  });

  $effect(() => {
    const id = setInterval(() => {
      elapsedSec = Math.floor((Date.now() - startedAt) / 1000);
      glyph = (glyph + 1) % GLYPHS.length;
    }, 120);
    return () => clearInterval(id);
  });
</script>

<div class="inline-flex items-baseline gap-2 self-start px-2 py-1 text-xs text-muted-foreground" role="status">
  <span class="star-glow font-mono text-[0.85rem] font-semibold">{GLYPHS[glyph]}</span>
  <span class="shimmer font-medium">{label ?? verb}…</span>
  {#if !label}
    <span class="font-mono text-[0.7rem] text-muted-foreground">({elapsedSec}s)</span>
  {/if}
</div>

<style>
  /* background-clip masks the text onto a moving gradient: no JS, GPU-composited. */
  .shimmer {
    background-image: linear-gradient(
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
</style>
