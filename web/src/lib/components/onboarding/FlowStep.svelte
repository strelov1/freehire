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

<!-- On a phone the icon sits beside the title only and the content takes the full width; a
     40px column beside every step would truncate the job titles and squeeze the lists. The
     column dissolves there (`contents`), so its header and content wrap around the icon. -->
<section
  class="flex flex-wrap gap-3 border-t border-dashed border-border py-5 first:border-t-0 first:pt-0 last:pb-0 sm:flex-nowrap sm:gap-4"
>
  <span
    class="flex size-8 shrink-0 items-center justify-center rounded-lg bg-brand-muted text-brand-strong sm:size-10 sm:rounded-xl"
  >
    <Icon class="size-4 sm:size-5" />
  </span>
  <div class="contents sm:flex sm:min-w-0 sm:flex-1 sm:flex-col sm:gap-3">
    <div class="flex min-w-0 flex-1 items-start gap-3 sm:flex-none">
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
    {#if children}
      <div class="flex w-full min-w-0 flex-col gap-3 empty:hidden">
        {@render children()}
      </div>
    {/if}
  </div>
</section>
