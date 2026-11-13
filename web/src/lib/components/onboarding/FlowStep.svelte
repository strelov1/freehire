<script lang="ts">
  // One step of the onboarding flow's card: an icon tile, a title with an optional line
  // under it, a check once the step is done, and the step's own content beneath. The steps
  // stack one under another as the flow reveals them, divided by a dashed rule, so the
  // whole run reads top to bottom as it happened.
  import type { Component, Snippet } from 'svelte';
  import { Check, LoaderCircle } from '@lucide/svelte';

  let {
    icon: Icon,
    title,
    subtitle,
    state = 'active',
    children,
  }: {
    icon: Component<{ class?: string }>;
    title: string;
    subtitle?: string;
    /** `working` spins in place of the check; `done` shows the check. */
    state?: 'active' | 'working' | 'done';
    children?: Snippet;
  } = $props();
</script>

<section class="flex gap-4 border-t border-dashed border-border py-5 first:border-t-0 first:pt-0 last:pb-0">
  <span class="flex size-10 shrink-0 items-center justify-center rounded-xl bg-brand-muted text-brand-strong">
    <Icon class="size-5" />
  </span>
  <div class="flex min-w-0 flex-1 flex-col gap-3">
    <div class="flex items-start gap-3">
      <div class="min-w-0 flex-1">
        <h2 class="text-base font-semibold tracking-tight text-balance">{title}</h2>
        {#if subtitle}
          <p class="mt-0.5 text-sm text-muted-foreground text-pretty">{subtitle}</p>
        {/if}
      </div>
      {#if state === 'done'}
        <!-- The site's done mark (the account setup checklist's): a brand check, no badge. -->
        <Check class="mt-0.5 size-5 shrink-0 text-brand" aria-label="Done" />
      {:else if state === 'working'}
        <LoaderCircle class="size-5 shrink-0 animate-spin text-muted-foreground" aria-hidden="true" />
      {/if}
    </div>
    {@render children?.()}
  </div>
</section>
