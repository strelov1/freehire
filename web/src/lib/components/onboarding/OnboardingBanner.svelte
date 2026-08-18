<script lang="ts">
  import { Target, X } from '@lucide/svelte';

  // The unobtrusive nudge above the /jobs feed. Presentational only: it knows
  // whether it's shown and emits open/dismiss — JobsView owns visibility (unseen
  // AND no active filters) and lifecycle. Never blocks the feed below it.
  //
  // Server-rendered, and hidden before first paint for a visitor who already
  // dismissed it — the same no-flash arrangement the Product Hunt strip uses, and
  // for the same reason: this sits in the document flow above the feed, so
  // mounting it after hydration shoves every job row down by 74px. It did exactly
  // that, and CrUX put the resulting CLS at 0.28 on the mobile home page (31% of
  // visits "poor"). `data-onboarding-banner` is what app.css keys the hide on.
  let { onOpen, onDismiss }: { onOpen: () => void; onDismiss: () => void } = $props();
</script>

<div
  data-onboarding-banner
  class="mb-3 flex items-center gap-3 rounded-xl border border-border bg-card p-3 pl-4 sm:pl-4"
>
  <div class="flex size-9 shrink-0 items-center justify-center rounded-lg bg-brand/10 text-brand-strong">
    <Target class="size-4.5" />
  </div>
  <div class="min-w-0 flex-1">
    <p class="text-sm font-semibold tracking-tight">Make this your feed</p>
    <p class="truncate text-xs text-muted-foreground">Answer 2 quick questions — see only jobs that fit you.</p>
  </div>
  <button
    type="button"
    onclick={onOpen}
    class="inline-flex h-9 shrink-0 items-center rounded-lg bg-brand px-4 text-sm font-semibold text-brand-foreground transition-opacity hover:opacity-90"
  >
    Set up
  </button>
  <button
    type="button"
    onclick={onDismiss}
    aria-label="Dismiss"
    class="flex size-8 shrink-0 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
  >
    <X class="size-4" />
  </button>
</div>
