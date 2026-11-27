<!-- Auto-apply (openspec/changes/archive/2026-11-25-auto-apply-submit-trigger): beside the apply link, not a
     replacement for it — auto-apply still goes through the same ATS in the end, this
     button only starts the tailor-then-review sequence. What it SAYS comes from the CTA
     plan; how loud it is does not vary, the brand fill belonging to `Tailor my CV`. The
     only thing decided here is that a submission already in flight also disables it, which
     is a fact about THIS component's request rather than about the posting.
     The `Pro` marker is a span, not the `Badge` primitive: Badge's variants carry their own
     background and foreground, none of which read as a plan marker. It takes the PAGE's
     `foreground` for its fill and `background` for its text — near-black on white in the
     light theme, near-white on black in the dark one — so it stays the highest contrast
     thing on the button in either. A tint of the button's own foreground was the first try
     and it dissolved into the fill. -->
<script lang="ts">
  import { resolve } from '$app/paths';
  import type { JobCtaPlan } from '#lib/autoApplyButton.js';
  import { Button } from '#lib/ui/index.js';

  let {
    autoApply,
    slug,
    size,
    className,
    busy,
    onclick,
  }: {
    autoApply: NonNullable<JobCtaPlan['autoApply']>;
    slug: string;
    size: 'md' | 'lg';
    className: string;
    /** A request from this page is in flight. */
    busy: boolean;
    onclick: () => void;
  } = $props();
</script>

{#if autoApply.chat}
  <Button variant="secondary" {size} href={resolve('/my/tracking/[id]?tab=auto_apply', { id: slug })} class={className}>
    {autoApply.label}
  </Button>
{:else}
  <Button variant="secondary" {size} disabled={autoApply.disabled || busy} {onclick} class={className}>
    {autoApply.label}
    {#if autoApply.pro}
      <span class="rounded-sm bg-foreground px-1.5 py-0.5 text-xs font-semibold leading-none text-background">
        Pro
      </span>
    {/if}
  </Button>
{/if}
